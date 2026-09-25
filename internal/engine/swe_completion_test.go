package engine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

func mkSWETaskWithPlanned(t *testing.T, ctx context.Context, d *db.DB, taskID db.ID, date string, planned int, areaID db.ID) {
	t.Helper()
	now := time.Now()
	snapID := db.ID("snap-" + string(taskID))
	_ = runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snapID, LocalDate: date, Revision: 1, GeneratedAt: now})
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: areaID})
	tj := raw
	if err := runtime.InsertTodayTask(ctx, d, db.TodayTask{
		ID: taskID, SnapshotID: snapID, Position: 1,
		Domain: db.DomainSWE, Kind: db.SessionKindSWE,
		Title: "SWE", PlannedSeconds: planned, TargetJSON: &tj,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedSWEAreaForCompletion(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	if err := catalog.InsertSWEArea(ctx, d, db.SWEArea{ID: "area-go", Name: "Go", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestSWEStoppedExactTargetCompleted(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	mkSWETaskWithPlanned(t, ctx, d, "swe-exact", "2026-09-20", 1200, "area-go")
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "swe-exact", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	ev, err := engine.EvaluateTodayTask(ctx, d, "swe-exact")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("stopped exact target must be COMPLETED, got %s", ev.State)
	}
}

func TestSWEStoppedOverTargetCompleted(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	// cURL case: planned 20m, stopped 23m7s.
	mkSWETaskWithPlanned(t, ctx, d, "swe-over", "2026-09-20", 1200, "area-go")
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "swe-over", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(23*time.Minute).Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	ev, err := engine.EvaluateTodayTask(ctx, d, "swe-over")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("stopped over target must be COMPLETED, got %s", ev.State)
	}
}

func TestSWECumulativeSessionsCompleted(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	mkSWETaskWithPlanned(t, ctx, d, "swe-cum", "2026-09-20", 3600, "area-go")
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "swe-cum", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	ev, _ := engine.EvaluateTodayTask(ctx, d, "swe-cum")
	if ev.State != engine.StateTodo {
		t.Fatalf("30m of 60m must stay TODO, got %s", ev.State)
	}
	if _, err := engine.StartTodayTask(ctx, d, "swe-cum", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour).Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	ev, _ = engine.EvaluateTodayTask(ctx, d, "swe-cum")
	if ev.State != engine.StateCompleted {
		t.Fatalf("30m+30m vs 60m must be COMPLETED, got %s", ev.State)
	}
}

func TestSWEPartialStaysTodo(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	mkSWETaskWithPlanned(t, ctx, d, "swe-part", "2026-09-20", 3600, "area-go")
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "swe-part", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	ev, _ := engine.EvaluateTodayTask(ctx, d, "swe-part")
	if ev.State != engine.StateTodo {
		t.Fatalf("partial must stay TODO, got %s", ev.State)
	}
}

func TestSWEActiveReachesTargetCompletedButStaysActive(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	mkSWETaskWithPlanned(t, ctx, d, "swe-live", "2026-09-20", 1200, "area-go")
	// Start 25m ago: elapsed 25m >= 20m target while still ACTIVE.
	start := time.Now().Add(-25 * time.Minute)
	if _, err := engine.StartTodayTask(ctx, d, "swe-live", start); err != nil {
		t.Fatal(err)
	}
	ev, err := engine.EvaluateTodayTask(ctx, d, "swe-live")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("active reaching target must be COMPLETED, got %s", ev.State)
	}
	if ev.Active == nil {
		t.Fatal("session must remain ACTIVE (ev.Active set)")
	}
	loaded, err := engine.LoadActiveSession(ctx, d)
	if err != nil || loaded == nil {
		t.Fatalf("active session must persist, got %+v %v", loaded, err)
	}
	if loaded.Status != db.SessionStatusActive {
		t.Fatalf("session status must stay ACTIVE, got %s", loaded.Status)
	}
	// Explicit stop preserves COMPLETED.
	closed, err := engine.StopActiveSession(ctx, d, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != db.SessionStatusStopped {
		t.Fatal("must be STOPPED after explicit stop")
	}
	ev2, _ := engine.EvaluateTodayTask(ctx, d, "swe-live")
	if ev2.State != engine.StateCompleted {
		t.Fatalf("explicit stop must preserve COMPLETED, got %s", ev2.State)
	}
}

func TestSWEActiveOverrunRemainsActive(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	mkSWETaskWithPlanned(t, ctx, d, "swe-overrun", "2026-09-20", 1200, "area-go")
	start := time.Now().Add(-35 * time.Minute)
	if _, err := engine.StartTodayTask(ctx, d, "swe-overrun", start); err != nil {
		t.Fatal(err)
	}
	ev, _ := engine.EvaluateTodayTask(ctx, d, "swe-overrun")
	if ev.State != engine.StateCompleted {
		t.Fatalf("overrun active must be COMPLETED, got %s", ev.State)
	}
	if ev.Active == nil || ev.Active.Status != db.SessionStatusActive {
		t.Fatal("overrun session must remain ACTIVE")
	}
}

func TestSWETodayAndDailySummaryAgree(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	cal := engine.DefaultCalendar()
	day := "2026-09-21"
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, cal.Location())
	snapID := db.ID("snap-agree")
	_ = runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snapID, LocalDate: day, Revision: 1, GeneratedAt: now})
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	tj := raw
	_ = runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: "swe-agree", SnapshotID: snapID, Position: 1, Domain: db.DomainSWE, Kind: db.SessionKindSWE, Title: "SWE", PlannedSeconds: 1200, TargetJSON: &tj})
	if _, err := engine.StartTodayTask(ctx, d, "swe-agree", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(25*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Today (live evaluation).
	ev, _ := engine.EvaluateTodayTask(ctx, d, "swe-agree")
	if ev.State != engine.StateCompleted {
		t.Fatalf("Today must show COMPLETED, got %s", ev.State)
	}
	// Daily Summary.
	rep, err := engine.BuildDailyReport(ctx, d, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Activities) != 1 || rep.Activities[0].State != engine.StateCompleted {
		t.Fatalf("Daily Summary must show COMPLETED, got %+v", rep.Activities)
	}
	if rep.TasksCompleted != 1 {
		t.Fatalf("Daily Summary TasksCompleted=%d want 1", rep.TasksCompleted)
	}
	if len(rep.Unfinished) != 0 {
		t.Fatalf("Daily Summary Unfinished must be empty, got %+v", rep.Unfinished)
	}
	// History/detail uses same semantics via cutoff evaluation.
	dayStart, dayEnd, _ := cal.DayBounds(day)
	_ = dayStart
	evH, _ := engine.EvaluateTodayTaskAt(ctx, d, "swe-agree", dayEnd)
	if evH.State != engine.StateCompleted {
		t.Fatalf("History/detail cutoff eval must be COMPLETED, got %s", evH.State)
	}
}

func TestSWECompletedCannotRestartAndNoEvidenceForm(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWEAreaForCompletion(t, ctx, d)
	mkSWETaskWithPlanned(t, ctx, d, "swe-norestart", "2026-09-20", 1200, "area-go")
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "swe-norestart", now); err != nil {
		t.Fatal(err)
	}
	// No evidence form exists: stopping suffices and completes.
	if _, err := engine.StopActiveSession(ctx, d, now.Add(25*time.Minute)); err != nil {
		t.Fatal(err)
	}
	ev, _ := engine.EvaluateTodayTask(ctx, d, "swe-norestart")
	if ev.State != engine.StateCompleted {
		t.Fatalf("must be COMPLETED without any evidence form, got %s", ev.State)
	}
	if _, err := engine.StartTodayTask(ctx, d, "swe-norestart", now.Add(time.Hour)); !errors.Is(err, engine.ErrTaskCompleted) {
		t.Fatalf("completed SWE must reject restart with ErrTaskCompleted, got %v", err)
	}
}

