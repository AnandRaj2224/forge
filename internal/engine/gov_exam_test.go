package engine_test

// Exam-wise GOV progression: the current exam is derived as the first
// incomplete exam in sort_order, its own subject order drives the four
// lecture slots and gates practice selection, completion is global, and a
// later exam's new topics never resurrect an earlier exam's subjects.

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

// seedExamWise builds a banking-style world with shared global subjects and
// per-exam subject membership:
//
//	SBI Clerk (1):   Quant, Reasoning, English, Banking/FA
//	IBPS Clerk (2):  Quant, Reasoning, English, Banking/FA
//	IBPS SO IT (3):  Quant, Reasoning, English, IT
//
// Quant has Number System + Percentage (one lecture each), Reasoning has
// Series, English has Grammar, Banking/FA has Banking Awareness, IT has IT
// Fundamentals. Every exam maps the shared topics of its own subjects.
func seedExamWise(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2},
		{ID: "subj-english", Name: "English", SortOrder: 3},
		{ID: "subj-banking", Name: "Banking/Financial Awareness", SortOrder: 4},
		{ID: "subj-it", Name: "IT", SortOrder: 5},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, tp := range []db.GovTopic{
		{ID: "topic-ns", SubjectID: "subj-quant", Name: "Number System", SortOrder: 1},
		{ID: "topic-perc", SubjectID: "subj-quant", Name: "Percentage", SortOrder: 2},
		{ID: "topic-series", SubjectID: "subj-reasoning", Name: "Series", SortOrder: 1},
		{ID: "topic-coding", SubjectID: "subj-reasoning", Name: "Coding-Decoding", SortOrder: 2},
		{ID: "topic-grammar", SubjectID: "subj-english", Name: "Grammar", SortOrder: 1},
		{ID: "topic-ba", SubjectID: "subj-banking", Name: "Banking Awareness", SortOrder: 1},
		{ID: "topic-it", SubjectID: "subj-it", Name: "IT Fundamentals", SortOrder: 1},
	} {
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
	}
	for i, l := range []db.GovLecture{
		{ID: "lec-ns", TopicID: "topic-ns", Title: "NS Core", SortOrder: 1},
		{ID: "lec-perc", TopicID: "topic-perc", Title: "Perc Core", SortOrder: 1},
		{ID: "lec-series", TopicID: "topic-series", Title: "Series Core", SortOrder: 1},
		{ID: "lec-coding", TopicID: "topic-coding", Title: "Coding Core", SortOrder: 1},
		{ID: "lec-grammar", TopicID: "topic-grammar", Title: "Grammar Core", SortOrder: 1},
		{ID: "lec-ba", TopicID: "topic-ba", Title: "BA Core", SortOrder: 1},
		{ID: "lec-it", TopicID: "topic-it", Title: "IT Core", SortOrder: 1},
	} {
		_ = i
		if err := catalog.InsertGovLecture(ctx, d, l); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []db.GovExam{
		{ID: "exam-sbi-clerk", Name: "SBI Clerk", SortOrder: 1},
		{ID: "exam-ibps-clerk", Name: "IBPS Clerk", SortOrder: 2},
		{ID: "exam-so-it", Name: "IBPS SO IT", SortOrder: 3},
	} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "exam-sbi-clerk", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-sbi-clerk", SubjectID: "subj-reasoning", SortOrder: 2},
		{ExamID: "exam-sbi-clerk", SubjectID: "subj-english", SortOrder: 3},
		{ExamID: "exam-sbi-clerk", SubjectID: "subj-banking", SortOrder: 4},
		{ExamID: "exam-ibps-clerk", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-ibps-clerk", SubjectID: "subj-reasoning", SortOrder: 2},
		{ExamID: "exam-ibps-clerk", SubjectID: "subj-english", SortOrder: 3},
		{ExamID: "exam-ibps-clerk", SubjectID: "subj-banking", SortOrder: 4},
		{ExamID: "exam-so-it", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-so-it", SubjectID: "subj-reasoning", SortOrder: 2},
		{ExamID: "exam-so-it", SubjectID: "subj-english", SortOrder: 3},
		{ExamID: "exam-so-it", SubjectID: "subj-it", SortOrder: 4},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	sbiTopics := []db.ID{"topic-ns", "topic-perc", "topic-series", "topic-grammar", "topic-ba"}
	for i, tid := range sbiTopics {
		for _, exam := range []db.ID{"exam-sbi-clerk", "exam-ibps-clerk"} {
			if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: exam, TopicID: tid, SortOrder: i + 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// IBPS Clerk additionally maps Coding-Decoding: shared topics stay
	// complete across exams, but it keeps its own remaining work.
	if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-ibps-clerk", TopicID: "topic-coding", SortOrder: 6}); err != nil {
		t.Fatal(err)
	}
	soTopics := []db.ID{"topic-ns", "topic-perc", "topic-series", "topic-grammar", "topic-it"}
	for i, tid := range soTopics {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-so-it", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		if err := catalog.InsertGovLectureRoutineSlot(ctx, d, db.GovLectureRoutineSlot{SlotOrder: i, DurationSeconds: 3600}); err != nil {
			t.Fatal(err)
		}
	}
	q := db.ID("subj-quant")
	r := db.ID("subj-reasoning")
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &q}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 2, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &r}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 3, DurationSeconds: 3600, SelectionMode: db.GovSelectionRoundRobin}); err != nil {
		t.Fatal(err)
	}
}

