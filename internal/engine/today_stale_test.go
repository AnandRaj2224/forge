package engine_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// Shared-lecture fixture: exams A and B share the Number System topic
// (lectures lec-ns-1, lec-ns-2); each exam also owns distinct topics.
// All three fixed practice subjects exist; reasoning topics are zero-lecture
// (immediately eligible) so practice noise is deterministic.
func seedSharedExams(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-english", Name: "English", SortOrder: 2},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 3},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	topics := []db.GovTopic{
		{ID: "topic-ns", SubjectID: "subj-quant", Name: "Number System", SortOrder: 1},
		{ID: "topic-perc", SubjectID: "subj-quant", Name: "Percentage", SortOrder: 2},
		{ID: "topic-gram", SubjectID: "subj-english", Name: "Grammar", SortOrder: 1},
		{ID: "topic-logic", SubjectID: "subj-reasoning", Name: "Logic", SortOrder: 1},
	}
	for _, tp := range topics {
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
	}
	lecs := []db.GovLecture{
		{ID: "lec-ns-1", TopicID: "topic-ns", Title: "Core Concepts", SortOrder: 1},
		{ID: "lec-ns-2", TopicID: "topic-ns", Title: "Worked Examples", SortOrder: 2},
		{ID: "lec-perc-1", TopicID: "topic-perc", Title: "Core Concepts", SortOrder: 1},
		{ID: "lec-gram-1", TopicID: "topic-gram", Title: "Core Concepts", SortOrder: 1},
	}
	for _, l := range lecs {
		if err := catalog.InsertGovLecture(ctx, d, l); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []db.GovExam{
		{ID: "exam-a", Name: "AFCAT", SortOrder: 1},
		{ID: "exam-b", Name: "SSC", SortOrder: 2},
	} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "exam-a", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-a", SubjectID: "subj-english", SortOrder: 2},
		{ExamID: "exam-a", SubjectID: "subj-reasoning", SortOrder: 3},
		{ExamID: "exam-b", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-b", SubjectID: "subj-english", SortOrder: 2},
		{ExamID: "exam-b", SubjectID: "subj-reasoning", SortOrder: 3},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	// A owns ns/perc/gram/logic; B shares ns/gram/logic but not perc.
	aSyll := []db.ID{"topic-ns", "topic-perc", "topic-gram", "topic-logic"}
	for i, tid := range aSyll {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-a", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	bSyll := []db.ID{"topic-ns", "topic-gram", "topic-logic"}
	for i, tid := range bSyll {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-b", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, sid := range []db.ID{"subj-quant", "subj-english", "subj-reasoning"} {
		if err := engine.EnsureGovSubjectActive(ctx, d, sid, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
}

// lectureTargets maps each task to its lecture ID (tasks without a parsable
// lecture target are absent from the map).
func lectureTargets(t *testing.T, tasks []db.TodayTask) map[db.ID][]db.ID {
	t.Helper()
	out := map[db.ID][]db.ID{}
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture {
			continue
		}
		if tk.TargetJSON == nil {
			continue
		}
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		out[lt.LectureID] = append(out[lt.LectureID], tk.ID)
	}
	return out
}

func lectureTaskIDs(tasks []db.TodayTask) []db.ID {
	var out []db.ID
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture {
			out = append(out, tk.ID)
		}
	}
	return out
}

func taskStates(t *testing.T, ctx context.Context, d *db.DB, ids []db.ID) map[db.ID]engine.TaskState {
	t.Helper()
	out := map[db.ID]engine.TaskState{}
	for _, id := range ids {
		ev, err := engine.EvaluateTodayTask(ctx, d, id)
		if err != nil {
			t.Fatal(err)
		}
		out[id] = ev.State
	}
	return out
}

func completeLectureTask(t *testing.T, ctx context.Context, d *db.DB, taskID db.ID, at time.Time) {
	t.Helper()
	sess, err := engine.StartTodayTask(ctx, d, taskID, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ev, err := engine.SaveGovLectureEvidence(ctx, d, taskID, sess.ID, true, at.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("lecture save must complete, got %s", ev.State)
	}
}

// TestV2SharedLectureNeverReplanned: complete a shared canonical lecture
// under exam A, switch A→B→A→B→A. The same lecture must never appear again
// as a new TODO assignment and the completed task keeps its ID. Under the
// daily subject-allocation rule the completed Quant allocation consumes
// Quant's daily slot: the same date must not gain a second Quant lecture
// (even a different lecture ID) and the remaining Quant work becomes
// available again only on the next local date.
func TestV2SharedLectureNeverReplanned(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSharedExams(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	// Find the lec-ns-1 task and complete it.
	lm := lectureTargets(t, tasks)
	lecTasks, ok := lm["lec-ns-1"]
	if !ok || len(lecTasks) != 1 {
		t.Fatalf("r1 must assign lec-ns-1 exactly once, got %v", lm)
	}
	lecTask := lecTasks[0]
	completeLectureTask(t, ctx, d, lecTask, now.Add(time.Hour))
	states := taskStates(t, ctx, d, []db.ID{lecTask})
	if states[lecTask] != engine.StateCompleted {
		t.Fatalf("lec-ns-1 task must be COMPLETED, got %s", states[lecTask])
	}

	for i, focus := range []db.ID{"exam-b", "exam-a", "exam-b", "exam-a"} {
		_, wtasks, err := engine.SwitchFocusExam(ctx, d, focus, "2026-09-21", now.Add(time.Duration(2+i)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		wm := lectureTargets(t, wtasks)
		// The completed task survives every switch with its original ID.
		kept, ok := wm["lec-ns-1"]
		if !ok || len(kept) != 1 || kept[0] != lecTask {
			t.Fatalf("switch %d (%s): completed lec-ns-1 task must be preserved by ID, got %v", i, focus, wm)
		}
		keptStates := taskStates(t, ctx, d, kept)
		if keptStates[lecTask] != engine.StateCompleted {
			t.Fatalf("switch %d (%s): preserved task must stay COMPLETED, got %s", i, focus, keptStates[lecTask])
		}
		// Exactly one assignment references lec-ns-1: no TODO duplicate.
		if len(wm["lec-ns-1"]) != 1 {
			t.Fatalf("switch %d (%s): lec-ns-1 referenced %d times, want 1", i, focus, len(wm["lec-ns-1"]))
		}
		for lid, tids := range wm {
			if lid == "lec-ns-1" {
				continue
			}
			for _, tid := range tids {
				ev, err := engine.EvaluateTodayTask(ctx, d, tid)
				if err != nil {
					t.Fatal(err)
				}
				_ = ev
			}
		}
		// Daily subject-allocation rule: the completed Quant allocation
		// consumes Quant's daily slot. No same-date revision may carry a
		// second Quant lecture, even for a different unfinished lecture
		// (lec-ns-2 or lec-perc-1). The remaining Quant work is a next-day
		// concern, not a same-day second allocation.
		quantAllocs := 0
		for _, tk := range wtasks {
			if tk.TargetJSON == nil {
				continue
			}
			lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
			if err != nil || lt.TopicID == "" {
				continue
			}
			tp, err := catalog.GetGovTopic(ctx, d, lt.TopicID)
			if err != nil {
				t.Fatal(err)
			}
			if tp.SubjectID == "subj-quant" {
				quantAllocs++
				if lt.LectureID != "lec-ns-1" {
					t.Fatalf("switch %d (%s): second Quant allocation %q forbidden same date (daily subject slot consumed), got %v", i, focus, lt.LectureID, wm)
				}
			}
		}
		if quantAllocs != 1 {
			t.Fatalf("switch %d (%s): Quant allocations=%d want exactly 1 (completed only), got %v", i, focus, quantAllocs, wm)
		}
	}
}

// TestV2ActiveLectureNotDuplicated: a lecture with an ACTIVE session is
// locked; switching focus must not create a second TODO assignment for the
// same canonical lecture, nor a second allocation for the same subject.
func TestV2ActiveLectureNotDuplicated(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSharedExams(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	lm := lectureTargets(t, tasks)
	lecTasks, ok := lm["lec-ns-1"]
	if !ok || len(lecTasks) != 1 {
		t.Fatalf("r1 must assign lec-ns-1 exactly once, got %v", lm)
	}
	activeID := lecTasks[0]
	if _, err := engine.StartTodayTask(ctx, d, activeID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, wtasks, err := engine.SwitchFocusExam(ctx, d, "exam-b", "2026-09-21", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	wm := lectureTargets(t, wtasks)
	if len(wm["lec-ns-1"]) != 1 || wm["lec-ns-1"][0] != activeID {
		t.Fatalf("exactly the locked ACTIVE task may target lec-ns-1, got %v", wm)
	}
	// No lecture ID appears twice across the revision.
	for lid, tids := range wm {
		if len(tids) != 1 {
			t.Fatalf("lecture %q assigned %d times", lid, len(tids))
		}
	}
	// Daily subject-allocation rule: the ACTIVE Quant allocation already
	// consumes Quant's slot, so no second Quant lecture may appear.
	quantAllocs := 0
	for _, tk := range wtasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
			continue
		}
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		tp, err := catalog.GetGovTopic(ctx, d, lt.TopicID)
		if err != nil {
			t.Fatal(err)
		}
		if tp.SubjectID == "subj-quant" {
			quantAllocs++
		}
	}
	if quantAllocs != 1 {
		t.Fatalf("Quant allocations=%d want exactly 1 (active only), got %v", quantAllocs, wm)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

// seedCapExams builds 8 single-lecture subjects; exam A owns S1..S6,
// exam B owns S5..S8 first (then shared S1..S4).
func seedCapExams(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for i := 1; i <= 8; i++ {
		sid := db.ID(fmt.Sprintf("cap-s%d", i))
		if err := catalog.InsertGovSubject(ctx, d, db.GovSubject{ID: sid, Name: fmt.Sprintf("S%d", i), SortOrder: 10 + i}); err != nil {
			t.Fatal(err)
		}
		tid := db.ID(fmt.Sprintf("cap-t%d", i))
		if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: tid, SubjectID: sid, Name: fmt.Sprintf("T%d", i), SortOrder: 1}); err != nil {
			t.Fatal(err)
		}
		if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: db.ID(fmt.Sprintf("cap-l%d", i)), TopicID: tid, Title: fmt.Sprintf("Lesson %d", i), SortOrder: 1}); err != nil {
			t.Fatal(err)
		}
		if err := engine.EnsureGovSubjectActive(ctx, d, sid, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "cap-a", Name: "CapA", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "cap-b", Name: "CapB", SortOrder: 2}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: "cap-a", SubjectID: db.ID(fmt.Sprintf("cap-s%d", i)), SortOrder: i}); err != nil {
			t.Fatal(err)
		}
	}
	order := []int{5, 6, 7, 8, 1, 2, 3, 4}
	for pos, i := range order {
		if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: "cap-b", SubjectID: db.ID(fmt.Sprintf("cap-s%d", i)), SortOrder: pos + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 6; i++ {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "cap-a", TopicID: db.ID(fmt.Sprintf("cap-t%d", i)), SortOrder: i}); err != nil {
			t.Fatal(err)
		}
	}
	for pos, i := range order {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "cap-b", TopicID: db.ID(fmt.Sprintf("cap-t%d", i)), SortOrder: pos + 1}); err != nil {
			t.Fatal(err)
		}
	}
}

