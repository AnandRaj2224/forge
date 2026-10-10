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

// seedSubjectAllocationWorld builds the Forge V2 regression world for the
// daily subject-allocation rule: SBI PO, IBPS SO (IT) and AFCAT share Quant
// (plus Reasoning/English) with per-exam fourth subjects. Quant's first
// topic carries two lectures so completing one lecture leaves the topic
// lecture-incomplete and practice stays empty under the
// fully-lecture-complete rule.
func seedSubjectAllocationWorld(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2},
		{ID: "subj-english", Name: "English", SortOrder: 3},
		{ID: "subj-banking", Name: "Banking Awareness", SortOrder: 4},
		{ID: "subj-it", Name: "IT", SortOrder: 5},
		{ID: "subj-history", Name: "History", SortOrder: 6},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, tp := range []db.GovTopic{
		{ID: "quant-t1", SubjectID: "subj-quant", Name: "Quant T1", SortOrder: 1},
		{ID: "quant-t2", SubjectID: "subj-quant", Name: "Quant T2", SortOrder: 2},
		{ID: "reasoning-t1", SubjectID: "subj-reasoning", Name: "R T1", SortOrder: 1},
		{ID: "english-t1", SubjectID: "subj-english", Name: "E T1", SortOrder: 1},
		{ID: "banking-t1", SubjectID: "subj-banking", Name: "B T1", SortOrder: 1},
		{ID: "it-t1", SubjectID: "subj-it", Name: "IT T1", SortOrder: 1},
		{ID: "history-t1", SubjectID: "subj-history", Name: "H T1", SortOrder: 1},
	} {
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range []db.GovLecture{
		{ID: "quant-l1a", TopicID: "quant-t1", Title: "Q1a", SortOrder: 1},
		{ID: "quant-l1b", TopicID: "quant-t1", Title: "Q1b", SortOrder: 2},
		{ID: "quant-l2a", TopicID: "quant-t2", Title: "Q2a", SortOrder: 1},
		{ID: "quant-l2b", TopicID: "quant-t2", Title: "Q2b", SortOrder: 2},
		{ID: "reasoning-l1a", TopicID: "reasoning-t1", Title: "R Core A", SortOrder: 1},
		{ID: "reasoning-l1b", TopicID: "reasoning-t1", Title: "R Core B", SortOrder: 2},
		{ID: "english-l1a", TopicID: "english-t1", Title: "E Core A", SortOrder: 1},
		{ID: "english-l1b", TopicID: "english-t1", Title: "E Core B", SortOrder: 2},
		{ID: "banking-l1", TopicID: "banking-t1", Title: "B Core", SortOrder: 1},
		{ID: "it-l1", TopicID: "it-t1", Title: "IT Core", SortOrder: 1},
		{ID: "history-l1", TopicID: "history-t1", Title: "H Core", SortOrder: 1},
	} {
		if err := catalog.InsertGovLecture(ctx, d, l); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []db.GovExam{
		{ID: "exam-sbi-po", Name: "SBI PO", SortOrder: 1},
		{ID: "exam-ibps-so", Name: "IBPS SO (IT)", SortOrder: 2},
		{ID: "exam-afcat", Name: "AFCAT", SortOrder: 3},
	} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "exam-sbi-po", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-sbi-po", SubjectID: "subj-reasoning", SortOrder: 2},
		{ExamID: "exam-sbi-po", SubjectID: "subj-english", SortOrder: 3},
		{ExamID: "exam-sbi-po", SubjectID: "subj-banking", SortOrder: 4},
		{ExamID: "exam-ibps-so", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-ibps-so", SubjectID: "subj-reasoning", SortOrder: 2},
		{ExamID: "exam-ibps-so", SubjectID: "subj-english", SortOrder: 3},
		{ExamID: "exam-ibps-so", SubjectID: "subj-it", SortOrder: 4},
		{ExamID: "exam-afcat", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-afcat", SubjectID: "subj-reasoning", SortOrder: 2},
		{ExamID: "exam-afcat", SubjectID: "subj-english", SortOrder: 3},
		{ExamID: "exam-afcat", SubjectID: "subj-history", SortOrder: 4},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	sbiSyll := []db.ID{"quant-t1", "quant-t2", "reasoning-t1", "english-t1", "banking-t1"}
	for i, tid := range sbiSyll {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-sbi-po", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	soSyll := []db.ID{"quant-t1", "quant-t2", "reasoning-t1", "english-t1", "it-t1"}
	for i, tid := range soSyll {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-ibps-so", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	afSyll := []db.ID{"quant-t1", "quant-t2", "reasoning-t1", "english-t1", "history-t1"}
	for i, tid := range afSyll {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-afcat", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
}

func quantLectureTasks(t *testing.T, tasks []db.TodayTask) []db.TodayTask {
	t.Helper()
	var out []db.TodayTask
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
			continue
		}
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if lt.SubjectID == "subj-quant" {
			out = append(out, tk)
		}
	}
	return out
}

func lectureSubjectCounts(t *testing.T, tasks []db.TodayTask) map[db.ID]int {
	t.Helper()
	out := map[db.ID]int{}
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
			continue
		}
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		out[lt.SubjectID]++
	}
	return out
}

func completeQuantLectureTask(t *testing.T, ctx context.Context, d *db.DB, taskID db.ID, at time.Time) db.ID {
	t.Helper()
	sess, err := engine.StartTodayTask(ctx, d, taskID, at)
	if err != nil {
		t.Fatalf("start quant task: %v", err)
	}
	if _, err := engine.StopActiveSession(ctx, d, at.Add(time.Minute)); err != nil {
		t.Fatalf("stop quant session: %v", err)
	}
	ev, err := engine.SaveGovLectureEvidence(ctx, d, taskID, sess.ID, true, at.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("save quant evidence: %v", err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("quant save must complete, got %s", ev.State)
	}
	return sess.ID
}

// TestGovSubjectAllocationOnePerDayAcrossFocusSwitches is the required
// regression test for the daily GOV subject-allocation rule: schedule Quant
// under SBI PO, complete it, switch to IBPS SO (IT), AFCAT, back to SBI PO,
// and reopen TODAY. There must remain exactly one Quant allocation for that
// local date with the completed task/session intact and no replacement Quant
// introduced. The next local date may schedule Quant again.
func TestGovSubjectAllocationOnePerDayAcrossFocusSwitches(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSubjectAllocationWorld(t, ctx, d)
	cal := engine.DefaultCalendar()
	base := time.Date(2026, time.September, 21, 9, 0, 0, 0, cal.Location())
	today := "2026-09-21"
	nextDate := "2026-09-22"

	if _, err := engine.SetFocusExam(ctx, d, "exam-sbi-po", base); err != nil {
		t.Fatal(err)
	}
	snap1, tasks1, err := engine.GenerateToday(ctx, d, today, base)
	if err != nil {
		t.Fatal(err)
	}
	if snap1.Revision != 1 {
		t.Fatalf("r1 revision=%d want 1", snap1.Revision)
	}
	q1 := quantLectureTasks(t, tasks1)
	if len(q1) != 1 {
		t.Fatalf("r1 quant allocations=%d want exactly 1", len(q1))
	}
	quantTaskID := q1[0].ID
	lt1, err := db.UnmarshalGovLectureTarget(*q1[0].TargetJSON)
	if err != nil {
		t.Fatal(err)
	}
	completedLectureID := lt1.LectureID

	// Complete the Quant allocation under SBI PO.
	quantSessionID := completeQuantLectureTask(t, ctx, d, quantTaskID, base.Add(time.Hour))
	ev, err := engine.EvaluateTodayTask(ctx, d, quantTaskID)
	if err != nil || ev.State != engine.StateCompleted {
		t.Fatalf("quant task must be COMPLETED after save: %v %v", ev.State, err)
	}

	assertSingleQuant := func(label string, tasks []db.TodayTask) {
		t.Helper()
		qs := quantLectureTasks(t, tasks)
		if len(qs) != 1 {
			t.Fatalf("%s: quant allocations=%d want exactly 1", label, len(qs))
		}
		if qs[0].ID != quantTaskID {
			t.Fatalf("%s: quant task ID changed: got %q want completed %q (replacement introduced)", label, qs[0].ID, quantTaskID)
		}
		lt, err := db.UnmarshalGovLectureTarget(*qs[0].TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if lt.LectureID != completedLectureID {
			t.Fatalf("%s: quant lecture changed: got %q want completed %q", label, lt.LectureID, completedLectureID)
		}
		if lt.SubjectID != "subj-quant" {
			t.Fatalf("%s: quant subject=%q want subj-quant", label, lt.SubjectID)
		}
		// Completed task must still evaluate COMPLETED with its session intact.
		cev, err := engine.EvaluateTodayTask(ctx, d, quantTaskID)
		if err != nil || cev.State != engine.StateCompleted {
			t.Fatalf("%s: completed quant must stay COMPLETED: %v %v", label, cev.State, err)
		}
		sess, err := runtime.GetSession(ctx, d, quantSessionID)
		if err != nil {
			t.Fatalf("%s: quant session lost: %v", label, err)
		}
		if sess.TodayTaskID != quantTaskID {
			t.Fatalf("%s: session retargeted to %q", label, sess.TodayTaskID)
		}
		if _, err := runtime.GetLectureCompletion(ctx, d, completedLectureID); err != nil {
			t.Fatalf("%s: lecture completion lost: %v", label, err)
		}
		// One-per-subject across all lecture tasks and max four per day.
		counts := lectureSubjectCounts(t, tasks)
		for sid, n := range counts {
			if n != 1 {
				t.Fatalf("%s: subject %q allocated %d times, want at most 1", label, sid, n)
			}
		}
		lecTotal := 0
		for _, tk := range tasks {
			if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture {
				lecTotal++
			}
		}
		if lecTotal > 4 {
			t.Fatalf("%s: lecture tasks=%d want <=4", label, lecTotal)
		}
	}

	// Switch to IBPS SO (IT): Quant must not receive another task even though
	// IBPS SO has other eligible Quant lectures (quant-l1b and quant-t2).
	_, tasks2, err := engine.SwitchFocusExam(ctx, d, "exam-ibps-so", today, base.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	assertSingleQuant("after IBPS SO switch", tasks2)

	// Switch to AFCAT.
	_, tasks3, err := engine.SwitchFocusExam(ctx, d, "exam-afcat", today, base.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	assertSingleQuant("after AFCAT switch", tasks3)

	// Switch back to SBI PO.
	snap4, tasks4, err := engine.SwitchFocusExam(ctx, d, "exam-sbi-po", today, base.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	assertSingleQuant("after back-to-SBI switch", tasks4)
	if snap4.Revision != snap1.Revision+3 {
		t.Fatalf("revisions: got r%d want r%d after three switches", snap4.Revision, snap1.Revision+3)
	}

	// Reopen TODAY (restart simulation): still exactly one Quant allocation.
	_, tasksReopen, err := engine.EnsureToday(ctx, d, today, base.Add(5*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	assertSingleQuant("after TODAY reopen", tasksReopen)

	// Historical snapshots remain immutable: r1 still holds the original task.
	r1Tasks, err := runtime.GetTodayTasks(ctx, d, snap1.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range r1Tasks {
		if tk.ID == quantTaskID {
			found = true
		}
	}
	if !found {
		t.Fatal("r1 snapshot must still contain the completed quant task")
	}

	// Next local date: Quant may be scheduled again (daily limit resets).
	// Global lecture-completion state is unchanged (quant-l1a still complete),
	// but the subject-allocation slot is fresh for the new date.
	nextSnap, nextTasks, err := engine.EnsureToday(ctx, d, nextDate, base.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if nextSnap.LocalDate != nextDate {
		t.Fatalf("next snapshot date=%q want %q", nextSnap.LocalDate, nextDate)
	}
	qNext := quantLectureTasks(t, nextTasks)
	if len(qNext) != 1 {
		t.Fatalf("next date quant allocations=%d want exactly 1 (limit resets)", len(qNext))
	}
	ltNext, err := db.UnmarshalGovLectureTarget(*qNext[0].TargetJSON)
	if err != nil {
		t.Fatal(err)
	}
	if ltNext.SubjectID != "subj-quant" {
		t.Fatalf("next quant subject=%q", ltNext.SubjectID)
	}
	if ltNext.LectureID == completedLectureID {
		t.Fatalf("next date must advance to unfinished Quant lecture, got completed %q again", ltNext.LectureID)
	}
	if qNext[0].ID == quantTaskID {
		t.Fatal("next date quant task must be a new task ID")
	}
	countsNext := lectureSubjectCounts(t, nextTasks)
	for sid, n := range countsNext {
		if n != 1 {
			t.Fatalf("next date subject %q allocated %d times", sid, n)
		}
	}
}

// subjectLectureTasks returns lecture tasks for one subject.
func subjectLectureTasks(t *testing.T, tasks []db.TodayTask, subjectID db.ID) []db.TodayTask {
	t.Helper()
	var out []db.TodayTask
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
			continue
		}
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if lt.SubjectID == subjectID {
			out = append(out, tk)
		}
	}
	return out
}

func completeLectureTaskGeneric(t *testing.T, ctx context.Context, d *db.DB, taskID db.ID, at time.Time) db.ID {
	t.Helper()
	sess, err := engine.StartTodayTask(ctx, d, taskID, at)
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	if _, err := engine.StopActiveSession(ctx, d, at.Add(time.Minute)); err != nil {
		t.Fatalf("stop session: %v", err)
	}
	ev, err := engine.SaveGovLectureEvidence(ctx, d, taskID, sess.ID, true, at.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("save evidence: %v", err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("save must complete, got %s", ev.State)
	}
	return sess.ID
}

// TestGovSubjectAllocationVariantsOtherSubjects repeats the daily-allocation
// invariant for Reasoning and English (completed), plus ACTIVE/stopped/
// untouched semantics: an ACTIVE or stopped-session allocation consumes the
// slot, an untouched assignment may be replaced but never duplicated.
func TestGovSubjectAllocationVariantsOtherSubjects(t *testing.T) {
	for _, subjectID := range []db.ID{"subj-reasoning", "subj-english"} {
		t.Run(string(subjectID)+"/completed", func(t *testing.T) {
			d := openEngineDB(t)
			ctx := context.Background()
			seedSubjectAllocationWorld(t, ctx, d)
			cal := engine.DefaultCalendar()
			base := time.Date(2026, time.September, 21, 9, 0, 0, 0, cal.Location())
			today := "2026-09-21"
			if _, err := engine.SetFocusExam(ctx, d, "exam-sbi-po", base); err != nil {
				t.Fatal(err)
			}
			_, tasks1, err := engine.GenerateToday(ctx, d, today, base)
			if err != nil {
				t.Fatal(err)
			}
			s1 := subjectLectureTasks(t, tasks1, subjectID)
			if len(s1) != 1 {
				t.Fatalf("r1 %q allocations=%d want 1", subjectID, len(s1))
			}
			taskID := s1[0].ID
			lt0, _ := db.UnmarshalGovLectureTarget(*s1[0].TargetJSON)
			lecID := lt0.LectureID
			sessID := completeLectureTaskGeneric(t, ctx, d, taskID, base.Add(time.Hour))
			for i, focus := range []db.ID{"exam-ibps-so", "exam-afcat", "exam-sbi-po"} {
				_, wtasks, err := engine.SwitchFocusExam(ctx, d, focus, today, base.Add(time.Duration(2+i)*time.Hour))
				if err != nil {
					t.Fatal(err)
				}
				got := subjectLectureTasks(t, wtasks, subjectID)
				if len(got) != 1 || got[0].ID != taskID {
					t.Fatalf("switch %d (%s): %q allocations must stay completed %q, got %v", i, focus, subjectID, taskID, got)
				}
				lt, _ := db.UnmarshalGovLectureTarget(*got[0].TargetJSON)
				if lt.LectureID != lecID {
					t.Fatalf("lecture changed: got %q want %q", lt.LectureID, lecID)
				}
				if ev, _ := engine.EvaluateTodayTask(ctx, d, taskID); ev.State != engine.StateCompleted {
					t.Fatalf("must stay COMPLETED, got %s", ev.State)
				}
				if sess, err := runtime.GetSession(ctx, d, sessID); err != nil || sess.TodayTaskID != taskID {
					t.Fatalf("session lost: %v", err)
				}
				for sid, n := range lectureSubjectCounts(t, wtasks) {
					if n != 1 {
						t.Fatalf("subject %q x%d", sid, n)
					}
				}
			}
		})
	}
	t.Run("reasoning/ACTIVE-consumes", func(t *testing.T) {
		d := openEngineDB(t)
		ctx := context.Background()
		seedSubjectAllocationWorld(t, ctx, d)
		cal := engine.DefaultCalendar()
		base := time.Date(2026, time.September, 21, 9, 0, 0, 0, cal.Location())
		today := "2026-09-21"
		if _, err := engine.SetFocusExam(ctx, d, "exam-sbi-po", base); err != nil {
			t.Fatal(err)
		}
		_, tasks1, err := engine.GenerateToday(ctx, d, today, base)
		if err != nil {
			t.Fatal(err)
		}
		s1 := subjectLectureTasks(t, tasks1, "subj-reasoning")
		if len(s1) != 1 {
			t.Fatalf("r1 reasoning=%d want 1", len(s1))
		}
		activeID := s1[0].ID
		if _, err := engine.StartTodayTask(ctx, d, activeID, base.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		_, wtasks, err := engine.SwitchFocusExam(ctx, d, "exam-ibps-so", today, base.Add(2*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		got := subjectLectureTasks(t, wtasks, "subj-reasoning")
		if len(got) != 1 || got[0].ID != activeID {
			t.Fatalf("ACTIVE reasoning must stay single %q, got %v", activeID, got)
		}
		if _, err := engine.StopActiveSession(ctx, d, base.Add(3*time.Hour)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("untouched-may-be-replaced-but-not-duplicated", func(t *testing.T) {
		d := openEngineDB(t)
		ctx := context.Background()
		seedSubjectAllocationWorld(t, ctx, d)
		cal := engine.DefaultCalendar()
		base := time.Date(2026, time.September, 21, 9, 0, 0, 0, cal.Location())
		today := "2026-09-21"
		if _, err := engine.SetFocusExam(ctx, d, "exam-sbi-po", base); err != nil {
			t.Fatal(err)
		}
		_, tasks1, err := engine.GenerateToday(ctx, d, today, base)
		if err != nil {
			t.Fatal(err)
		}
		// No sessions at all: everything untouched. Switching focus may
		// replace assignments with new-exam candidates, but must keep
		// at most one per subject and at most four total.
		_, wtasks, err := engine.SwitchFocusExam(ctx, d, "exam-ibps-so", today, base.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		for sid, n := range lectureSubjectCounts(t, wtasks) {
			if n != 1 {
				t.Fatalf("untouched switch: subject %q x%d", sid, n)
			}
		}
		lecTotal := 0
		for _, tk := range wtasks {
			if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture {
				lecTotal++
			}
		}
		if lecTotal == 0 || lecTotal > 4 {
			t.Fatalf("untouched switch lectures=%d want 1..4", lecTotal)
		}
		_ = tasks1
	})
}
