package engine_test

// SWE time-allocation model: routine slot -> area -> Today block ->
// generic session -> elapsed time. No tasks, no bundles, no outcomes.

import (
	"context"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/engine"
)

func openSWETimeDB(t *testing.T) (*db.DB, context.Context) {
	t.Helper()
	d := openEngineDB(t)
	return d, context.Background()
}

func seedSWEAreas(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, a := range []db.SWEArea{
		{ID: "area-go", Name: "Go", SortOrder: 1},
		{ID: "area-docker", Name: "Docker", SortOrder: 2},
		{ID: "area-linux", Name: "Linux", SortOrder: 3},
		{ID: "area-sql", Name: "SQL", SortOrder: 4},
	} {
		if err := catalog.InsertSWEArea(ctx, d, a); err != nil {
			t.Fatal(err)
		}
	}
}

// 1+2+3+4: routine slots generate one Today block per slot with correct
// area and duration.
func TestSWERoutineSlotToTodayBlock(t *testing.T) {
	d, ctx := openSWETimeDB(t)
	seedSWEAreas(t, ctx, d)
	// Monday 2026-09-21: Go 60m + Docker 60m.
	for _, sl := range []db.SWERoutineSlot{
		{ID: "mon-1", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 3600},
		{ID: "mon-2", Weekday: 1, Position: 2, AreaID: "area-docker", DurationSeconds: 3600},
	} {
		if err := catalog.InsertSWERoutineSlot(ctx, d, sl); err != nil {
			t.Fatal(err)
		}
	}
	// Tuesday: Linux + SQL (must not leak into Monday).
	for _, sl := range []db.SWERoutineSlot{
		{ID: "tue-1", Weekday: 2, Position: 1, AreaID: "area-linux", DurationSeconds: 3600},
		{ID: "tue-2", Weekday: 2, Position: 2, AreaID: "area-sql", DurationSeconds: 3600},
	} {
		if err := catalog.InsertSWERoutineSlot(ctx, d, sl); err != nil {
			t.Fatal(err)
		}
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var swe []db.TodayTask
	for _, tk := range tasks {
		if tk.Domain == db.DomainSWE {
			swe = append(swe, tk)
		}
	}
	if len(swe) != 2 {
		t.Fatalf("want 2 SWE blocks, got %d (%v)", len(swe), tasks)
	}
	// Correct area attached + duration from slot.
	want := map[db.ID]int{"area-go": 3600, "area-docker": 3600}
	for _, tk := range swe {
		if tk.TargetJSON == nil {
			t.Fatal("SWE block has no target")
		}
		st, err := db.UnmarshalSWETarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		wantSecs, ok := want[st.AreaID]
		if !ok {
			t.Fatalf("unexpected area %q", st.AreaID)
		}
		if tk.PlannedSeconds != wantSecs {
			t.Fatalf("area %q planned=%d want %d", st.AreaID, tk.PlannedSeconds, wantSecs)
		}
		delete(want, st.AreaID)
		// Title identifies the area.
		area, _ := catalog.GetSWEArea(ctx, d, st.AreaID)
		if !strings.Contains(tk.Title, area.Name) {
			t.Fatalf("title %q must name area %q", tk.Title, area.Name)
		}
		if !strings.Contains(tk.Title, "SWE") {
			t.Fatalf("title %q must identify SWE", tk.Title)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing areas: %v", want)
	}
}

// 5+6: starting creates a normal session; stopping records elapsed time.
func TestSWEBlockSessionFlow(t *testing.T) {
	d, ctx := openSWETimeDB(t)
	seedSWEAreas(t, ctx, d)
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	mkTask(t, ctx, d, "swe-go", "2026-09-21", db.DomainSWE, db.SessionKindSWE, raw)
	now := time.Now()
	sess, err := engine.StartTodayTask(ctx, d, "swe-go", now)
	if err != nil {
		t.Fatalf("start must succeed: %v", err)
	}
	if sess.Domain != db.DomainSWE || sess.PlannedSeconds != 3600 {
		t.Fatalf("session must carry area block: %+v", sess)
	}
	if sess.Status != db.SessionStatusActive {
		t.Fatal("session must be ACTIVE")
	}
	closed, err := engine.StopActiveSession(ctx, d, now.Add(45*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if closed.ElapsedSeconds() != 2700 {
		t.Fatalf("elapsed=%d want 2700", closed.ElapsedSeconds())
	}
	if closed.Status != db.SessionStatusStopped {
		t.Fatal("must be STOPPED")
	}
}

// 7: no SWE task lookup occurs (no swe_tasks table, no task queries).
func TestSWENoTaskLookup(t *testing.T) {
	d, ctx := openSWETimeDB(t)
	seedSWEAreas(t, ctx, d)
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='swe_tasks'`).Scan(&name); err == nil {
		t.Fatalf("swe_tasks must not exist, found %q", name)
	}
	if err := d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='swe_executions'`).Scan(&name); err == nil {
		t.Fatalf("swe_executions must not exist, found %q", name)
	}
	// Generation works with areas only.
	if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", time.Now()); err != nil {
		t.Fatalf("generation must not need task lookup: %v", err)
	}
}

// 8: no SWE evidence/result form is required (stopping suffices).
func TestSWENoEvidenceRequired(t *testing.T) {
	d, ctx := openSWETimeDB(t)
	seedSWEAreas(t, ctx, d)
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 1800}); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	mkTask(t, ctx, d, "swe-noev", "2026-09-21", db.DomainSWE, db.SessionKindSWE, raw)
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "swe-noev", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// No further call needed. Evaluation stays TODO by design; the time
	// is already recorded on the generic session.
	ev, err := engine.EvaluateTodayTask(ctx, d, "swe-noev")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("SWE stays TODO (no evidence model), got %s", ev.State)
	}
}

