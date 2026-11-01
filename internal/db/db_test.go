package db_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
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

func testNow() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) }

// makeSession creates snapshot+task+session chain. Status controls stopped_at.
func makeSession(t *testing.T, ctx context.Context, q db.DBTX, sessID string, status db.SessionStatus) {
	t.Helper()
	now := testNow()
	snapID := db.ID("snap-" + sessID)
	taskID := db.ID("task-" + sessID)
	if err := runtime.CreateTodaySnapshot(ctx, q, db.TodaySnapshot{ID: snapID, LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now}); err != nil {
		// Snapshot may already exist for this date/revision in tests that reuse DB;
		// use a unique date per session to avoid collision.
		snapID = db.ID("snap-" + sessID + "-u")
		taskID = db.ID("task-" + sessID + "-u")
		if err2 := runtime.CreateTodaySnapshot(ctx, q, db.TodaySnapshot{ID: snapID, LocalDate: "2026-09-2" + sessID[len(sessID)-1:], Revision: 1, GeneratedAt: now}); err2 != nil {
			t.Fatalf("create snapshot: %v (first err %v)", err2, err)
		}
	}
	if err := runtime.InsertTodayTask(ctx, q, db.TodayTask{
		ID: snapIDToTask(taskID), SnapshotID: snapID, Position: 1,
		Domain: db.DomainGOV, Kind: db.SessionKindLecture,
		Title: "T", PlannedSeconds: 3600,
	}); err != nil {
		t.Fatalf("insert today task: %v", err)
	}
	s := db.Session{
		ID: db.ID(sessID), TodayTaskID: snapIDToTask(taskID),
		Domain: db.DomainGOV, Kind: db.SessionKindLecture,
		TitleSnapshot: "T", PlannedSeconds: 3600,
		StartedAt: now, Status: status,
	}
	if status == db.SessionStatusStopped {
		st := now.Add(time.Hour)
		s.StoppedAt = &st
	}
	if err := runtime.CreateSession(ctx, q, s); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func snapIDToTask(id db.ID) db.ID { return id }

// uniqueSnapshot creates a snapshot with unique date to avoid revision collisions.
var snapshotSeq = 0

func Test01_OpenAndMigrate(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("schema_migrations missing: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected at least 1 migration, got %d", count)
	}
	// Re-migrate must be idempotent.
	if err := database.Migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

func Test02_ForeignKeysEnforced(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	err := catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t1", SubjectID: "missing-subject", Name: "X", SortOrder: 1})
	if err == nil {
		t.Fatal("expected FK violation inserting topic with missing subject")
	}
}

func Test03_DuplicateSubjectSortOrderRejected(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1}); err != nil {
		t.Fatalf("insert s1: %v", err)
	}
	if err := catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s2", Name: "Reasoning", SortOrder: 1}); err == nil {
		t.Fatal("expected duplicate sort_order rejection")
	}
}

func Test04_DuplicateTopicOrderWithinSubjectRejected(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s2", Name: "Reasoning", SortOrder: 2}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t1", SubjectID: "s1", Name: "A", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t2", SubjectID: "s1", Name: "B", SortOrder: 1}); err == nil {
		t.Fatal("expected duplicate (subject_id, sort_order) rejection")
	}
	// Same sort_order across different subjects must succeed.
	if err := catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t3", SubjectID: "s2", Name: "C", SortOrder: 1}); err != nil {
		t.Fatalf("same order across subjects should succeed: %v", err)
	}
}

func Test05_TopicMayHaveZeroLectures(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t1", SubjectID: "s1", Name: "A", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	lectures, err := catalog.GetGovLectures(ctx, database, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(lectures) != 0 {
		t.Fatalf("expected zero lectures, got %d", len(lectures))
	}
}

