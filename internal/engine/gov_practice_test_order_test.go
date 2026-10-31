package engine_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

func openTestDBPractice(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "practice-test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func seedPracticeTestOrderFixtures(t *testing.T, ctx context.Context, d *db.DB, now time.Time) {
	t.Helper()
	// Subjects: Quant, English, Reasoning
	subjs := []db.GovSubject{
		{ID: db.PracticeSubjectQuant, Name: "QUANT", SortOrder: 1},
		{ID: db.PracticeSubjectEnglish, Name: "ENGLISH", SortOrder: 2},
		{ID: db.PracticeSubjectReasoning, Name: "REASONING", SortOrder: 3},
	}
	for _, s := range subjs {
		_ = catalog.InsertGovSubject(ctx, d, s)
		_ = runtime.ActivateGovSubject(ctx, d, s.ID, now)
	}

	// Exam: SBI PO containing all three
	examID := db.ID("exam-sbi-po")
	_ = catalog.InsertGovExam(ctx, d, db.GovExam{ID: examID, Name: "SBI PO", SortOrder: 1})
	for i, s := range subjs {
		_ = catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: examID, SubjectID: s.ID, SortOrder: i + 1})
		// Add at least one syllabus topic per subject so exam is configured
		tpID := db.ID("top-" + string(s.ID))
		_ = catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: tpID, SubjectID: s.ID, Name: string(s.ID) + " Core", SortOrder: 1})
		_ = catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: examID, TopicID: tpID, SortOrder: i + 1})
	}
	_ = catalog.SetGovFocusState(ctx, d, db.GovFocusState{ID: 1, FocusedExamID: &examID, UpdatedAt: now})

	// Practice Routine slots
	_ = catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: "FIXED_SUBJECT", FixedSubjectID: &subjs[0].ID})
	_ = catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 2, DurationSeconds: 3600, SelectionMode: "FIXED_SUBJECT", FixedSubjectID: &subjs[1].ID})
	_ = catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 3, DurationSeconds: 3600, SelectionMode: "FIXED_SUBJECT", FixedSubjectID: &subjs[2].ID})

	// Seed Sessions and Tests:
	// Quant: ses_1 (3 tests), ses_2 (3 tests)
	_ = catalog.InsertGovPracticeSession(ctx, d, db.GovPracticeSession{ID: "prac-ses-quant-1", SubjectID: db.PracticeSubjectQuant, SessionCode: "ses_1", SessionOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-quant-ses_1-1", SessionID: "prac-ses-quant-1", SubjectID: db.PracticeSubjectQuant, Title: "Number System - Beginner Level", SortOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-quant-ses_1-2", SessionID: "prac-ses-quant-1", SubjectID: db.PracticeSubjectQuant, Title: "Percentages - Beginner Level", SortOrder: 2})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-quant-ses_1-3", SessionID: "prac-ses-quant-1", SubjectID: db.PracticeSubjectQuant, Title: "Simplification - Clerk Prelims Level", SortOrder: 3})

	_ = catalog.InsertGovPracticeSession(ctx, d, db.GovPracticeSession{ID: "prac-ses-quant-2", SubjectID: db.PracticeSubjectQuant, SessionCode: "ses_2", SessionOrder: 2})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-quant-ses_2-1", SessionID: "prac-ses-quant-2", SubjectID: db.PracticeSubjectQuant, Title: "Ratio and Proportion - Beginner Level", SortOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-quant-ses_2-2", SessionID: "prac-ses-quant-2", SubjectID: db.PracticeSubjectQuant, Title: "Quadratic Equations 1 - Beginner Level", SortOrder: 2})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-quant-ses_2-3", SessionID: "prac-ses-quant-2", SubjectID: db.PracticeSubjectQuant, Title: "Tabular DI - Beginner Level", SortOrder: 3})

	// English: ses_1 (2 tests), ses_2 (2 tests)
	_ = catalog.InsertGovPracticeSession(ctx, d, db.GovPracticeSession{ID: "prac-ses-english-1", SubjectID: db.PracticeSubjectEnglish, SessionCode: "ses_1", SessionOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-english-ses_1-1", SessionID: "prac-ses-english-1", SubjectID: db.PracticeSubjectEnglish, Title: "Grammar Adverbs - Clerk Prelims Level", SortOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-english-ses_1-2", SessionID: "prac-ses-english-1", SubjectID: db.PracticeSubjectEnglish, Title: "Grammar Nouns - Clerk Prelims Level", SortOrder: 2})

	_ = catalog.InsertGovPracticeSession(ctx, d, db.GovPracticeSession{ID: "prac-ses-english-2", SubjectID: db.PracticeSubjectEnglish, SessionCode: "ses_2", SessionOrder: 2})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-english-ses_2-1", SessionID: "prac-ses-english-2", SubjectID: db.PracticeSubjectEnglish, Title: "Modifiers and Parallelism - Clerk Prelims Level", SortOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-english-ses_2-2", SessionID: "prac-ses-english-2", SubjectID: db.PracticeSubjectEnglish, Title: "Pronouns - Clerk Prelims Level", SortOrder: 2})

	// Reasoning: ses_1 (3 tests), ses_2 (3 tests)
	_ = catalog.InsertGovPracticeSession(ctx, d, db.GovPracticeSession{ID: "prac-ses-reasoning-1", SubjectID: db.PracticeSubjectReasoning, SessionCode: "ses_1", SessionOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-reasoning-ses_1-1", SessionID: "prac-ses-reasoning-1", SubjectID: db.PracticeSubjectReasoning, Title: "Grid Puzzles - Beginner Level", SortOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-reasoning-ses_1-2", SessionID: "prac-ses-reasoning-1", SubjectID: db.PracticeSubjectReasoning, Title: "Inequalities - Beginner Level", SortOrder: 2})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-reasoning-ses_1-3", SessionID: "prac-ses-reasoning-1", SubjectID: db.PracticeSubjectReasoning, Title: "Syllogism - Beginner Level", SortOrder: 3})

	_ = catalog.InsertGovPracticeSession(ctx, d, db.GovPracticeSession{ID: "prac-ses-reasoning-2", SubjectID: db.PracticeSubjectReasoning, SessionCode: "ses_2", SessionOrder: 2})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-reasoning-ses_2-1", SessionID: "prac-ses-reasoning-2", SubjectID: db.PracticeSubjectReasoning, Title: "Coding Decoding - Beginner Level", SortOrder: 1})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-reasoning-ses_2-2", SessionID: "prac-ses-reasoning-2", SubjectID: db.PracticeSubjectReasoning, Title: "Blood Relations 1 - Beginner Level", SortOrder: 2})
	_ = catalog.InsertGovPracticeTest(ctx, d, db.GovPracticeTest{ID: "test-reasoning-ses_2-3", SessionID: "prac-ses-reasoning-2", SubjectID: db.PracticeSubjectReasoning, Title: "Direction Sense - Beginner Level", SortOrder: 3})

	// Initialize Tracks
	_ = catalog.SetGovPracticeTrack(ctx, d, db.GovPracticeTrack{SubjectID: db.PracticeSubjectQuant, CurrentSessionOrder: 1, UpdatedAt: now})
	_ = catalog.SetGovPracticeTrack(ctx, d, db.GovPracticeTrack{SubjectID: db.PracticeSubjectEnglish, CurrentSessionOrder: 1, UpdatedAt: now})
	_ = catalog.SetGovPracticeTrack(ctx, d, db.GovPracticeTrack{SubjectID: db.PracticeSubjectReasoning, CurrentSessionOrder: 1, UpdatedAt: now})
}