func TestSWECNAndCurlRealCases(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	if err := catalog.InsertSWEArea(ctx, d, db.SWEArea{ID: "swe-area-cn", Name: "CN", SortOrder: 9}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWEArea(ctx, d, db.SWEArea{ID: "swe-area-curl", Name: "curl", SortOrder: 10}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// CN: planned 1h40m (6000), two stopped sessions totalling 6037.
	for _, tid := range []string{"cn-task"} {
		snapID := db.ID("snap-" + tid)
		_ = runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snapID, LocalDate: "2026-09-25", Revision: 1, GeneratedAt: now})
		raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "swe-area-cn"})
		tj := raw
		_ = runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: db.ID(tid), SnapshotID: snapID, Position: 1, Domain: db.DomainSWE, Kind: db.SessionKindSWE, Title: "SWE — CN — 1h 40m", PlannedSeconds: 6000, TargetJSON: &tj})
	}
	if _, err := engine.StartTodayTask(ctx, d, "cn-task", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(3046*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StartTodayTask(ctx, d, "cn-task", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour).Add(2991*time.Second)); err != nil {
		t.Fatal(err)
	}
	ev, _ := engine.EvaluateTodayTask(ctx, d, "cn-task")
	if ev.State != engine.StateCompleted {
		t.Fatalf("CN 6037s vs 6000s must be COMPLETED, got %s", ev.State)
	}
	// cURL: planned 20m (1200), stopped 1387.
	snapID2 := db.ID("snap-curl-task2")
	_ = runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snapID2, LocalDate: "2026-09-26", Revision: 1, GeneratedAt: now})
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "swe-area-curl"})
	tj := raw
	_ = runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: "curl-task", SnapshotID: snapID2, Position: 1, Domain: db.DomainSWE, Kind: db.SessionKindSWE, Title: "SWE — curl — 20m", PlannedSeconds: 1200, TargetJSON: &tj})
	if _, err := engine.StartTodayTask(ctx, d, "curl-task", now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(3*time.Hour).Add(1387*time.Second)); err != nil {
		t.Fatal(err)
	}
	ev2, _ := engine.EvaluateTodayTask(ctx, d, "curl-task")
	if ev2.State != engine.StateCompleted {
		t.Fatalf("cURL 1387s vs 1200s must be COMPLETED, got %s", ev2.State)
	}
}
