package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

// SWE TUI is an area/routine view: areas list + routine slots, no task
// lists, no completion percentages, no outcomes.

func seedSWETUI(t *testing.T, database *db.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	_ = catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 0, CreatedAt: now})
	for _, a := range []db.SWEArea{
		{ID: "area-go", Name: "Go", SortOrder: 1},
		{ID: "area-docker", Name: "Docker", SortOrder: 2},
	} {
		_ = catalog.InsertSWEArea(ctx, database, a)
	}
	_ = catalog.InsertSWERoutineSlot(ctx, database, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 3600})
	_ = catalog.InsertSWERoutineSlot(ctx, database, db.SWERoutineSlot{ID: "mon-2", Weekday: 1, Position: 2, AreaID: "area-docker", DurationSeconds: 3600})
}

func TestSWEScreenShowsAreasAndSlots(t *testing.T) {
	database := seedFlowDB(t)
	seedSWETUI(t, database)
	app := NewApp(database)
	app.width, app.height = 120, 36
	app.goTab(TabSWE)
	view := stripANSIForTest(app.swe.View())
	for _, want := range []string{"SWE AREAS", "Go", "Docker", "ROUTINE SLOTS", "SCHEDULED TIME"} {
		if !strings.Contains(strings.ToUpper(view), strings.ToUpper(want)) {
			t.Fatalf("SWE view missing %q:\n%s", want, view)
		}
	}
	for _, bad := range []string{"COMPLETED", "PARTIAL", "BLOCKED", "✓  DONE", "○  TODO", "EXECUTIONS", "TASKS"} {
		if strings.Contains(strings.ToUpper(view), strings.ToUpper(bad)) && bad != "TASKS" {
			// TASKS appears only if leaked from old UI; routine UI must not show it.
			t.Fatalf("SWE view must not show %q:\n%s", bad, view)
		}
	}
	if strings.Contains(view, "TASKS") {
		t.Fatalf("SWE view must not show task lists:\n%s", view)
	}
}

func TestTodayShowsSWEBlocksNoResultForm(t *testing.T) {
	database := seedFlowDB(t)
	seedSWETUI(t, database)
	app := NewApp(database)
	app.width, app.height = 140, 40
	// Pin to Monday 2026-09-21 so routine slots generate.
	app.today.nowFn = func() time.Time { return time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC) }
	app.goTab(TabToday)
	app.reloadCurrent()
	found := false
	for _, tk := range app.today.tasks {
		if tk.Domain == db.DomainSWE {
			found = true
			if tk.TargetJSON == nil {
				t.Fatal("SWE block has no target")
			}
			st, err := db.UnmarshalSWETarget(*tk.TargetJSON)
			if err != nil {
				t.Fatal(err)
			}
			if st.AreaID == "" {
				t.Fatal("SWE block must carry area_id")
			}
		}
	}
	if !found {
		t.Fatal("Today must show SWE blocks from routine slots")
	}
	// Start the first SWE block, stop it: no result form appears.
	var sweIdx = -1
	for i, tk := range app.today.tasks {
		if tk.Domain == db.DomainSWE {
			sweIdx = i
			break
		}
	}
	app.today.tbl.SetCursor(sweIdx)
	app.today.startSelected()
	if app.today.active == nil {
		t.Fatalf("SWE start failed: %s", app.today.err)
	}
	app.today.stopActive()
	if app.today.right == "result" {
		t.Fatal("SWE stop must not open a result form")
	}
	if app.today.right != "detail" {
		t.Fatalf("SWE stop must stay on detail, got %q", app.today.right)
	}
}
