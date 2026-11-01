package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

func testKey(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space", " ":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		if len([]rune(s)) == 1 {
			r := []rune(s)[0]
			return tea.KeyPressMsg{Text: s, Code: r}
		}
		return tea.KeyPressMsg{Text: s, Code: tea.KeyExtended}
	}
}

func testSend(a *App, k string) {
	_, _ = a.Update(testKey(k))
}

func seedFlowDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	// Canonical Forge calendar settings (Asia/Kolkata, midnight): the engine
	// verifies these on Today generation, so fixtures must carry them.
	_ = catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 0, CreatedAt: now})
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "subj-quant", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-a", SubjectID: "subj-quant", Name: "Number System", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-b", SubjectID: "subj-quant", Name: "Percentage", SortOrder: 2})
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "lec-a", TopicID: "topic-a", Title: "Core Concepts", SortOrder: 1})
	_ = catalog.InsertGovExam(ctx, database, db.GovExam{ID: "exam-sbi", Name: "SBI Clerk", SortOrder: 1})
	_ = catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "exam-sbi", TopicID: "topic-a", SortOrder: 1})
	_ = catalog.InsertDSAProblem(ctx, database, db.DSAProblem{ID: "dsa-1", Title: "Two Sum", Pattern: "Hash Map", Difficulty: "EASY", SortOrder: 1})
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-a", LectureID: "lec-a"})
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-a", "topic-b"}})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "task1", SnapshotID: "snap1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "Quant lecture", PlannedSeconds: 3600, TargetJSON: &lec})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "task2", SnapshotID: "snap1", Position: 2, Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "Quant practice", PlannedSeconds: 3600, TargetJSON: &prac})
	_ = runtime.InsertWeeklyReviewOnce(ctx, database, db.WeeklyReview{ID: "2026-W38", WeekStart: "2026-09-14", WeekEnd: "2026-09-20", GeneratedAt: now, UpdatedAt: now, PlannedSeconds: 3600, ElapsedSeconds: 3000, OverrunSeconds: -600, ReportJSON: "{}"})
	t.Cleanup(func() { database.Close() })
	return database
}

// newFlowApp builds the Today workspace on the flow fixture with the clock
// pinned to the fixture date (2026-09-20): the Today screen resolves its
// date exclusively through engine.EnsureToday, so tests pin time instead of
// relying on any snapshot fallback (there is none).
func newFlowApp(t *testing.T) *App {
	t.Helper()
	app := NewApp(seedFlowDB(t))
	app.today.nowFn = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) }
	return app
}

// Today workspace: selection drives detail, start/stop/result stay in place.
func TestTodayWorkspaceFlow(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()

	before := app.today.detailView()
	testSend(app, "j")
	if after := app.today.detailView(); before == after {
		t.Fatal("selected-task detail did not auto-update on selection move")
	}
	testSend(app, "k")
	testSend(app, "s")
	if app.today.active == nil {
		t.Fatalf("start failed: %s", app.err)
	}
	if app.today.right != "detail" {
		t.Fatal("start must stay inside Today")
	}
	testSend(app, "x")
	if app.today.right != "result" {
		t.Fatal("stop must swap the right-bottom panel to the result form in place")
	}
	// Lecture form: choose YES, advance to SAVE, save.
	testSend(app, " ")
	testSend(app, "down")
	testSend(app, "enter")
	if app.today.right != "detail" {
		t.Fatalf("save did not complete: %s", app.today.result.err)
	}
	if st := ComputeTaskStatus(app.ctx(), app.DB, "task1"); st != TaskCompleted {
		t.Fatalf("expected task COMPLETED after YES save, got %s", st)
	}
}

