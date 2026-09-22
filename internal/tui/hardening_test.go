package tui

import (
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"github.com/charmbracelet/x/ansi"
)

// Time-by-domain must read "% of tracked time", never "% of day".
func TestTrackedTimeWording(t *testing.T) {
	app := NewApp(seedMockHistoryDB(t))
	app.width, app.height = 150, 40
	app.history.nowFn = func() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }
	app.goTab(TabHistory)
	h := stripANSIForTest(app.history.dayBody(90))
	if !strings.Contains(h, "of tracked time") {
		t.Fatalf("history must use tracked-time percentage:\n%s", h)
	}
	if strings.Contains(strings.ToLower(h), "% of day") {
		t.Fatalf("history must not use %% of day:\n%s", h)
	}
	app2 := newFlowApp(t)
	app2.width, app2.height = 150, 40
	app2.goTab(TabReviews)
	r := stripANSIForTest(app2.reviews.reportView(90))
	if !strings.Contains(r, "of tracked time") {
		t.Fatalf("reviews must use tracked-time percentage:\n%s", r)
	}
}

// Mock creation exposes the direct product model: real GOV subject
// selector, staged weak topics, real SAVE — all reachable by scrolling
// from one structured field model. No arbitrary section layer.
func TestMockCreateStructuredFields(t *testing.T) {
	app := NewApp(seedMockHistoryDB(t))
	app.width, app.height = 150, 40
	app.goTab(TabMocks)
	testSend(app, "n")
	body := stripANSIForTest(app.mocks.createRightBody(40))
	for _, want := range []string{"GOV subject", "ADD SUBJECT", "SAVE"} {
		if !strings.Contains(body, want) {
			t.Fatalf("create form missing %q:\n%s", want, body)
		}
	}
	for _, bad := range []string{"Section name", "ADD SECTION"} {
		if strings.Contains(body, bad) {
			t.Fatalf("create form must not contain arbitrary section layer %q:\n%s", bad, body)
		}
	}
	// SAVE is the last focusable and stays visible in a short viewport.
	if app.mocks.nCreateFocus() != 15 {
		t.Fatalf("focusables=%d want 15", app.mocks.nCreateFocus())
	}
	app.width, app.height = 150, 20 // short viewport forces scrolling
	app.mocks.cFocus = app.mocks.nCreateFocus() - 1
	app.mocks.followCreateFocus()
	lines := strings.Split(app.mocks.createRightBody(40), "\n")
	fl := app.mocks.createFocusLine()
	// SAVE button box starts 4 lines before the end (box 3 lines + help).
	if fl != len(lines)-4 {
		t.Fatalf("SAVE line=%d total=%d", fl, len(lines))
	}
	innerH := 5
	total := len(lines)
	app.mocks.formScroll.Clamp(total, innerH)
	if fl < app.mocks.formScroll.Offset || fl > app.mocks.formScroll.Offset+innerH-1 {
		t.Fatalf("SAVE hidden: line=%d offset=%d", fl, app.mocks.formScroll.Offset)
	}
}

// Weak-topic choices filter to the staged subject AND exam syllabus.
func TestMockWeakFilteredBySubject(t *testing.T) {
	app := NewApp(seedMockHistoryDB(t))
	app.width, app.height = 150, 40
	app.goTab(TabMocks)
	testSend(app, "n")
	if got := app.mocks.secSubjName(); got != "Quant" {
		t.Fatalf("subject=%q want Quant", got)
	}
	app.mocks.addStagedSubject()
	if app.mocks.mode != "weak" {
		t.Fatalf("mode=%q want weak (%s)", app.mocks.mode, app.mocks.cErr)
	}
	if len(app.mocks.wChoices) == 0 {
		t.Fatal("no topic choices")
	}
	for _, tp := range app.mocks.wChoices {
		if tp.SubjectID != "subj-quant" {
			t.Fatalf("choice %q leaks across subjects", tp.Name)
		}
	}
	// AFCAT syllabus only: Coding-Decoding (SBI) must not appear.
	for _, tp := range app.mocks.wChoices {
		if tp.ID == "topic-cd" {
			t.Fatal("off-syllabus topic leaked into choices")
		}
	}
}

