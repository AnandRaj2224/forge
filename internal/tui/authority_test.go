package tui

// Today authority regressions: the workspace resolves its date exclusively
// through engine.EnsureToday. It must generate the current date when only
// older snapshots exist (never render old data as today), and a database
// failure must surface as a data error with nothing stale rendered.

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

func seedOldSnapshotDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	_ = catalog.UpsertSettings(ctx, database, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 0, CreatedAt: now})
	_ = catalog.InsertGovSubject(ctx, database, db.GovSubject{ID: "subj-quant", Name: "Quant", SortOrder: 1})
	_ = catalog.InsertGovTopic(ctx, database, db.GovTopic{ID: "topic-a", SubjectID: "subj-quant", Name: "Number System", SortOrder: 1})
	_ = catalog.InsertGovLecture(ctx, database, db.GovLecture{ID: "lec-a", TopicID: "topic-a", Title: "Core Concepts", SortOrder: 1})
	_ = runtime.CreateTodaySnapshot(ctx, database, db.TodaySnapshot{ID: "snap-old", LocalDate: "2026-09-20", Revision: 1, GeneratedAt: now})
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-a", LectureID: "lec-a"})
	_ = runtime.InsertTodayTask(ctx, database, db.TodayTask{ID: "old-task", SnapshotID: "snap-old", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "Old lecture", PlannedSeconds: 3600, TargetJSON: &lec})
	t.Cleanup(func() { database.Close() })
	return database
}

// An older snapshot must never render as today: with the clock on a date
// that has no snapshot, Reload generates the current date via the engine.
func TestTodayGeneratesCurrentNotOlder(t *testing.T) {
	app := NewApp(seedOldSnapshotDB(t))
	app.width, app.height = 120, 36
	app.today.nowFn = func() time.Time { return time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC) }
	app.today.Reload()
	if app.today.err != "" {
		t.Fatalf("reload failed: %s", app.today.err)
	}
	if app.today.snap == nil {
		t.Fatal("engine must supply a snapshot")
	}
	if app.today.snap.LocalDate != "2026-09-22" {
		t.Fatalf("today=%q want 2026-09-22 (older snapshot hijacked Today)", app.today.snap.LocalDate)
	}
	for _, tk := range app.today.tasks {
		if string(tk.ID) == "old-task" {
			t.Fatal("older snapshot tasks must not render as today")
		}
	}
	// The older snapshot still exists untouched (history preserved).
	snap, err := runtime.GetLatestTodaySnapshot(context.Background(), app.DB, "2026-09-20")
	if err != nil || snap.ID != "snap-old" {
		t.Fatalf("older snapshot must be preserved, got %+v %v", snap, err)
	}
}

// Strict numeric parsing: inputs must be consumed completely ("100abc"
// is rejected, never truncated to 100).
func TestMockStrictNumericInput(t *testing.T) {
	if _, err := parseOptInt("100abc"); err == nil {
		t.Fatal("100abc must be rejected")
	}
	if _, err := parseOptInt("10 20"); err == nil {
		t.Fatal("10 20 must be rejected")
	}
	if _, err := parseOptInt("-3"); err == nil {
		t.Fatal("-3 must be rejected")
	}
	if _, err := parseOptInt("1.5"); err == nil {
		t.Fatal("1.5 must be rejected for int fields")
	}
	if n, err := parseOptInt("100"); err != nil || n == nil || *n != 100 {
		t.Fatalf("100 must parse, got %v %v", n, err)
	}
	if n, err := parseOptInt(""); err != nil || n != nil {
		t.Fatalf("empty must stay missing (nil), got %v %v", n, err)
	}
	if _, err := parseOptFloat("100abc"); err == nil {
		t.Fatal("100abc must be rejected")
	}
	if _, err := parseOptFloat("NaN"); err == nil {
		t.Fatal("NaN must be rejected")
	}
	if f, err := parseOptFloat("100.5"); err != nil || f == nil || *f != 100.5 {
		t.Fatalf("100.5 must parse, got %v %v", f, err)
	}
	if f, err := parseOptFloat(""); err != nil || f != nil {
		t.Fatalf("empty must stay missing (nil), got %v %v", f, err)
	}
}
func TestTodayBrokenDBNoFallback(t *testing.T) {
	// A database failure must surface as a data error with nothing stale
	// rendered: no fallback to older snapshots, no fabricated Today.
	app := NewApp(seedOldSnapshotDB(t))
	app.width, app.height = 120, 36
	app.today.nowFn = func() time.Time { return time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC) }
	_ = app.DB.Close()
	app.today.Reload()
	if app.today.err == "" || !strings.Contains(app.today.err, "data error") {
		t.Fatalf("broken DB must surface a data error, got %q", app.today.err)
	}
	if len(app.today.tasks) != 0 {
		t.Fatalf("broken DB must render no stale tasks, got %d", len(app.today.tasks))
	}
}