func TestGovPracticeTestOrder_InitialPlanning(t *testing.T) {
	d := openTestDBPractice(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	seedPracticeTestOrderFixtures(t, ctx, d, now)

	snap, tasks, err := engine.EnsureToday(ctx, d, "2026-10-19", now)
	if err != nil {
		t.Fatalf("EnsureToday failed: %v", err)
	}
	if snap.ID == "" {
		t.Fatal("empty snapshot ID")
	}

	var practiceTasks []db.TodayTask
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			practiceTasks = append(practiceTasks, tk)
		}
	}
	if len(practiceTasks) != 3 {
		t.Fatalf("expected 3 practice tasks (1 per subject), got %d", len(practiceTasks))
	}

	// Verify Quant has ses_1
	target0, err := db.UnmarshalGovPracticeTarget(*practiceTasks[0].TargetJSON)
	if err != nil {
		t.Fatalf("unmarshal target0: %v", err)
	}
	if target0.SubjectID != db.PracticeSubjectQuant || target0.SessionOrder != 1 || target0.SessionCode != "ses_1" {
		t.Fatalf("quant target mismatch: %+v", target0)
	}
	if len(target0.TopicIDs) != 3 || target0.TopicIDs[0] != "test-quant-ses_1-1" {
		t.Fatalf("quant test IDs mismatch: %v", target0.TopicIDs)
	}
	if !strings.Contains(practiceTasks[0].Title, "Number System - Beginner Level") {
		t.Fatalf("quant title mismatch: %q", practiceTasks[0].Title)
	}

	// Verify English has ses_1
	target1, err := db.UnmarshalGovPracticeTarget(*practiceTasks[1].TargetJSON)
	if err != nil {
		t.Fatalf("unmarshal target1: %v", err)
	}
	if target1.SubjectID != db.PracticeSubjectEnglish || target1.SessionOrder != 1 || len(target1.TopicIDs) != 2 {
		t.Fatalf("english target mismatch: %+v", target1)
	}

	// Verify Reasoning has ses_1
	target2, err := db.UnmarshalGovPracticeTarget(*practiceTasks[2].TargetJSON)
	if err != nil {
		t.Fatalf("unmarshal target2: %v", err)
	}
	if target2.SubjectID != db.PracticeSubjectReasoning || target2.SessionOrder != 1 || len(target2.TopicIDs) != 3 {
		t.Fatalf("reasoning target mismatch: %+v", target2)
	}
}

