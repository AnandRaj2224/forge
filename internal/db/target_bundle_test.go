package db_test

import (
	"testing"

	db "forge/internal/db"
)

// Section 31: bundle-capable targets. Legacy singular payloads must keep
// parsing exactly as before; bundle payloads must normalize to item lists.

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
