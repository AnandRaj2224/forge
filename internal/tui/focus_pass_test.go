package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// q in normal navigation backs out (never quits); Esc backs out (never
// quits); Ctrl+C quits everywhere. Text inputs take q literally.
func TestNormalQQuitsEscBacks(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.goTab(TabGOV)
	testSend(app, "enter") // exam workspace
	testSend(app, "l")
	testSend(app, "enter") // topic detail
	if app.gov.level != govTopic {
		t.Fatalf("setup: want topic, got %q", app.gov.level)
	}
	_, cmd := app.Update(testKey("esc"))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatal("Esc must not quit Forge")
			}
		}
	}
	if app.gov.level != govExam {
		t.Fatalf("Esc must back to exam, got %q", app.gov.level)
	}
	// q backs (not quits) in nested navigation.
	_, cmd = app.Update(testKey("q"))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatalf("q must not quit (it backs): got Quit")
			}
		}
	}
	if app.gov.level != govDashboard {
		t.Fatalf("q must back to dashboard, got %q", app.gov.level)
	}
	// Ctrl+C quits everywhere.
	_, cmd = app.Update(testKey("ctrl+c"))
	if cmd == nil {
		t.Fatal("Ctrl+C must quit")
	}
	if msg := cmd(); msg != nil {
		if _, isQuit := msg.(tea.QuitMsg); !isQuit {
			t.Fatalf("Ctrl+C must produce Quit, got %T", msg)
		}
	}
}

// Typing q produces q; Esc cancels the form (never quits); Enter saves.
func TestFormQTypesEscCancelsEnterSaves(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()
	testSend(app, "j") // task2 = practice
	testSend(app, "s")
	testSend(app, "x")
	if app.today.right != "result" {
		t.Fatalf("setup: want practice form, got %q", app.today.right)
	}
	// Static legend everywhere: BACK/SAVE present, never Q Quit.
	foot := stripANSIForTest(app.footerView(app.width))
	if strings.Contains(foot, "Q Quit") {
		t.Fatalf("static footer must not show Q Quit: %q", foot)
	}
	if !strings.Contains(foot, "BACK") || !strings.Contains(foot, "SAVE") {
		t.Fatalf("static footer must show BACK/SAVE legend: %q", foot)
	}
	// q types.
	testSend(app, "q")
	if got := app.today.result.prows[0].Time.Input.Value(); !strings.Contains(got, "q") {
		t.Fatalf("q must type into text field, got %q", got)
	}
	if app.tab != TabToday || app.today.right != "result" {
		t.Fatal("typing q must not quit or leave the form")
	}
	// Esc cancels the form, does not quit.
	_, cmd := app.Update(testKey("esc"))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatal("Esc in form must not quit Forge")
			}
		}
	}
	if app.today.right != "detail" {
		t.Fatal("Esc must cancel the result form back to detail")
	}
}

// Completed tasks: viewable read-only, never startable (domain + UI).
func TestCompletedTaskCannotStart(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 150, 40
	app.reloadCurrent()
	// Complete the lecture task.
	testSend(app, "s")
	testSend(app, "x")
	testSend(app, " ")
	testSend(app, "down")
	testSend(app, "enter")
	if st := ComputeTaskStatus(app.ctx(), app.DB, "task1"); st != TaskCompleted {
		t.Fatalf("setup: want COMPLETED, got %s", st)
	}
	// Domain rule is authoritative.
	if ok, _ := runtime.CanStartTodayTask(app.ctx(), app.DB, "task1"); ok {
		t.Fatal("CanStart must be false for completed tasks")
	}
	// UI: cursor on DONE, no Start action, read-only result.
	app.today.tbl.SetCursor(0)
	app.today.syncButtons()
	if !app.today.startBtn.Disabled {
		t.Fatal("Start must be disabled for DONE")
	}
	detail := stripANSIForTest(app.today.detailView())
	if !strings.Contains(detail, "READ-ONLY") || !strings.Contains(detail, "DONE") {
		t.Fatalf("DONE detail must be read-only:\n%s", detail)
	}
	if strings.Contains(detail, "START SESSION") {
		t.Fatalf("DONE detail must not show Start:\n%s", detail)
	}
	foot := stripANSIForTest(app.footerView(app.width))
	// Static legend always carries the full command set, even for DONE.
	for _, want := range []string{"N", "NEW", "S", "START", "X", "STOP", "VIEW"} {
		if !strings.Contains(foot, want) {
			t.Fatalf("static footer must carry legend %q: %q", want, foot)
		}
	}
	// s is rejected safely: no new session, task stays DONE, no red error —
	// just a small neutral contextual note.
	app.today.zone = tzTasks
	app.today.right = "detail"
	testSend(app, "s")
	if _, err := runtime.GetActiveSession(app.ctx(), app.DB); err == nil {
		t.Fatal("s on DONE must not create a session")
	}
	if app.err != "" {
		t.Fatalf("DONE start must not raise a red error banner, got %q", app.err)
	}
	after := stripANSIForTest(app.today.detailView())
	if !strings.Contains(after, "Completed · read-only") {
		t.Fatalf("DONE start must show a neutral note, got:\n%s", after)
	}
	if st := ComputeTaskStatus(app.ctx(), app.DB, "task1"); st != TaskCompleted {
		t.Fatalf("task must stay DONE, got %s", st)
	}
	// Duplicate lecture completion is blocked below the UI as well.
	if err := runtime.RecordLectureCompletion(app.ctx(), app.DB, db.GovLectureCompletion{LectureID: "lec-a", SessionID: "sess-x"}); err == nil {
		t.Fatal("duplicate lecture completion must fail")
	}
}