var examWiseSeq int

// completeExamLecture force-completes one global lecture through FK-safe
// runtime rows (snapshot + task + stopped session + completion row).
func completeExamLecture(t *testing.T, ctx context.Context, d *db.DB, lectureID db.ID, now time.Time) {
	t.Helper()
	examWiseSeq++
	snap := db.ID(fmt.Sprintf("xw-snap-%d", examWiseSeq))
	task := db.ID(fmt.Sprintf("xw-task-%d", examWiseSeq))
	sess := db.ID(fmt.Sprintf("xw-sess-%d", examWiseSeq))
	if err := runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snap, LocalDate: "2026-09-20", Revision: examWiseSeq, GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: task, SnapshotID: snap, Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "X", PlannedSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	stopped := now.Add(time.Hour)
	if err := runtime.CreateSession(ctx, d, db.Session{ID: sess, TodayTaskID: task, Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "X", PlannedSeconds: 3600, StartedAt: now, StoppedAt: &stopped, Status: db.SessionStatusStopped}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.RecordLectureCompletion(ctx, d, db.GovLectureCompletion{LectureID: lectureID, SessionID: sess, CompletedAt: now}); err != nil {
		t.Fatal(err)
	}
}

func currentExamName(t *testing.T, ctx context.Context, d *db.DB) string {
	t.Helper()
	cur, err := engine.CurrentGovExam(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if cur == nil {
		return ""
	}
	return cur.Name
}

func currentSubjectNames(t *testing.T, ctx context.Context, d *db.DB) []string {
	t.Helper()
	_, subs, err := engine.CurrentGovSubjects(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range subs {
		out = append(out, s.Subject.Name)
	}
	return out
}

func eqNames(a, b []string) bool {
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

// 1. Current exam is the first incomplete exam by exam sort_order.
func TestExamWiseCurrentIsFirstIncomplete(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	if got := currentExamName(t, ctx, d); got != "SBI Clerk" {
		t.Fatalf("current=%q want SBI Clerk", got)
	}
}

// 2. A completed exam automatically advances to the next exam.
func TestExamWiseCompletionAdvances(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	now := time.Now()
	for _, l := range []db.ID{"lec-ns", "lec-perc", "lec-series", "lec-grammar", "lec-ba"} {
		completeExamLecture(t, ctx, d, l, now)
	}
	done, err := engine.IsGovExamComplete(ctx, d, "exam-sbi-clerk")
	if err != nil || !done {
		t.Fatalf("SBI Clerk must be complete: %v %v", done, err)
	}
	done2, err := engine.IsGovExamComplete(ctx, d, "exam-ibps-clerk")
	if err != nil || done2 {
		t.Fatalf("IBPS Clerk must still be incomplete (Coding-Decoding): %v %v", done2, err)
	}
	if got := currentExamName(t, ctx, d); got != "IBPS Clerk" {
		t.Fatalf("current=%q want IBPS Clerk", got)
	}
}

// 3+4. Exam subject order determines the four slots; completing a subject
// replaces it with the next incomplete subject in exam order.
func TestExamWiseThreeSubjectsRotate(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	now := time.Now()
	if got := currentSubjectNames(t, ctx, d); !eqNames(got, []string{"Quant", "Reasoning", "English", "Banking/Financial Awareness"}) {
		t.Fatalf("slots=%v want [Quant Reasoning English Banking/Financial Awareness]", got)
	}
	// Complete Reasoning (Series): English slides up, Banking enters.
	completeExamLecture(t, ctx, d, "lec-series", now)
	if got := currentSubjectNames(t, ctx, d); !eqNames(got, []string{"Quant", "English", "Banking/Financial Awareness"}) {
		t.Fatalf("slots=%v want [Quant English Banking/Financial Awareness]", got)
	}
	// Complete Quant (both lectures): English leads.
	completeExamLecture(t, ctx, d, "lec-ns", now)
	completeExamLecture(t, ctx, d, "lec-perc", now)
	if got := currentSubjectNames(t, ctx, d); !eqNames(got, []string{"English", "Banking/Financial Awareness"}) {
		t.Fatalf("slots=%v want [English Banking/Financial Awareness]", got)
	}
}

// 5. Shared global topics remain completed when entering a later exam.
func TestExamWiseSharedTopicsStayComplete(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	now := time.Now()
	for _, l := range []db.ID{"lec-ns", "lec-perc", "lec-series", "lec-grammar", "lec-ba"} {
		completeExamLecture(t, ctx, d, l, now)
	}
	// IBPS Clerk is current with every shared topic already complete and
	// only its own Coding-Decoding remaining.
	if got := currentExamName(t, ctx, d); got != "IBPS Clerk" {
		t.Fatalf("current=%q want IBPS Clerk", got)
	}
	for _, tid := range []db.ID{"topic-ns", "topic-perc", "topic-series", "topic-grammar", "topic-ba"} {
		ok, err := engine.IsGovTopicLectureComplete(ctx, d, tid)
		if err != nil || !ok {
			t.Fatalf("shared topic %q must stay complete: %v %v", tid, ok, err)
		}
	}
	if got := currentSubjectNames(t, ctx, d); !eqNames(got, []string{"Reasoning"}) {
		t.Fatalf("slots=%v want [Reasoning] (only Coding-Decoding left)", got)
	}
	completeExamLecture(t, ctx, d, "lec-coding", now)
	done, err := engine.IsGovExamComplete(ctx, d, "exam-ibps-clerk")
	if err != nil || !done {
		t.Fatalf("IBPS Clerk must be complete via shared topics: %v %v", done, err)
	}
	if got := currentExamName(t, ctx, d); got != "IBPS SO IT" {
		t.Fatalf("current=%q want IBPS SO IT", got)
	}
	// SO IT still needs IT Fundamentals but skips the shared Quant work.
	if got := currentSubjectNames(t, ctx, d); !eqNames(got, []string{"IT"}) {
		t.Fatalf("slots=%v want [IT]", got)
	}
}

// 6+12. A later exam with genuinely new topics does not resurrect the
// current exam's subjects; a new exam added later becomes the next track.
func TestExamWiseFutureExamIsolated(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	now := time.Now()
	// Finish SBI Clerk so IBPS Clerk is current.
	for _, l := range []db.ID{"lec-ns", "lec-perc", "lec-series", "lec-grammar", "lec-ba", "lec-coding"} {
		completeExamLecture(t, ctx, d, l, now)
	}
	// Later: a future exam arrives with a brand-new Quant topic mapped
	// ONLY to it.
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "topic-algebra", SubjectID: "subj-quant", Name: "Algebra", SortOrder: 3}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lec-algebra", TopicID: "topic-algebra", Title: "Core", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "exam-afcat", Name: "AFCAT", SortOrder: 9}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: "exam-afcat", SubjectID: "subj-quant", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-afcat", TopicID: "topic-algebra", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	// IBPS Clerk is still current (complete via shared topics), then SO IT;
	// Algebra must not pull Quant back into either track.
	if got := currentExamName(t, ctx, d); got != "IBPS SO IT" {
		t.Fatalf("current=%q want IBPS SO IT", got)
	}
	if got := currentSubjectNames(t, ctx, d); !eqNames(got, []string{"IT"}) {
		t.Fatalf("slots=%v want [IT]", got)
	}
	// Today serves only SO IT work, never the future Algebra lecture.
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
			continue
		}
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if lt.LectureID == "lec-algebra" {
			t.Fatal("future exam lecture must not be served early")
		}
	}
	// Finish SO IT: the new exam becomes the next incomplete track.
	completeExamLecture(t, ctx, d, "lec-it", now)
	if got := currentExamName(t, ctx, d); got != "AFCAT" {
		t.Fatalf("current=%q want AFCAT", got)
	}
	if got := currentSubjectNames(t, ctx, d); !eqNames(got, []string{"Quant"}) {
		t.Fatalf("slots=%v want [Quant]", got)
	}
}

// 7. Adding a subject to one exam does not affect other exams.
func TestExamWiseSubjectAddIsolated(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	// Add Computer Awareness to SBI PO only... here: add IT to IBPS Clerk
	// (SBI Clerk must be untouched and no global duplication occurs).
	if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: "exam-ibps-clerk", SubjectID: "subj-it", SortOrder: 5}); err != nil {
		t.Fatal(err)
	}
	prog, err := engine.ExamSubjectProgress(ctx, d, "exam-sbi-clerk")
	if err != nil {
		t.Fatal(err)
	}
	if len(prog) != 4 {
		t.Fatalf("SBI Clerk subjects=%d want 4", len(prog))
	}
	for _, p := range prog {
		if p.Subject.ID == "subj-it" {
			t.Fatal("SBI Clerk must not gain IT")
		}
	}
	prog2, err := engine.ExamSubjectProgress(ctx, d, "exam-ibps-clerk")
	if err != nil {
		t.Fatal(err)
	}
	if len(prog2) != 5 || prog2[4].Subject.ID != "subj-it" {
		t.Fatalf("IBPS Clerk must gain IT last: %+v", prog2)
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subjects WHERE id = 'subj-it'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("IT must exist exactly once globally: n=%d err=%v", n, err)
	}
}