// GOV: dashboard cards -> exam workspace -> topic detail, context kept.
func TestGovDashboardFlow(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 40
	app.goTab(TabGOV)
	if app.gov.level != govDashboard {
		t.Fatalf("GOV must open on the exam dashboard, got %q", app.gov.level)
	}
	testSend(app, "enter")
	if app.gov.level != govExam {
		t.Fatalf("Enter must open the exam workspace, got %q", app.gov.level)
	}
	if app.gov.coverage == nil || app.gov.coverage.Exam != "SBI Clerk" {
		t.Fatalf("workspace must show the selected exam, got %+v", app.gov.coverage)
	}
	for _, sc := range app.gov.coverage.Subjects {
		if sc.SubjectID != "subj-quant" {
			t.Fatalf("exam workspace must list only mapped subjects, got %q", sc.Subject)
		}
	}
	testSend(app, "l")     // focus topics
	testSend(app, "enter") // open the topic in the right panel
	if app.gov.level != govTopic {
		t.Fatalf("topic must open in-panel, got %q", app.gov.level)
	}
	if app.gov.detail == nil || app.gov.detail.Topic.ID != "topic-a" {
		t.Fatalf("wrong topic detail: %+v", app.gov.detail)
	}
	testSend(app, "esc")
	if app.gov.level != govExam {
		t.Fatalf("Esc must return to the exam workspace, got %q", app.gov.level)
	}
	testSend(app, "esc")
	if app.gov.level != govDashboard {
		t.Fatalf("Esc must return to the dashboard, got %q", app.gov.level)
	}
}

// Practice form: digits reach inputs (not global 1-5 tab shortcuts),
// every bundled topic validates, SAVE persists all rows.
// Raw facts only: Time spent + Attempted + Correct (no Avg field).
func TestPracticeFormTypingAndSave(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()
	testSend(app, "j") // task2 = practice
	testSend(app, "s")
	testSend(app, "x")
	if app.today.right != "result" {
		t.Fatalf("expected practice form, got %q", app.today.right)
	}
	typeKeys := []string{"1", "2", "0", "0", "down", "1", "5", "down", "1", "2", "down",
		"9", "0", "0", "down", "1", "0", "down", "8", "down"}
	for _, k := range typeKeys {
		testSend(app, k)
	}
	if app.tab != TabToday {
		t.Fatalf("typing digits must not switch tabs, tab=%v", app.tab)
	}
	if got := app.today.result.prows[0].Time.Input.Value(); got != "1200" {
		t.Fatalf("time field = %q, want 1200", got)
	}
	testSend(app, "enter")
	if app.today.right != "detail" {
		t.Fatalf("practice save failed: %s", app.today.result.err)
	}
	n, err := countRows(app, `SELECT COUNT(*) FROM gov_practice_records`)
	if err != nil || n != 2 {
		t.Fatalf("expected 2 practice rows, got %d (%v)", n, err)
	}
}

func countRows(app *App, q string) (int, error) {
	var n int
	err := app.DB.QueryRowContext(app.ctx(), q).Scan(&n)
	return n, err
}

// Every top-level workspace renders at desktop and narrow widths with no
// row exceeding the viewport and no fake-text controls.
func TestWorkspacesFitViewport(t *testing.T) {
	app := newFlowApp(t)
	fakes := []string{"[ START SESSION ]", "[ STOP SESSION ]", "[ SAVE ]", "[ YES ] [ NO ]", "1:TODAY", "EXAMS (tab:"}
	for _, size := range [][2]int{{120, 36}, {70, 24}} {
		app.width, app.height = size[0], size[1]
		for _, tab := range []TopTab{TabToday, TabGOV, TabDSA, TabReviews} {
			app.goTab(tab)
			view := app.View().Content
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size[0] {
					t.Fatalf("tab %v %dx%d: row width %d exceeds viewport: %q", tab, size[0], size[1], lipgloss.Width(line), line)
				}
			}
			for _, bad := range fakes {
				if strings.Contains(view, bad) {
					t.Fatalf("tab %v contains fake control %q", tab, bad)
				}
			}
		}
	}
	for _, want := range []string{"ACTIVE SESSION", "SELECTED TASK", "SYLLABUS COVERAGE", "PATTERNS", "WEEKLY REPORT"} {
		found := false
		app.width, app.height = 140, 40
		for _, tab := range []TopTab{TabToday, TabGOV, TabDSA, TabReviews} {
			app.goTab(tab)
			if strings.Contains(app.View().Content, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no workspace renders %q", want)
		}
	}
}

