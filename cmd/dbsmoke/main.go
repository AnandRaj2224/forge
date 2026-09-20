package main

import (
	"context"
	"fmt"
	"os"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// Tiny DB smoke-test: create DB, migrate, insert representative
// static + runtime data, query it successfully.
// Never touches a production DB; uses a temp file unless FORGE_SMOKE_DB is set.
func main() {
	path := os.Getenv("FORGE_SMOKE_DB")
	cleanup := func() {}
	if path == "" {
		f, err := os.CreateTemp("", "forge-smoke-*.db")
		if err != nil {
			fatal(err)
		}
		path = f.Name()
		_ = f.Close()
		cleanup = func() { _ = os.Remove(path) }
	}
	defer cleanup()

	ctx := context.Background()
	database, err := db.Open(path)
	if err != nil {
		fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 300, CreatedAt: now}); err != nil {
		fatal(err)
	}
	if err := catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "quant", Name: "Quant", SortOrder: 1}); err != nil {
		fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "perc", SubjectID: "quant", Name: "Percentage", SortOrder: 1}); err != nil {
		fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "lec1", TopicID: "perc", Title: "Percentage Basics", SortOrder: 1}); err != nil {
		fatal(err)
	}
	if err := catalog.InsertDSAProblem(ctx, database, db.DSAProblem{ID: "two-sum", Title: "Two Sum", Pattern: "HashMap", Difficulty: "Easy", SortOrder: 1}); err != nil {
		fatal(err)
	}
	if err := catalog.UpsertDSARoutine(ctx, database, db.DSARoutine{ID: 1, PlannedSeconds: 9000}); err != nil {
		fatal(err)
	}
	if err := catalog.InsertSWEArea(ctx, database, db.SWEArea{ID: "go", Name: "Go", SortOrder: 1}); err != nil {
		fatal(err)
	}
	if err := catalog.InsertSWETask(ctx, database, db.SWETask{ID: "go-1", AreaID: "go", Title: "Slices", SortOrder: 1}); err != nil {
		fatal(err)
	}

	if err := runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap1", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now}); err != nil {
		fatal(err)
	}
	target, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "quant", TopicID: "perc", LectureID: "lec1"})
	if err := runtime.InsertTodayTask(ctx, database, db.TodayTask{
		ID: "task1", SnapshotID: "snap1", Position: 1,
		Domain: db.DomainGOV, Kind: db.SessionKindLecture,
		Title: "Percentage Basics", PlannedSeconds: 3600, TargetJSON: &target,
	}); err != nil {
		fatal(err)
	}
	if err := runtime.CreateSession(ctx, database, db.Session{
		ID: "sess1", TodayTaskID: "task1", Domain: db.DomainGOV, Kind: db.SessionKindLecture,
		TitleSnapshot: "Percentage Basics", PlannedSeconds: 3600, StartedAt: now, Status: db.SessionStatusActive,
	}); err != nil {
		fatal(err)
	}
	if err := runtime.RecordLectureCompletion(ctx, database, db.GovLectureCompletion{LectureID: "lec1", SessionID: "sess1", CompletedAt: now}); err != nil {
		fatal(err)
	}
	if err := runtime.InsertDSAAttempt(ctx, database, db.DSAAttempt{
		ID: "att1", SessionID: "sess1", ProblemID: "two-sum", Mode: db.DSAModeNew,
		AttemptedAt: now, Independent: true, Optimal: true,
	}); err != nil {
		fatal(err)
	}

	subjects, err := catalog.GetGovSubjects(ctx, database)
	if err != nil {
		fatal(err)
	}
	lectures, err := catalog.GetGovLectures(ctx, database, "perc")
	if err != nil {
		fatal(err)
	}
	active, err := runtime.GetActiveSession(ctx, database)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("smoke ok: subjects=%d lectures=%d active_session=%s\n", len(subjects), len(lectures), active.ID)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "smoke failed:", err)
	os.Exit(1)
}