// 9: history reports SWE planned/actual/sessions.
func TestSWEHistoryReportsTime(t *testing.T) {
	d, ctx := openSWETimeDB(t)
	seedSWEAreas(t, ctx, d)
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-2", Weekday: 1, Position: 2, AreaID: "area-docker", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	cal := engine.DefaultCalendar()
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, cal.Location())
	snap, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	_ = snap
	var first db.ID
	for _, tk := range tasks {
		if tk.Domain == db.DomainSWE {
			first = tk.ID
			break
		}
	}
	if first == "" {
		t.Fatal("no SWE block generated")
	}
	if _, err := engine.StartTodayTask(ctx, d, first, now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rep, err := engine.BuildDailyReport(ctx, d, "2026-09-21")
	if err != nil {
		t.Fatal(err)
	}
	if rep.SWEPlanned != 7200 {
		t.Fatalf("SWE planned=%d want 7200", rep.SWEPlanned)
	}
	if rep.SWESeconds != 3600 {
		t.Fatalf("SWE actual=%d want 3600", rep.SWESeconds)
	}
	if rep.SWESessions != 1 {
		t.Fatalf("SWE sessions=%d want 1", rep.SWESessions)
	}
	if rep.SWE.Planned != 7200 || rep.SWE.Actual != 3600 || rep.SWE.Sessions != 1 {
		t.Fatalf("SWE evidence=%+v want planned 7200 actual 3600 sessions 1", rep.SWE)
	}
	if rep.SWE.Variance() != 3600-7200 {
		t.Fatalf("SWE variance=%d want -3600", rep.SWE.Variance())
	}
}

// 10: reviews report SWE planned/actual/sessions.
func TestSWEReviewReportsTime(t *testing.T) {
	d, ctx := openSWETimeDB(t)
	seedSWEAreas(t, ctx, d)
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	cal := engine.DefaultCalendar()
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, cal.Location())
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Domain != db.DomainSWE {
			continue
		}
		if _, err := engine.StartTodayTask(ctx, d, tk.ID, now); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.StopActiveSession(ctx, d, now.Add(30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		break
	}
	rep, err := engine.BuildWeeklyDraft(ctx, d, "2026-W39", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatal(err)
	}
	if rep.SWEPlanned != 3600 {
		t.Fatalf("weekly SWE planned=%d want 3600", rep.SWEPlanned)
	}
	if rep.SWESeconds != 1800 {
		t.Fatalf("weekly SWE actual=%d want 1800", rep.SWESeconds)
	}
	if rep.SWESessions != 1 {
		t.Fatalf("weekly SWE sessions=%d want 1", rep.SWESessions)
	}
	if rep.SWEVariance() != 1800-3600 {
		t.Fatalf("weekly SWE variance=%d want -1800", rep.SWEVariance())
	}
}

// 11: final schema has areas + routine slots with FK + uniqueness.
func TestSWEFinalSchema(t *testing.T) {
	d, ctx := openSWETimeDB(t)
	// Tables.
	for _, tbl := range []string{"swe_areas", "swe_routine_slots"} {
		var n int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+tbl).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
	for _, tbl := range []string{"swe_tasks", "swe_executions"} {
		var name string
		if err := d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name); err == nil {
			t.Fatalf("table %s must not exist", tbl)
		}
	}
	// FK: routine slot with unknown area fails.
	if err := catalog.InsertSWEArea(ctx, d, db.SWEArea{ID: "a1", Name: "Go", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "s1", Weekday: 1, Position: 1, AreaID: "a1", DurationSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "s2", Weekday: 1, Position: 1, AreaID: "a1", DurationSeconds: 60}); err == nil {
		t.Fatal("UNIQUE(weekday, position) must hold")
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "s3", Weekday: 1, Position: 2, AreaID: "missing", DurationSeconds: 60}); err == nil {
		t.Fatal("FK area_id must hold")
	}
}
