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

// Legacy SWE area-only targets: readable historically, read-only for new evidence.
func TestLegacySWEAreaOnlyCoherent(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWE(t, ctx, d)
	now := time.Now()
	// Current TaskID-based target validates for both historical and new paths.
	rawCurrent, _ := db.MarshalTarget(db.SWETarget{Tasks: []db.SWETaskTarget{{AreaID: "area-go", TaskID: idp("go-1")}}})
	stCurrent, _ := db.UnmarshalSWETarget(rawCurrent)
	if err := engine.ValidateSWETarget(ctx, d, stCurrent); err != nil {
		t.Fatalf("current target must validate: %v", err)
	}
	if err := engine.ValidateSWECurrentTarget(ctx, d, stCurrent); err != nil {
		t.Fatalf("current target must pass strict validation: %v", err)
	}
	if engine.IsLegacySWETarget(stCurrent) {
		t.Fatal("current target must not be legacy")
	}
	// Legacy area-only target: historical validation allows, strict rejects.
	rawLegacy, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	stLegacy, _ := db.UnmarshalSWETarget(rawLegacy)
	if err := engine.ValidateSWETarget(ctx, d, stLegacy); err != nil {
		t.Fatalf("legacy must remain readable via ValidateSWETarget: %v", err)
	}
	if !engine.IsLegacySWETarget(stLegacy) {
		t.Fatal("legacy not detected")
	}
	if err := engine.ValidateSWECurrentTarget(ctx, d, stLegacy); !errors.Is(err, engine.ErrLegacyTarget) {
		t.Fatalf("strict validation must return ErrLegacyTarget, got %v", err)
	}
	// New evidence on legacy must return ErrLegacyTarget (not invalid-target
	// confusion, not silent attach).
	mkTask(t, ctx, d, "swe-legacy", "2026-09-20", db.DomainSWE, db.SessionKindSWE, rawLegacy)
	sess, err := engine.StartTodayTask(ctx, d, "swe-legacy", now)
	if err == nil {
		// Starting legacy for new evidence must also be read-only.
		// If start unexpectedly succeeds (e.g. legacy allowed to start),
		// the save must still be read-only.
		_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
		_, err = engine.SaveSWEEvidence(ctx, d, "swe-legacy", sess.ID, []engine.SWEOutcomeInput{{TaskID: "go-1", Status: db.SWEStatusCompleted}}, now)
		if !errors.Is(err, engine.ErrLegacyTarget) {
			t.Fatalf("legacy save must return ErrLegacyTarget, got %v", err)
		}
	} else {
		if !errors.Is(err, engine.ErrLegacyTarget) {
			t.Fatalf("legacy start must return ErrLegacyTarget, got %v", err)
		}
	}
	// Current target still saves fine (different date avoids snapshot revision clash).
	raw2, _ := db.MarshalTarget(db.SWETarget{Tasks: []db.SWETaskTarget{{AreaID: "area-go", TaskID: idp("go-1")}}})
	mkTask(t, ctx, d, "swe-cur", "2026-09-21", db.DomainSWE, db.SessionKindSWE, raw2)
	s2, err := engine.StartTodayTask(ctx, d, "swe-cur", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("current start failed: %v", err)
	}
	_, _ = engine.StopActiveSession(ctx, d, now.Add(3*time.Hour))
	if _, err := engine.SaveSWEEvidence(ctx, d, "swe-cur", s2.ID, []engine.SWEOutcomeInput{{TaskID: "go-1", Status: db.SWEStatusCompleted}}, now); err != nil {
		t.Fatalf("current save failed: %v", err)
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
	// SWE completion map matches single checks.
	seedSWE(t, ctx, d)
	sweIDs := []db.ID{"go-1", "go-2"}
	singleMap := map[db.ID]bool{}
	for _, id := range sweIDs {
		ok, err := engine.IsSWETaskComplete(ctx, d, id)
		if err != nil {
			t.Fatal(err)
		}
		singleMap[id] = ok
	}
	batch, err := runtime.GetSWETaskCompletionMap(ctx, d, sweIDs)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range sweIDs {
		if batch[id] != singleMap[id] {
			t.Fatalf("swe %s batch=%v single=%v", id, batch[id], singleMap[id])
		}
	}
	_ = catalog.GetGovSubjects
}
