package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// ---------- FINAL UX: keybinding model ----------

func TestFinalKeybindingModel(t *testing.T) {
	// Nested q/Esc = Back, never quit.
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.goTab(TabGOV)
	testSend(app, "enter")
	testSend(app, "l")
	testSend(app, "enter")
	if app.gov.level != govTopic {
		t.Fatalf("setup topic: %q", app.gov.level)
	}
	if _, cmd := app.Update(testKey("q")); cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatal("nested q must Back, never Quit")
			}
		}
	}
	if app.gov.level != govExam {
		t.Fatalf("nested q must Back to exam, got %q", app.gov.level)
	}
	if _, cmd := app.Update(testKey("esc")); cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatal("Esc must never quit")
			}
		}
	}
	if app.gov.level != govDashboard {
		t.Fatalf("nested Esc must Back to dashboard, got %q", app.gov.level)
	}
	// Ctrl+C quits in normal navigation.
	if _, cmd := app.Update(testKey("ctrl+c")); cmd == nil {
		t.Fatal("Ctrl+C must quit in normal nav")
	} else if msg := cmd(); msg != nil {
		if _, isQuit := msg.(tea.QuitMsg); !isQuit {
			t.Fatalf("Ctrl+C must Quit, got %T", msg)
		}
	}

	// Text input: q types, Esc cancels, Ctrl+C quits, Esc never quits.
	app2 := newFlowApp(t)
	app2.width, app2.height = 120, 36
	app2.reloadCurrent()
	testSend(app2, "j")
	testSend(app2, "s")
	testSend(app2, "x")
	if app2.today.right != "result" {
		t.Fatalf("setup form: %q", app2.today.right)
	}
	testSend(app2, "q")
	if got := app2.today.result.prows[0].Time.Input.Value(); !strings.Contains(got, "q") {
		t.Fatalf("text q must type, got %q", got)
	}
	if _, cmd := app2.Update(testKey("esc")); cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatal("text Esc must cancel, never quit")
			}
		}
	}
	if app2.today.right != "detail" {
		t.Fatal("text Esc must cancel form to detail")
	}
	// Ctrl+C quits even inside text inputs.
	app3 := newFlowApp(t)
	app3.width, app3.height = 120, 36
	app3.reloadCurrent()
	testSend(app3, "j")
	testSend(app3, "s")
	testSend(app3, "x")
	if _, cmd := app3.Update(testKey("ctrl+c")); cmd == nil {
		t.Fatal("Ctrl+C must quit inside text input")
	} else if msg := cmd(); msg != nil {
		if _, isQuit := msg.(tea.QuitMsg); !isQuit {
			t.Fatalf("Ctrl+C in text must Quit, got %T", msg)
		}
	}

	// Mock create text: qqq types, Q types Q.
	app4 := NewApp(seedMockHistoryDB(t))
	app4.width, app4.height = 150, 40
	app4.goTab(TabMocks)
	testSend(app4, "n")
	testSend(app4, "down")
	for _, k := range []string{"q", "q", "q"} {
		testSend(app4, k)
	}
	if got := app4.mocks.cInputs[0].Value(); got != "qqq" {
		t.Fatalf("mock name qqq=%q", got)
	}
	testSend(app4, "Q")
	if got := app4.mocks.cInputs[0].Value(); got != "qqqQ" {
		t.Fatalf("mock name Q must type Q, got %q", got)
	}
	if _, cmd := app4.Update(testKey("ctrl+c")); cmd == nil {
		t.Fatal("Ctrl+C must quit in mock create")
	}
}