func TestGovPracticeTestOrder_MissedDayRepeatsSession(t *testing.T) {
	d := openTestDBPractice(t)
	ctx := context.Background()
	day1 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	seedPracticeTestOrderFixtures(t, ctx, d, day1)

	// Day 1 generated, but user misses it (no session completed)
	_, tasks1, err := engine.EnsureToday(ctx, d, "2026-10-19", day1)
	if err != nil {
		t.Fatalf("EnsureToday day 1: %v", err)
	}
	_ = tasks1

	// Day 2 generated
	day2 := day1.Add(24 * time.Hour)
	_, tasks2, err := engine.EnsureToday(ctx, d, "2026-10-20", day2)
	if err != nil {
		t.Fatalf("EnsureToday day 2: %v", err)
	}

	var practiceTasks2 []db.TodayTask
	for _, tk := range tasks2 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			practiceTasks2 = append(practiceTasks2, tk)
		}
	}
	if len(practiceTasks2) != 3 {
		t.Fatalf("expected 3 practice tasks on day 2, got %d", len(practiceTasks2))
	}

	// All 3 subjects must STILL be on ses_1 because day 1 was missed!
	for i, name := range []string{"Quant", "English", "Reasoning"} {
		tgt, err := db.UnmarshalGovPracticeTarget(*practiceTasks2[i].TargetJSON)
		if err != nil {
			t.Fatalf("%s unmarshal: %v", name, err)
		}
		if tgt.SessionOrder != 1 || tgt.SessionCode != "ses_1" {
			t.Fatalf("%s session order was %d (%s), wanted ses_1 because day 1 was missed!", name, tgt.SessionOrder, tgt.SessionCode)
		}
	}
}

