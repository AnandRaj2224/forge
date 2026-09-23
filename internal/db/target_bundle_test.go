package db_test

import (
	"testing"

	db "forge/internal/db"
)

// Section 31: bundle-capable targets. Legacy singular payloads must keep
// parsing exactly as before; bundle payloads must normalize to item lists.
// SWE is a time-allocation target: exactly one area_id, no tasks.

func TestDSATargetLegacySingular(t *testing.T) {
	got, err := db.UnmarshalDSATarget(`{"problem_id":"dsa-001","mode":"NEW"}`)
	if err != nil {
		t.Fatal(err)
	}
	items := got.Items()
	if len(items) != 1 || items[0].ProblemID != "dsa-001" || items[0].Mode != db.DSAModeNew {
		t.Fatalf("legacy singular not preserved: %+v", got)
	}
}

func TestDSATargetBundle(t *testing.T) {
	raw := `{"problems":[{"problem_id":"dsa-002","mode":"REPEAT"},{"problem_id":"dsa-004","mode":"REPEAT"},{"problem_id":"dsa-009","mode":"NEW"}]}`
	got, err := db.UnmarshalDSATarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	items := got.Items()
	if len(items) != 3 {
		t.Fatalf("expected 3 bundle items, got %d", len(items))
	}
	if items[0].Mode != db.DSAModeRepeat || items[2].Mode != db.DSAModeNew {
		t.Fatalf("modes not preserved: %+v", items)
	}
}

func TestSWETargetAreaOnly(t *testing.T) {
	got, err := db.UnmarshalSWETarget(`{"area_id":"area-go"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.AreaID != "area-go" {
		t.Fatalf("area not preserved: %+v", got)
	}
}

func TestSWETargetRejectsUnknownFields(t *testing.T) {
	if _, err := db.UnmarshalSWETarget(`{"area_id":"area-go","task_id":"go-1"}`); err == nil {
		t.Fatal("task_id must be rejected in time-allocation model")
	}
	if _, err := db.UnmarshalSWETarget(`{"tasks":[{"area_id":"a"}]}`); err == nil {
		t.Fatal("tasks bundle must be rejected in time-allocation model")
	}
}

func TestTargetBundleRoundTrip(t *testing.T) {
	raw, err := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{{ProblemID: "dsa-001", Mode: db.DSAModeNew}}})
	if err != nil {
		t.Fatal(err)
	}
	back, err := db.UnmarshalDSATarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Items()) != 1 || back.Items()[0].ProblemID != "dsa-001" {
		t.Fatalf("round trip failed: %s", raw)
	}
}

func TestSWETargetRoundTrip(t *testing.T) {
	raw, err := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	if err != nil {
		t.Fatal(err)
	}
	back, err := db.UnmarshalSWETarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back.AreaID != "area-go" {
		t.Fatalf("round trip failed: %s", raw)
	}
}