// 8+9+10. Practice respects the current exam's subject set: SO IT can
// include IT and excludes Banking/FA; common exams do the reverse.
func TestExamWisePracticeRespectsMembership(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	now := time.Now()
	// Finish both clerk exams so IBPS SO IT is current.
	for _, l := range []db.ID{"lec-ns", "lec-perc", "lec-series", "lec-grammar", "lec-ba", "lec-coding"} {
		completeExamLecture(t, ctx, d, l, now)
	}
	if got := currentExamName(t, ctx, d); got != "IBPS SO IT" {
		t.Fatalf("current=%q want IBPS SO IT", got)
	}
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindPractice || tk.TargetJSON == nil {
			continue
		}
		pt, err := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		subj, err := catalog.GetGovSubject(ctx, d, pt.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if subj.Name == "Banking/Financial Awareness" {
			t.Fatal("Banking/FA must not be practiced under SO IT")
		}
	}
	// Common-exam mirror: fresh world, SBI Clerk current, IT never served.
	d2 := openEngineDB(t)
	seedExamWise(t, ctx, d2)
	seedDSA(t, ctx, d2)
	seedSWE(t, ctx, d2)
	_, tasks2, err := engine.GenerateToday(ctx, d2, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks2 {
		if tk.Domain != db.DomainGOV || tk.TargetJSON == nil {
			continue
		}
		if tk.Kind == db.SessionKindLecture {
			lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
			if err != nil {
				t.Fatal(err)
			}
			if lt.SubjectID == "subj-it" {
				t.Fatal("IT lecture must not be served under SBI Clerk")
			}
		} else {
			pt, err := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if err != nil {
				t.Fatal(err)
			}
			if pt.SubjectID == "subj-it" {
				t.Fatal("IT practice must not be served under SBI Clerk")
			}
		}
	}
}

// 10 (fixed slots). When the current exam excludes Reasoning, the fixed
// Reasoning practice slot is skipped instead of serving foreign work.
func TestExamWiseFixedSlotSkipsNonMembers(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	now := time.Now()
	// Finish the clerk exams; SO IT (no Banking) is current and only IT
	// remains incomplete — Reasoning's fixed slot must not fire.
	for _, l := range []db.ID{"lec-ns", "lec-perc", "lec-series", "lec-grammar", "lec-ba", "lec-coding"} {
		completeExamLecture(t, ctx, d, l, now)
	}
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindPractice || tk.TargetJSON == nil {
			continue
		}
		pt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
		if pt.SubjectID == "subj-reasoning" {
			t.Fatal("Reasoning practice must be skipped when it has no eligible topic")
		}
	}
}

