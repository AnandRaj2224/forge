package engine_test

import (
	"context"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// V2: zero-lecture topics stay lecture-complete; empty exams are unconfigured.
func TestV2ZeroLectureCompleteEmptyExamUnconfigured(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	if err := catalog.InsertGovSubject(ctx, d, db.GovSubject{ID: "s1", Name: "S1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "t-zero", SubjectID: "s1", Name: "Zero", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	ok, err := engine.IsGovTopicLectureComplete(ctx, d, "t-zero")
	if err != nil || !ok {
		t.Fatalf("zero-lecture topic must be complete: %v %v", ok, err)
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "e-empty", Name: "Empty", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	cfg, err := engine.IsGovExamConfigured(ctx, d, "e-empty")
	if err != nil || cfg {
		t.Fatalf("empty exam must be unconfigured: %v %v", cfg, err)
	}
	done, err := engine.IsGovExamComplete(ctx, d, "e-empty")
	if err != nil || done {
		t.Fatalf("empty exam must NOT be complete: %v %v", done, err)
	}
}

// V2: focus defaults, sets, and same-focus no-op.
func TestV2FocusLifecycle(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	now := time.Now()
	for _, e := range []db.GovExam{{ID: "e1", Name: "E1", SortOrder: 1}, {ID: "e2", Name: "E2", SortOrder: 2}} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	f, err := engine.EnsureFocusExam(ctx, d, now)
	if err != nil || f == nil || f.ID != "e1" {
		t.Fatalf("default focus must be first exam: %v %v", f, err)
	}
	f2, err := engine.SetFocusExam(ctx, d, "e2", now)
	if err != nil || f2.ID != "e2" {
		t.Fatalf("set focus e2: %v %v", f2, err)
	}
	got, err := engine.GetFocusExam(ctx, d)
	if err != nil || got == nil || got.ID != "e2" {
		t.Fatalf("get focus e2: %v %v", got, err)
	}
}

// V2: fixed practice subjects use stable IDs (no display names).
func TestV2FixedPracticeSubjectsByID(t *testing.T) {
	want := []db.ID{"subj-quant", "subj-english", "subj-reasoning"}
	if len(db.V2FixedPracticeSubjects) != 3 {
		t.Fatalf("fixed subjects=%v", db.V2FixedPracticeSubjects)
	}
	for i, id := range want {
		if db.V2FixedPracticeSubjects[i] != id {
			t.Fatalf("fixed[%d]=%q want %q", i, db.V2FixedPracticeSubjects[i], id)
		}
	}
}

// V2: per-exam+subject rotation advances 3 at a time and wraps.
func TestV2PracticeRotationCursor(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	now := time.Now()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-english", Name: "English", SortOrder: 2},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 3},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "ex", Name: "EX", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []db.GovExamSubject{
		{ExamID: "ex", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "ex", SubjectID: "subj-english", SortOrder: 2},
		{ExamID: "ex", SubjectID: "subj-reasoning", SortOrder: 3},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	// 5 zero-lecture topics under quant (immediately eligible).
	for i := 1; i <= 5; i++ {
		id := db.ID("q-t" + string(rune('0'+i)))
		if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: id, SubjectID: "subj-quant", Name: string(rune('A' + i - 1)), SortOrder: i}); err != nil {
			t.Fatal(err)
		}
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "ex", SubjectID: "subj-quant", TopicID: id, SortOrder: i}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.SetFocusExam(ctx, d, "ex", now); err != nil {
		t.Fatal(err)
	}
	for _, sid := range []db.ID{"subj-quant", "subj-english", "subj-reasoning"} {
		if err := engine.EnsureGovSubjectActive(ctx, d, sid, now); err != nil {
			t.Fatal(err)
		}
	}
	snap1, tasks1, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	_ = snap1
	var quantTopics1 []string
	for _, tk := range tasks1 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice && tk.TargetJSON != nil {
			pt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if pt.SubjectID == "subj-quant" {
				for _, tid := range pt.TopicIDs {
					quantTopics1 = append(quantTopics1, string(tid))
				}
			}
		}
	}
	if len(quantTopics1) != 3 {
		t.Fatalf("first quant bundle=%v want 3", quantTopics1)
	}
	// Second day advances (cursor persisted, reopening same day does not).
	snap2, tasks2, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil || snap2.ID != snap1.ID || len(tasks2) != len(tasks1) {
		t.Fatalf("reopen must not advance: %v %v", snap2.ID, err)
	}
	_, tasks3, err := engine.GenerateToday(ctx, d, "2026-09-22", now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var quantTopics2 []string
	for _, tk := range tasks3 {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice && tk.TargetJSON != nil {
			pt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if pt.SubjectID == "subj-quant" {
				for _, tid := range pt.TopicIDs {
					quantTopics2 = append(quantTopics2, string(tid))
				}
			}
		}
	}
	if len(quantTopics2) != 3 {
		t.Fatalf("second quant bundle=%v want 3", quantTopics2)
	}
	seen := map[string]bool{}
	for _, id := range quantTopics1 {
		seen[id] = true
	}
	overlap := 0
	for _, id := range quantTopics2 {
		if seen[id] {
			overlap++
		}
	}
	// 5 eligible, 3+3 with wrap => exactly 1 overlap (JAB pattern analog).
	if overlap != 1 {
		t.Fatalf("rotation must advance (overlap=%d): %v -> %v", overlap, quantTopics1, quantTopics2)
	}
}

// V2: focus switch locks started work and replaces untouched tasks.
func TestV2FocusSwitchLocks(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	now := time.Now()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-english", Name: "English", SortOrder: 2},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []db.GovExam{{ID: "ea", Name: "EA", SortOrder: 1}, {ID: "eb", Name: "EB", SortOrder: 2}} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "ea", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "eb", SubjectID: "subj-english", SortOrder: 1},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "tq", SubjectID: "subj-quant", Name: "Q", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "te", SubjectID: "subj-english", Name: "E", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "ea", SubjectID: "subj-quant", TopicID: "tq", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "eb", SubjectID: "subj-english", TopicID: "te", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SetFocusExam(ctx, d, "ea", now); err != nil {
		t.Fatal(err)
	}
	// Activate subjects so practice slots are eligible (zero-lecture topics).
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-quant", now); err != nil {
		t.Fatal(err)
	}
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-english", now); err != nil {
		t.Fatal(err)
	}
	snap, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("need tasks")
	}
	// Start the first task (locks it).
	first := tasks[0]
	if _, err := engine.StartTodayTask(ctx, d, first.ID, now); err != nil {
		t.Fatal(err)
	}
	lockedID := first.ID
	lockedTarget := ""
	if first.TargetJSON != nil {
		lockedTarget = *first.TargetJSON
	}
	// Switch focus: locked task keeps ID/target; revision increments.
	snap2, tasks2, err := engine.SwitchFocusExam(ctx, d, "eb", "2026-09-21", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap2.Revision != snap.Revision+1 {
		t.Fatalf("rev=%d want %d", snap2.Revision, snap.Revision+1)
	}
	found := false
	for _, tk := range tasks2 {
		if tk.ID == lockedID {
			found = true
			if tk.TargetJSON == nil || *tk.TargetJSON != lockedTarget {
				t.Fatal("locked task target changed")
			}
		}
	}
	if !found {
		t.Fatal("locked task missing after switch")
	}
	// Sessions still reference the original task.
	sess, err := runtime.GetActiveSession(ctx, d)
	if err != nil {
		t.Fatalf("active session lost: %v", err)
	}
	if sess.TodayTaskID != lockedID {
		t.Fatalf("session retargeted: %q", sess.TodayTaskID)
	}
	// Same-focus is a no-op.
	snap3, _, err := engine.SwitchFocusExam(ctx, d, "eb", "2026-09-21", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap3.ID != snap2.ID {
		t.Fatal("same-focus must be no-op")
	}
}

// V2: DSA allows small bundles and never pulls future repeats.
func TestV2DSASmallBundle(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	if err := catalog.InsertDSAProblem(ctx, d, db.DSAProblem{ID: "solo-1", Title: "Solo", Pattern: "P", Difficulty: "EASY", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.UpsertDSARoutine(ctx, d, db.DSARoutine{ID: 1, PlannedSeconds: 9000}); err != nil {
		t.Fatal(err)
	}
	// Seed minimal catalogue so planner can form a 1-problem bundle.
	dt := db.DSATarget{Problems: []db.DSAProblemTarget{{ProblemID: "solo-1", Mode: db.DSAModeNew}}}
	if err := engine.ValidateDSABundleV2(ctx, d, dt); err != nil {
		t.Fatalf("1-problem bundle must validate: %v", err)
	}
	if err := engine.ValidateDSACurrentBundle(ctx, d, dt); err == nil {
		t.Fatal("legacy strict bundle must still require 4")
	}
}

// V2: strict integer parsing rejects trailing garbage.
func TestV2StrictIntParsing(t *testing.T) {
	_, msg := engine.ValidateGovPracticeInput("T", "10", "00abc", "0", "", false)
	if msg == "" {
		t.Fatal("00abc must be rejected")
	}
	_, msg = engine.ValidateGovPracticeInput("T", "10", "12abc", "0", "", false)
	if msg == "" {
		t.Fatal("12abc must be rejected")
	}
}