func Test06_DuplicateExamTopicMappingRejected(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t1", SubjectID: "s1", Name: "A", SortOrder: 1})
	if err := catalog.InsertGovExam(ctx, database, db.GovExam{ID: "afcat", Name: "AFCAT", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "afcat", TopicID: "t1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "afcat", TopicID: "t1", SortOrder: 2}); err == nil {
		t.Fatal("expected duplicate (exam_id, topic_id) rejection")
	}
}

func seedPracticePrereqs(t *testing.T, ctx context.Context, database *db.DB, sessID string) {
	t.Helper()
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t1", SubjectID: "s1", Name: "A", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t2", SubjectID: "s1", Name: "B", SortOrder: 2})
	now := testNow()
	snap := db.ID("snap-" + sessID)
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: snap, LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: db.ID("task-" + sessID), SnapshotID: snap, Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "P", PlannedSeconds: 1800}); err != nil {
		t.Fatalf("task: %v", err)
	}
	st := now.Add(time.Hour)
	_ = runtime.CreateSession(ctx, database, db.Session{ID: db.ID(sessID), TodayTaskID: db.ID("task-" + sessID), Domain: db.DomainGOV, Kind: db.SessionKindPractice, TitleSnapshot: "P", PlannedSeconds: 1800, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped})
}

func Test07_PracticeRecordCorrectLteAttempted(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedPracticePrereqs(t, ctx, database, "sess1")
	now := testNow()
	err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{
		ID: "pr1", SessionID: "sess1", TopicID: "t1",
		TimeSpentSeconds: 100, Attempted: 5, Correct: 9, RecordedAt: now,
	})
	if err == nil {
		t.Fatal("expected correct > attempted rejection")
	}
	// Valid boundary correct == attempted must succeed.
	if err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{
		ID: "pr2", SessionID: "sess1", TopicID: "t1",
		TimeSpentSeconds: 100, Attempted: 5, Correct: 5, RecordedAt: now,
	}); err != nil {
		t.Fatalf("valid record should succeed: %v", err)
	}
}

func Test08_OneSessionTopicPracticeRecordEnforced(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedPracticePrereqs(t, ctx, database, "sess1")
	now := testNow()
	if err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{ID: "pr1", SessionID: "sess1", TopicID: "t1", TimeSpentSeconds: 10, Attempted: 2, Correct: 1, RecordedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{ID: "pr2", SessionID: "sess1", TopicID: "t1", TimeSpentSeconds: 10, Attempted: 3, Correct: 2, RecordedAt: now}); err == nil {
		t.Fatal("expected UNIQUE(session_id, topic_id) violation")
	}
	// Different topic same session must succeed.
	if err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{ID: "pr3", SessionID: "sess1", TopicID: "t2", TimeSpentSeconds: 10, Attempted: 3, Correct: 2, RecordedAt: now}); err != nil {
		t.Fatalf("different topic should succeed: %v", err)
	}
}

func seedDSAPrereqs(t *testing.T, ctx context.Context, database *db.DB, sessID string) {
	t.Helper()
	_ = catalog.InsertDSAProblem(ctx, database, db.DSAProblem{ID: "p1", Title: "Two Sum", Pattern: "HashMap", Difficulty: "Easy", SortOrder: 1})
	now := testNow()
	snap := db.ID("snap-" + sessID)
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: snap, LocalDate: "2026-09-21", Revision: 1, GeneratedAt: now}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: db.ID("task-" + sessID), SnapshotID: snap, Position: 1, Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "D", PlannedSeconds: 3600}); err != nil {
		t.Fatalf("task: %v", err)
	}
	st := now.Add(time.Hour)
	_ = runtime.CreateSession(ctx, database, db.Session{ID: db.ID(sessID), TodayTaskID: db.ID("task-" + sessID), Domain: db.DomainDSA, Kind: db.SessionKindDSA, TitleSnapshot: "D", PlannedSeconds: 3600, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped})
}