// TestGlobalFooterAndColumns guards the static shell: one identical footer
// everywhere, structured Today columns, no bullet composites. The footer
// is the global legend only; it never carries metadata and never shows
// Q Quit (only CTRL+C QUIT).
func TestGlobalFooterAndColumns(t *testing.T) {
	for _, size := range [][2]int{{170, 44}, {150, 40}} {
		w, h := size[0], size[1]
		app := newFlowApp(t)
		app.width, app.height = w, h
		var ref string
		for _, tab := range []TopTab{TabToday, TabGOV, TabDSA, TabMocks, TabHistory, TabReviews} {
			app.goTab(tab)
			view := app.View().Content
			for i, ln := range strings.Split(view, "\n") {
				if lipgloss.Width(ln) > w {
					t.Fatalf("%dx%d tab %v line %d overflow", w, h, tab, i)
				}
			}
			plain := stripANSIForTest(view)
			fl := strings.Split(plain, "\n")
			foot := fl[len(fl)-2] + "\n" + fl[len(fl)-1]
			if ref == "" {
				ref = foot
			} else if foot != ref {
				t.Fatalf("%dx%d tab %v footer differs:\nref: %q\ngot: %q", w, h, tab, ref, foot)
			}
			// Static legend: complete uppercase keys, single quit command.
			for _, want := range []string{"CTRL+C", "QUIT", "Q", "ESC", "J/K", "↑/↓", "H/L", "ENTER", "SPACE", "N", "S", "X", "BACK", "MOVE", "PANEL", "VIEW/SAVE", "TOGGLE", "NEW", "START", "STOP"} {
				if !strings.Contains(foot, want) {
					t.Fatalf("%dx%d tab %v footer missing %q: %q", w, h, tab, want, foot)
				}
			}
			if strings.Contains(foot, "Q Quit") {
				t.Fatalf("%dx%d tab %v footer must never show Q Quit: %q", w, h, tab, foot)
			}
			for _, bad := range []string{"TODAY", "TODO", "DONE", "DRAFT", "ACTIVE", "DELETE", "r Refresh", "2026-"} {
				if strings.Contains(foot, bad) {
					t.Fatalf("%dx%d tab %v footer has metadata %q: %q", w, h, tab, bad, foot)
				}
			}
			if !strings.Contains(plain, "Everyone sees") || !strings.Contains(plain, "Machiavelli") {
				t.Fatalf("%dx%d tab %v quote broken", w, h, tab)
			}
		}
	}
	app := newFlowApp(t)
	app.width, app.height = 170, 44
	app.goTab(TabToday)
	v := stripANSIForTest(app.View().Content)
	for _, want := range []string{"TRACK", "AREA", "TYPE", "TASK", "DURATION", "STATUS", "LECTURE", "PRACTICE", "Quant", "Number System"} {
		if !strings.Contains(v, want) {
			t.Fatalf("today missing %q", want)
		}
	}
	if strings.Contains(v, "Quant •") {
		t.Fatalf("today still has bullet composite")
	}
	// Shared row MetaSub path (actual + planned metrics).
	var rl RowList
	rl.SetWidth(46)
	rl.SetHeight(2)
	rl.SetItems([]RowItem{{ID: "d", Title: "Mon · 21 Sep 2026", Meta: "15h 41m", MetaSub: "PLAN 7h 30m", Mark: "•"}})
	rv := stripANSIForTest(rl.View())
	if !strings.Contains(rv, "15h 41m") || !strings.Contains(rv, "PLAN 7h 30m") {
		t.Fatalf("row metrics broken:\n%s", rv)
	}
}