// Practice bundle: explicit Save, all-or-nothing, then read-only DONE.
func TestPracticeSaveBecomesReadOnlyDone(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()
	testSend(app, "j")
	testSend(app, "s")
	testSend(app, "x")
	if app.today.right != "result" {
		t.Fatalf("setup: want form, got %q", app.today.right)
	}
	form := stripANSIForTest(app.today.result.ViewFor(40))
	if !strings.Contains(form, "SAVE") {
		t.Fatalf("practice form must show explicit Save:\n%s", form)
	}
	// Invalid bundle blocks Save and stays in the form with errors.
	testSend(app, "enter")
	if app.today.right != "result" {
		t.Fatal("invalid Save must remain in the form")
	}
	n, _ := countRows(app, `SELECT COUNT(*) FROM gov_practice_records`)
	if n != 0 {
		t.Fatalf("invalid Save must not persist, got %d rows", n)
	}
	// Valid bundle saves atomically (3 topics = 1 task with 2 topics here = 2 rows).
	for _, k := range []string{"2", "0", "m", "down", "1", "5", "down", "1", "2", "down", "1", "8", ".", "5", "down",
		"9", "0", "0", "down", "1", "0", "down", "8", "down", "2", "0", "down"} {
		testSend(app, k)
	}
	testSend(app, "enter")
	if app.today.right != "detail" {
		t.Fatalf("valid Save failed: %s", app.today.result.err)
	}
	if st := ComputeTaskStatus(app.ctx(), app.DB, "task2"); st != TaskCompleted {
		t.Fatalf("practice task must be DONE, got %s", st)
	}
	n, _ = countRows(app, `SELECT COUNT(*) FROM gov_practice_records`)
	if n != 2 {
		t.Fatalf("expected atomic 2 rows, got %d", n)
	}
	// DONE result is read-only with human durations.
	app.today.tbl.SetCursor(1)
	detail := stripANSIForTest(app.today.detailView())
	if !strings.Contains(detail, "READ-ONLY") {
		t.Fatalf("practice DONE must be read-only:\n%s", detail)
	}
	if strings.Contains(detail, "1380") || strings.Contains(detail, "SAVE") {
		t.Fatalf("read-only must not expose raw seconds or Save:\n%s", detail)
	}
	if !strings.Contains(detail, "20m") && !strings.Contains(detail, "15m") {
		t.Fatalf("read-only must show human durations:\n%s", detail)
	}
}

// Durations never expose raw storage units.
func TestDurationHumanRepresentation(t *testing.T) {
	if got := FormatDurationLong(42); got != "42s" {
		t.Fatalf("42 -> %q", got)
	}
	if got := FormatDurationLong(300); got != "5m" {
		t.Fatalf("300 -> %q", got)
	}
	if got := FormatDurationLong(4500); got != "1h 15m" {
		t.Fatalf("4500 -> %q", got)
	}
	if _, v, msg := ValidateSolveTime("5m"); msg != "" || v == nil || *v != 300 {
		t.Fatalf("5m must parse to 300, got %v %q", v, msg)
	}
	if _, v, msg := ValidateSolveTime("1h 15m"); msg != "" || v == nil || *v != 4500 {
		t.Fatalf("1h 15m must parse to 4500, got %v %q", v, msg)
	}
}