// Exact static footer string/layout: single source of truth, fixed order,
// uppercase labels, fixed geometry.
func TestStaticFooterExact(t *testing.T) {
	model := globalFooterModel()
	wantKeys := []string{"Q", "ESC", "CTRL+C", "J/K", "↑/↓", "H/L", "ENTER", "SPACE", "N", "S", "X"}
	wantDescs := []string{"BACK", "BACK/CANCEL", "QUIT", "MOVE", "MOVE", "PANEL", "VIEW/SAVE", "TOGGLE", "NEW", "START", "STOP"}
	if len(model) != len(wantKeys) {
		t.Fatalf("footer model len=%d want %d", len(model), len(wantKeys))
	}
	for i := range wantKeys {
		if model[i].Key != wantKeys[i] || model[i].Desc != wantDescs[i] {
			t.Fatalf("footer item %d = %q/%q want %q/%q", i, model[i].Key, model[i].Desc, wantKeys[i], wantDescs[i])
		}
	}
	// Rendered layout: single row, fixed height, same badges everywhere.
	app := newFlowApp(t)
	app.width, app.height = 150, 40
	rendered := stripANSIForTest(GlobalFooter(app.width))
	for _, want := range []string{"Q", "BACK", "ESC", "BACK/CANCEL", "CTRL+C", "QUIT", "J/K", "MOVE", "H/L", "PANEL", "ENTER", "VIEW/SAVE", "SPACE", "TOGGLE", "NEW", "START", "STOP"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered footer missing %q: %q", want, rendered)
		}
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) != 2 {
		// Border top + single content row.
		t.Fatalf("footer must be fixed 2-line geometry, got %d lines: %q", len(lines), rendered)
	}
}

// ---------- STATIC FOOTER: one app-wide legend ----------

func staticFooterStates(t *testing.T) []*App {
	t.Helper()
	// Collect apps in diverse states: tabs, nested/detail, forms, text inputs.
	var out []*App
	base := newFlowApp(t)
	base.width, base.height = 150, 40
	out = append(out, base)
	for _, tab := range []TopTab{TabGOV, TabDSA, TabMocks, TabHistory, TabReviews} {
		a := newFlowApp(t)
		a.width, a.height = 150, 40
		a.goTab(tab)
		out = append(out, a)
	}
	// Nested/detail: GOV topic, DSA problem, mock detail, history detail, review report.
	nested := newFlowApp(t)
	nested.width, nested.height = 150, 40
	nested.goTab(TabGOV)
	testSend(nested, "enter")
	testSend(nested, "l")
	testSend(nested, "enter")
	out = append(out, nested)
	// Text input: Today practice form.
	form := newFlowApp(t)
	form.width, form.height = 150, 40
	form.reloadCurrent()
	testSend(form, "j")
	testSend(form, "s")
	testSend(form, "x")
	out = append(out, form)
	// Mock create form.
	mc := NewApp(seedMockHistoryDB(t))
	mc.width, mc.height = 150, 40
	mc.goTab(TabMocks)
	testSend(mc, "n")
	out = append(out, mc)
	return out
}

