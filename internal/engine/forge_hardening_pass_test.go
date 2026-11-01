package engine_test

import (
	"context"
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
	_ = catalog.GetGovSubjects
}
