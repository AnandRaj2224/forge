package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

func seedMockHistoryDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mockhist.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC) // Monday current week
	// Canonical Forge calendar settings (Asia/Kolkata, midnight): the engine
	// verifies these on Today generation, so fixtures must carry them.
	_ = catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 0, CreatedAt: now})
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "subj-quant", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-perc", SubjectID: "subj-quant", Name: "Percentage", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-ratio", SubjectID: "subj-quant", Name: "Ratio", SortOrder: 2})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-tw", SubjectID: "subj-quant", Name: "Time & Work", SortOrder: 3})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-cd", SubjectID: "subj-reasoning", Name: "Coding-Decoding", SortOrder: 1})
	_ = catalog.InsertGovExam(ctx, database, db.GovExam{ID: "exam-afcat", Name: "AFCAT", SortOrder: 1})
	_ = catalog.InsertGovExam(ctx, database, db.GovExam{ID: "exam-sbi", Name: "SBI Clerk", SortOrder: 2})
	_ = catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "exam-afcat", TopicID: "topic-perc", SortOrder: 1})
	_ = catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "exam-afcat", TopicID: "topic-ratio", SortOrder: 2})
	_ = catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "exam-afcat", TopicID: "topic-tw", SortOrder: 3})
	_ = catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "exam-sbi", TopicID: "topic-cd", SortOrder: 1})
	// Old snapshot outside current week (should never appear in History).
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap-old", LocalDate: "2026-09-10", Revision: 1, GeneratedAt: now})
	// Current-week snapshot for Monday.
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap-mon", LocalDate: "2026-09-21", Revision: 1, GeneratedAt: now})
	t.Cleanup(func() { database.Close() })
	return database
}

func TestHistoryOnlyCurrentWeek(t *testing.T) {
	database := seedMockHistoryDB(t)
	app := NewApp(database)
	app.width, app.height = 150, 40
	// Force "now" to Monday 21 Sep 2026.
	app.history.nowFn = func() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }
	app.goTab(TabHistory)
	if len(app.history.dates) != 7 {
		t.Fatalf("history must show 7 days, got %d", len(app.history.dates))
	}
	want := []string{"2026-09-21", "2026-09-22", "2026-09-23", "2026-09-24", "2026-09-25", "2026-09-26", "2026-09-27"}
	for i, d := range want {
		if app.history.dates[i] != d {
			t.Fatalf("day %d = %q want %q", i, app.history.dates[i], d)
		}
	}
	for _, d := range app.history.dates {
		if d == "2026-09-10" {
			t.Fatal("older DB record must not appear in History")
		}
	}
	view := app.View().Content
	if !strings.Contains(view, "Mon · 21 Sep") || !strings.Contains(view, "Sun · 27 Sep") {
		t.Fatalf("history day list missing current week:\n%s", view)
	}
	if strings.Contains(view, "2026-09-10") {
		t.Fatal("old date leaked into History view")
	}
	// Selecting a day updates the right panel (Tue).
	testSend(app, "j")
	if got := app.history.selectedDate(); got != "2026-09-22" {
		t.Fatalf("j must move to Tue, got %q", got)
	}
}

