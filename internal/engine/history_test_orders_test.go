package engine_test

import (
	"context"
	"testing"
	"time"

	"forge/internal/engine"
)

func TestBuildDailyReport_WithPracticeTestOrders(t *testing.T) {
	d := openTestDBPractice(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	seedPracticeTestOrderFixtures(t, ctx, d, day)

	// Generate today tasks
	_, tasks, err := engine.EnsureToday(ctx, d, "2026-10-31", day)
	if err != nil {
		t.Fatalf("EnsureToday: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("no tasks generated")
	}

	// Build daily report for today
	rep, err := engine.BuildDailyReport(ctx, d, "2026-10-31")
	if err != nil {
		t.Fatalf("BuildDailyReport failed: %v", err)
	}

	if rep.Date != "2026-10-31" {
		t.Errorf("expected date 2026-10-31, got %s", rep.Date)
	}
	if len(rep.Activities) == 0 {
		t.Fatal("expected activities in report")
	}

	foundPractice := false
	for _, tk := range rep.Activities {
		if tk.Kind == "PRACTICE" {
			foundPractice = true
			if tk.Detail == "" {
				t.Errorf("expected practice task %s to have non-empty Detail", tk.Title)
			}
		}
	}
	if !foundPractice {
		t.Fatal("expected practice tasks in daily report")
	}
}
