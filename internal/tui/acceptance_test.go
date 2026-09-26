package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// seedAcceptanceDB builds a small deterministic world with a Today snapshot
// for the real current day plus history on 2026-09-22.
func seedAcceptanceDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "accept.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cal := engine.DefaultCalendar()
	todayStr := cal.LocalDate(time.Now())
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "subj-quant", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-a", SubjectID: "subj-quant", Name: "Alpha", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-b", SubjectID: "subj-quant", Name: "Beta", SortOrder: 2})
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "lec-a", TopicID: "topic-a", Title: "Core", SortOrder: 1})
	_ = catalog.InsertGovExam(ctx, database, db.GovExam{ID: "exam-1", Name: "E1", SortOrder: 1})
	_ = catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "exam-1", TopicID: "topic-a", SortOrder: 1})
	_ = catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "exam-1", TopicID: "topic-b", SortOrder: 2})
	for i, p := range []db.DSAProblem{
		{ID: "dsa-1", Title: "One", Pattern: "Hash", Difficulty: "EASY"},
		{ID: "dsa-2", Title: "Two", Pattern: "Hash", Difficulty: "EASY"},
		{ID: "dsa-3", Title: "Three", Pattern: "Stack", Difficulty: "MEDIUM"},
	} {
		p.SortOrder = i + 1
		_ = catalog.InsertDSAProblem(ctx, database, p)
	}
	now := time.Now()
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap-today", LocalDate: todayStr, Revision: 1, GeneratedAt: now})
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-a", LectureID: "lec-a"})
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-a", "topic-b"}})
	dsaT, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeNew},
	}})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "acc-lec", SnapshotID: "snap-today", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "Lecture", PlannedSeconds: 3600, TargetJSON: &lec})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "acc-prac", SnapshotID: "snap-today", Position: 2, Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "Practice", PlannedSeconds: 3600, TargetJSON: &prac})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "acc-dsa", SnapshotID: "snap-today", Position: 3, Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "DSA", PlannedSeconds: 9000, TargetJSON: &dsaT})
	// Repeat states so the DSA bundle validates steady-state.
	for _, pid := range []db.ID{"dsa-1", "dsa-2"} {
		_ = runtime.UpsertDSARepeatState(ctx, database, db.DSARepeatState{ProblemID: pid, RepeatStage: 1, CurrentIntervalDays: 3, NextReviewDate: todayStr, UpdatedAt: now})
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func acceptApp(t *testing.T) *App {
	t.Helper()
	app := NewApp(seedAcceptanceDB(t))
	app.width, app.height = 150, 40
	app.reloadCurrent()
	return app
}

// A. COMPLETED TASK: select DONE, view result, press s: nothing starts,
// no red error, task stays read-only.
func TestAcceptCompletedTask(t *testing.T) {
	app := acceptApp(t)
	testSend(app, "s")
	testSend(app, "x")
	testSend(app, " ")
	testSend(app, "down")
	testSend(app, "enter")
	if st := ComputeTaskStatus(app.ctx(), app.DB, "acc-lec"); st != TaskCompleted {
		t.Fatalf("setup: want COMPLETED, got %s", st)
	}
	app.today.tbl.SetCursor(0)
	app.today.syncButtons()
	if !app.today.startBtn.Disabled {
		t.Fatal("Start must be disabled for DONE")
	}
	testSend(app, "s")
	if _, err := runtime.GetActiveSession(app.ctx(), app.DB); err == nil {
		t.Fatal("s on DONE must not create a session")
	}
	if app.err != "" {
		t.Fatalf("no red error allowed, got %q", app.err)
	}
	detail := stripANSIForTest(app.today.detailView())
	if !strings.Contains(detail, "READ-ONLY") {
		t.Fatalf("DONE must be read-only:\n%s", detail)
	}
	foot := stripANSIForTest(app.footerView(app.width))
	if strings.Contains(foot, "Start") {
		t.Fatalf("footer must not advertise Start: %q", foot)
	}
}

// B. TEXT INPUT: qqq types; Esc cancels the form; Forge keeps running.
func TestAcceptTextInput(t *testing.T) {
	app := acceptApp(t)
	app.goTab(TabMocks)
	testSend(app, "n")
	testSend(app, "down") // Name
	for _, k := range []string{"q", "q", "q"} {
		testSend(app, k)
	}
	if got := app.mocks.cInputs[0].Value(); got != "qqq" {
		t.Fatalf("name=%q want qqq", got)
	}
	_, cmd := app.Update(testKey("esc"))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatal("Esc must not quit")
			}
		}
	}
	if app.mocks.mode != "browse" || app.tab != TabMocks {
		t.Fatal("Esc must cancel back to browse, Forge running")
	}
}

