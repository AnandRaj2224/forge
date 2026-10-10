package engine_test

// Fourth GOV lecture slot + future globally-unique fallback regressions.
// All four slots are normal 1h lecture slots with identical semantics.

import (
	"context"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

func seedFallbackBase(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "fb-quant", Name: "Quant", SortOrder: 1},
		{ID: "fb-reasoning", Name: "Reasoning", SortOrder: 2},
		{ID: "fb-english", Name: "English", SortOrder: 3},
		{ID: "fb-history", Name: "History", SortOrder: 4},
		{ID: "fb-computer", Name: "Computer", SortOrder: 5},
		{ID: "fb-bank", Name: "Banking Awareness", SortOrder: 6},
		{ID: "fb-pk", Name: "Professional Knowledge", SortOrder: 7},
		{ID: "fb-extra", Name: "Extra Unique", SortOrder: 8},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	topics := []db.GovTopic{
		{ID: "fb-t-quant", SubjectID: "fb-quant", Name: "Q Topic", SortOrder: 1},
		{ID: "fb-t-reasoning", SubjectID: "fb-reasoning", Name: "R Topic", SortOrder: 1},
		{ID: "fb-t-english", SubjectID: "fb-english", Name: "E Topic", SortOrder: 1},
		{ID: "fb-t-history", SubjectID: "fb-history", Name: "H Topic", SortOrder: 1},
		{ID: "fb-t-computer", SubjectID: "fb-computer", Name: "C Topic", SortOrder: 1},
		{ID: "fb-t-bank", SubjectID: "fb-bank", Name: "B Topic", SortOrder: 1},
		{ID: "fb-t-pk", SubjectID: "fb-pk", Name: "P Topic", SortOrder: 1},
		{ID: "fb-t-extra", SubjectID: "fb-extra", Name: "X Topic", SortOrder: 1},
	}
	for _, tp := range topics {
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
	}
	lecs := []db.GovLecture{
		{ID: "fb-l-quant", TopicID: "fb-t-quant", Title: "Q Core", SortOrder: 1},
		{ID: "fb-l-reasoning", TopicID: "fb-t-reasoning", Title: "R Core", SortOrder: 1},
		{ID: "fb-l-english", TopicID: "fb-t-english", Title: "E Core", SortOrder: 1},
		{ID: "fb-l-history", TopicID: "fb-t-history", Title: "H Core", SortOrder: 1},
		{ID: "fb-l-computer", TopicID: "fb-t-computer", Title: "C Core", SortOrder: 1},
		{ID: "fb-l-bank", TopicID: "fb-t-bank", Title: "B Core", SortOrder: 1},
		{ID: "fb-l-pk", TopicID: "fb-t-pk", Title: "P Core", SortOrder: 1},
		{ID: "fb-l-extra", TopicID: "fb-t-extra", Title: "X Core", SortOrder: 1},
	}
	for _, l := range lecs {
		if err := catalog.InsertGovLecture(ctx, d, l); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []db.GovExam{
		{ID: "fb-exam-cur", Name: "Current", SortOrder: 1},
		{ID: "fb-exam-f2", Name: "Future2", SortOrder: 2},
		{ID: "fb-exam-f3", Name: "Future3", SortOrder: 3},
		{ID: "fb-exam-f4", Name: "Future4", SortOrder: 4},
	} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "fb-exam-cur", SubjectID: "fb-quant", SortOrder: 1},
		{ExamID: "fb-exam-cur", SubjectID: "fb-reasoning", SortOrder: 2},
		{ExamID: "fb-exam-cur", SubjectID: "fb-english", SortOrder: 3},
		{ExamID: "fb-exam-cur", SubjectID: "fb-history", SortOrder: 4},
		{ExamID: "fb-exam-f2", SubjectID: "fb-quant", SortOrder: 1},
		{ExamID: "fb-exam-f2", SubjectID: "fb-reasoning", SortOrder: 2},
		{ExamID: "fb-exam-f2", SubjectID: "fb-english", SortOrder: 3},
		{ExamID: "fb-exam-f2", SubjectID: "fb-computer", SortOrder: 4},
		{ExamID: "fb-exam-f2", SubjectID: "fb-bank", SortOrder: 5},
		{ExamID: "fb-exam-f3", SubjectID: "fb-quant", SortOrder: 1},
		{ExamID: "fb-exam-f3", SubjectID: "fb-reasoning", SortOrder: 2},
		{ExamID: "fb-exam-f3", SubjectID: "fb-english", SortOrder: 3},
		{ExamID: "fb-exam-f3", SubjectID: "fb-computer", SortOrder: 4},
		{ExamID: "fb-exam-f3", SubjectID: "fb-pk", SortOrder: 5},
		{ExamID: "fb-exam-f4", SubjectID: "fb-quant", SortOrder: 1},
		{ExamID: "fb-exam-f4", SubjectID: "fb-reasoning", SortOrder: 2},
		{ExamID: "fb-exam-f4", SubjectID: "fb-english", SortOrder: 3},
		{ExamID: "fb-exam-f4", SubjectID: "fb-extra", SortOrder: 4},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	syllabus := map[db.ID][]db.ID{
		"fb-exam-cur": {"fb-t-quant", "fb-t-reasoning", "fb-t-english", "fb-t-history"},
		"fb-exam-f2":  {"fb-t-quant", "fb-t-reasoning", "fb-t-english", "fb-t-computer", "fb-t-bank"},
		"fb-exam-f3":  {"fb-t-quant", "fb-t-reasoning", "fb-t-english", "fb-t-computer", "fb-t-pk"},
		"fb-exam-f4":  {"fb-t-quant", "fb-t-reasoning", "fb-t-english", "fb-t-extra"},
	}
	for examID, tids := range syllabus {
		for i, tid := range tids {
			if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: examID, TopicID: tid, SortOrder: i + 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Lecture routine 1..4 seeded by 001_initial.sql.
	for i := 1; i <= 3; i++ {
		if err := catalog.InsertGovLectureRoutineSlot(ctx, d, db.GovLectureRoutineSlot{SlotOrder: i, DurationSeconds: 3600}); err != nil {
			t.Fatal(err)
		}
	}
}

func fbLectureSubjects(t *testing.T, ctx context.Context, d *db.DB, tasks []db.TodayTask) []string {
	t.Helper()
	var out []string
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
		out = append(out, subj.Name)
	}
	return out
}

func fbLectureTasks(t *testing.T, tasks []db.TodayTask) []db.TodayTask {
	t.Helper()
	var out []db.TodayTask
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture {
			out = append(out, tk)
		}
	}
	return out
}

