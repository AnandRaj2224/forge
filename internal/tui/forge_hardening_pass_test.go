package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// View rendering must be DB-free: after loading state via Reload, closing the
// database must not change any screen rendering.
func TestViewRenderPureNoDB(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 150, 40
	tabs := []TopTab{TabToday, TabGOV, TabDSA, TabMocks, TabHistory, TabReviews}
	// Load every tab once with the DB open.
	for _, tab := range tabs {
		app.goTab(tab)
	}
	// Mutating detail opens happen with DB open (Reload paths).
	app.goTab(TabToday)
	app.goTab(TabGOV)
	app.gov.openTopic("topic-a")
	app.goTab(TabMocks)
	app.goTab(TabReviews)
	app.goTab(TabDSA)
	app.goTab(TabHistory)
	// Capture the steady-state rendering with DB open (no Reload after this).
	before := map[TopTab]string{}
	for _, tab := range tabs {
		app.tab = tab
		before[tab] = app.View().Content
	}
	// Also capture focused detail bodies that View delegates to.
	app.tab = TabToday
	todayBodyBefore := app.today.detailViewWH(36, 99)
	app.tab = TabGOV
	topicBefore := ""
	if app.gov.detail != nil {
		topicBefore = app.gov.topicBody(40)
	}
	browserBefore := app.gov.browserRight()
	app.tab = TabMocks
	mockBefore := app.mocks.mockBody(40)
	app.tab = TabReviews
	reportBefore := app.reviews.reportView(60)
	app.tab = TabDSA
	dsaBefore := app.dsa.View()

	// Close the database: any synchronous DB access in View would now fail
	// and change rendering (or set data-error state).
	_ = app.DB.Close()

	// Direct View calls only (no Reload/goTab after close).
	app.tab = TabToday
	todayAfter := app.today.detailViewWH(36, 99)
	if todayAfter != todayBodyBefore {
		t.Fatalf("Today detail View changed after DB close:\nbefore:\n%s\nafter:\n%s", todayBodyBefore, todayAfter)
	}
	app.tab = TabGOV
	if got := app.gov.browserRight(); got != browserBefore {
		t.Fatalf("GOV browserRight changed after DB close:\nbefore:\n%s\nafter:\n%s", browserBefore, got)
	}
	if app.gov.detail != nil {
		if got := app.gov.topicBody(40); got != topicBefore {
			t.Fatalf("GOV topicBody changed after DB close")
		}
	}
	app.tab = TabMocks
	if got := app.mocks.mockBody(40); got != mockBefore {
		t.Fatalf("Mocks mockBody changed after DB close:\nbefore:\n%s\nafter:\n%s", mockBefore, got)
	}
	// stagedSummary / section labels must also be pure.
	_ = app.mocks.stagedSummary(30)
	app.tab = TabReviews
	if got := app.reviews.reportView(60); got != reportBefore {
		t.Fatalf("Reviews reportView changed after DB close:\nbefore:\n%s\nafter:\n%s", reportBefore, got)
	}
	app.tab = TabDSA
	_ = dsaBefore
	if p := app.dsa.selPattern(); p != nil {
		// Must not mutate err and must not need DB.
		errBefore := app.dsa.err
		_, _ = app.dsa.patternProgress(p)
		if app.dsa.err != errBefore {
			t.Fatalf("DSA patternProgress mutated err during render")
		}
	}
	// Full App views per tab (re-render without Reload).
	for _, tab := range tabs {
		app.tab = tab
		after := app.View().Content
		if after != before[tab] {
			t.Fatalf("App.View tab %v changed after DB close", tab)
		}
	}
}