func TestFinalFooterUnified(t *testing.T) {
	// 1. Identical everywhere (tabs, nested/detail, forms, text inputs).
	apps := staticFooterStates(t)
	ref := stripANSIForTest(apps[0].footerView(apps[0].width))
	for i, a := range apps[1:] {
		got := stripANSIForTest(a.footerView(a.width))
		if got != ref {
			t.Fatalf("footer differs at state %d:\nref:  %q\ngot:  %q", i+1, ref, got)
		}
	}
	// 2. Never contains contextual-only labels/dates/page names.
	// N NEW, S START, X STOP are permanent legend entries.
	for i, a := range apps {
		foot := stripANSIForTest(a.footerView(a.width))
		for _, bad := range []string{"DELETE", "TODO", "DONE", "ACTIVE", "DRAFT"} {
			if strings.Contains(foot, bad) {
				t.Fatalf("state %d footer must never contain %q: %q", i, bad, foot)
			}
		}
		if strings.Contains(foot, "2026") || strings.Contains(foot, "TODAY") || strings.Contains(foot, "MOCKS") || strings.Contains(foot, "HISTORY") || strings.Contains(foot, "REVIEWS") {
			t.Fatalf("state %d footer has page/date metadata: %q", i, foot)
		}
		if strings.Contains(foot, "Q Quit") || strings.Contains(foot, "Q  QUIT") {
			t.Fatalf("state %d footer must never show Q Quit: %q", i, foot)
		}
	}
	// 3+4. Always contains the complete uppercase legend.
	for i, a := range apps {
		foot := stripANSIForTest(a.footerView(a.width))
		for _, want := range []string{"Q", "ESC", "CTRL+C", "J/K", "↑/↓", "H/L", "ENTER", "SPACE", "N", "S", "X"} {
			if !strings.Contains(foot, want) {
				t.Fatalf("state %d footer missing %q: %q", i, want, foot)
			}
		}
		for _, want := range []string{"BACK", "BACK/CANCEL", "QUIT", "MOVE", "PANEL", "VIEW/SAVE", "TOGGLE", "NEW", "START", "STOP"} {
			if !strings.Contains(foot, want) {
				t.Fatalf("state %d footer missing legend %q: %q", i, want, foot)
			}
		}
		// No lowercase key labels in rendered footer.
		for _, bad := range []string{"q Back", "Ctrl+C Quit", "Enter View", "Esc Back", "j/k", "h/l", "n New", "s Start", "x Stop"} {
			if strings.Contains(foot, bad) {
				t.Fatalf("state %d footer has lowercase label %q: %q", i, bad, foot)
			}
		}
	}
	// 5. Lowercase bindings still work (q/j/k/h/l).
	lb := newFlowApp(t)
	lb.width, lb.height = 150, 40
	lb.goTab(TabGOV)
	before := lb.gov.cursor
	testSend(lb, "j")
	if lb.gov.cursor == before {
		t.Fatal("lowercase j must move")
	}
	testSend(lb, "k")
	testSend(lb, "enter") // dashboard -> exam workspace
	if lb.gov.level != govExam {
		t.Fatalf("setup exam: %q", lb.gov.level)
	}
	testSend(lb, "l") // h/l panel switch
	if lb.gov.zone != gzRight {
		t.Fatal("lowercase l must switch panel")
	}
	testSend(lb, "h")
	if lb.gov.zone != gzLeft {
		t.Fatal("lowercase h must switch panel")
	}
	testSend(lb, "l")
	testSend(lb, "enter") // topics -> first topic detail
	if lb.gov.level != govTopic {
		t.Fatalf("setup topic: %q", lb.gov.level)
	}
	testSend(lb, "q")
	if lb.gov.level != govExam {
		t.Fatalf("lowercase q must Back, got %q", lb.gov.level)
	}
	// Screen actions trigger where valid (legend always shows them).
	ma := NewApp(seedMockHistoryDB(t))
	ma.width, ma.height = 150, 40
	ma.goTab(TabMocks)
	testSend(ma, "n")
	if ma.mocks.mode != "create" {
		t.Fatal("n New mock must still work")
	}
	sa := newFlowApp(t)
	sa.width, sa.height = 120, 36
	sa.reloadCurrent()
	testSend(sa, "s")
	if sa.today.active == nil {
		t.Fatal("s Start must still work")
	}
	testSend(sa, "x")
	if sa.today.right != "result" {
		t.Fatal("x Stop must still work")
	}
}

// ---------- FINAL UX: scrolling system-wide ----------