func Test09_OnlyOneNewDSAAttemptPerProblem(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedDSAPrereqs(t, ctx, database, "sess1")
	now := testNow()
	if err := runtime.InsertDSAAttempt(ctx, database, db.DSAAttempt{ID: "a1", SessionID: "sess1", ProblemID: "p1", Mode: db.DSAModeNew, AttemptedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.InsertDSAAttempt(ctx, database, db.DSAAttempt{ID: "a2", SessionID: "sess1", ProblemID: "p1", Mode: db.DSAModeNew, AttemptedAt: now.Add(time.Minute)}); err == nil {
		t.Fatal("expected second NEW attempt rejection")
	}
}

func Test10_UnlimitedRepeatAttemptsAllowed(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedDSAPrereqs(t, ctx, database, "sess1")
	now := testNow()
	for i, id := range []string{"r1", "r2", "r3", "r4", "r5"} {
		if err := runtime.InsertDSAAttempt(ctx, database, db.DSAAttempt{ID: db.ID(id), SessionID: "sess1", ProblemID: "p1", Mode: db.DSAModeRepeat, AttemptedAt: now.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatalf("repeat %s should succeed: %v", id, err)
		}
	}
	got, err := runtime.GetDSAAttempts(ctx, database, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 repeats, got %d", len(got))
	}
}

func makeSessionChain(t *testing.T, ctx context.Context, database *db.DB, tag, date string, rev int, status db.SessionStatus) db.ID {
	t.Helper()
	now := testNow()
	snapID := db.ID("snap-" + tag)
	taskID := db.ID("task-" + tag)
	sessID := db.ID("sess-" + tag)
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: snapID, LocalDate: date, Revision: rev, GeneratedAt: now}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: taskID, SnapshotID: snapID, Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "T", PlannedSeconds: 60}); err != nil {
		t.Fatalf("task: %v", err)
	}
	s := db.Session{ID: sessID, TodayTaskID: taskID, Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "T", PlannedSeconds: 60, StartedAt: now, Status: status}
	if status == db.SessionStatusStopped {
		st := now.Add(time.Minute)
		s.StoppedAt = &st
	}
	if err := runtime.CreateSession(ctx, database, s); err != nil {
		t.Fatalf("session: %v", err)
	}
	return sessID
}

func Test12_OnlyOneActiveSession(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	makeSessionChain(t, ctx, database, "a", "2026-09-20", 1, db.SessionStatusActive)
	// Second ACTIVE session must fail (partial unique index).
	now := testNow()
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap-b", LocalDate: "2026-09-21", Revision: 1, GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "task-b", SnapshotID: "snap-b", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "T", PlannedSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	err := runtime.CreateSession(ctx, database, db.Session{ID: "sess-b", TodayTaskID: "task-b", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "T", PlannedSeconds: 60, StartedAt: now, Status: db.SessionStatusActive})
	if err == nil {
		t.Fatal("expected single-ACTIVE enforcement")
	}
}

func Test13_ActiveMustHaveNullStoppedAt(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "s1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "t1", SnapshotID: "s1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "T", PlannedSeconds: 60})
	st := now.Add(time.Minute)
	err := runtime.CreateSession(ctx, database, db.Session{ID: "sess1", TodayTaskID: "t1", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "T", PlannedSeconds: 60, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusActive})
	if err == nil {
		t.Fatal("expected ACTIVE with stopped_at rejection")
	}
}

func Test14_StoppedMustHaveNonNullStoppedAt(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "s1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "t1", SnapshotID: "s1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "T", PlannedSeconds: 60})
	err := runtime.CreateSession(ctx, database, db.Session{ID: "sess1", TodayTaskID: "t1", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "T", PlannedSeconds: 60, StartedAt: now, Status: db.SessionStatusStopped})
	if err == nil {
		t.Fatal("expected STOPPED with NULL stopped_at rejection")
	}
}

func Test15_SnapshotRevisionUniqueness(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "s1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "s2", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now}); err == nil {
		t.Fatal("expected UNIQUE(local_date, revision) violation")
	}
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "s3", LocalDate: "2026-09-20", Revision: 2, GeneratedAt: now}); err != nil {
		t.Fatalf("new revision should succeed: %v", err)
	}
}

func Test16_TaskPositionUniqueness(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "s1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "t1", SnapshotID: "s1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "A", PlannedSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "t2", SnapshotID: "s1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "B", PlannedSeconds: 60}); err == nil {
		t.Fatal("expected UNIQUE(snapshot_id, position) violation")
	}
}