func fbEq(a, b []string) bool {
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

// 1. Four lecture slots are available after migration.
func TestGovLectureFourSlotsAvailable(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	routine, err := catalog.GetGovLectureRoutine(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(routine) != 4 {
		t.Fatalf("routine slots=%d want 4", len(routine))
	}
	for i, s := range routine {
		if s.SlotOrder != i+1 {
			t.Fatalf("slot order=%d want %d", s.SlotOrder, i+1)
		}
		if s.DurationSeconds != 3600 {
			t.Fatalf("slot %d duration=%d want 3600", s.SlotOrder, s.DurationSeconds)
		}
	}
	// Slot 4 has no special semantics: same shape as others.
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_lecture_routine WHERE slot_order = 4 AND duration_seconds = 3600`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("slot 4 row missing: n=%d err=%v", n, err)
	}
}

// 2. When the current exam has 4+ incomplete subjects: all 4 from current, no future.
func TestGovLectureFourFromCurrent(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant", "Reasoning", "English", "History"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v", got, want)
	}
	for _, name := range got {
		if name == "Banking Awareness" || name == "Professional Knowledge" || name == "Extra Unique" || name == "Computer" {
			t.Fatalf("no future subject allowed, got %v", got)
		}
	}
	if cur, err := engine.CurrentGovExam(ctx, d); err != nil || cur == nil || cur.Name != "Current" {
		t.Fatalf("current exam must stay Current, got %v %v", cur, err)
	}
}

// 3. Current exactly 3 incompletes: V2 plans only the focus exam (no
// future filler). 3 current lectures, slots left empty rather than filled.
func TestGovLectureThreePlusOne(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant", "Reasoning", "English"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v (V2: focus only, no filler)", got, want)
	}
}

// 4. Current exactly 2 incompletes: V2 plans 2 focus lectures (no filler).
func TestGovLectureTwoPlusTwo(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-english", now)
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant", "Reasoning"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v (V2: focus only)", got, want)
	}
}

// 5. Current exactly 1 incomplete: V2 plans 1 focus lecture (no filler).
func TestGovLectureOnePlusThree(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-reasoning", now)
	completeExamLecture(t, ctx, d, "fb-l-english", now)
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v (V2: focus only)", got, want)
	}
}

// 6. V2 never pulls future subjects: only focus incompletes appear.
func TestGovLectureSkipsSharedFuture(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-reasoning", now)
	completeExamLecture(t, ctx, d, "fb-l-english", now)
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant"}
	if !fbEq(got, want) {
		t.Fatalf("V2 focus-only, got %v want %v", got, want)
	}
	for _, name := range got {
		if name == "Computer" || name == "Banking Awareness" {
			t.Fatalf("future subject must never appear in V2, got %v", got)
		}
	}
}

// 7. V2: no future scan — exhausted future exams are irrelevant.
func TestGovLectureSkipsExhaustedFutureExam(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	// Exhaust a future unique: V2 still plans only focus incompletes.
	completeExamLecture(t, ctx, d, "fb-l-bank", now)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant", "Reasoning", "English"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v (V2 focus only)", got, want)
	}
}

// 8. V2: completed future work is irrelevant to focus planning.
func TestGovLectureSkipsCompleteUnique(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	completeExamLecture(t, ctx, d, "fb-l-bank", now)
	completeExamLecture(t, ctx, d, "fb-l-pk", now)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant", "Reasoning", "English"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v (V2 focus only)", got, want)
	}
}

// 9. V2: focus exam alone determines lectures; future order irrelevant.
func TestGovLectureFutureExamOrder(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-reasoning", now)
	completeExamLecture(t, ctx, d, "fb-l-english", now)
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	want := []string{"Quant"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v (V2 focus only)", got, want)
	}
}

// 10. V2: subjects come from the focus exam order only.
func TestGovLectureFutureSubjectOrder(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	// Custom world: current has 3 incompletes, future has two uniques in order.
	for _, s := range []db.GovSubject{
		{ID: "so-q", Name: "Quant", SortOrder: 1},
		{ID: "so-r", Name: "Reasoning", SortOrder: 2},
		{ID: "so-e", Name: "English", SortOrder: 3},
		{ID: "so-u1", Name: "UniqueA", SortOrder: 4},
		{ID: "so-u2", Name: "UniqueB", SortOrder: 5},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, tp := range []db.GovTopic{
		{ID: "so-t-q", SubjectID: "so-q", Name: "Q", SortOrder: 1},
		{ID: "so-t-r", SubjectID: "so-r", Name: "R", SortOrder: 1},
		{ID: "so-t-e", SubjectID: "so-e", Name: "E", SortOrder: 1},
		{ID: "so-t-u1", SubjectID: "so-u1", Name: "A", SortOrder: 1},
		{ID: "so-t-u2", SubjectID: "so-u2", Name: "B", SortOrder: 1},
	} {
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range []db.GovLecture{
		{ID: "so-l-q", TopicID: "so-t-q", Title: "C", SortOrder: 1},
		{ID: "so-l-r", TopicID: "so-t-r", Title: "C", SortOrder: 1},
		{ID: "so-l-e", TopicID: "so-t-e", Title: "C", SortOrder: 1},
		{ID: "so-l-u1", TopicID: "so-t-u1", Title: "C", SortOrder: 1},
		{ID: "so-l-u2", TopicID: "so-t-u2", Title: "C", SortOrder: 1},
	} {
		if err := catalog.InsertGovLecture(ctx, d, l); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []db.GovExam{
		{ID: "so-cur", Name: "Cur", SortOrder: 1},
		{ID: "so-fut", Name: "Fut", SortOrder: 2},
	} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "so-cur", SubjectID: "so-q", SortOrder: 1},
		{ExamID: "so-cur", SubjectID: "so-r", SortOrder: 2},
		{ExamID: "so-cur", SubjectID: "so-e", SortOrder: 3},
		{ExamID: "so-fut", SubjectID: "so-q", SortOrder: 1},
		{ExamID: "so-fut", SubjectID: "so-u1", SortOrder: 2},
		{ExamID: "so-fut", SubjectID: "so-u2", SortOrder: 3},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	for i, tid := range []db.ID{"so-t-q", "so-t-r", "so-t-e"} {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "so-cur", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i, tid := range []db.ID{"so-t-q", "so-t-u1", "so-t-u2"} {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "so-fut", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		if err := catalog.InsertGovLectureRoutineSlot(ctx, d, db.GovLectureRoutineSlot{SlotOrder: i, DurationSeconds: 3600}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	// V2 focus defaults to the first exam (Cur): 3 focus lectures, no filler.
	want := []string{"Quant", "Reasoning", "English"}
	if !fbEq(got, want) {
		t.Fatalf("lecture subjects=%v want %v (V2 focus only)", got, want)
	}
}

// 11. CurrentGovExam() remains unchanged.
func TestGovLectureCurrentExamUnchanged(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	before, err := engine.CurrentGovExam(ctx, d)
	if err != nil || before == nil {
		t.Fatal("need current exam")
	}
	if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", now); err != nil {
		t.Fatal(err)
	}
	after, err := engine.CurrentGovExam(ctx, d)
	if err != nil || after == nil {
		t.Fatal("need current exam after")
	}
	if before.ID != after.ID || before.Name != after.Name {
		t.Fatalf("current exam changed: before=%v after=%v", before, after)
	}
	if after.Name != "Current" {
		t.Fatalf("current must stay Current, got %q", after.Name)
	}
}

// 12. Existing behavior remains unchanged when the current exam itself provides 3 or more candidates.
func TestGovLectureFirstThreeUnchanged(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	if len(got) < 3 {
		t.Fatalf("need >=3, got %v", got)
	}
	wantFirst3 := []string{"Quant", "Reasoning", "English"}
	if !fbEq(got[:3], wantFirst3) {
		t.Fatalf("first three=%v want %v", got[:3], wantFirst3)
	}
	// Slot 4 identical semantics: same kind/domain/duration.
	lecs := fbLectureTasks(t, tasks)
	if len(lecs) != 4 {
		t.Fatalf("lectures=%d want 4", len(lecs))
	}
	for _, tk := range lecs {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture {
			t.Fatalf("slot must be GOV LECTURE, got %v %v", tk.Domain, tk.Kind)
		}
		if tk.PlannedSeconds != 3600 {
			t.Fatalf("slot planned=%d want 3600", tk.PlannedSeconds)
		}
		if tk.TargetJSON == nil {
			t.Fatal("lecture target missing")
		}
		if _, err := db.UnmarshalGovLectureTarget(*tk.TargetJSON); err != nil {
			t.Fatalf("bad lecture target: %v", err)
		}
	}
}

// 13. Subject activation remains atomic.
func TestGovLectureActivationAtomic(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	for _, sid := range []db.ID{"fb-bank", "fb-quant"} {
		if active, err := runtime.IsGovSubjectActive(ctx, d, sid); err != nil || active {
			t.Fatalf("subject %q must start inactive", sid)
		}
	}
	if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", now); err != nil {
		t.Fatal(err)
	}
	// V2 focus-only: only focus subjects activate; future filler never runs.
	for _, sid := range []db.ID{"fb-quant", "fb-reasoning", "fb-english"} {
		if active, err := runtime.IsGovSubjectActive(ctx, d, sid); err != nil || !active {
			t.Fatalf("subject %q must be active after planning: %v %v", sid, active, err)
		}
	}
	if active, _ := runtime.IsGovSubjectActive(ctx, d, "fb-bank"); active {
		t.Fatalf("future subject fb-bank must stay inactive in V2 (no filler)")
	}
	// Failure rolls back activation: sabotage activation insert.
	d2 := openEngineDB(t)
	seedFallbackBase(t, ctx, d2)
	completeExamLecture(t, ctx, d2, "fb-l-history", now)
	var before int
	if err := d2.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := d2.ExecContext(ctx, `CREATE TRIGGER sab_act BEFORE INSERT ON gov_subject_activation BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.GenerateToday(ctx, d2, "2026-09-22", now); err == nil {
		t.Fatal("sabotaged activation must fail")
	}
	if _, err := runtime.GetLatestTodaySnapshot(ctx, d2, "2026-09-22"); err == nil {
		t.Fatal("partial snapshot must roll back")
	}
	var after int
	if err := d2.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("orphan activation: before=%d after=%d", before, after)
	}
}

// 14. Practice behavior remains unchanged.
func TestGovLecturePracticeUnchanged(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	// Practice routine: fixed Quant, fixed Reasoning, round-robin.
	q := db.ID("fb-quant")
	r := db.ID("fb-reasoning")
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &q}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 2, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &r}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 3, DurationSeconds: 3600, SelectionMode: db.GovSelectionRoundRobin}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Force fallback: current has 1 incomplete (Quant), futures fill rest.
	completeExamLecture(t, ctx, d, "fb-l-reasoning", now)
	completeExamLecture(t, ctx, d, "fb-l-english", now)
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	// Complete Quant's lecture track? No: keep Quant incomplete for lecture,
	// but practice needs eligible topics (lecture-complete or zero-lecture).
	// Add a zero-lecture topic to Quant so practice has eligible work without
	// affecting lecture planning (zero-lecture counts as complete for lectures).
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "fb-t-qzero", SubjectID: "fb-quant", Name: "Q Zero", SortOrder: 2}); err != nil {
		t.Fatal(err)
	}
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := engine.CurrentGovExam(ctx, d)
	if err != nil || cur == nil {
		t.Fatal("need current exam")
	}
	members, err := catalog.GetGovExamSubjects(ctx, d, cur.ID)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[db.ID]bool{}
	for _, m := range members {
		allowed[m.SubjectID] = true
	}
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindPractice || tk.TargetJSON == nil {
			continue
		}
		pt, err := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
		if err != nil {
			t.Fatal(err)
		}
		if !allowed[pt.SubjectID] {
			t.Fatalf("practice subject %q not in current exam", pt.SubjectID)
		}
		if pt.SubjectID == "fb-bank" || pt.SubjectID == "fb-pk" || pt.SubjectID == "fb-extra" || pt.SubjectID == "fb-computer" {
			t.Fatalf("future subject must never appear in practice: %q", pt.SubjectID)
		}
	}
}

// 15. Today generation remains idempotent.
func TestGovLectureIdempotent(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedFallbackBase(t, ctx, d)
	now := time.Now()
	completeExamLecture(t, ctx, d, "fb-l-history", now)
	snap, tasks, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	snap2, tasks2, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID != snap2.ID {
		t.Fatal("EnsureToday must not duplicate snapshot")
	}
	if len(tasks) != len(tasks2) {
		t.Fatalf("task counts differ: %d vs %d", len(tasks), len(tasks2))
	}
	got := fbLectureSubjects(t, ctx, d, tasks)
	got2 := fbLectureSubjects(t, ctx, d, tasks2)
	if !fbEq(got, got2) {
		t.Fatalf("idempotent subjects differ: %v vs %v", got, got2)
	}
}