func TestFinalScrollingSystemWide(t *testing.T) {
	// Selectable pane follows focus.
	var rl RowList
	rl.SetWidth(30)
	rl.SetHeight(3)
	items := make([]RowItem, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, RowItem{ID: string(rune('a' + i)), Title: "item"})
	}
	rl.SetItems(items)
	for i := 0; i < 19; i++ {
		rl.CursorDown()
	}
	if rl.Cursor != 19 {
		t.Fatalf("cursor=%d want 19", rl.Cursor)
	}
	if rl.Cursor < rl.Offset || rl.Cursor >= rl.Offset+rl.Height {
		t.Fatalf("selected must remain visible: cursor=%d offset=%d h=%d", rl.Cursor, rl.Offset, rl.Height)
	}
	// Detail pane scrolls with hints.
	content := strings.Repeat("line\n", 40)
	var vp ScrollViewport
	v := ViewportView(content, &vp, 10)
	if !strings.Contains(stripANSIForTest(v), "more below") {
		t.Fatal("overflow must hint below")
	}
	vp.ScrollDown(41, 10)
	v2 := ViewportView(content, &vp, 10)
	if !strings.Contains(stripANSIForTest(v2), "more above") {
		t.Fatal("scrolled detail must hint above")
	}
	// Form SAVE reachable.
	app := newFlowApp(t)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-a", "topic-b", "topic-c"}})
	rf := NewResultForm(app)
	rf.Reload("sess-x", db.TodayTask{ID: "t-prac3", Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "P", PlannedSeconds: 3600, TargetJSON: &prac})
	rf.focus = rf.nFocus() - 1
	rf.updateFocusVisual()
	var vp2 ScrollViewport
	rf.ensureFocusVisible(&vp2, 5)
	if fl := rf.focusLine(); fl < vp2.Offset || fl > vp2.Offset+5-1 {
		t.Fatalf("form SAVE hidden: line=%d offset=%d", fl, vp2.Offset)
	}
	// Mock detail scrolls via shared viewport.
	appM := NewApp(seedMockHistoryDB(t))
	appM.width, appM.height = 150, 40
	appM.goTab(TabMocks)
	var mvp ScrollViewport
	mv := ViewportView(strings.Repeat("row\n", 30), &mvp, 8)
	if !strings.Contains(stripANSIForTest(mv), "more below") {
		t.Fatal("mock detail must scroll when overflowing")
	}
	// History + Reviews scroll.
	appH := NewApp(seedMockHistoryDB(t))
	appH.width, appH.height = 150, 40
	appH.history.nowFn = func() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }
	appH.goTab(TabHistory)
	var hvp ScrollViewport
	hv := ViewportView(strings.Repeat("h\n", 30), &hvp, 8)
	if !strings.Contains(stripANSIForTest(hv), "more below") {
		t.Fatal("history must scroll when overflowing")
	}
	appR := newFlowApp(t)
	appR.width, appR.height = 150, 40
	appR.goTab(TabReviews)
	testSend(appR, "l")
	appR.reviews.bodyScroll.Offset = 1 << 30
	if v := stripANSIForTest(appR.View().Content); !strings.Contains(v, "What worked well") {
		t.Fatal("review scrolled must reveal reflections")
	}
}

// ---------- FINAL UX: mock direct mapping ----------

func TestFinalMockDirectMapping(t *testing.T) {
	app := NewApp(seedMockHistoryDB(t))
	app.width, app.height = 150, 40
	app.goTab(TabMocks)
	testSend(app, "n")
	body := stripANSIForTest(app.mocks.createRightBody(40))
	for _, bad := range []string{"Section name", "ADD SECTION", "Add Section"} {
		if strings.Contains(body, bad) {
			t.Fatalf("mock create must not have arbitrary section layer %q", bad)
		}
	}
	for _, want := range []string{"GOV subject", "ADD SUBJECT", "GOV SUBJECTS IN THIS MOCK"} {
		if !strings.Contains(body, want) {
			t.Fatalf("mock create missing direct mapping %q:\n%s", want, body)
		}
	}
	// Direct subject selection + filtered weak topics + max 3 + ordering.
	if got := app.mocks.secSubjName(); got != "Quant" {
		t.Fatalf("subject=%q want Quant", got)
	}
	app.mocks.addStagedSubject()
	if app.mocks.mode != "weak" {
		t.Fatalf("mode=%q want weak", app.mocks.mode)
	}
	for _, tp := range app.mocks.wChoices {
		if tp.SubjectID != "subj-quant" {
			t.Fatalf("subject filter leak: %q", tp.Name)
		}
		if tp.ID == "topic-cd" {
			t.Fatal("syllabus filter leak: topic-cd")
		}
	}
	testSend(app, "down")
	testSend(app, "enter")
	testSend(app, "down")
	testSend(app, "enter")
	if len(app.mocks.wStaged) != 2 || app.mocks.wStaged[0] == app.mocks.wStaged[1] {
		t.Fatalf("ordering/duplicates wrong: %v", app.mocks.wStaged)
	}
	// Max 3: stage third, then fourth must fail.
	testSend(app, "down")
	testSend(app, "enter")
	if len(app.mocks.wStaged) != 3 {
		t.Fatalf("want 3 staged, got %v", app.mocks.wStaged)
	}
	app.mocks.toggleWeak("topic-fake-extra")
	if len(app.mocks.wStaged) != 3 {
		t.Fatalf("max 3 violated: %v", app.mocks.wStaged)
	}
	if app.mocks.wErr == "" {
		t.Fatal("max 3 must set error")
	}
	// No Delete in normal UI.
	before := len(app.mocks.mocks)
	testSend(app, "esc") // weak(staged) -> create
	testSend(app, "esc") // create -> browse
	if app.mocks.mode != "browse" {
		t.Fatalf("mode=%q want browse", app.mocks.mode)
	}
	testSend(app, "D")
	if len(app.mocks.mocks) != before {
		t.Fatal("D must not delete in normal UI")
	}
	if app.mocks.mode != "browse" {
		t.Fatalf("D must not change mode, got %q", app.mocks.mode)
	}
}