func Test17_OneReviewPerWeek(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	r := db.WeeklyReview{ID: "2026-W38", WeekStart: "2026-09-14", WeekEnd: "2026-09-20", GeneratedAt: now, UpdatedAt: now, PlannedSeconds: 100, ElapsedSeconds: 90, OverrunSeconds: -10, ReportJSON: `{}`, ReflectionWorked: "x", ReflectionChange: "y"}
	if err := runtime.InsertWeeklyReviewOnce(ctx, database, r); err != nil {
		t.Fatal(err)
	}
	// Raw duplicate primary key must fail.
	if _, err := database.ExecContext(ctx, `INSERT INTO weekly_reviews (week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-W38", "2026-09-14", "2026-09-20", now.Unix(), now.Unix(), 1, 1, 0, "{}"); err == nil {
		t.Fatal("expected duplicate week PK rejection")
	}
	// Frozen metrics win: a second snapshot insert must fail, never overwrite.
	r2 := r
	r2.ElapsedSeconds = 95
	if err := runtime.InsertWeeklyReviewOnce(ctx, database, r2); err == nil {
		t.Fatal("second snapshot insert must fail (frozen metrics)")
	}
	got, err := runtime.ListWeeklyReviews(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 review row, got %d", len(got))
	}
	if got[0].ElapsedSeconds != 90 {
		t.Fatalf("frozen metrics must stay 90, got %d", got[0].ElapsedSeconds)
	}
	// Reflections remain editable without touching frozen metrics.
	if err := runtime.UpdateWeeklyReflections(ctx, database, "2026-W38", "new-worked", "new-change", now); err != nil {
		t.Fatal(err)
	}
	got2, err := runtime.GetWeeklyReview(ctx, database, "2026-W38")
	if err != nil {
		t.Fatal(err)
	}
	if got2.ReflectionWorked != "new-worked" || got2.ElapsedSeconds != 90 {
		t.Fatalf("reflections-only edit violated freeze: %+v", got2)
	}
}

func Test18_FKCannotPointToMissingStatic(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "s1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "t1", SnapshotID: "s1", Position: 1, Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "D", PlannedSeconds: 60})
	st := now.Add(time.Minute)
	_ = runtime.CreateSession(ctx, database, db.Session{ID: "sess1", TodayTaskID: "t1", Domain: db.DomainDSA, Kind: db.SessionKindDSA, TitleSnapshot: "D", PlannedSeconds: 60, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped})

	cases := map[string]func() error{
		"lecture_completion_missing_lecture": func() error {
			return runtime.RecordLectureCompletion(ctx, database, db.GovLectureCompletion{LectureID: "nope", SessionID: "sess1", CompletedAt: now})
		},
		"dsa_attempt_missing_problem": func() error {
			return runtime.InsertDSAAttempt(ctx, database, db.DSAAttempt{ID: "a1", SessionID: "sess1", ProblemID: "nope", Mode: db.DSAModeRepeat, AttemptedAt: now})
		},
		"practice_missing_topic": func() error {
			return runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{ID: "p1", SessionID: "sess1", TopicID: "nope", TimeSpentSeconds: 1, Attempted: 1, Correct: 1, RecordedAt: now})
		},
		"session_missing_task": func() error {
			return runtime.CreateSession(ctx, database, db.Session{ID: "sessX", TodayTaskID: "missing-task", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "T", PlannedSeconds: 60, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped})
		},
	}
	for name, fn := range cases {
		if err := fn(); err == nil {
			t.Fatalf("%s: expected FK violation", name)
		}
	}
}

func Test19_RuntimeIntactWhenStaticRead(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t1", SubjectID: "s1", Name: "A", SortOrder: 1})
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "l1", TopicID: "t1", Title: "L", SortOrder: 1})
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "task1", SnapshotID: "snap1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "L", PlannedSeconds: 60})
	st := now.Add(time.Minute)
	_ = runtime.CreateSession(ctx, database, db.Session{ID: "sess1", TodayTaskID: "task1", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "L", PlannedSeconds: 60, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped})
	if err := runtime.RecordLectureCompletion(ctx, database, db.GovLectureCompletion{LectureID: "l1", SessionID: "sess1", CompletedAt: now}); err != nil {
		t.Fatal(err)
	}
	// Reads must not mutate or delete runtime evidence.
	subs, err := catalog.GetGovSubjects(ctx, database)
	if err != nil || len(subs) != 1 {
		t.Fatalf("read subjects: %v %d", err, len(subs))
	}
	complete, err := runtime.IsLectureComplete(ctx, database, "l1")
	if err != nil || !complete {
		t.Fatalf("completion should remain: %v %v", err, complete)
	}
	// Static delete with dependent runtime evidence must be RESTRICTed.
	if _, err := database.ExecContext(ctx, `DELETE FROM gov_lectures WHERE id = 'l1'`); err == nil {
		t.Fatal("expected RESTRICT on static delete with runtime evidence")
	}
	complete, err = runtime.IsLectureComplete(ctx, database, "l1")
	if err != nil || !complete {
		t.Fatalf("runtime must remain intact: %v %v", err, complete)
	}
}

func Test20_TransactionRollback(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "t1", SubjectID: "s1", Name: "A", SortOrder: 1})
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "l1", TopicID: "t1", Title: "L", SortOrder: 1})
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "task1", SnapshotID: "snap1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "L", PlannedSeconds: 60})
	st := now.Add(time.Minute)
	_ = runtime.CreateSession(ctx, database, db.Session{ID: "sess1", TodayTaskID: "task1", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "L", PlannedSeconds: 60, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped})

	err := database.WithTx(ctx, func(q db.DBTX) error {
		if err := runtime.RecordLectureCompletion(ctx, q, db.GovLectureCompletion{LectureID: "l1", SessionID: "sess1", CompletedAt: now}); err != nil {
			return err
		}
		// Second step violates PK (duplicate lecture completion) -> whole tx must roll back.
		return runtime.RecordLectureCompletion(ctx, q, db.GovLectureCompletion{LectureID: "l1", SessionID: "sess1", CompletedAt: now})
	})
	if err == nil {
		t.Fatal("expected tx failure")
	}
	// Verify rollback: use a fresh session + lecture to prove the first insert did not persist.
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "l2", TopicID: "t1", Title: "L2", SortOrder: 2})
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap2", LocalDate: "2026-09-21", Revision: 1, GeneratedAt: now})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "task2", SnapshotID: "snap2", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "L2", PlannedSeconds: 60})
	_ = runtime.CreateSession(ctx, database, db.Session{ID: "sess2", TodayTaskID: "task2", Domain: db.DomainGOV, Kind: db.SessionKindLecture, TitleSnapshot: "L2", PlannedSeconds: 60, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped})

	// The rolled-back completion for l1/sess1 must NOT exist. Inserting it fresh must succeed.
	if err := runtime.RecordLectureCompletion(ctx, database, db.GovLectureCompletion{LectureID: "l1", SessionID: "sess1", CompletedAt: now}); err != nil {
		t.Fatalf("rollback failed, row persisted: %v", err)
	}
}

func TestSmoke_RepresentativeData(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	if err := catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 300, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []db.GovSubject{{ID: "quant", Name: "Quant", SortOrder: 1}, {ID: "reas", Name: "Reasoning", SortOrder: 2}} {
		if err := catalog.InsertGovSubject(ctx, database, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "perc", SubjectID: "quant", Name: "Percentage", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctx, database, db.GovExam{ID: "afcat", Name: "AFCAT", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctx, database, db.GovExamSyllabus{ExamID: "afcat", TopicID: "perc", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLectureRoutineSlot(ctx, database, db.GovLectureRoutineSlot{SlotOrder: 1, DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	quant := db.ID("quant")
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, database, db.GovPracticeRoutineSlot{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &quant}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, database, db.GovPracticeRoutineSlot{SlotOrder: 3, DurationSeconds: 3600, SelectionMode: db.GovSelectionRoundRobin}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertDSAProblem(ctx, database, db.DSAProblem{ID: "two-sum", Title: "Two Sum", Pattern: "HashMap", Difficulty: "Easy", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.UpsertDSARoutine(ctx, database, db.DSARoutine{ID: 1, PlannedSeconds: 9000}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	target, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "quant", TopicIDs: []db.ID{"perc"}})
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "task1", SnapshotID: "snap1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "Practice", PlannedSeconds: 1800, TargetJSON: &target}); err != nil {
		t.Fatal(err)
	}
	st := now.Add(30 * time.Minute)
	if err := runtime.CreateSession(ctx, database, db.Session{ID: "sess1", TodayTaskID: "task1", Domain: db.DomainGOV, Kind: db.SessionKindPractice, TitleSnapshot: "Practice", PlannedSeconds: 1800, StartedAt: now, StoppedAt: &st, Status: db.SessionStatusStopped}); err != nil {
		t.Fatal(err)
	}
	avg := 17.2
	if err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{ID: "pr1", SessionID: "sess1", TopicID: "perc", TimeSpentSeconds: 1380, Attempted: 20, Correct: 16, AvgQuestionSpeedSeconds: &avg, RecordedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.UpsertDSARepeatState(ctx, database, db.DSARepeatState{ProblemID: "two-sum", RepeatStage: 0, CurrentIntervalDays: 1, NextReviewDate: "2026-09-21", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.InsertWeeklyReviewOnce(ctx, database, db.WeeklyReview{ID: "2026-W38", WeekStart: "2026-09-14", WeekEnd: "2026-09-20", GeneratedAt: now, UpdatedAt: now, PlannedSeconds: 100, ElapsedSeconds: 90, OverrunSeconds: -10, ReportJSON: `{"weeks":1}`}); err != nil {
		t.Fatal(err)
	}
	// Query everything back.
	if _, err := catalog.GetGovSubjects(ctx, database); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetGovPracticeHistory(ctx, database, "perc"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetDSARepeatState(ctx, database, "two-sum"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetTodayTasks(ctx, database, "snap1"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetWeeklyReview(ctx, database, "2026-W38"); err != nil {
		t.Fatal(err)
	}
}

func TestPracticeRoutineShapeChecks(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "quant", Name: "Quant", SortOrder: 1})
	quant := db.ID("quant")
	// FIXED without subject must fail.
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, database, db.GovPracticeRoutineSlot{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed}); err == nil {
		t.Fatal("expected FIXED without subject rejection")
	}
	// ROUND_ROBIN with subject must fail.
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, database, db.GovPracticeRoutineSlot{SlotOrder: 3, DurationSeconds: 3600, SelectionMode: db.GovSelectionRoundRobin, FixedSubjectID: &quant}); err == nil {
		t.Fatal("expected ROUND_ROBIN with subject rejection")
	}
	// Invalid mode must fail.
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, database, db.GovPracticeRoutineSlot{SlotOrder: 4, DurationSeconds: 3600, SelectionMode: "RANDOM"}); err == nil {
		t.Fatal("expected invalid selection_mode rejection")
	}
}

func TestSettingsDayStartConstraint(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	now := testNow()
	if err := catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 1440, CreatedAt: now}); err == nil {
		t.Fatal("expected day_start_minute < 1440 rejection")
	}
	if err := catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: -1, CreatedAt: now}); err == nil {
		t.Fatal("expected day_start_minute >= 0 rejection")
	}
}

func TestAvgSpeedAndSolveTimeConstraints(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedPracticePrereqs(t, ctx, database, "sess1")
	now := testNow()
	zero := 0.0
	if err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{ID: "bad-avg", SessionID: "sess1", TopicID: "t1", TimeSpentSeconds: 10, Attempted: 1, Correct: 1, AvgQuestionSpeedSeconds: &zero, RecordedAt: now}); err == nil {
		t.Fatal("expected avg speed > 0 rejection")
	}
	neg := -5
	if err := runtime.InsertGovPracticeRecord(ctx, database, db.GovPracticeRecord{ID: "bad-time", SessionID: "sess1", TopicID: "t1", TimeSpentSeconds: neg, Attempted: 1, Correct: 1, RecordedAt: now}); err == nil {
		t.Fatal("expected time_spent >= 0 rejection")
	}
	seedDSAPrereqs(t, ctx, database, "sessD")
	negSolve := -1
	if err := runtime.InsertDSAAttempt(ctx, database, db.DSAAttempt{ID: "bad-solve", SessionID: "sessD", ProblemID: "p1", Mode: db.DSAModeRepeat, AttemptedAt: now, SolveTimeSeconds: &negSolve}); err == nil {
		t.Fatal("expected solve_time >= 0 rejection")
	}
	if err := runtime.InsertDSAAttempt(ctx, database, db.DSAAttempt{ID: "bad-mode", SessionID: "sessD", ProblemID: "p1", Mode: "HARD", AttemptedAt: now}); err == nil {
		t.Fatal("expected invalid mode rejection")
	}
}

func TestTargetJSONStrictness(t *testing.T) {
	raw := `{"subject_id":"quant","topic_id":"perc","lecture_id":"lec1"}`
	got, err := db.UnmarshalGovLectureTarget(raw)
	if err != nil {
		t.Fatalf("valid target should parse: %v", err)
	}
	if got.LectureID != "lec1" {
		t.Fatalf("unexpected lecture %q", got.LectureID)
	}
	if _, err := db.UnmarshalGovLectureTarget(`{"subject_id":"q","bogus":1,"topic_id":"t","lecture_id":"l"}`); err == nil {
		t.Fatal("expected unknown field rejection")
	}
	if !strings.Contains(string(mustMarshal(t, db.GovPracticeTarget{SubjectID: "quant", TopicIDs: []db.ID{"a"}})), "topic_ids") {
		t.Fatal("marshal missing topic_ids")
	}
	_ = snapshotSeq
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	s, err := db.MarshalTarget(v)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func seedExamSubjectPrereqs(t *testing.T, ctx context.Context, database *db.DB) {
	t.Helper()
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s1", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "s2", Name: "Reasoning", SortOrder: 2})
	_ = catalog.InsertGovExam(ctx, database, db.GovExam{ID: "e1", Name: "E1", SortOrder: 1})
	_ = catalog.InsertGovExam(ctx, database, db.GovExam{ID: "e2", Name: "E2", SortOrder: 2})
}

func TestExamSubjectMembershipUniquePerExam(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedExamSubjectPrereqs(t, ctx, database)
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	// Same subject twice in one exam must fail.
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s1", SortOrder: 2}); err == nil {
		t.Fatal("expected duplicate (exam_id, subject_id) rejection")
	}
	// Same subject in another exam must succeed (shared global subject).
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e2", SubjectID: "s1", SortOrder: 1}); err != nil {
		t.Fatalf("shared subject across exams should succeed: %v", err)
	}
}

func TestExamSubjectOrderUniquePerExam(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedExamSubjectPrereqs(t, ctx, database)
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	// Two subjects sharing one exam sort position must fail.
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s2", SortOrder: 1}); err == nil {
		t.Fatal("expected duplicate (exam_id, sort_order) rejection")
	}
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s2", SortOrder: 2}); err != nil {
		t.Fatalf("distinct order should succeed: %v", err)
	}
}

func TestExamSubjectFKEnforced(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedExamSubjectPrereqs(t, ctx, database)
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "missing-exam", SubjectID: "s1", SortOrder: 1}); err == nil {
		t.Fatal("expected FK violation for missing exam")
	}
	if err := catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "missing-subject", SortOrder: 1}); err == nil {
		t.Fatal("expected FK violation for missing subject")
	}
}

func TestExamSubjectOrderReturned(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	seedExamSubjectPrereqs(t, ctx, database)
	// Insert out of order; reads must follow sort_order.
	_ = catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s2", SortOrder: 2})
	_ = catalog.InsertGovExamSubject(ctx, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s1", SortOrder: 1})
	got, err := catalog.GetGovExamSubjects(ctx, database, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SubjectID != "s1" || got[1].SubjectID != "s2" {
		t.Fatalf("exam order not preserved: %+v", got)
	}
}