// 11. No current exam when all exams are complete; GOV planning produces
// no exam work rather than inventing it.
func TestExamWiseAllCompleteNoWork(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	now := time.Now()
	for _, l := range []db.ID{"lec-ns", "lec-perc", "lec-series", "lec-grammar", "lec-ba", "lec-coding", "lec-it"} {
		completeExamLecture(t, ctx, d, l, now)
	}
	cur, err := engine.CurrentGovExam(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if cur != nil {
		t.Fatalf("current=%v want nil", cur)
	}
	cur2, subs, err := engine.CurrentGovSubjects(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if cur2 != nil || len(subs) != 0 {
		t.Fatalf("subjects=%v want none", subs)
	}
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV {
			t.Fatalf("no GOV work allowed when all exams complete, got %q", tk.Title)
		}
	}
	if len(tasks) == 0 {
		t.Fatal("DSA/SWE blocks must still generate")
	}
}

// Today serves the current exam's incomplete topics through the same
// global records: lecture slots hit SBI Clerk work first, then advance.
func TestExamWiseTodayServesCurrentExam(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedExamWise(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	var lecSubjects []string
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
			continue
		}
		lt, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		subj, err := catalog.GetGovSubject(ctx, d, lt.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		lecSubjects = append(lecSubjects, subj.Name)
	}
	if !eqNames(lecSubjects, []string{"Quant", "Reasoning", "English", "Banking/Financial Awareness"}) {
		t.Fatalf("lecture subjects=%v want [Quant Reasoning English Banking/Financial Awareness]", lecSubjects)
	}
	// No duplicate historical topics: exactly one Number System topic row.
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_topics WHERE name = 'Number System'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("Number System rows=%d want 1", n)
	}
	// EnsureToday stays idempotent on the exam-wise plan.
	snap, _, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	snap2, _, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID != snap2.ID {
		t.Fatal("EnsureToday must not duplicate the exam-wise snapshot")
	}
}
