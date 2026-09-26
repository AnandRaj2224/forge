package engine_test

import (
	"context"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// Raw-facts form: user enters Time + Attempted + Correct; Incorrect and
// Avg time/question are derived, never entered.
func TestPracticeDerivedValues(t *testing.T) {
	// Incorrect = attempted - correct.
	in, msg := engine.ValidateGovPracticeInput("T", "600", "10", "8", "", false)
	if msg != "" {
		t.Fatalf("valid raw rejected: %s", msg)
	}
	if got := in.Attempted - in.Correct; got != 2 {
		t.Fatalf("incorrect=%d want 2", got)
	}
	// Avg = time / attempted.
	if in.AvgSpeed == nil || *in.AvgSpeed < 59.9 || *in.AvgSpeed > 60.1 {
		t.Fatalf("avg=%v want 60", in.AvgSpeed)
	}
	// Accuracy = correct / attempted (display helper lives in TUI, engine
	// stores raw; check math here).
	if acc := float64(in.Correct) / float64(in.Attempted); acc < 0.799 || acc > 0.801 {
		t.Fatalf("acc=%v want 0.8", acc)
	}
}

func TestPracticeInvalidCorrectExceeds(t *testing.T) {
	if _, msg := engine.ValidateGovPracticeInput("T", "600", "10", "11", "", false); msg == "" {
		t.Fatal("correct > attempted must be rejected")
	}
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-perc"}})
	mkTask(t, ctx, d, "inv1", "2026-09-20", db.DomainGOV, db.SessionKindPractice, prac)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "inv1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	if _, err := engine.SaveGovPracticeEvidence(ctx, d, "inv1", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-perc", TimeSecs: 100, Attempted: 1, Correct: 5},
	}, now); err == nil {
		t.Fatal("save with correct>attempted must fail")
	}
}

func TestPracticeAttemptedZero(t *testing.T) {
	in, msg := engine.ValidateGovPracticeInput("T", "300", "0", "0", "", false)
	if msg != "" {
		t.Fatalf("attempted 0 must be allowed: %s", msg)
	}
	if in.AvgSpeed != nil {
		t.Fatalf("avg with 0 attempted must be nil, got %v", *in.AvgSpeed)
	}
	// Save with attempted 0 persists nil avg (displays as —).
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio"}})
	mkTask(t, ctx, d, "z1", "2026-09-20", db.DomainGOV, db.SessionKindPractice, prac)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "z1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	if _, err := engine.SaveGovPracticeEvidence(ctx, d, "z1", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-ratio", TimeSecs: 300, Attempted: 0, Correct: 0, AvgSpeed: nil},
	}, now); err != nil {
		t.Fatalf("zero-attempt save must succeed: %v", err)
	}
	recs, err := runtime.GetGovPracticeRecordsBySession(ctx, d, sess.ID)
	if err != nil || len(recs) != 1 {
		t.Fatalf("recs=%v err=%v", recs, err)
	}
	if recs[0].AvgQuestionSpeedSeconds != nil {
		t.Fatalf("stored avg must be nil for 0 attempted, got %v", *recs[0].AvgQuestionSpeedSeconds)
	}
}

func TestPracticeAvgAutoPersisted(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio"}})
	mkTask(t, ctx, d, "a1", "2026-09-20", db.DomainGOV, db.SessionKindPractice, prac)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "a1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	// No Avg provided: engine derives 600/10 = 60.
	if _, err := engine.SaveGovPracticeEvidence(ctx, d, "a1", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-ratio", TimeSecs: 600, Attempted: 10, Correct: 8, AvgSpeed: nil},
	}, now); err != nil {
		t.Fatal(err)
	}
	recs, _ := runtime.GetGovPracticeRecordsBySession(ctx, d, sess.ID)
	if len(recs) != 1 || recs[0].AvgQuestionSpeedSeconds == nil {
		t.Fatalf("avg must be auto-persisted: %+v", recs)
	}
	if got := *recs[0].AvgQuestionSpeedSeconds; got < 59.9 || got > 60.1 {
		t.Fatalf("stored avg=%v want 60", got)
	}
}

func TestPracticeOldRecordsReadable(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio"}})
	mkTask(t, ctx, d, "old1", "2026-09-20", db.DomainGOV, db.SessionKindPractice, prac)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "old1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	oldAvg := 17.5
	if _, err := engine.SaveGovPracticeEvidence(ctx, d, "old1", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-ratio", TimeSecs: 600, Attempted: 10, Correct: 8, AvgSpeed: &oldAvg},
	}, now); err != nil {
		t.Fatal(err)
	}
	recs, err := runtime.GetGovPracticeHistory(ctx, d, "topic-ratio")
	if err != nil || len(recs) == 0 {
		t.Fatalf("old history missing: %v", err)
	}
	if recs[0].AvgQuestionSpeedSeconds == nil || *recs[0].AvgQuestionSpeedSeconds != 17.5 {
		t.Fatalf("old avg must be preserved: %+v", recs[0])
	}
}
