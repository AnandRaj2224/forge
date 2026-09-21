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
	_ = catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 600, CreatedAt: now})
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
	app := NewApp(seedFlowDB(t))
	app.width, app.height = 150, 40
	app.goTab(TabReviews)
	view := app.View().Content
	for _, want := range []string{"PLANNED", "ACTUAL", "COMPLETION", "SESSIONS", "TASKS", "OVER/UNDER", "TIME BY DOMAIN", "GOV", "DSA", "SWE", "What worked well", "What should change"} {
		found := false
		// case-insensitive for reflections
		if strings.Contains(strings.ToUpper(view), strings.ToUpper(want)) {
			found = true
		}
		if !found {
			t.Fatalf("reviews missing %q", want)
		}
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

// TestMockSectionEnterFlow guards Enter-driven section/weak-topic editing:
// no a/w shortcuts; the "+ New section" row opens the section form, Enter
// on a section opens the weak-topic editor.
func TestMockSectionEnterFlow(t *testing.T) {
	database := seedMockHistoryDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	score, total := 72, 100
	_ = runtime.CreateMock(ctx, database, db.Mock{ID: "mock-1", ExamID: "exam-afcat", Name: "AFCAT Mock 07", MockDate: "2026-09-18", Platform: "Self", Score: &score, TotalScore: &total, CreatedAt: now})
	_ = runtime.CreateMockSection(ctx, database, db.MockSection{ID: "sec-q", MockID: "mock-1", Name: "QUANT", SortOrder: 1})
	app := NewApp(database)
	app.width, app.height = 150, 40
	app.goTab(TabMocks)
	if v := stripANSIForTest(app.View().Content); !strings.Contains(v, "New section") {
		t.Fatalf("new-section row missing")
	}
	testSend(app, "l")
	testSend(app, "j")
	if app.mocks.secCur != 1 {
		t.Fatalf("secCur=%d want 1 (new-section row)", app.mocks.secCur)
	}
	testSend(app, "enter")
	if app.mocks.mode != "section" {
		t.Fatalf("mode=%q want section", app.mocks.mode)
	}
	for _, k := range []string{"V", "E", "R", "B", "A", "L"} {
		testSend(app, k)
	}
	testSend(app, "enter")
	if app.mocks.mode != "browse" || len(app.mocks.sections) != 2 {
		t.Fatalf("section not saved: mode=%q sections=%d", app.mocks.mode, len(app.mocks.sections))
	}
	testSend(app, "k")
	testSend(app, "enter")
	if app.mocks.mode != "weak" {
		t.Fatalf("mode=%q want weak", app.mocks.mode)
	}
}