// ---------- FINAL UX: reviews simplified ----------

func TestFinalReviewsSimplified(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 150, 40
	// Pin to Monday 2026-09-21 (Asia/Kolkata current week).
	app.reviews.nowFn = func() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }
	app.goTab(TabReviews)
	if len(app.reviews.entries) == 0 {
		t.Fatal("current week entry must auto-exist")
	}
	curID := ""
	if cur := lastWeeks(app.reviews.nowFn(), 1); len(cur) > 0 {
		curID = cur[0].ID
	}
	found := false
	for _, e := range app.reviews.entries {
		if e.ID == curID {
			found = true
		}
	}
	if !found {
		t.Fatalf("current week %q must auto-exist in %v", curID, app.reviews.entries)
	}
	// No draft terminology in UI.
	full := stripANSIForTest(app.reviews.reportView(120)) + "\n" + stripANSIForTest(app.View().Content)
	if strings.Contains(strings.ToLower(full), "draft") {
		t.Fatalf("review UI must not contain draft language:\n%s", full)
	}
	// Metrics automated.
	for _, want := range []string{"PLANNED", "ACTUAL", "COMPLETION", "SESSIONS", "TASKS", "OVER/UNDER", "TIME BY DOMAIN", "GOV PERFORMANCE", "DSA PERFORMANCE"} {
		if !strings.Contains(strings.ToUpper(full), strings.ToUpper(want)) {
			t.Fatalf("review missing automated %q", want)
		}
	}
	// Exactly two reflection inputs.
	testSend(app, "l")
	testSend(app, "enter")
	if !app.reviews.editing {
		t.Fatal("enter must start editing live week")
	}
	if len(app.reviews.editIn) != 2 {
		t.Fatalf("reflections=%d want exactly 2", len(app.reviews.editIn))
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
		t.Fatal("SAVE must exit editing")
	}
	id := app.reviews.weeks.SelectedID()
	r, err := runtime.GetWeeklyReview(app.ctx(), app.DB, id)
	if err != nil {
		t.Fatalf("saved review missing: %v", err)
	}
	if r.ReflectionWorked != "Good" || r.ReflectionChange != "Bad" {
		t.Fatalf("reflections=%q/%q", r.ReflectionWorked, r.ReflectionChange)
	}
	// Saved is read-only.
	app.goTab(TabToday)
	app.goTab(TabReviews)
	// Reselect saved week.
	for i, e := range app.reviews.entries {
		if e.ID == id {
			app.reviews.weeks.Select(i)
		}
	}
	app.reviews.syncSelected()
	testSend(app, "l")
	testSend(app, "enter")
	if app.reviews.editing {
		t.Fatal("saved review must be read-only (no editing)")
	}
	rep := stripANSIForTest(app.reviews.reportView(120))
	if !strings.Contains(strings.ToLower(rep), "read-only") {
		t.Fatalf("saved review must show read-only:\n%s", rep)
	}
}

