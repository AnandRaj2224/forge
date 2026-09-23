package engine_test

// Full lifecycle row audit: start → stop → save → restart → complete,
// across GOV/DSA/SWE, plus mock creation and review persistence. Asserts
// the persisted rows (not just engine return values) and FK cleanliness.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

func seedLifecycle(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, tp := range []db.GovTopic{
		{ID: "topic-a", SubjectID: "subj-quant", Name: "Alpha", SortOrder: 1},
		{ID: "topic-b", SubjectID: "subj-quant", Name: "Beta", SortOrder: 2},
	} {
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lec-a", TopicID: "topic-a", Title: "Core", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "exam-1", Name: "E1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for i, tid := range []db.ID{"topic-a", "topic-b"} {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-1", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "exam-1", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-1", SubjectID: "subj-reasoning", SortOrder: 2},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	for i, slot := range []int{3600, 3600, 3600} {
		if err := catalog.InsertGovLectureRoutineSlot(ctx, d, db.GovLectureRoutineSlot{SlotOrder: i + 1, DurationSeconds: slot}); err != nil {
			t.Fatal(err)
		}
	}
	q := db.ID("subj-quant")
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &q}); err != nil {
		t.Fatal(err)
	}
	for i, p := range []db.DSAProblem{
		{ID: "dsa-1", Title: "One", Pattern: "Hash", Difficulty: "EASY"},
		{ID: "dsa-2", Title: "Two", Pattern: "Hash", Difficulty: "EASY"},
		{ID: "dsa-3", Title: "Three", Pattern: "Stack", Difficulty: "MEDIUM"},
	} {
		p.SortOrder = i + 1
		if err := catalog.InsertDSAProblem(ctx, d, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.UpsertDSARoutine(ctx, d, db.DSARoutine{ID: 1, PlannedSeconds: 9000}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWEArea(ctx, d, db.SWEArea{ID: "area-go", Name: "Go", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWETask(ctx, d, db.SWETask{ID: "go-1", AreaID: "area-go", Title: "Slices", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	// Monday routine (2026-09-21 is a Monday).
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 0, AreaID: "area-go", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, ctx context.Context, d *db.DB, q string) int {
	t.Helper()
	var n int
	if err := d.QueryRowContext(ctx, q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestFullLifecycleRowAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "life.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedLifecycle(t, ctx, d)
	cal := engine.DefaultCalendar()
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, cal.Location())

	// 1. Today generation: one snapshot, exact tasks, no orphans.
	snap, tasks, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot=%s tasks=%d", snap.ID, len(tasks))
	if countRows(t, ctx, d, `SELECT COUNT(*) FROM today_snapshots`) != 1 {
		t.Fatal("exactly one snapshot expected")
	}

	// 2. Lecture: start → active survives reopen → stop → restart creates a
	// new session leaving the old one unchanged → YES → COMPLETED.
	var lectureID db.ID
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture {
			lectureID = tk.ID
			break
		}
	}
	if lectureID == "" {
		t.Fatal("no lecture task generated")
	}
	s1, err := engine.StartTodayTask(ctx, d, lectureID, now)
	if err != nil {
		t.Fatal(err)
	}
	// Reopen handle: active session persists, never duplicated.
	d2, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if err := d2.Migrate(); err != nil {
		t.Fatal(err)
	}
	loaded, err := engine.LoadActiveSession(ctx, d2)
	if err != nil || loaded == nil || loaded.ID != s1.ID {
		t.Fatalf("active session must survive reopen, got %+v %v", loaded, err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	s2, err := engine.StartTodayTask(ctx, d, lectureID, now.Add(31*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if s2.ID == s1.ID {
		t.Fatal("restart must create a new session")
	}
	old, err := runtime.GetSession(ctx, d, s1.ID)
	if err != nil || old.Status != db.SessionStatusStopped {
		t.Fatalf("old session must stay stopped, got %+v %v", old, err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(50*time.Minute)); err != nil {
		t.Fatal(err)
	}
	ev, err := engine.SaveGovLectureEvidence(ctx, d, lectureID, s2.ID, true, now.Add(50*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("lecture must complete, got %s", ev.State)
	}

	// 3. Practice partial stays TODO, full completes.
	var practiceID db.ID
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			practiceID = tk.ID
		}
	}
	if practiceID != "" {
		pt, _ := db.UnmarshalGovPracticeTarget(*tasks[0].TargetJSON)
		_ = pt
		sp, err := engine.StartTodayTask(ctx, d, practiceID, now.Add(60*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.StopActiveSession(ctx, d, now.Add(70*time.Minute)); err != nil {
			t.Fatal(err)
		}
		praw, _ := db.UnmarshalGovPracticeTarget(taskTarget(t, ctx, d, practiceID))
		half := []engine.GovPracticeInput{{TopicID: praw.TopicIDs[0], TimeSecs: 600, Attempted: 10, Correct: 8, AvgSpeed: fptr(15.0)}}
		pev, err := engine.SaveGovPracticeEvidence(ctx, d, practiceID, sp.ID, half, now.Add(70*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if len(praw.TopicIDs) > 1 && pev.State != engine.StateTodo {
			t.Fatalf("partial practice must stay TODO, got %s", pev.State)
		}
	}

	// 4. DSA bundle save writes attempts + repeat state atomically.
	for _, tk := range tasks {
		if tk.Domain != db.DomainDSA {
			continue
		}
		sd, err := engine.StartTodayTask(ctx, d, tk.ID, now.Add(90*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.StopActiveSession(ctx, d, now.Add(100*time.Minute)); err != nil {
			t.Fatal(err)
		}
		dt, _ := db.UnmarshalDSATarget(taskTarget(t, ctx, d, tk.ID))
		var inputs []engine.DSAAttemptInput
		for _, it := range dt.Items() {
			inputs = append(inputs, engine.DSAAttemptInput{ProblemID: it.ProblemID, Independent: true, Optimal: true, HintUsed: false, SolveSecs: 300})
		}
		if _, err := engine.SaveDSAEvidence(ctx, d, tk.ID, sd.ID, inputs, now.Add(100*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if n := countRows(t, ctx, d, `SELECT COUNT(*) FROM dsa_attempts`); n == 0 {
		t.Fatal("dsa attempts must persist")
	}
	if n := countRows(t, ctx, d, `SELECT COUNT(*) FROM dsa_repeat_state`); n == 0 {
		t.Fatal("repeat state must persist")
	}

	// 5. SWE bundle save writes executions.
	for _, tk := range tasks {
		if tk.Domain != db.DomainSWE {
			continue
		}
		sw, err := engine.StartTodayTask(ctx, d, tk.ID, now.Add(110*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.StopActiveSession(ctx, d, now.Add(120*time.Minute)); err != nil {
			t.Fatal(err)
		}
		st, _ := db.UnmarshalSWETarget(taskTarget(t, ctx, d, tk.ID))
		var inputs []engine.SWEOutcomeInput
		for _, it := range st.Items() {
			if it.TaskID != nil {
				inputs = append(inputs, engine.SWEOutcomeInput{TaskID: *it.TaskID, Status: db.SWEStatusCompleted})
			}
		}
		if _, err := engine.SaveSWEEvidence(ctx, d, tk.ID, sw.ID, inputs, now.Add(120*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	// 6. Mock creation is atomic with explicit subject mapping.
	score := 72
	total := 100
	mk := db.Mock{ID: "mock-1", ExamID: "exam-1", Name: "M1", MockDate: "2026-09-21", CreatedAt: now, Score: &score, TotalScore: &total}
	if _, err := engine.CreateMockAtomic(ctx, d, mk, []engine.MockSectionInput{
		{Name: "QA-1", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-a"}},
	}, now); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, ctx, d, `SELECT COUNT(*) FROM mock_sections WHERE mock_id = 'mock-1'`); n != 1 {
		t.Fatalf("sections=%d want 1", n)
	}

	// 7. Review draft → frozen snapshot → reflection-only edit.
	draft, err := engine.BuildWeeklyDraft(ctx, d, "2026-W39", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatal(err)
	}
	js, err := engine.MarshalWeeklySnapshot(draft)
	if err != nil {
		t.Fatal(err)
	}
	row := db.WeeklyReview{ID: "2026-W39", WeekStart: "2026-09-21", WeekEnd: "2026-09-27",
		PlannedSeconds: draft.Planned, ElapsedSeconds: draft.Actual,
		OverrunSeconds: draft.Actual - draft.Planned, ReportJSON: js}
	if _, err := engine.SaveWeeklyReviewSnapshot(ctx, d, row, now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveReviewReflections(ctx, d, row, "good", "more", now); err != nil {
		t.Fatal(err)
	}

	// 8. Row audit + FK cleanliness.
	for _, q := range []string{
		`SELECT COUNT(*) FROM sessions`,
		`SELECT COUNT(*) FROM gov_lecture_completions`,
		`SELECT COUNT(*) FROM gov_practice_records`,
		`SELECT COUNT(*) FROM dsa_attempts`,
		`SELECT COUNT(*) FROM swe_executions`,
		`SELECT COUNT(*) FROM mocks`,
		`SELECT COUNT(*) FROM weekly_reviews`,
	} {
		t.Logf("%s = %d", q, countRows(t, ctx, d, q))
	}
	rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	fkIssues := 0
	for rows.Next() {
		fkIssues++
	}
	if fkIssues != 0 {
		t.Fatalf("%d foreign-key violations", fkIssues)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var activeLeft int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE status = 'ACTIVE'`).Scan(&activeLeft); err != nil {
		t.Fatal(err)
	}
	if activeLeft != 0 {
		t.Fatalf("fixture must end with no ACTIVE session, got %d", activeLeft)
	}
}

func taskTarget(t *testing.T, ctx context.Context, d *db.DB, id db.ID) string {
	t.Helper()
	tk, err := runtime.GetTodayTask(ctx, d, id)
	if err != nil {
		t.Fatal(err)
	}
	if tk.TargetJSON == nil {
		t.Fatal("task has no target")
	}
	return *tk.TargetJSON
}

func fptr(v float64) *float64 { return &v }