func TestGovPracticeTestOrder_CompletedSessionAdvancesNextDay(t *testing.T) {
	d := openTestDBPractice(t)
	ctx := context.Background()
	day1 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	seedPracticeTestOrderFixtures(t, ctx, d, day1)

	// Day 1 generated
	_, tasks1, err := engine.EnsureToday(ctx, d, "2026-10-19", day1)
	if err != nil {
		t.Fatalf("EnsureToday day 1: %v", err)
	}

	// Find English task
	var englishTask db.TodayTask
	for _, tk := range tasks1 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			tgt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if tgt.SubjectID == db.PracticeSubjectEnglish {
				englishTask = tk
				break
			}
		}
	}
	if englishTask.ID == "" {
		t.Fatal("english practice task not found")
	}

	// User runs and completes the English practice session
	_, err = engine.StartTodayTask(ctx, d, englishTask.ID, day1)
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	stopped, err := engine.StopActiveSession(ctx, d, day1.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("stop session: %v", err)
	}

	// Record evidence for both English tests in ses_1
	inputs := []engine.GovPracticeInput{
		{TopicID: "test-english-ses_1-1", TimeSecs: 900, Attempted: 15, Correct: 13},
		{TopicID: "test-english-ses_1-2", TimeSecs: 900, Attempted: 15, Correct: 14},
	}
	ev, err := engine.SaveGovPracticeEvidence(ctx, d, englishTask.ID, stopped.ID, inputs, day1.Add(35*time.Minute))
	if err != nil {
		t.Fatalf("save practice evidence: %v", err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("expected English practice task to be COMPLETED, got %s", ev.State)
	}

	// Quant and Reasoning tasks were NOT completed.

	// Day 2 arrives!
	day2 := day1.Add(24 * time.Hour)
	_, tasks2, err := engine.EnsureToday(ctx, d, "2026-10-20", day2)
	if err != nil {
		t.Fatalf("EnsureToday day 2: %v", err)
	}

	var ptQuant, ptEnglish, ptReasoning db.GovPracticeTarget
	for _, tk := range tasks2 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			tgt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			switch tgt.SubjectID {
			case db.PracticeSubjectQuant:
				ptQuant = tgt
			case db.PracticeSubjectEnglish:
				ptEnglish = tgt
			case db.PracticeSubjectReasoning:
				ptReasoning = tgt
			}
		}
	}

	// English was completed on Day 1 -> must advance to ses_2 on Day 2!
	if ptEnglish.SessionOrder != 2 || ptEnglish.SessionCode != "ses_2" {
		t.Fatalf("English should advance to ses_2, got order %d code %s", ptEnglish.SessionOrder, ptEnglish.SessionCode)
	}
	if len(ptEnglish.TopicIDs) != 2 || ptEnglish.TopicIDs[0] != "test-english-ses_2-1" {
		t.Fatalf("English ses_2 test IDs mismatch: %v", ptEnglish.TopicIDs)
	}

	// Quant and Reasoning were missed -> must still be ses_1!
	if ptQuant.SessionOrder != 1 || ptQuant.SessionCode != "ses_1" {
		t.Fatalf("Quant was missed, should stay on ses_1, got order %d", ptQuant.SessionOrder)
	}
	if ptReasoning.SessionOrder != 1 || ptReasoning.SessionCode != "ses_1" {
		t.Fatalf("Reasoning was missed, should stay on ses_1, got order %d", ptReasoning.SessionOrder)
	}
}