// ---------- FINAL UX: difficulty colors ----------

func TestFinalDifficultyColors(t *testing.T) {
	easy := DifficultyBadge("EASY")
	med := DifficultyBadge("MEDIUM")
	hard := DifficultyBadge("HARD")
	if stripANSIForTest(easy) != "EASY" || stripANSIForTest(med) != "MEDIUM" || stripANSIForTest(hard) != "HARD" {
		t.Fatalf("badges must read EASY/MEDIUM/HARD: %q %q %q", stripANSIForTest(easy), stripANSIForTest(med), stripANSIForTest(hard))
	}
	if easy == med || med == hard || easy == hard {
		t.Fatal("difficulty badges must use distinct semantic colors")
	}
	// Case-insensitive.
	if stripANSIForTest(DifficultyBadge("Easy")) != "EASY" {
		t.Fatalf("Easy must normalize: %q", stripANSIForTest(DifficultyBadge("Easy")))
	}
	// Row keeps badge text; selection stays independent (row still renders).
	rl := RowList{Width: 30, Height: 5, Focused: true}
	rl.SetItems([]RowItem{
		{ID: "a", Title: "Two Sum", Meta: "EASY", Mark: "○"},
		{ID: "b", Title: "Group", Meta: "MEDIUM", Mark: "○"},
		{ID: "c", Title: "Trap", Meta: "HARD", Mark: "○"},
	})
	rv := stripANSIForTest(rl.View())
	for _, want := range []string{"EASY", "MEDIUM", "HARD"} {
		if !strings.Contains(rv, want) {
			t.Fatalf("row list missing %q:\n%s", want, rv)
		}
	}
	rl.Select(1)
	rs := stripANSIForTest(rl.View())
	if !strings.Contains(rs, "MEDIUM") {
		t.Fatalf("selected row must keep difficulty:\n%s", rs)
	}
}

// ---------- MOCKS layout cleanup ----------