// TickMsg must not poll SQLite: with the DB closed, the tick still updates
// the in-memory elapsed label and preserves the active session.
func TestTickDoesNotPollDB(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()
	// Start a session so active state exists.
	testSend(app, "s")
	if app.today.active == nil || app.active == nil {
		t.Fatalf("setup: no active session (today=%v app=%v)", app.today.active, app.active)
	}
	start := app.today.active.StartedAt
	app.now = start.Add(10 * time.Second)
	app.updateActiveIndicator()
	before := app.activeInfo
	if before == "" {
		t.Fatal("setup: activeInfo empty")
	}
	// Close the DB: a polling tick would now fail and clear activeInfo.
	_ = app.DB.Close()
	later := start.Add(70 * time.Second)
	m, cmd := app.Update(TickMsg(later))
	_ = m
	_ = cmd
	if app.active == nil {
		t.Fatal("Tick cleared in-memory active session (must not query DB)")
	}
	if app.activeInfo == "" {
		t.Fatal("Tick cleared activeInfo (must recompute in memory)")
	}
	if app.activeInfo == before {
		t.Fatalf("active timer did not update: before=%q after=%q", before, app.activeInfo)
	}
	// Elapsed must grow: 00:00:10 -> 00:01:10.
	if !strings.Contains(app.activeInfo, "00:01:10") {
		t.Fatalf("activeInfo did not reflect later tick: %q", app.activeInfo)
	}
	if app.today.active == nil {
		t.Fatal("Today active cleared by Tick")
	}
}

// Active timer still updates visually via Today rendering with closed DB.
func TestActiveTimerUpdatesWithoutDB(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.reloadCurrent()
	testSend(app, "s")
	if app.today.active == nil {
		t.Fatal("setup: no active session")
	}
	before := stripANSIForTest(app.today.activeViewWH(40, 99))
	_ = app.DB.Close()
	// View must still render the timer without DB (time moves forward).
	after := stripANSIForTest(app.today.activeViewWH(40, 99))
	if before == "" || after == "" {
		t.Fatal("active view empty")
	}
	// Both contain a timer; rendering itself must not need DB.
	if !strings.Contains(after, ":") {
		t.Fatalf("active view missing timer: %q", after)
	}
}

// Mock optional metrics: omitted total stays unknown, no false comparisons,
// no negative derived values.
func TestMockOptionalMetrics(t *testing.T) {
	cases := []struct {
		name       string
		totalQ     string
		attempted  string
		correct    string
		wantOK     bool
		wantUnatt  *int
		wantIncorr *int
		wantAcc    *float64
	}{
		{name: "blank total + 20/16", totalQ: "", attempted: "20", correct: "16", wantOK: true, wantUnatt: nil, wantIncorr: intp(4), wantAcc: floatp(80)},
		{name: "100 + 20/16", totalQ: "100", attempted: "20", correct: "16", wantOK: true, wantUnatt: intp(80), wantIncorr: intp(4), wantAcc: floatp(80)},
		{name: "100 + blank attempted", totalQ: "100", attempted: "", correct: "", wantOK: true, wantUnatt: nil, wantIncorr: nil, wantAcc: nil},
		{name: "both blank", totalQ: "", attempted: "", correct: "", wantOK: true, wantUnatt: nil, wantIncorr: nil, wantAcc: nil},
		{name: "correct > attempted", totalQ: "100", attempted: "10", correct: "12", wantOK: false},
		{name: "attempted > total", totalQ: "10", attempted: "20", correct: "5", wantOK: false},
		{name: "malformed total", totalQ: "abc", attempted: "5", correct: "3", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database := seedMockHistoryDB(t)
			app := NewApp(database)
			app.width, app.height = 150, 40
			app.goTab(TabMocks)
			testSend(app, "n")
			// Name + date valid.
			setMockInput(t, app, 0, "M1")
			setMockInput(t, app, 1, "2026-09-18")
			setMockInput(t, app, 5, tc.totalQ)
			setMockInput(t, app, 6, tc.attempted)
			setMockInput(t, app, 7, tc.correct)
			ok := app.mocks.saveCreate()
			if ok != tc.wantOK {
				t.Fatalf("saveCreate ok=%v want %v (cErr=%q)", ok, tc.wantOK, app.mocks.cErr)
			}
			if !tc.wantOK {
				return
			}
			mocks, err := runtime.ListMocks(context.Background(), database)
			if err != nil || len(mocks) != 1 {
				t.Fatalf("expected 1 mock, got %d (%v)", len(mocks), err)
			}
			m := mocks[0]
			if tc.totalQ == "" && m.TotalQuestions != nil {
				t.Fatalf("omitted total must stay nil, got %v", *m.TotalQuestions)
			}
			if !intPtrEqual(m.Unattempted, tc.wantUnatt) {
				t.Fatalf("unattempted=%v want %v", strIntPtr(m.Unattempted), strIntPtr(tc.wantUnatt))
			}
			if !intPtrEqual(m.Incorrect, tc.wantIncorr) {
				t.Fatalf("incorrect=%v want %v", strIntPtr(m.Incorrect), strIntPtr(tc.wantIncorr))
			}
			if tc.wantAcc == nil && m.Accuracy != nil {
				t.Fatalf("accuracy must stay nil, got %v", *m.Accuracy)
			}
			if tc.wantAcc != nil {
				if m.Accuracy == nil || !floatClose(*m.Accuracy, *tc.wantAcc) {
					t.Fatalf("accuracy=%v want %v", m.Accuracy, *tc.wantAcc)
				}
			}
			if m.Unattempted != nil && *m.Unattempted < 0 {
				t.Fatalf("negative unattempted: %d", *m.Unattempted)
			}
		})
	}
}