func TestGovPracticeTestOrder_PartialCompletionDoesNotAdvance(t *testing.T) {
	d := openTestDBPractice(t)
	ctx := context.Background()
	day1 := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	seedPracticeTestOrderFixtures(t, ctx, d, day1)

	// Day 1 generated
	_, tasks1, err := engine.EnsureToday(ctx, d, "2026-10-19", day1)
	if err != nil {
		t.Fatalf("EnsureToday day 1: %v", err)
	}

	// Find Quant task (has 3 tests in ses_1)
	var quantTask db.TodayTask
	for _, tk := range tasks1 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			tgt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if tgt.SubjectID == db.PracticeSubjectQuant {
				quantTask = tk
				break
			}
		}
	}

	// User only finishes 1 of the 3 tests
	_, _ = engine.StartTodayTask(ctx, d, quantTask.ID, day1)
	stopped, _ := engine.StopActiveSession(ctx, d, day1.Add(15*time.Minute))
	inputs := []engine.GovPracticeInput{
		{TopicID: "test-quant-ses_1-1", TimeSecs: 600, Attempted: 10, Correct: 8},
	}
	ev, err := engine.SaveGovPracticeEvidence(ctx, d, quantTask.ID, stopped.ID, inputs, day1.Add(20*time.Minute))
	if err != nil {
		t.Fatalf("save partial evidence: %v", err)
	}
	// Task is partially satisfied -> remains TODO (not COMPLETED)
	if ev.State == engine.StateCompleted {
		t.Fatal("partial evidence must not mark task COMPLETED")
	}

	// Day 2 arrives! Since ses_1 was NOT completed, Quant MUST still be ses_1!
	day2 := day1.Add(24 * time.Hour)
	_, tasks2, err := engine.EnsureToday(ctx, d, "2026-10-20", day2)
	if err != nil {
		t.Fatalf("EnsureToday day 2: %v", err)
	}

	var quantTask2 db.TodayTask
	for _, tk := range tasks2 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			tgt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if tgt.SubjectID == db.PracticeSubjectQuant {
				quantTask2 = tk
				break
			}
		}
	}
	tgt2, _ := db.UnmarshalGovPracticeTarget(*quantTask2.TargetJSON)
	if tgt2.SessionOrder != 1 || tgt2.SessionCode != "ses_1" {
		t.Fatalf("Quant should still be ses_1, got order %d code %s", tgt2.SessionOrder, tgt2.SessionCode)
	}

	// Now complete the session on Day 2 (tests 1, 2, and 3)
	_, _ = engine.StartTodayTask(ctx, d, quantTask2.ID, day2)
	stopped2, _ := engine.StopActiveSession(ctx, d, day2.Add(30*time.Minute))
	inputs2 := []engine.GovPracticeInput{
		{TopicID: "test-quant-ses_1-1", TimeSecs: 600, Attempted: 10, Correct: 10},
		{TopicID: "test-quant-ses_1-2", TimeSecs: 600, Attempted: 10, Correct: 9},
		{TopicID: "test-quant-ses_1-3", TimeSecs: 600, Attempted: 10, Correct: 8},
	}
	ev2, err := engine.SaveGovPracticeEvidence(ctx, d, quantTask2.ID, stopped2.ID, inputs2, day2.Add(35*time.Minute))
	if err != nil {
		t.Fatalf("save remaining evidence: %v", err)
	}
	// Now all 3 tests of ses_1 are satisfied!
	if ev2.State != engine.StateCompleted {
		t.Fatalf("expected Quant ses_1 to now be COMPLETED, got %s", ev2.State)
	}

	// Day 3 arrives! Quant must now advance to ses_2!
	day3 := day2.Add(24 * time.Hour)
	_, tasks3, err := engine.EnsureToday(ctx, d, "2026-10-21", day3)
	if err != nil {
		t.Fatalf("EnsureToday day 3: %v", err)
	}

	for _, tk := range tasks3 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			tgt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if tgt.SubjectID == db.PracticeSubjectQuant {
				if tgt.SessionOrder != 2 || tgt.SessionCode != "ses_2" {
					t.Fatalf("Quant should advance to ses_2 on Day 3, got order %d code %s", tgt.SessionOrder, tgt.SessionCode)
				}
			}
		}
	}
}