func TestMocksLayoutCleanup(t *testing.T) {
	// Wider left pane: ~40-42% via the shared layout calculation.
	if mockBrowseLeftPct < 40 || mockBrowseLeftPct > 42 {
		t.Fatalf("mockBrowseLeftPct=%d want 40-42", mockBrowseLeftPct)
	}
	lwNew, _ := TwoPaneLayout(150, mockBrowseLeftPct, 26, 40)
	lwOld, _ := TwoPaneLayout(150, 36, 26, 40)
	if lwNew <= lwOld {
		t.Fatalf("left pane not widened: new=%d old=%d", lwNew, lwOld)
	}

	ctx := context.Background()
	database := seedMockHistoryDB(t)
	now := time.Now().UTC()
	for _, m := range []db.Mock{
		{ID: "m-long-1", ExamID: "exam-afcat", Name: "AFCAT Full Mock • Manual 01", MockDate: "2026-09-22", Platform: "Testbook", CreatedAt: now},
		{ID: "m-long-2", ExamID: "exam-sbi", Name: "SBI Clerk Full Mock • Manual 01", MockDate: "2026-09-19", Platform: "Testbook", CreatedAt: now},
	} {
		if err := runtime.CreateMock(ctx, database, m); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp(database)
	app.width, app.height = 180, 40
	app.goTab(TabMocks)

	// Left pane is purely the mock list: no New mock action row.
	browse := stripANSIForTest(app.mocks.browseView())
	if strings.Contains(browse, "New mock") {
		t.Fatalf("mock list must not render a New mock row:\n%s", browse)
	}
	// Long rows sit naturally: number + name + date + exam on one row.
	for _, want := range []string{
		"AFCAT Full Mock • Manual 01", "22 Sep 2026", "AFCAT",
		"SBI Clerk Full Mock • Manual 01", "19 Sep 2026", "SBI Clerk",
	} {
		if !strings.Contains(browse, want) {
			t.Fatalf("mock list missing %q:\n%s", want, browse)
		}
	}

	// N still opens New Mock; footer-only legend, behavior unchanged.
	testSend(app, "n")
	if app.mocks.mode != "create" {
		t.Fatalf("N must open create, mode=%q", app.mocks.mode)
	}
	footCreate := stripANSIForTest(app.footerView(app.width))
	testSend(app, "esc")
	if app.mocks.mode != "browse" {
		t.Fatalf("Esc must leave create, mode=%q", app.mocks.mode)
	}
	footBrowse := stripANSIForTest(app.footerView(app.width))
	for _, foot := range []string{footBrowse, footCreate} {
		if !strings.Contains(foot, "N") || !strings.Contains(foot, "NEW") {
			t.Fatalf("static footer must keep N NEW: %q", foot)
		}
	}

	// Weak-topic hierarchy: edge header, 4-space maps/topics/dividers.
	if err := runtime.CreateMockSection(ctx, database, db.MockSection{ID: "sec-clean", MockID: "m-long-1", Name: "Quant", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SetWeakTopics(ctx, database, "m-long-1", "sec-clean", []db.ID{"topic-perc", "topic-ratio"}); err != nil {
		t.Fatal(err)
	}
	app.mocks.focusR = false
	app.mocks.Reload()
	body := stripANSIForTest(app.mocks.mockBody(60))
	lines := strings.Split(body, "\n")
	headIdx := -1
	for i, ln := range lines {
		if ln == "QUANT" {
			headIdx = i
			break
		}
	}
	if headIdx < 0 {
		t.Fatalf("subject header must sit at content edge:\n%s", body)
	}
	if got := lines[headIdx+1]; !strings.HasPrefix(got, "    maps to ") {
		t.Fatalf("maps line must be indented 4, got %q", got)
	}
	// Topic rows + in-group divider share one 4-space indentation.
	foundTopic, foundDiv := false, false
	for _, ln := range lines[headIdx+2:] {
		if len(ln) >= 8 && strings.HasPrefix(ln, "    ") && ln[4] >= '0' && ln[4] <= '9' {
			foundTopic = true
		}
		if strings.HasPrefix(ln, "    ─") {
			foundDiv = true
			rest := strings.TrimPrefix(ln, "    ")
			if strings.Trim(rest, "─") != "" {
				t.Fatalf("topic divider must span content width: %q", ln)
			}
		}
	}
	if !foundTopic {
		t.Fatalf("weak topics missing:\n%s", body)
	}
	if !foundDiv {
		t.Fatalf("in-group topic divider missing:\n%s", body)
	}
	for _, ln := range lines {
		if strings.HasPrefix(ln, "  maps to ") || strings.HasPrefix(ln, "  01") || strings.HasPrefix(ln, "  ─") {
			t.Fatalf("stale 2-space weak indent: %q", ln)
		}
	}

	// Navigation/scrolling unchanged: j/k list, h/l panel, Enter opens.
	first := app.mocks.list.SelectedID()
	testSend(app, "j")
	if app.mocks.list.SelectedID() == first {
		t.Fatal("j must move the mock list")
	}
	testSend(app, "k")
	if app.mocks.list.SelectedID() != first {
		t.Fatal("k must move the mock list back")
	}
	testSend(app, "l")
	if !app.mocks.focusR {
		t.Fatal("l must focus the detail pane")
	}
	testSend(app, "h")
	if app.mocks.focusR {
		t.Fatal("h must focus the list pane")
	}
	testSend(app, "l")
	testSend(app, "enter")
	if app.mocks.mode != "weak" {
		t.Fatalf("Enter must open weak topics, mode=%q (%s)", app.mocks.mode, app.mocks.wErr)
	}
	testSend(app, "esc")
	if app.mocks.mode != "browse" {
		t.Fatalf("Esc must leave weak editor, mode=%q", app.mocks.mode)
	}
}