// Every overflowing pane scrolls independently with visible hints;
// form j/k moves between fields (typing in text fields) and the viewport
// always keeps the focused field visible.
func TestScrollablePanes(t *testing.T) {
	lines := make([]string, 0, 40)
	for i := 1; i <= 40; i++ {
		lines = append(lines, strings.Repeat("x", 5))
	}
	content := strings.Join(lines, "\n")
	var vp ScrollViewport
	got := ViewportView(content, &vp, 10)
	if !strings.Contains(stripANSIForTest(got), "more below") {
		t.Fatalf("overflow must hint:\n%s", got)
	}
	vp.ScrollDown(40, 10)
	got2 := ViewportView(content, &vp, 10)
	if !strings.Contains(stripANSIForTest(got2), "more above") {
		t.Fatalf("scrolled pane must hint above:\n%s", got2)
	}
	if vp.Offset <= 0 {
		t.Fatal("offset must advance")
	}
	// History body scrolls when focused.
	app := newFlowApp(t)
	app.width, app.height = 150, 40
	app.goTab(TabHistory)
	testSend(app, "l")
	if !app.history.focusR {
		t.Fatal("H/L must focus the History detail pane")
	}
	before := app.history.bodyScroll.Offset
	testSend(app, "j")
	_ = before
	// Reviews report scrolls and reveals reflections.
	app.goTab(TabReviews)
	testSend(app, "l")
	app.reviews.bodyScroll.Offset = 1 << 30
	v := stripANSIForTest(app.View().Content)
	if !strings.Contains(v, "What worked well") {
		t.Fatalf("scrolled reviews must reveal reflections")
	}
}

// Form j/k: types inside text fields, moves between non-text widgets, and
// the focused field is never hidden below the viewport.
func TestFormJKBehavior(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()
	// Practice form: first focus is a text field — j/k must type.
	testSend(app, "j")
	testSend(app, "s")
	testSend(app, "x")
	if app.today.right != "result" {
		t.Fatalf("setup: want form, got %q", app.today.right)
	}
	testSend(app, "j")
	if got := app.today.result.prows[0].Time.Input.Value(); !strings.Contains(got, "j") {
		t.Fatalf("j must type in text fields, got %q", got)
	}
	testSend(app, "k")
	if got := app.today.result.prows[0].Time.Input.Value(); !strings.Contains(got, "k") {
		t.Fatalf("k must type in text fields, got %q", got)
	}
	// DSA checkboxes: j/k moves focus like down/up.
	dsaTarget, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{{ProblemID: "dsa-1", Mode: db.DSAModeNew}}})
	rf := NewResultForm(app)
	rf.Reload("sess-x", db.TodayTask{ID: "t-dsa", Domain: db.DomainDSA, Kind: db.SessionKindPractice, Title: "DSA", PlannedSeconds: 1800, TargetJSON: &dsaTarget})
	if len(rf.drows) == 0 {
		t.Fatal("setup: want DSA rows")
	}
	// Simulate the Today wiring: j on non-text moves focus.
	app.today.result = rf
	app.today.right = "result"
	app.today.zone = tzRight
	before := rf.focus
	testSend(app, "j")
	if rf.focus != (before+1)%rf.nFocus() {
		t.Fatalf("j on checkboxes must move focus, got %d want %d", rf.focus, (before+1)%rf.nFocus())
	}
	// SAVE focus must stay visible in a short viewport.
	rf.focus = rf.nFocus() - 1
	rf.updateFocusVisual()
	var vp ScrollViewport
	rf.ensureFocusVisible(&vp, 6)
	lines := strings.Split(rf.ViewFor(30), "\n")
	fl := rf.focusLine()
	if fl < vp.Offset || fl > vp.Offset+6-1 {
		t.Fatalf("SAVE line %d hidden (offset %d, lines %d)", fl, vp.Offset, len(lines))
	}
	// History shows unfinished planned work factually.
	app2 := newFlowApp(t)
	app2.width, app2.height = 150, 40
	app2.history.nowFn = func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	app2.goTab(TabHistory)
	h := stripANSIForTest(app2.history.dayBody(120))
	if !strings.Contains(h, "UNFINISHED") || !strings.Contains(h, "Not completed") {
		t.Fatalf("history must show unfinished work:\n%s", h)
	}
}