func TestMockWeakTopicsValidation(t *testing.T) {
	database := seedMockHistoryDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	score := 72
	total := 100
	if err := runtime.CreateMock(ctx, database, db.Mock{ID: "mock-1", ExamID: "exam-afcat", Name: "AFCAT Mock 07", MockDate: "2026-09-18", Platform: "Testbook", Score: &score, TotalScore: &total, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.CreateMockSection(ctx, database, db.MockSection{ID: "sec-q", MockID: "mock-1", Name: "QUANT", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	// Valid: 3 topics from exam syllabus, ordered.
	if err := runtime.SetWeakTopics(ctx, database, "mock-1", "sec-q", []db.ID{"topic-perc", "topic-ratio", "topic-tw"}); err != nil {
		t.Fatalf("valid 3 topics rejected: %v", err)
	}
	ws, _ := runtime.ListWeakTopics(ctx, database, "sec-q")
	if len(ws) != 3 || ws[0].TopicID != "topic-perc" || ws[0].RankOrder != 1 || ws[2].RankOrder != 3 {
		t.Fatalf("ordered weak topics wrong: %+v", ws)
	}
	// Cannot select more than 3.
	if err := runtime.SetWeakTopics(ctx, database, "mock-1", "sec-q", []db.ID{"topic-perc", "topic-ratio", "topic-tw", "topic-cd"}); err == nil {
		t.Fatal("more than 3 must fail")
	}
	// Cannot select same topic twice.
	if err := runtime.SetWeakTopics(ctx, database, "mock-1", "sec-q", []db.ID{"topic-perc", "topic-perc"}); err == nil {
		t.Fatal("duplicate must fail")
	}
	// Topics must come from actual exam syllabus (topic-cd is SBI only).
	if err := runtime.SetWeakTopics(ctx, database, "mock-1", "sec-q", []db.ID{"topic-cd"}); err == nil {
		t.Fatal("off-syllabus topic must fail")
	}
	// Stored IDs reference actual gov topics (unknown id).
	if err := runtime.SetWeakTopics(ctx, database, "mock-1", "sec-q", []db.ID{"topic-fake"}); err == nil {
		t.Fatal("unknown topic must fail")
	}
	// Mock must belong to a real exam.
	if err := runtime.CreateMock(ctx, database, db.Mock{ID: "mock-bad", ExamID: "exam-fake", Name: "Bad", MockDate: "2026-09-18", CreatedAt: now}); err == nil {
		t.Fatal("mock with fake exam must fail")
	}
}

func TestMocksScreenDetail(t *testing.T) {
	database := seedMockHistoryDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	score, total, attempted, totalQ, correct, incorrect, unatt := 72, 100, 87, 100, 71, 16, 13
	rankV := 1842
	perc := 84.3
	acc := 81.2
	avg := 24.8
	_ = runtime.CreateMock(ctx, database, db.Mock{ID: "mock-1", ExamID: "exam-afcat", Name: "AFCAT Mock 07", MockDate: "2026-09-18", Platform: "Self",
		Score: &score, TotalScore: &total, Attempted: &attempted, TotalQuestions: &totalQ, Correct: &correct, Incorrect: &incorrect, Unattempted: &unatt,
		RankValue: &rankV, Percentile: &perc, Accuracy: &acc, AvgSpeedSeconds: &avg, CreatedAt: now})
	_ = runtime.CreateMockSection(ctx, database, db.MockSection{ID: "sec-q", MockID: "mock-1", Name: "QUANT", SortOrder: 1})
	_ = runtime.SetWeakTopics(ctx, database, "mock-1", "sec-q", []db.ID{"topic-perc", "topic-ratio"})
	app := NewApp(database)
	app.width, app.height = 150, 40
	app.goTab(TabMocks)
	view := app.View().Content
	for _, want := range []string{"AFCAT Mock 07", "18 Sep 2026", "AFCAT", "Self", "SCORE", "72 / 100", "RANK", "1842", "PERCENTILE", "84.3", "ACCURACY", "81.2%", "ATTEMPTED", "87 / 100", "CORRECT", "71", "INCORRECT", "16", "UNATTEMPTED", "13", "QUANT", "Percentage", "Ratio"} {
		if !strings.Contains(view, want) {
			t.Fatalf("mock detail missing %q:\n%s", want, view)
		}
	}
}

func TestReviewsMetricsStructure(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 150, 40
	app.goTab(TabReviews)
	// Full report (unclipped) must read as an analytical weekly report with
	// explicit week context, performance sections, and reflections. The
	// right pane scrolls independently, so the clipped app view is checked
	// separately for the top bento.
	full := stripANSIForTest(app.reviews.reportView(90))
	for _, want := range []string{"PLANNED", "ACTUAL", "COMPLETION", "SESSIONS", "TASKS", "OVER/UNDER", "TIME BY DOMAIN", "GOV PERFORMANCE", "DSA PERFORMANCE", "SWE TIME", "What worked well", "What should change", "DOMAIN SUMMARY", "Questions attempted", "Questions correct", "Questions incorrect", "Not passed"} {
		found := false
		// case-insensitive for reflections
		if strings.Contains(strings.ToUpper(full), strings.ToUpper(want)) {
			found = true
		}
		if !found {
			t.Fatalf("reviews missing %q", want)
		}
	}
	view := app.View().Content
	for _, want := range []string{"PLANNED", "ACTUAL", "COMPLETION", "SESSIONS", "TASKS", "OVER/UNDER", "TIME BY DOMAIN"} {
		if !strings.Contains(strings.ToUpper(view), strings.ToUpper(want)) {
			t.Fatalf("reviews viewport missing top %q", want)
		}
	}
	// Scrolling the report must reveal the reflections tail.
	app.reviews.bodyScroll.Offset = 1 << 30
	scrolled := stripANSIForTest(app.View().Content)
	if !strings.Contains(scrolled, "What worked well") || !strings.Contains(scrolled, "What should change") {
		t.Fatalf("reviews reflections not reachable by scrolling")
	}
}

func TestRadioRoundTripStillVisible(t *testing.T) {
	r := NewRadio([]string{"YES", "NO"})
	r.Focused = true
	for _, want := range []int{0, 1, 0} {
		for r.Cursor != want {
			r.Move(1)
		}
		r.Choose()
		v := r.View()
		if strings.Count(v, "◉") != 1 || !strings.Contains(v, "▶") {
			t.Fatalf("radio marker missing at %d:\n%s", want, v)
		}
	}
}

// TestMockCreateFlow guards the single mock creation flow: raw facts in,
// derived metrics out, direct GOV-subject selection (no arbitrary section
// layer), weak topics picked per subject, one atomic engine save.
func TestMockCreateFlow(t *testing.T) {
	database := seedMockHistoryDB(t)
	app := NewApp(database)
	app.width, app.height = 150, 40
	app.goTab(TabMocks)
	testSend(app, "n")
	if app.mocks.mode != "create" {
		t.Fatalf("mode=%q want create", app.mocks.mode)
	}
	if len(app.mocks.cInputs) != 11 {
		t.Fatalf("create fields=%d want 11 raw facts", len(app.mocks.cInputs))
	}
	typeKeys := func(s string) {
		for _, r := range s {
			testSend(app, string(r))
		}
	}
	testSend(app, "down") // Name
	typeKeys("AFCAT 9")
	testSend(app, "down") // Date
	typeKeys("2026-09-18")
	testSend(app, "down") // Platform (left empty)
	testSend(app, "down") // Score
	typeKeys("72")
	testSend(app, "down") // Total Marks
	typeKeys("100")
	testSend(app, "down") // Total Questions
	typeKeys("100")
	testSend(app, "down") // Attempted
	typeKeys("87")
	testSend(app, "down") // Correct (cFocus 8)
	typeKeys("71")
	for range [3]struct{}{} {
		testSend(app, "down")
	} // Rank, Percentile, Average Speed (cFocus 11)
	testSend(app, "down") // GOV subject (cFocus 12)
	if app.mocks.cFocus != 12 {
		t.Fatalf("cFocus=%d want GOV subject selector", app.mocks.cFocus)
	}
	// Direct GOV-subject selection (Quant is first).
	if got := app.mocks.secSubjName(); got != "Quant" {
		t.Fatalf("default subject=%q want Quant", got)
	}
	testSend(app, "down") // ADD SUBJECT (cFocus 13)
	testSend(app, "enter")
	// Straight into the weak-topic editor for the staged subject.
	if app.mocks.mode != "weak" {
		t.Fatalf("mode=%q want weak editor (%s)", app.mocks.mode, app.mocks.cErr)
	}
	if app.mocks.wSection.GovSubjectID == nil || *app.mocks.wSection.GovSubjectID != "subj-quant" {
		t.Fatalf("editor must carry the Quant mapping: %+v", app.mocks.wSection)
	}
	// Toggle two weak topics, save back into the create form.
	testSend(app, "down") // filter -> list
	testSend(app, "enter")
	testSend(app, "down")
	testSend(app, "enter")
	testSend(app, "s")
	if app.mocks.mode != "create" {
		t.Fatalf("mode=%q want create (%s)", app.mocks.mode, app.mocks.wErr)
	}
	if len(app.mocks.stagedSecs) != 1 || len(app.mocks.stagedSecs[0].Weak) != 2 {
		t.Fatalf("staged=%+v", app.mocks.stagedSecs)
	}
	// Reach SAVE and persist atomically via the engine.
	for app.mocks.cFocus != app.mocks.nCreateFocus()-1 {
		testSend(app, "down")
	}
	testSend(app, "enter")
	if app.mocks.mode != "browse" {
		t.Fatalf("mode=%q want browse (%s)", app.mocks.mode, app.mocks.cErr)
	}
	ctx := context.Background()
	mocks, err := runtime.ListMocks(ctx, database)
	if err != nil || len(mocks) != 1 {
		t.Fatalf("mocks=%d err=%v", len(mocks), err)
	}
	m := mocks[0]
	if m.Incorrect == nil || *m.Incorrect != 16 {
		t.Fatalf("incorrect=%v want 16", m.Incorrect)
	}
	if m.Unattempted == nil || *m.Unattempted != 13 {
		t.Fatalf("unattempted=%v want 13", m.Unattempted)
	}
	if m.Accuracy == nil {
		t.Fatal("accuracy not derived")
	}
	secs, _ := runtime.ListMockSections(ctx, database, m.ID)
	if len(secs) != 1 {
		t.Fatalf("sections=%d want 1", len(secs))
	}
	if secs[0].GovSubjectID == nil || *secs[0].GovSubjectID != "subj-quant" {
		t.Fatalf("section mapping=%v want Quant", secs[0].GovSubjectID)
	}
	ws, _ := runtime.ListWeakTopics(ctx, database, secs[0].ID)
	if len(ws) != 2 {
		t.Fatalf("weak=%d want 2", len(ws))
	}
}

// TestMockCreateValidation rejects impossible raw combinations.
func TestMockCreateValidation(t *testing.T) {
	database := seedMockHistoryDB(t)
	app := NewApp(database)
	app.width, app.height = 150, 40
	app.goTab(TabMocks)
	testSend(app, "n")
	for _, k := range []string{"B", "a", "d"} {
		testSend(app, k)
	}
	// Date valid, then Attempted 5 / Correct 9 (correct > attempted).
	testSend(app, "down")
	for _, k := range []string{"2", "0", "2", "6", "-", "0", "9", "-", "1", "8"} {
		testSend(app, k)
	}
	for range [4]struct{}{} {
		testSend(app, "down")
	} // score, total, totalQ
	testSend(app, "down") // attempted
	testSend(app, "5")
	testSend(app, "down") // correct
	testSend(app, "9")
	// Reach SAVE through the section block, then attempt the save.
	for app.mocks.cFocus != app.mocks.nCreateFocus()-1 {
		testSend(app, "down")
	}
	testSend(app, "enter")
	if app.mocks.mode != "create" {
		t.Fatalf("invalid combo left create mode")
	}
	if app.mocks.cErr == "" {
		t.Fatal("expected validation error")
	}
}

// TestReviewReflectionEdit guards the weekly reflection workflow: Enter
// opens editing on the selected reflection, typing works, SAVE persists to
// the stored review model, and reopening shows the saved text.
func TestReviewReflectionEdit(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 150, 40
	app.goTab(TabReviews)
	testSend(app, "l")
	testSend(app, "enter")
	if !app.reviews.editing {
		t.Fatal("enter should start editing")
	}
	for _, k := range []string{"G", "o", "o", "d"} {
		testSend(app, k)
	}
	testSend(app, "down")
	for _, k := range []string{"B", "a", "d"} {
		testSend(app, k)
	}
	testSend(app, "down")
	testSend(app, "enter")
	if app.reviews.editing {
		t.Fatal("save should exit editing")
	}
	id := app.reviews.weeks.SelectedID()
	r, err := runtime.GetWeeklyReview(app.ctx(), app.DB, id)
	if err != nil {
		t.Fatalf("review not persisted: %v", err)
	}
	if r.ReflectionWorked != "Good" || r.ReflectionChange != "Bad" {
		t.Fatalf("reflections=%q/%q", r.ReflectionWorked, r.ReflectionChange)
	}
	app.goTab(TabToday)
	app.goTab(TabReviews)
	for i, e := range app.reviews.entries {
		if e.ID == id {
			app.reviews.weeks.Select(i)
		}
	}
	app.reviews.syncSelected()
	if app.reviews.selected.ReflectionWorked != "Good" {
		t.Fatalf("not reloaded: %q", app.reviews.selected.ReflectionWorked)
	}
}