// C. GOV PRACTICE: partial shows read-only satisfied + editable missing,
// Save blocks while invalid and completes the exact bundle when valid.
func TestAcceptGovPractice(t *testing.T) {
	app := acceptApp(t)
	ctx := app.ctx()
	now := time.Now()
	// Satisfy topic-a directly (partial bundle) via a stopped session.
	sess, err := engine.StartTodayTask(ctx, app.DB, "acc-prac", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, app.DB, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	avg := 20.0
	if _, err := engine.SaveGovPracticeEvidence(ctx, app.DB, "acc-prac", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-a", TimeSecs: 1200, Attempted: 20, Correct: 16, AvgSpeed: &avg},
	}, now); err != nil {
		t.Fatal(err)
	}
	if st := ComputeTaskStatus(ctx, app.DB, "acc-prac"); st != TaskTodo {
		t.Fatalf("partial must be TODO, got %s", st)
	}
	app.reloadCurrent()
	app.today.tbl.SetCursor(1)
	testSend(app, "s")
	testSend(app, "x")
	if app.today.right != "result" {
		t.Fatal("want result form")
	}
	if len(app.today.result.prows) != 1 || app.today.result.prows[0].TopicID != "topic-b" {
		t.Fatalf("only the missing topic must be editable: %+v", app.today.result.prows)
	}
	if !strings.Contains(app.today.result.satisfiedNote, "already recorded") {
		t.Fatalf("form must note satisfied items: %q", app.today.result.satisfiedNote)
	}
	// Empty save blocked.
	testSend(app, "enter")
	if app.today.right != "result" {
		t.Fatal("invalid save must stay in the form")
	}
	// Fill the missing row with human durations and save.
	// Raw facts only: Time spent + Attempted + Correct (Incorrect and
	// Avg time/question are derived, never entered).
	r := &app.today.result.prows[0]
	r.Time.Input.SetValue("15m")
	r.Att.Input.SetValue("10")
	r.Corr.Input.SetValue("8")
	app.today.result.updateFocusVisual()
	if app.today.result.save.Disabled {
		t.Fatal("valid bundle must enable SAVE")
	}
	testSend(app, "enter")
	// Enter off-SAVE moves focus; jump to SAVE explicitly.
	for app.today.right == "result" && app.today.result.focus != app.today.result.nFocus()-1 {
		testSend(app, "down")
	}
	testSend(app, "enter")
	if app.today.right != "detail" {
		t.Fatalf("valid save failed: %s", app.today.result.err)
	}
	if st := ComputeTaskStatus(ctx, app.DB, "acc-prac"); st != TaskCompleted {
		t.Fatalf("exact bundle must complete, got %s", st)
	}
}

