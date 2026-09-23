package engine_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// NaN/Inf must be rejected by engine float validation.
func TestMockMetricsRejectNaNInf(t *testing.T) {
	nan := math.NaN()
	infP := math.Inf(1)
	infN := math.Inf(-1)
	for name, v := range map[string]*float64{"nan": &nan, "+inf": &infP, "-inf": &infN} {
		for _, m := range []db.Mock{
			{ID: "m", ExamID: "e", Name: "M", MockDate: "2026-09-18", Percentile: v},
			{ID: "m", ExamID: "e", Name: "M", MockDate: "2026-09-18", Accuracy: v},
			{ID: "m", ExamID: "e", Name: "M", MockDate: "2026-09-18", AvgSpeedSeconds: v},
		} {
			if err := engine.ValidateMockMetrics(m); err == nil {
				t.Fatalf("%s %v must fail", name, *v)
			}
		}
	}
	// Valid finite values still pass.
	p := 84.3
	a := 81.2
	s := 24.8
	if err := engine.ValidateMockMetrics(db.Mock{ID: "m", ExamID: "e", Name: "M", MockDate: "2026-09-18", Percentile: &p, Accuracy: &a, AvgSpeedSeconds: &s}); err != nil {
		t.Fatalf("finite values must pass: %v", err)
	}
	// attempted > total must fail even when unattempted is omitted.
	att, tot := 20, 10
	if err := engine.ValidateMockMetrics(db.Mock{ID: "m", ExamID: "e", Name: "M", MockDate: "2026-09-18", Attempted: &att, TotalQuestions: &tot}); err == nil {
		t.Fatal("attempted > total must fail")
	}
	// Omitted metrics stay valid.
	if err := engine.ValidateMockMetrics(db.Mock{ID: "m", ExamID: "e", Name: "M", MockDate: "2026-09-18"}); err != nil {
		t.Fatalf("omitted must pass: %v", err)
	}
}

// ParseAvgSpeed must reject NaN/Inf.
func TestParseAvgSpeedRejectNaNInf(t *testing.T) {
	for _, s := range []string{"NaN", "nan", "+Inf", "-Inf", "inf", "Inf"} {
		if _, msg := engine.ParseAvgSpeed(s); msg == "" {
			t.Fatalf("avg %q must fail", s)
		}
	}
	if _, msg := engine.ParseAvgSpeed("17.2"); msg != "" {
		t.Fatalf("valid avg rejected: %s", msg)
	}
}

// SWE area targets validate and start/stop as time blocks (no evidence).
func TestSWEAreaTargetCoherent(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWE(t, ctx, d)
	now := time.Now()
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	st, _ := db.UnmarshalSWETarget(raw)
	if err := engine.ValidateSWETarget(ctx, d, st); err != nil {
		t.Fatalf("area target must validate: %v", err)
	}
	mkTask(t, ctx, d, "swe-area", "2026-09-20", db.DomainSWE, db.SessionKindSWE, raw)
	sess, err := engine.StartTodayTask(ctx, d, "swe-area", now)
	if err != nil {
		t.Fatalf("area start failed: %v", err)
	}
	closed, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if closed.ElapsedSeconds() != 3600 {
		t.Fatalf("elapsed=%d want 3600", closed.ElapsedSeconds())
	}
	if sess.ID != closed.ID {
		t.Fatal("stop must close the same session")
	}
	// Unknown area rejects.
	if err := engine.ValidateSWETarget(ctx, d, db.SWETarget{AreaID: "no-area"}); !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("unknown area must be ErrInvalidTarget, got %v", err)
	}
}

// Batch lecture completion map must match single-lecture logic.
func TestLectureCompletionMapMatches(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	// Complete lec-1 via a real task so the authoritative path is used.
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "lec-map", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	sess, err := engine.StartTodayTask(ctx, d, "lec-map", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "lec-map", sess.ID, true, now); err != nil {
		t.Fatal(err)
	}
	ids := []db.ID{"lec-1", "lec-2"}
	m, err := runtime.GetLectureCompletionMap(ctx, d, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		single, err := engine.IsLectureComplete(ctx, d, id)
		if err != nil {
			t.Fatal(err)
		}
		if m[id] != single {
			t.Fatalf("lecture %s map=%v single=%v", id, m[id], single)
		}
	}
	// Empty input stays empty without a query error.
	if got, err := runtime.GetLectureCompletionMap(ctx, d, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty map=%v err=%v", got, err)
	}
	// No SWE task tables exist in the time-allocation model.
	var name string
	err = d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name IN ('swe_tasks','swe_executions')`).Scan(&name)
	if err == nil {
		t.Fatalf("task-dependent SWE table %q must not exist", name)
	}
	_ = catalog.GetGovSubjects
}