func countKind(tasks []db.TodayTask, domain db.Domain, kind db.SessionKind) int {
	n := 0
	for _, tk := range tasks {
		if tk.Domain == domain && tk.Kind == kind {
			n++
		}
	}
	return n
}

// TestV2SwitchCapsLecturesWhenLockedConsumeAllSlots: 4 locked lectures leave
// no room; fresh candidates must be dropped, never appended past the cap,
// and locked tasks keep their IDs.
func TestV2SwitchCapsLecturesWhenLockedConsumeAllSlots(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedCapExams(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "cap-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := countKind(tasks, db.DomainGOV, db.SessionKindLecture); got != 4 {
		t.Fatalf("r1 lectures=%d want 4", got)
	}
	base := now.Add(time.Hour)
	for i, tid := range lectureTaskIDs(tasks) {
		completeLectureTask(t, ctx, d, tid, base.Add(time.Duration(i)*time.Hour))
	}
	_, wtasks, err := engine.SwitchFocusExam(ctx, d, "cap-b", "2026-09-21", base.Add(8*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := countKind(wtasks, db.DomainGOV, db.SessionKindLecture); got != 4 {
		t.Fatalf("final lectures=%d want exactly 4 (locked only)", got)
	}
	wantIDs := map[db.ID]bool{}
	for _, id := range lectureTaskIDs(tasks) {
		wantIDs[id] = true
	}
	for _, id := range lectureTaskIDs(wtasks) {
		if !wantIDs[id] {
			t.Fatalf("locked lecture task %q missing after switch", id)
		}
	}
	for lid, tids := range lectureTargets(t, wtasks) {
		if lid == "" || len(tids) != 1 {
			t.Fatalf("lecture %q assigned %d times", lid, len(tids))
		}
	}
}

// TestV2SwitchCapsLecturesMixedLockedAndFresh: 2 locked + 4 fresh candidates
// must yield exactly 4 (locked preserved, first fresh in order kept).
func TestV2SwitchCapsLecturesMixedLockedAndFresh(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedCapExams(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "cap-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	lecIDs := lectureTaskIDs(tasks)
	if len(lecIDs) != 4 {
		t.Fatalf("r1 lectures=%d want 4", len(lecIDs))
	}
	base := now.Add(time.Hour)
	completeLectureTask(t, ctx, d, lecIDs[0], base)
	completeLectureTask(t, ctx, d, lecIDs[1], base.Add(time.Hour))
	_, wtasks, err := engine.SwitchFocusExam(ctx, d, "cap-b", "2026-09-21", base.Add(8*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := countKind(wtasks, db.DomainGOV, db.SessionKindLecture); got != 4 {
		t.Fatalf("final lectures=%d want exactly 4", got)
	}
	gotIDs := map[db.ID]bool{}
	for _, id := range lectureTaskIDs(wtasks) {
		gotIDs[id] = true
	}
	if !gotIDs[lecIDs[0]] || !gotIDs[lecIDs[1]] {
		t.Fatalf("locked lecture IDs must survive, got %v want %v + fresh", gotIDs, lecIDs[:2])
	}
	// Fresh survivors must be the first in-focus candidates (cap-l5, cap-l6).
	seen := map[string]bool{}
	for lid := range lectureTargets(t, wtasks) {
		seen[string(lid)] = true
	}
	for _, want := range []string{"cap-l5", "cap-l6"} {
		if !seen[want] {
			t.Fatalf("fresh lectures must be cap-l5/cap-l6, got %v", seen)
		}
	}
	if seen["cap-l7"] || seen["cap-l8"] {
		t.Fatalf("surplus fresh lectures must be dropped, got %v", seen)
	}
}

// TestV2SwitchDedupesDSABundle: an ACTIVE DSA bundle locks its problems;
// the fresh bundle must not reassign them.
func TestV2SwitchDedupesDSABundle(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSharedExams(t, ctx, d)
	seedDSA(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	var dsaID db.ID
	var dsaProblems []db.ID
	for _, tk := range tasks {
		if tk.Domain != db.DomainDSA || tk.TargetJSON == nil {
			continue
		}
		dt, err := db.UnmarshalDSATarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		dsaID = tk.ID
		for _, it := range dt.Items() {
			dsaProblems = append(dsaProblems, it.ProblemID)
		}
	}
	if dsaID == "" {
		t.Fatal("r1 must contain a DSA task")
	}
	if _, err := engine.StartTodayTask(ctx, d, dsaID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, wtasks, err := engine.SwitchFocusExam(ctx, d, "exam-b", "2026-09-21", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	locked := map[db.ID]bool{}
	for _, p := range dsaProblems {
		locked[p] = true
	}
	dsaCount := 0
	for _, tk := range wtasks {
		if tk.Domain != db.DomainDSA || tk.TargetJSON == nil {
			continue
		}
		dsaCount++
		if tk.ID != dsaID {
			dt, err := db.UnmarshalDSATarget(*tk.TargetJSON)
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range dt.Items() {
				if locked[it.ProblemID] {
					t.Fatalf("fresh DSA bundle reassigns locked problem %q", it.ProblemID)
				}
			}
		}
	}
	if dsaCount != 1 {
		t.Fatalf("DSA tasks=%d want exactly 1 (locked only)", dsaCount)
	}
}

// TestV2SwitchKeepsSWEWithinRoutine: one stopped SWE session locks its area;
// the revision must keep routine-count blocks with distinct areas.
func TestV2SwitchKeepsSWEWithinRoutine(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSharedExams(t, ctx, d)
	seedSWE(t, ctx, d) // Monday slots: area-go + area-sql.
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	// 2026-09-21 is a Monday.
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	var sweGo db.ID
	for _, tk := range tasks {
		if tk.Domain != db.DomainSWE || tk.TargetJSON == nil {
			continue
		}
		st, err := db.UnmarshalSWETarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if st.AreaID == "area-go" {
			sweGo = tk.ID
		}
	}
	if sweGo == "" {
		t.Fatal("r1 must contain the area-go SWE block")
	}
	sess, err := engine.StartTodayTask(ctx, d, sweGo, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_ = sess
	_, wtasks, err := engine.SwitchFocusExam(ctx, d, "exam-b", "2026-09-21", now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var areas []db.ID
	sweCount := 0
	for _, tk := range wtasks {
		if tk.Domain != db.DomainSWE || tk.TargetJSON == nil {
			continue
		}
		sweCount++
		st, err := db.UnmarshalSWETarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		areas = append(areas, st.AreaID)
	}
	if sweCount != 2 {
		t.Fatalf("SWE tasks=%d want routine count 2", sweCount)
	}
	sort.Slice(areas, func(i, j int) bool { return areas[i] < areas[j] })
	for i := 1; i < len(areas); i++ {
		if areas[i] == areas[i-1] {
			t.Fatalf("duplicate SWE area %q in revision", areas[i])
		}
	}
	foundLocked := false
	for _, tk := range wtasks {
		if tk.ID == sweGo {
			foundLocked = true
		}
	}
	if !foundLocked {
		t.Fatal("locked SWE task must keep its ID")
	}
}

// seedPracticeFixture builds 4 eligible zero-lecture topics per fixed subject
// under exam-a, with routine durations 1800/2400/3000.
func seedPracticeFixture(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	subs := []struct {
		id   db.ID
		name string
		n    int
	}{
		{"subj-quant", "Quant", 4},
		{"subj-english", "English", 4},
		{"subj-reasoning", "Reasoning", 4},
	}
	for i, s := range subs {
		if err := catalog.InsertGovSubject(ctx, d, db.GovSubject{ID: s.id, Name: s.name, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "exam-a", Name: "AFCAT", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for i, s := range subs {
		if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: "exam-a", SubjectID: s.id, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
		for k := 1; k <= s.n; k++ {
			tid := db.ID(fmt.Sprintf("%s-t%d", string(s.id)[5:], k))
			if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: tid, SubjectID: s.id, Name: fmt.Sprintf("T%d", k), SortOrder: k}); err != nil {
				t.Fatal(err)
			}
			if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-a", TopicID: tid, SortOrder: (i*4 + k)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := engine.EnsureGovSubjectActive(ctx, d, s.id, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	durs := []int{1800, 2400, 3000}
	for i, s := range subs {
		fixed := s.id
		if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: i + 1, DurationSeconds: durs[i], SelectionMode: db.GovSelectionFixed, FixedSubjectID: &fixed}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestV2PracticeAllThreeSubjects: with eligible candidates everywhere, all
// three fixed subjects generate, each with ≤3 distinct topics (≤9 total),
// honoring configured routine durations.
func TestV2PracticeAllThreeSubjects(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedPracticeFixture(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	type bundle struct {
		planned int
		topics  []db.ID
	}
	got := map[db.ID]bundle{}
	total := 0
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindPractice || tk.TargetJSON == nil {
			continue
		}
		pt, err := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if len(pt.TopicIDs) == 0 || len(pt.TopicIDs) > 3 {
			t.Fatalf("subject %q bundle size=%d, want 1..3", pt.SubjectID, len(pt.TopicIDs))
		}
		seen := map[db.ID]bool{}
		for _, tid := range pt.TopicIDs {
			if seen[tid] {
				t.Fatalf("duplicate topic %q in bundle", tid)
			}
			seen[tid] = true
		}
		got[pt.SubjectID] = bundle{planned: tk.PlannedSeconds, topics: pt.TopicIDs}
		total += len(pt.TopicIDs)
	}
	for _, sid := range []db.ID{"subj-quant", "subj-english", "subj-reasoning"} {
		b, ok := got[sid]
		if !ok {
			t.Fatalf("missing practice task for %q (got %v)", sid, got)
		}
		_ = b
	}
	if total > 9 {
		t.Fatalf("practice targets=%d want ≤9", total)
	}
	wantDur := map[db.ID]int{"subj-quant": 1800, "subj-english": 2400, "subj-reasoning": 3000}
	for sid, want := range wantDur {
		if got[sid].planned != want {
			t.Fatalf("subject %q planned=%d want %d (routine)", sid, got[sid].planned, want)
		}
	}
	// Every topic belongs to the focused exam's syllabus for its subject.
	for sid, b := range got {
		syll, err := catalog.GetGovExamSyllabusForSubject(ctx, d, "exam-a", sid)
		if err != nil {
			t.Fatal(err)
		}
		inSyll := map[db.ID]bool{}
		for _, s := range syll {
			inSyll[s.TopicID] = true
		}
		for _, tid := range b.topics {
			if !inSyll[tid] {
				t.Fatalf("topic %q not in exam-a syllabus for %q", tid, sid)
			}
		}
	}
}

// TestV2PracticeEmptyWhenGenuinelyIneligible: subjects whose topics all have
// incomplete lectures yield no practice tasks (and no off-syllabus fill).
func TestV2PracticeEmptyWhenGenuinelyIneligible(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-english", Name: "English", SortOrder: 2},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "tq", SubjectID: "subj-quant", Name: "Q", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lq", TopicID: "tq", Title: "QC", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "te", SubjectID: "subj-english", Name: "E", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "le", TopicID: "te", Title: "EC", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "exam-a", Name: "A", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "exam-a", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-a", SubjectID: "subj-english", SortOrder: 2},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-a", TopicID: "tq", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-a", TopicID: "te", SortOrder: 2}); err != nil {
		t.Fatal(err)
	}
	q := db.ID("subj-quant")
	e := db.ID("subj-english")
	for _, slot := range []db.GovPracticeRoutineSlot{
		{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &q},
		{SlotOrder: 2, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &e},
	} {
		if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, slot); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			t.Fatalf("no eligible topics, yet practice planned: %q", tk.Title)
		}
	}
	// Lectures still plan (no crash, no invented practice).
	if got := countKind(tasks, db.DomainGOV, db.SessionKindLecture); got == 0 {
		t.Fatal("lectures should still plan for incomplete topics")
	}
}

// TestV2PracticeDefaultsWhenRoutineMissing: an empty routine table still
// yields fixed-subject practice at 1h defaults once subjects are eligible.
func TestV2PracticeDefaultsWhenRoutineMissing(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	if err := catalog.InsertGovSubject(ctx, d, db.GovSubject{ID: "subj-quant", Name: "Quant", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "exam-a", Name: "A", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: "exam-a", SubjectID: "subj-quant", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for k := 1; k <= 3; k++ {
		tid := db.ID(fmt.Sprintf("qt%d", k))
		if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: tid, SubjectID: "subj-quant", Name: fmt.Sprintf("Q%d", k), SortOrder: k}); err != nil {
			t.Fatal(err)
		}
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-a", TopicID: tid, SortOrder: k}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-quant", now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindPractice || tk.TargetJSON == nil {
			continue
		}
		pt, err := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if pt.SubjectID != "subj-quant" {
			continue
		}
		found = true
		if tk.PlannedSeconds != 3600 {
			t.Fatalf("default planned=%d want 3600", tk.PlannedSeconds)
		}
		if len(pt.TopicIDs) != 3 {
			t.Fatalf("topics=%v want all 3 eligible", pt.TopicIDs)
		}
	}
	if !found {
		t.Fatal("practice must plan from defaults when routine is missing")
	}
}

// TestV2EnsureTodayHealsPrematureEmpty: opening TODAY before seeding must
// not poison the day. After configuration arrives, the next open creates a
// real revision (preserving the empty one); further reopens are stable and
// rotation-neutral.
func TestV2EnsureTodayHealsPrematureEmpty(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	now := time.Now()
	snap1, tasks1, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks1) != 0 {
		t.Fatalf("empty catalogue must yield no tasks, got %d", len(tasks1))
	}
	if snap1.Revision != 1 {
		t.Fatalf("first revision=%d want 1", snap1.Revision)
	}
	// Catalogue arrives after first open (fixture-style seeding).
	seedSharedExams(t, ctx, d)
	seedDSA(t, ctx, d)
	snap2, tasks2, err := engine.EnsureToday(ctx, d, "2026-09-21", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks2) == 0 {
		t.Fatal("healed revision must contain tasks once configured")
	}
	if snap2.Revision != 2 || snap2.ID == snap1.ID {
		t.Fatalf("heal must create revision 2, got %+v after %+v", snap2, snap1)
	}
	// The premature empty snapshot is preserved, not rewritten.
	oldTasks, err := runtime.GetTodayTasks(ctx, d, snap1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldTasks) != 0 {
		t.Fatalf("premature snapshot must stay empty, got %d tasks", len(oldTasks))
	}
	// Reopening is stable: same revision, same task IDs, same cursors.
	rotBefore := rotationSnapshot(t, ctx, d)
	snap3, tasks3, err := engine.EnsureToday(ctx, d, "2026-09-21", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap3.ID != snap2.ID {
		t.Fatal("reopen must not create another revision")
	}
	if len(tasks3) != len(tasks2) {
		t.Fatalf("task count changed on reopen: %d vs %d", len(tasks3), len(tasks2))
	}
	for i := range tasks2 {
		if tasks3[i].ID != tasks2[i].ID {
			t.Fatal("task IDs must be stable across reopen")
		}
	}
	if got := rotationSnapshot(t, ctx, d); got != rotBefore {
		t.Fatalf("reopen must not advance rotation: %q vs %q", got, rotBefore)
	}
}

func rotationSnapshot(t *testing.T, ctx context.Context, d *db.DB) string {
	t.Helper()
	rows, err := d.QueryContext(ctx, `SELECT exam_id, subject_id, next_index FROM gov_practice_rotation ORDER BY exam_id, subject_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var e, s string
		var n int
		if err := rows.Scan(&e, &s, &n); err != nil {
			t.Fatal(err)
		}
		out += fmt.Sprintf("%s/%s=%d;", e, s, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestV2RestartPreservesSnapshot: a valid plan survives reopening with
// identical revisions, task IDs, and rotation cursors.
func TestV2RestartPreservesSnapshot(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSharedExams(t, ctx, d)
	seedDSA(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	snap1, tasks1, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	rotBefore := rotationSnapshot(t, ctx, d)
	snap2, tasks2, err := engine.EnsureToday(ctx, d, "2026-09-21", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap2.ID != snap1.ID || snap2.Revision != snap1.Revision {
		t.Fatalf("restart must load latest revision, got %+v want %+v", snap2, snap1)
	}
	if len(tasks2) != len(tasks1) {
		t.Fatalf("task count %d vs %d", len(tasks2), len(tasks1))
	}
	for i := range tasks1 {
		if tasks2[i].ID != tasks1[i].ID {
			t.Fatal("task IDs must survive restart")
		}
	}
	if got := rotationSnapshot(t, ctx, d); got != rotBefore {
		t.Fatalf("restart must not advance rotation: %q vs %q", got, rotBefore)
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_snapshots WHERE local_date='2026-09-21'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("snapshots for date=%d want 1", n)
	}
}

// TestV2SwitchFailureRollsBack: a failed focus switch leaves focus, plan,
// and rotation cursors exactly as before.
func TestV2SwitchFailureRollsBack(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSharedExams(t, ctx, d)
	seedDSA(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	snap1, tasks1, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	// exam-b shares the syllabus so the fresh plan attempts task inserts.
	rotBefore := rotationSnapshot(t, ctx, d)
	if _, err := d.ExecContext(ctx, `CREATE TRIGGER sab_switch BEFORE INSERT ON today_tasks BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.SwitchFocusExam(ctx, d, "exam-b", "2026-09-21", now.Add(time.Hour)); err == nil {
		t.Fatal("sabotaged switch must fail")
	}
	focus, err := engine.GetFocusExam(ctx, d)
	if err != nil || focus == nil || focus.ID != "exam-a" {
		t.Fatalf("focus must stay exam-a, got %+v %v", focus, err)
	}
	latest, err := runtime.GetLatestTodaySnapshot(ctx, d, "2026-09-21")
	if err != nil || latest.ID != snap1.ID {
		t.Fatalf("latest snapshot must stay r1, got %+v %v", latest, err)
	}
	wtasks, err := runtime.GetTodayTasks(ctx, d, snap1.ID)
	if err != nil || len(wtasks) != len(tasks1) {
		t.Fatalf("r1 tasks must be intact: %d vs %d (%v)", len(wtasks), len(tasks1), err)
	}
	if got := rotationSnapshot(t, ctx, d); got != rotBefore {
		t.Fatalf("rotation must roll back: %q vs %q", got, rotBefore)
	}
}

// TestV2NoPlanSwitchFailureRestoresFocus: switching focus before any plan
// exists for the date commits focus, snapshot/tasks, membership, activations,
// and rotations in ONE transaction. Aborting task insertion must leave all
// six areas unchanged (no compensating restore; crash-atomic).
func TestV2NoPlanSwitchFailureRestoresFocus(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedNoPlanAtomicWorld(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	focusBefore, err := engine.GetFocusExam(ctx, d)
	if err != nil || focusBefore == nil || focusBefore.ID != "exam-a" {
		t.Fatalf("setup focus must be exam-a, got %+v %v", focusBefore, err)
	}
	rotBefore := rotationSnapshot(t, ctx, d)
	actBefore := activationSnapshot(t, ctx, d)
	var tasksBefore, membersBefore int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_tasks`).Scan(&tasksBefore); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_snapshot_tasks`).Scan(&membersBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `CREATE TRIGGER sab_noplan BEFORE INSERT ON today_tasks BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.SwitchFocusExam(ctx, d, "exam-b", "2026-09-21", now.Add(time.Hour)); err == nil {
		t.Fatal("sabotaged no-plan switch must fail")
	}
	focus, err := engine.GetFocusExam(ctx, d)
	if err != nil || focus == nil || focus.ID != "exam-a" {
		t.Fatalf("focus must stay exam-a, got %+v %v", focus, err)
	}
	if _, err := runtime.GetLatestTodaySnapshot(ctx, d, "2026-09-21"); err == nil {
		t.Fatal("failed no-plan switch must create no snapshot")
	}
	var tasksAfter, membersAfter int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_tasks`).Scan(&tasksAfter); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_snapshot_tasks`).Scan(&membersAfter); err != nil {
		t.Fatal(err)
	}
	if tasksAfter != tasksBefore {
		t.Fatalf("today_tasks changed: before=%d after=%d", tasksBefore, tasksAfter)
	}
	if membersAfter != membersBefore {
		t.Fatalf("snapshot membership changed: before=%d after=%d", membersBefore, membersAfter)
	}
	if got := rotationSnapshot(t, ctx, d); got != rotBefore {
		t.Fatalf("rotation must roll back: %q vs %q", got, rotBefore)
	}
	if got := activationSnapshot(t, ctx, d); got != actBefore {
		t.Fatalf("activations must roll back: %q vs %q", got, actBefore)
	}
}

// TestV2NoPlanSwitchSuccess commits focus, snapshot, membership, activations,
// and rotations together and preserves planning invariants.
func TestV2NoPlanSwitchSuccess(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedNoPlanAtomicWorld(t, ctx, d)
	now := time.Now()
	if _, err := engine.SetFocusExam(ctx, d, "exam-a", now); err != nil {
		t.Fatal(err)
	}
	snap, tasks, err := engine.SwitchFocusExam(ctx, d, "exam-b", "2026-09-21", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Revision != 1 || snap.LocalDate != "2026-09-21" {
		t.Fatalf("no-plan switch must create r1 for date, got %+v", snap)
	}
	if len(tasks) == 0 {
		t.Fatal("no-plan switch must create tasks")
	}
	focus, err := engine.GetFocusExam(ctx, d)
	if err != nil || focus == nil || focus.ID != "exam-b" {
		t.Fatalf("focus must be exam-b, got %+v %v", focus, err)
	}
	latest, err := runtime.GetLatestTodaySnapshot(ctx, d, "2026-09-21")
	if err != nil || latest.ID != snap.ID {
		t.Fatalf("latest snapshot must be switched revision, got %+v %v", latest, err)
	}
	// All tasks belong to the committed revision via membership.
	var memberCount int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_snapshot_tasks WHERE snapshot_id = ?`, string(snap.ID)).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if memberCount != len(tasks) {
		t.Fatalf("membership rows=%d want %d", memberCount, len(tasks))
	}
	// Lectures come from the newly focused exam only, at most four, one per subject.
	lecTotal := 0
	seenSubj := map[db.ID]bool{}
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
			continue
		}
		lecTotal++
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if seenSubj[lt.SubjectID] {
			t.Fatalf("duplicate lecture subject %q", lt.SubjectID)
		}
		seenSubj[lt.SubjectID] = true
		syll, err := catalog.GetGovExamSyllabus(ctx, d, "exam-b")
		if err != nil {
			t.Fatal(err)
		}
		inSyll := false
		for _, s := range syll {
			if s.TopicID == lt.TopicID {
				inSyll = true
				break
			}
		}
		if !inSyll {
			t.Fatalf("lecture topic %q not in exam-b syllabus", lt.TopicID)
		}
	}
	if lecTotal == 0 || lecTotal > 4 {
		t.Fatalf("lecture tasks=%d want 1..4", lecTotal)
	}
	// Activation and rotation effects of the committed plan are persisted.
	if active, err := runtime.IsGovSubjectActive(ctx, d, "subj-quant"); err != nil || !active {
		t.Fatalf("quant must be activated by committed switch: %v %v", active, err)
	}
	rotAfter := rotationSnapshot(t, ctx, d)
	if rotAfter == "" {
		t.Fatal("practice rotation must be persisted by committed switch")
	}
	// Restart/reopen is stable.
	snap2, tasks2, err := engine.EnsureToday(ctx, d, "2026-09-21", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap2.ID != snap.ID || len(tasks2) != len(tasks) {
		t.Fatal("reopen must return the committed revision unchanged")
	}
}

// seedNoPlanAtomicWorld builds a world where a successful no-plan switch
// writes every commit area: quant starts inactive with an incomplete lecture
// (activation write), reasoning starts active with a zero-lecture topic and
// fixed practice subjects (rotation write). Both exams share the syllabus
// shape so the switch actually replans.
func seedNoPlanAtomicWorld(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-english", Name: "English", SortOrder: 2},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 3},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "topic-q1", SubjectID: "subj-quant", Name: "Q1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lec-q1", TopicID: "topic-q1", Title: "Q Core", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "topic-e1", SubjectID: "subj-english", Name: "E1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lec-e1", TopicID: "topic-e1", Title: "E Core", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	// Zero-lecture reasoning topic: immediately practice-eligible.
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "topic-r0", SubjectID: "subj-reasoning", Name: "R0", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []db.GovExam{
		{ID: "exam-a", Name: "AFCAT", SortOrder: 1},
		{ID: "exam-b", Name: "SSC", SortOrder: 2},
	} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "exam-a", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-a", SubjectID: "subj-english", SortOrder: 2},
		{ExamID: "exam-a", SubjectID: "subj-reasoning", SortOrder: 3},
		{ExamID: "exam-b", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-b", SubjectID: "subj-english", SortOrder: 2},
		{ExamID: "exam-b", SubjectID: "subj-reasoning", SortOrder: 3},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	for i, tid := range []db.ID{"topic-q1", "topic-e1", "topic-r0"} {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-a", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-b", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	// Reasoning starts active so practice can generate; quant/english start
	// inactive so the committed switch must activate them.
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-reasoning", time.Now()); err != nil {
		t.Fatal(err)
	}
}

func activationSnapshot(t *testing.T, ctx context.Context, d *db.DB) string {
	t.Helper()
	rows, err := d.QueryContext(ctx, `SELECT subject_id FROM gov_subject_activation ORDER BY subject_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out += s + ";"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
