package engine_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// seedRotationSubject creates a subject with n zero-lecture topics
// (immediately practice-eligible) in sort_order 1..n.
var rotationSubjectSort int

func seedRotationSubject(t *testing.T, ctx context.Context, d *db.DB, subjID, subjName string, n int) []db.GovTopic {
	t.Helper()
	rotationSubjectSort++
	if err := catalog.InsertGovSubject(ctx, d, db.GovSubject{ID: db.ID(subjID), Name: subjName, SortOrder: rotationSubjectSort}); err != nil {
		t.Fatal(err)
	}
	var out []db.GovTopic
	for i := 1; i <= n; i++ {
		tid := db.ID(fmt.Sprintf("%s-topic-%02d", subjID, i))
		tp := db.GovTopic{ID: tid, SubjectID: db.ID(subjID), Name: fmt.Sprintf("%c", 'A'+i-1), SortOrder: i}
		if n > 26 {
			tp.Name = fmt.Sprintf("T%d", i)
		}
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
		out = append(out, tp)
	}
	return out
}

var rotationSeq int

// recordRotationBlock persists one practice block (up to 3 topics) through the
// real engine save path with raw facts only (nil Avg -> derived).
func recordRotationBlock(t *testing.T, ctx context.Context, d *db.DB, subjectID db.ID, topics []db.GovTopic, at time.Time) {
	t.Helper()
	rotationSeq++
	snapID := db.ID(fmt.Sprintf("rot-snap-%d", rotationSeq))
	taskID := db.ID(fmt.Sprintf("rot-task-%d", rotationSeq))
	if err := runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snapID, LocalDate: "2026-09-20", Revision: rotationSeq, GeneratedAt: at}); err != nil {
		t.Fatal(err)
	}
	ids := make([]db.ID, 0, len(topics))
	for _, tp := range topics {
		ids = append(ids, tp.ID)
	}
	raw, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: subjectID, TopicIDs: ids})
	if err := runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: taskID, SnapshotID: snapID, Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "R", PlannedSeconds: 3600, TargetJSON: &raw}); err != nil {
		t.Fatal(err)
	}
	now := at
	if _, err := engine.StartTodayTask(ctx, d, taskID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Find the stopped session for this task.
	sessions, err := runtime.ListSessions(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	var stoppedID db.ID
	for _, s := range sessions {
		if s.TodayTaskID == taskID && s.Status == db.SessionStatusStopped {
			stoppedID = s.ID
		}
	}
	if stoppedID == "" {
		t.Fatal("no stopped session")
	}
	var inputs []engine.GovPracticeInput
	for _, tp := range topics {
		inputs = append(inputs, engine.GovPracticeInput{TopicID: tp.ID, TimeSecs: 600, Attempted: 10, Correct: 8, AvgSpeed: nil})
	}
	if _, err := engine.SaveGovPracticeEvidence(ctx, d, taskID, stoppedID, inputs, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func topicNames(ts []db.GovTopic) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}

func eqStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTopicRotationNine(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	eligible := seedRotationSubject(t, ctx, d, "subj-r9", "R9", 9)
	now := time.Now()
	// No history -> A B C.
	got, err := engine.NextPracticeTopics(ctx, d, eligible)
	if err != nil {
		t.Fatal(err)
	}
	if !eqStr(topicNames(got), []string{"A", "B", "C"}) {
		t.Fatalf("first=%v want ABC", topicNames(got))
	}
	recordRotationBlock(t, ctx, d, "subj-r9", got, now)
	got, _ = engine.NextPracticeTopics(ctx, d, eligible)
	if !eqStr(topicNames(got), []string{"D", "E", "F"}) {
		t.Fatalf("second=%v want DEF", topicNames(got))
	}
	recordRotationBlock(t, ctx, d, "subj-r9", got, now.Add(time.Hour))
	got, _ = engine.NextPracticeTopics(ctx, d, eligible)
	if !eqStr(topicNames(got), []string{"G", "H", "I"}) {
		t.Fatalf("third=%v want GHI", topicNames(got))
	}
	recordRotationBlock(t, ctx, d, "subj-r9", got, now.Add(2*time.Hour))
	got, _ = engine.NextPracticeTopics(ctx, d, eligible)
	if !eqStr(topicNames(got), []string{"A", "B", "C"}) {
		t.Fatalf("wrap=%v want ABC", topicNames(got))
	}
}

func TestTopicRotationTen(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	eligible := seedRotationSubject(t, ctx, d, "subj-r10", "R10", 10)
	// Rename to A..J for clarity (seed already does A..J for n<=26).
	now := time.Now()
	seq := [][]string{{"A", "B", "C"}, {"D", "E", "F"}, {"G", "H", "I"}, {"J", "A", "B"}, {"C", "D", "E"}}
	for i, want := range seq {
		got, err := engine.NextPracticeTopics(ctx, d, eligible)
		if err != nil {
			t.Fatal(err)
		}
		if !eqStr(topicNames(got), want) {
			t.Fatalf("step %d got=%v want=%v", i, topicNames(got), want)
		}
		recordRotationBlock(t, ctx, d, "subj-r10", got, now.Add(time.Duration(i)*time.Hour))
	}
}

func TestTopicRotationFewerThanThree(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	for _, n := range []int{1, 2} {
		sid := fmt.Sprintf("subj-f%d", n)
		eligible := seedRotationSubject(t, ctx, d, sid, sid, n)
		got, err := engine.NextPracticeTopics(ctx, d, eligible)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != n {
			t.Fatalf("n=%d got=%d want %d", n, len(got), n)
		}
	}
}

func TestTopicRotationExactlyThree(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	eligible := seedRotationSubject(t, ctx, d, "subj-e3", "E3", 3)
	now := time.Now()
	for i := 0; i < 3; i++ {
		got, err := engine.NextPracticeTopics(ctx, d, eligible)
		if err != nil {
			t.Fatal(err)
		}
		if !eqStr(topicNames(got), []string{"A", "B", "C"}) {
			t.Fatalf("iter %d got=%v want ABC", i, topicNames(got))
		}
		recordRotationBlock(t, ctx, d, "subj-e3", got, now.Add(time.Duration(i)*time.Hour))
	}
}

func TestTopicRotationEligibilityChange(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	// Start with 8 topics A..H.
	eligible8 := seedRotationSubject(t, ctx, d, "subj-el", "EL", 8)
	now := time.Now()
	got, _ := engine.NextPracticeTopics(ctx, d, eligible8)
	if !eqStr(topicNames(got), []string{"A", "B", "C"}) {
		t.Fatalf("first=%v", topicNames(got))
	}
	recordRotationBlock(t, ctx, d, "subj-el", got, now)
	got, _ = engine.NextPracticeTopics(ctx, d, eligible8)
	if !eqStr(topicNames(got), []string{"D", "E", "F"}) {
		t.Fatalf("second=%v", topicNames(got))
	}
	recordRotationBlock(t, ctx, d, "subj-el", got, now.Add(time.Hour))
	// New topic I becomes eligible (sort_order 9).
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "subj-el-topic-09", SubjectID: "subj-el", Name: "I", SortOrder: 9}); err != nil {
		t.Fatal(err)
	}
	eligible9, err := catalog.GetGovTopics(ctx, d, "subj-el")
	if err != nil {
		t.Fatal(err)
	}
	if len(eligible9) != 9 {
		t.Fatalf("eligible=%d want 9", len(eligible9))
	}
	// Must continue G H I, not reset to A B C.
	got, _ = engine.NextPracticeTopics(ctx, d, eligible9)
	if !eqStr(topicNames(got), []string{"G", "H", "I"}) {
		t.Fatalf("after add got=%v want GHI", topicNames(got))
	}
	// Ineligible skip: delete H from eligibility by removing the topic row
	// is restricted (FK), so simulate by filtering eligible list (topic
	// unavailable). The algorithm must skip safely and continue.
	filtered := []db.GovTopic{}
	for _, tp := range eligible9 {
		if tp.Name == "H" {
			continue
		}
		filtered = append(filtered, tp)
	}
	recordRotationBlock(t, ctx, d, "subj-el", got, now.Add(2*time.Hour))
	got2, err := engine.NextPracticeTopics(ctx, d, filtered)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 3 {
		t.Fatalf("filtered len=%d", len(got2))
	}
	for _, tp := range got2 {
		if tp.Name == "H" {
			t.Fatal("ineligible H must be skipped")
		}
	}
}