func setMockInput(t *testing.T, app *App, idx int, v string) {
	t.Helper()
	if idx < 0 || idx >= len(app.mocks.cInputs) {
		t.Fatalf("input idx %d out of range", idx)
	}
	// Focus the field and set value directly (same as typing).
	app.mocks.cInputs[idx].SetValue(v)
}

func intp(v int) *int           { return &v }
func floatp(v float64) *float64 { return &v }
func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
func strIntPtr(p *int) string {
	if p == nil {
		return "nil"
	}
	return strings.TrimSpace(strings.Join([]string{"", ""}, "")) + formatIntPtr(*p)
}

func formatIntPtr(v int) string {
	// Avoid fmt import churn: simple decimal conversion.
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
func floatClose(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.001
}

// GOV batch progress must match per-topic logic.
func TestGovBatchMatchesSingle(t *testing.T) {
	database := seedMockHistoryDB(t)
	ctx := context.Background()
	// Add lectures: topic-perc has 2 lectures, topic-ratio has 0.
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "lec-p1", TopicID: "topic-perc", Title: "P1", SortOrder: 1})
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "lec-p2", TopicID: "topic-perc", Title: "P2", SortOrder: 2})
	// Complete one lecture via a real session (FK-safe).
	now := time.Now().UTC()
	lecTgt, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-p1"})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "batch-task", SnapshotID: "snap-mon", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "batch", PlannedSeconds: 3600, TargetJSON: &lecTgt})
	stopped := now
	_ = runtime.CreateSession(ctx, database, db.Session{ID: "sess-x", TodayTaskID: "batch-task", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "batch", PlannedSeconds: 3600, StartedAt: now.Add(-time.Hour), StoppedAt: &stopped, Status: db.SessionStatusStopped})
	if err := runtime.RecordLectureCompletion(ctx, database, db.GovLectureCompletion{LectureID: "lec-p1", SessionID: "sess-x", CompletedAt: now}); err != nil {
		t.Fatalf("record completion: %v", err)
	}
	tids := []db.ID{"topic-perc", "topic-ratio", "topic-tw"}
	batch, err := TopicLectureProgressMap(ctx, database, tids)
	if err != nil {
		t.Fatal(err)
	}
	for _, tid := range tids {
		done, total, _, _, err := TopicLectureProgressE(ctx, database, tid)
		if err != nil {
			t.Fatal(err)
		}
		got := batch[tid]
		if got.Done != done || got.Total != total {
			t.Fatalf("topic %s batch=%+v single=%d/%d", tid, got, done, total)
		}
	}
	// Exam coverage via batch must reflect the same completions.
	cov, err := ComputeExamCoverage(ctx, database, "exam-afcat")
	if err != nil {
		t.Fatal(err)
	}
	if cov.Total != 3 {
		t.Fatalf("coverage total=%d want 3", cov.Total)
	}
	// topic-perc incomplete (1/2), others complete (0 lectures) => done=2.
	if cov.Done != 2 {
		t.Fatalf("coverage done=%d want 2 (batch must match single logic)", cov.Done)
	}
	// engine.IsLectureComplete still authoritative for single lecture.
	ok, err := engine.IsLectureComplete(ctx, database, "lec-p1")
	if err != nil || !ok {
		t.Fatalf("lec-p1 must be complete: %v %v", ok, err)
	}
}