// D. SESSION: stop then start again yields a new session; old unchanged.
func TestAcceptSessionLifecycle(t *testing.T) {
	app := acceptApp(t)
	ctx := app.ctx()
	app.today.tbl.SetCursor(1)
	testSend(app, "s")
	if app.today.active == nil {
		t.Fatalf("start failed: %s", app.err)
	}
	first := app.today.active.ID
	started := app.today.active.StartedAt
	testSend(app, "x")
	if app.today.right != "result" {
		t.Fatal("stop must open the result form")
	}
	testSend(app, "esc") // leave the form back to detail
	testSend(app, "s")
	if app.today.active == nil || app.today.active.ID == first {
		t.Fatal("second start must create a new session")
	}
	var startedUnix, stoppedUnix int64
	var n int
	err := app.DB.QueryRowContext(ctx, `SELECT started_at, stopped_at FROM sessions WHERE id = ?`, string(first)).Scan(&startedUnix, &stoppedUnix)
	if err != nil {
		t.Fatal(err)
	}
	if startedUnix != started.Unix() {
		t.Fatal("old started_at changed")
	}
	_ = stoppedUnix
	err = app.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n)
	if err != nil || n != 2 {
		t.Fatalf("sessions=%d want 2", n)
	}
	if _, err := engine.StopActiveSession(ctx, app.DB, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// E. DSA: checked items show a solid indicator; solve time renders human.
func TestAcceptDSAEvidence(t *testing.T) {
	app := acceptApp(t)
	ctx := app.ctx()
	now := time.Now()
	sess, err := engine.StartTodayTask(ctx, app.DB, "acc-dsa", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, app.DB, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveDSAEvidence(ctx, app.DB, "acc-dsa", sess.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 300},
		{ProblemID: "dsa-2", Independent: false, Optimal: true, HintUsed: false, SolveSecs: 150},
		{ProblemID: "dsa-3", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 540},
	}, now); err != nil {
		t.Fatal(err)
	}
	app.reloadCurrent()
	app.today.tbl.SetCursor(2)
	detail := stripANSIForTest(app.today.detailView())
	if !strings.Contains(detail, "DONE") || !strings.Contains(detail, "READ-ONLY") {
		t.Fatalf("DSA must be DONE read-only:\n%s", detail)
	}
	for _, want := range []string{"5m", "2m 30s", "9m"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail missing human duration %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "Solve time   300") || strings.Contains(detail, "Solve time   150") {
		t.Fatalf("raw seconds leaked:\n%s", detail)
	}
	if !strings.Contains(detail, "■") {
		t.Fatalf("checked items need a solid indicator:\n%s", detail)
	}
}

// G. HISTORY answers what actually happened that day, understandably.
func TestAcceptHistoryDesign(t *testing.T) {
	app := acceptApp(t)
	app.history.nowFn = func() time.Time { return time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC) }
	app.goTab(TabHistory)
	h := stripANSIForTest(app.history.dayBody(120))
	for _, want := range []string{"DAILY ACTIVITY", "PLANNED", "ACTUAL", "VARIANCE", "TASKS", "SESSIONS", "COMPLETION", "TIME BY DOMAIN", "DAILY WORK", "Lectures completed", "Questions attempted", "Questions correct", "Questions incorrect", "Accuracy", "Problems attempted", "Problems passed", "Problems not passed", "Planned SWE time", "Actual SWE time", "SWE variance", "SWE sessions", "UNFINISHED"} {
		if !strings.Contains(h, want) {
			t.Fatalf("history missing %q", want)
		}
	}
	if strings.Contains(h, "Practice questions 60") || strings.Contains(h, "Practice questions 291") {
		t.Fatal("vague practice prose must not appear")
	}
}

// H. REVIEWS answers how the week went.
func TestAcceptReviewDesign(t *testing.T) {
	app := acceptApp(t)
	app.goTab(TabReviews)
	r := stripANSIForTest(app.reviews.reportView(120))
	for _, want := range []string{"PLANNED", "ACTUAL", "COMPLETION", "SESSIONS", "TASKS", "OVER/UNDER", "TIME BY DOMAIN", "GOV PERFORMANCE", "DSA PERFORMANCE", "SWE TIME", "What worked well", "What should change"} {
		if !strings.Contains(strings.ToUpper(r), strings.ToUpper(want)) {
			t.Fatalf("reviews missing %q", want)
		}
	}
}