func TestTopicRotationResumesHistory(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	eligible := seedRotationSubject(t, ctx, d, "subj-hist", "H", 6)
	now := time.Now()
	// Simulate existing history: A B C then D E F already recorded.
	got, _ := engine.NextPracticeTopics(ctx, d, eligible)
	recordRotationBlock(t, ctx, d, "subj-hist", got, now)
	got, _ = engine.NextPracticeTopics(ctx, d, eligible)
	recordRotationBlock(t, ctx, d, "subj-hist", got, now.Add(time.Hour))
	// New planner call must resume at E F? No: after ABC+DEF with 6 topics,
	// next is A B C (wrap). Check.
	got, _ = engine.NextPracticeTopics(ctx, d, eligible)
	if !eqStr(topicNames(got), []string{"A", "B", "C"}) {
		t.Fatalf("resume=%v want ABC", topicNames(got))
	}
}

func TestTopicRotationRestartDoesNotReset(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	eligible := seedRotationSubject(t, ctx, d, "subj-rs", "RS", 9)
	now := time.Now()
	got, _ := engine.NextPracticeTopics(ctx, d, eligible)
	recordRotationBlock(t, ctx, d, "subj-rs", got, now)
	// Simulate restart: fresh query, no in-memory cursor. Must still be DEF,
	// not reset to ABC.
	first, _ := engine.NextPracticeTopics(ctx, d, eligible)
	second, _ := engine.NextPracticeTopics(ctx, d, eligible)
	if !eqStr(topicNames(first), []string{"D", "E", "F"}) || !eqStr(topicNames(second), []string{"D", "E", "F"}) {
		t.Fatalf("restart must not reset: %v %v", topicNames(first), topicNames(second))
	}
}

func TestSubjectRotationUnchanged(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	// Subject rotation is exam-order + activation + eligibility, persisted in
	// gov_practice_rotation_state. Topic rotation must not touch it.
	_, subs, err := engine.CurrentGovSubjects(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) == 0 {
		t.Fatal("need subjects")
	}
	// Practice rotation state starts absent; topic picks must not create it.
	_, err = runtime.GetGovPracticeRotationState(ctx, d)
	if err == nil {
		t.Fatal("rotation state must start absent")
	}
	// Exercise topic rotation on a subject; subject state must stay absent.
	eligible, err := engine.PracticeEligibleTopics(ctx, d, "subj-quant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.NextPracticeTopics(ctx, d, eligible); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetGovPracticeRotationState(ctx, d); err == nil {
		t.Fatal("topic rotation must not write subject rotation state")
	}
}