// A broken database surfaces a panel data error, never a silent empty state.
func TestTodayReloadDBError(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	_ = app.DB.Close()
	app.today.Reload()
	if app.today.err == "" {
		t.Fatal("broken DB must surface a data error")
	}
	if !strings.Contains(app.today.err, "data error") {
		t.Fatalf("err=%q want data-error state", app.today.err)
	}
}

// Starting while another session runs is a neutral notice: no new session,
// no red banner.
func TestBusyStartIsNeutral(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()
	testSend(app, "s") // task1 lecture starts
	if app.today.active == nil {
		t.Fatalf("setup start failed: %s", app.err)
	}
	testSend(app, "j") // task2 practice
	testSend(app, "s") // blocked: busy
	if app.err != "" {
		t.Fatalf("busy start must not raise a red error, got %q", app.err)
	}
	if !strings.Contains(app.today.notice, "active") {
		t.Fatalf("busy start must note neutrally, got %q", app.today.notice)
	}
	n, _ := countRows(app, `SELECT COUNT(*) FROM sessions`)
	if n != 1 {
		t.Fatalf("busy start must not create a session, got %d", n)
	}
}

// Empty days distinguish zero from missing: accuracy renders —, not 0%.
func TestHistoryZeroVsMissing(t *testing.T) {
	app := NewApp(seedMockHistoryDB(t))
	app.width, app.height = 150, 40
	app.history.nowFn = func() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }
	app.goTab(TabHistory)
	h := stripANSIForTest(app.history.dayBody(90))
	if !strings.Contains(h, "Accuracy") {
		t.Fatalf("history must show accuracy row:\n%s", h)
	}
	if !strings.Contains(h, "—") {
		t.Fatalf("missing accuracy must render —:\n%s", h)
	}
	if strings.Contains(h, "0.0%") {
		t.Fatalf("missing accuracy must not render 0.0%%:\n%s", h)
	}
}

// SAVE stays reachable by scrolling for a full 3-topic practice bundle.
func TestPracticeSaveReachableFullBundle(t *testing.T) {
	app := newFlowApp(t)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-a", "topic-b", "topic-c"}})
	rf := NewResultForm(app)
	rf.Reload("sess-x", db.TodayTask{ID: "t-prac3", Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "P", PlannedSeconds: 3600, TargetJSON: &prac})
	if len(rf.prows) != 3 {
		t.Fatalf("rows=%d want 3", len(rf.prows))
	}
	rf.focus = rf.nFocus() - 1
	rf.updateFocusVisual()
	var vp ScrollViewport
	rf.ensureFocusVisible(&vp, 5)
	fl := rf.focusLine()
	if fl < vp.Offset || fl > vp.Offset+5-1 {
		t.Fatalf("SAVE line %d hidden (offset %d)", fl, vp.Offset)
	}
	if !strings.Contains(stripANSIForTest(rf.ViewFor(30)), "SAVE") {
		t.Fatal("form must show explicit SAVE")
	}
}

// The mock create form (fields + subject block + SAVE) must fit narrow
// viewports without overflowing rows.
func TestMockCreateFitsNarrow(t *testing.T) {
	app := NewApp(seedMockHistoryDB(t))
	app.width, app.height = 70, 24
	app.goTab(TabMocks)
	testSend(app, "n")
	v := stripANSIForTest(app.mocks.createView())
	for i, ln := range strings.Split(v, "\n") {
		if ansi.StringWidth(ln) > 70 {
			t.Fatalf("line %d width %d overflows:\n%q", i, ansi.StringWidth(ln), ln)
		}
	}
}
