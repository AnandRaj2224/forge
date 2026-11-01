package engine_test

// Final authority-hardening regressions: competing paths eliminated,
// absence-vs-failure semantics, historical cutoffs, exhausted-area skips,
// review snapshot immutability, revision-collision translation, calendar
// verification, and weighted aggregation.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// ---- Today authority: older snapshot never becomes current Today ----

func TestEnsureTodayOlderSnapshotGeneratesCurrent(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	now := time.Now()
	old, _, err := engine.GenerateToday(ctx, d, "2026-09-20", now)
	if err != nil {
		t.Fatal(err)
	}
	cur, _, err := engine.EnsureToday(ctx, d, "2026-09-22", now)
	if err != nil {
		t.Fatal(err)
	}
	if cur.LocalDate != "2026-09-22" || cur.Revision != 1 {
		t.Fatalf("must generate current date, got %+v", cur)
	}
	if cur.ID == old.ID {
		t.Fatal("older snapshot must not become current Today")
	}
}

// ---- Routine lookup failures propagate, never default ----

func TestRoutineDBFailurePropagates(t *testing.T) {
	newSeeded := func(t *testing.T) *db.DB {
		t.Helper()
		d := openEngineDB(t)
		ctx := context.Background()
		seedGov(t, ctx, d)
		seedDSA(t, ctx, d)
		return d
	}
	ctx := context.Background()
	now := time.Now()
	t.Run("lecture routine", func(t *testing.T) {
		d := newSeeded(t)
		if _, err := d.ExecContext(ctx, `DROP TABLE gov_lecture_routine`); err != nil {
			t.Fatal(err)
		}
		if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", now); err == nil {
			t.Fatal("lecture routine query failure must propagate, not default")
		}
	})
	t.Run("practice routine", func(t *testing.T) {
		d := newSeeded(t)
		if _, err := d.ExecContext(ctx, `DROP TABLE gov_practice_routine`); err != nil {
			t.Fatal(err)
		}
		if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", now); err == nil {
			t.Fatal("practice routine query failure must propagate, not vanish")
		}
	})
	t.Run("dsa routine", func(t *testing.T) {
		d := newSeeded(t)
		if _, err := d.ExecContext(ctx, `DROP TABLE dsa_routine`); err != nil {
			t.Fatal(err)
		}
		if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", now); err == nil {
			t.Fatal("dsa routine query failure must propagate, not default")
		}
	})
}

// ---- Target validation: absence vs failure ----

func TestTargetValidationAbsenceVsFailure(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	broken, err := openBrokenDB(t)
	if err != nil {
		t.Fatal(err)
	}
	badLecture := db.GovLectureTarget{SubjectID: "no-subj", TopicID: "no-topic", LectureID: "no-lec"}
	if err := engine.ValidateGovLectureTarget(ctx, d, badLecture); !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("unknown subject must be ErrInvalidTarget, got %v", err)
	}
	if err := engine.ValidateGovLectureTarget(ctx, broken, badLecture); err == nil || errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("DB failure must not read as invalid target, got %v", err)
	}
	badPractice := db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"no-topic"}}
	if err := engine.ValidateGovPracticeTarget(ctx, d, badPractice); !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("unknown topic must be ErrInvalidTarget, got %v", err)
	}
	if err := engine.ValidateGovPracticeTarget(ctx, broken, badPractice); err == nil || errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("DB failure must not read as invalid target, got %v", err)
	}
	badDSA := db.DSATarget{Problems: []db.DSAProblemTarget{{ProblemID: "no-prob", Mode: db.DSAModeNew}}}
	if err := engine.ValidateDSATarget(ctx, d, badDSA); !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("unknown problem must be ErrInvalidTarget, got %v", err)
	}
	if err := engine.ValidateDSATarget(ctx, broken, badDSA); err == nil || errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("DB failure must not read as invalid target, got %v", err)
	}
	badSec := engine.MockSectionInput{Name: "QA", GovSubjectID: "no-subj"}
	if err := engine.ValidateMockSection(ctx, d, "exam-1", badSec); !errors.Is(err, engine.ErrValidation) {
		t.Fatalf("unknown subject must be validation error, got %v", err)
	}
	if err := engine.ValidateMockSection(ctx, broken, "exam-1", badSec); err == nil || errors.Is(err, engine.ErrValidation) {
		t.Fatalf("DB failure must not read as validation error, got %v", err)
	}
}

// mkTaskDate creates a task on date, allocating the next revision when the
// date already has snapshots (mkTask always uses revision 1, so same-date
// callers must use this helper).
func mkTaskDate(t *testing.T, ctx context.Context, d *db.DB, taskID, date string, domain db.Domain, kind db.SessionKind, target string) {
	t.Helper()
	rev := 1
	if snap, err := runtime.GetLatestTodaySnapshot(ctx, d, date); err == nil {
		rev = snap.Revision + 1
	}
	snapID := db.ID("snap-" + taskID)
	if err := runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snapID, LocalDate: date, Revision: rev, GeneratedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	tj := target
	if err := runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: db.ID(taskID), SnapshotID: snapID, Position: 1, Domain: domain, Kind: kind, Title: "T", PlannedSeconds: 3600, TargetJSON: &tj}); err != nil {
		t.Fatal(err)
	}
}

// ---- Evaluator: DB failure is an error, invalid target is ConfigError ----

func TestEvaluatorAbsenceVsFailure(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	// Target references a lecture that does not exist: invalid target, and
	// the evaluator must report ConfigError with a nil error.
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-missing"})
	mkTaskDate(t, ctx, d, "ev-bad", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	ev, err := engine.EvaluateTodayTask(ctx, d, "ev-bad")
	if err != nil {
		t.Fatalf("invalid target must not error, got %v", err)
	}
	if ev.ConfigError == "" {
		t.Fatal("invalid target must surface ConfigError")
	}
	// Catalogue query failure must propagate as an error, never fold into
	// ConfigError/TODO. A closed handle fails every query with a real DB
	// error (never sql.ErrNoRows).
	broken, err := openBrokenDB(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.EvaluateTodayTask(ctx, broken, "ev-bad"); err == nil {
		t.Fatal("evaluator DB failure must propagate as error")
	}
}

// ---- Weekly historical cutoff: later completion never rewrites the week ----

func TestWeeklyHistoricalCutoff(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTaskDate(t, ctx, d, "whist", "2026-09-15", db.DomainGOV, db.SessionKindLecture, lec)
	start := time.Date(2026, 9, 15, 10, 0, 0, 0, cal.Location())
	sess, err := engine.StartTodayTask(ctx, d, "whist", start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Completion recorded AFTER the week ended (Mon 2026-09-21 10:00 IST).
	recorded := time.Date(2026, 9, 21, 10, 0, 0, 0, cal.Location())
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "whist", sess.ID, true, recorded); err != nil {
		t.Fatal(err)
	}
	if ev, err := engine.EvaluateTodayTask(ctx, d, "whist"); err != nil || ev.State != engine.StateCompleted {
		t.Fatalf("current state must be completed: %+v %v", ev, err)
	}
	draft, err := engine.BuildWeeklyDraft(ctx, d, "2026-W38", "2026-09-14", "2026-09-20")
	if err != nil {
		t.Fatal(err)
	}
	if draft.TasksTotal != 1 {
		t.Fatalf("week tasks=%d want 1", draft.TasksTotal)
	}
	if draft.TasksCompleted != 0 {
		t.Fatal("completion after week end must not rewrite the historical week")
	}
	if draft.GovLectures != 0 {
		t.Fatal("evidence recorded after week end must not enter weekly evidence")
	}
}

// ---- Weekly session count uses interval overlap ----

func TestWeeklyOverlapSessionCount(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTaskDate(t, ctx, d, "wov", "2026-09-13", db.DomainGOV, db.SessionKindLecture, lec)
	// Sep 13 23:50 IST -> Sep 14 00:20 IST: 10m belongs to Sep 13, 20m to Sep 14.
	start := time.Date(2026, 9, 13, 23, 50, 0, 0, cal.Location())
	end := time.Date(2026, 9, 14, 0, 20, 0, 0, cal.Location())
	sess, err := engine.StartTodayTask(ctx, d, "wov", start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, end); err != nil {
		t.Fatal(err)
	}
	_ = sess
	week, err := engine.BuildWeeklyDraft(ctx, d, "2026-W38", "2026-09-14", "2026-09-20")
	if err != nil {
		t.Fatal(err)
	}
	if week.Sessions != 1 {
		t.Fatalf("overlapping session must count for the touched week, got %d", week.Sessions)
	}
	if week.Actual != 1200 {
		t.Fatalf("week actual=%d want 1200 (20m overlap)", week.Actual)
	}
	prev, err := engine.BuildWeeklyDraft(ctx, d, "2026-W37", "2026-09-07", "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if prev.Sessions != 1 {
		t.Fatalf("overlapping session must count for the prior week, got %d", prev.Sessions)
	}
	if prev.Actual != 600 {
		t.Fatalf("prior week actual=%d want 600 (10m overlap)", prev.Actual)
	}
}

// ---- Review engine semantics: frozen metrics, reflection-only edits ----

func mustWeeklyDraft(t *testing.T, ctx context.Context, d *db.DB, id, start, end string) engine.WeeklyReport {
	t.Helper()
	rep, err := engine.BuildWeeklyDraft(ctx, d, id, start, end)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestReviewEngineSemantics(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, cal.Location())
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTaskDate(t, ctx, d, "rev1", "2026-09-15", db.DomainGOV, db.SessionKindLecture, lec)
	sess, err := engine.StartTodayTask(ctx, d, "rev1", now.Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(-47*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "rev1", sess.ID, true, now.Add(-47*time.Hour)); err != nil {
		t.Fatal(err)
	}
	draft := mustWeeklyDraft(t, ctx, d, "2026-W38", "2026-09-14", "2026-09-20")
	js, err := engine.MarshalWeeklySnapshot(draft)
	if err != nil {
		t.Fatal(err)
	}
	row := db.WeeklyReview{ID: "2026-W38", WeekStart: "2026-09-14", WeekEnd: "2026-09-20",
		PlannedSeconds: draft.Planned, ElapsedSeconds: draft.Actual,
		OverrunSeconds: draft.Actual - draft.Planned, ReportJSON: js}
	stored, err := engine.SaveWeeklyReviewSnapshot(ctx, d, row, now)
	if err != nil {
		t.Fatal(err)
	}
	// Placeholder snapshots are rejected, never stored.
	bad := row
	bad.ReportJSON = "{}"
	if _, err := engine.SaveWeeklyReviewSnapshot(ctx, d, bad, now); err == nil {
		t.Fatal("placeholder report snapshot must be rejected")
	}
	// More evidence afterwards must not move the frozen snapshot.
	lec2, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-reasoning", TopicID: "topic-series", LectureID: "lec-series-1"})
	mkTaskDate(t, ctx, d, "rev2", "2026-09-16", db.DomainGOV, db.SessionKindLecture, lec2)
	sess2, err := engine.StartTodayTask(ctx, d, "rev2", now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(-23*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "rev2", sess2.ID, true, now.Add(-23*time.Hour)); err != nil {
		t.Fatal(err)
	}
	fresh := mustWeeklyDraft(t, ctx, d, "2026-W38", "2026-09-14", "2026-09-20")
	if fresh.TasksCompleted != 2 {
		t.Fatalf("live draft must move with new evidence, completed=%d", fresh.TasksCompleted)
	}
	reloaded, err := runtime.GetWeeklyReview(ctx, d, "2026-W38")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ReportJSON != stored.ReportJSON || reloaded.PlannedSeconds != stored.PlannedSeconds {
		t.Fatal("saved metrics must stay frozen while underlying rows change")
	}
	// Reflection edits preserve the metric snapshot even when the caller
	// passes tampered scalars.
	tampered := reloaded
	tampered.PlannedSeconds = 1
	tampered.ReportJSON = "{}"
	updated, err := engine.SaveReviewReflections(ctx, d, tampered, "worked", "change", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if updated.PlannedSeconds != stored.PlannedSeconds || updated.ReportJSON != stored.ReportJSON {
		t.Fatal("reflection edits must not touch the metric snapshot")
	}
	if updated.ReflectionWorked != "worked" || updated.ReflectionChange != "change" {
		t.Fatalf("reflections not saved: %+v", updated)
	}
}

// ---- Start concurrency: clean domain error, never duplicates ----

func TestStartConcurrencyCleanBusy(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTaskDate(t, ctx, d, "cc1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	mkTaskDate(t, ctx, d, "cc2", "2026-09-21", db.DomainGOV, db.SessionKindLecture, lec)
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "cc1", now); err != nil {
		t.Fatal(err)
	}
	// A racing second start (same or other task) maps to the domain-level
	// busy error: the partial unique index stays the final guard, raw
	// SQLite uniqueness errors never leak, duplicates never form.
	if _, err := engine.StartTodayTask(ctx, d, "cc2", now); !errors.Is(err, engine.ErrActiveSessionBusy) {
		t.Fatalf("busy start must be ErrActiveSessionBusy, got %v", err)
	}
	if _, err := engine.StartTodayTask(ctx, d, "cc1", now); !errors.Is(err, engine.ErrTaskActive) {
		t.Fatalf("re-start of active task must be ErrTaskActive, got %v", err)
	}
	var total, active int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE status = 'ACTIVE'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if total != 1 || active != 1 {
		t.Fatalf("exactly one session must exist, total=%d active=%d", total, active)
	}
	// The DB guard itself: a raw second ACTIVE row violates the partial
	// unique index (proves the race safety net the engine translates).
	if _, err := d.ExecContext(ctx, `INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, planned_seconds, started_at, status) VALUES ('raw-dup', 'cc2', 'GOV', 'LECTURE', 'x', 60, ?, 'ACTIVE')`, now.Unix()); err == nil {
		t.Fatal("partial unique index must reject a second ACTIVE session")
	}
}

// ---- Generation sabotage matrix: every phase-2 write rolls back ----

func sabotageSetup(t *testing.T, ctx context.Context, d *db.DB, now time.Time) {
	t.Helper()
	// Make the plan carry activations AND a rotation write: activate
	// English and complete its lecture so round-robin has a third subject.
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-english", now); err != nil {
		t.Fatal(err)
	}
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-english", TopicID: "topic-eng1", LectureID: "lec-eng-1"})
	mkTaskDate(t, ctx, d, "sab-eng", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	sess, err := engine.StartTodayTask(ctx, d, "sab-eng", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "sab-eng", sess.ID, true, now); err != nil {
		t.Fatal(err)
	}
}

func assertNoPartialToday(t *testing.T, ctx context.Context, d *db.DB, date string, actBefore, rotBefore int) {
	t.Helper()
	if _, err := runtime.GetLatestTodaySnapshot(ctx, d, date); err == nil {
		t.Fatal("partial snapshot must roll back")
	}
	var tasks int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_tasks WHERE snapshot_id LIKE ?`, "today-"+date+"%").Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 {
		t.Fatalf("partial tasks must roll back, got %d", tasks)
	}
	var actAfter int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation`).Scan(&actAfter); err != nil {
		t.Fatal(err)
	}
	if actAfter != actBefore {
		t.Fatalf("orphan activation: before=%d after=%d", actBefore, actAfter)
	}
	var rotAfter int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_rotation`).Scan(&rotAfter); err != nil {
		t.Fatal(err)
	}
	if rotAfter != rotBefore {
		t.Fatalf("consumed rotation state must roll back: before=%d after=%d", rotBefore, rotAfter)
	}
}

func TestGenerationSabotageMatrix(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	t.Run("snapshot failure", func(t *testing.T) {
		d := openEngineDB(t)
		seedGov(t, ctx, d)
		seedDSA(t, ctx, d)
		sabotageSetup(t, ctx, d, now)
		var actBefore int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation`).Scan(&actBefore); err != nil {
			t.Fatal(err)
		}
		var rotBefore int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_rotation`).Scan(&rotBefore); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ExecContext(ctx, `CREATE TRIGGER sab_snap BEFORE INSERT ON today_snapshots BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
			t.Fatal(err)
		}
		if _, _, err := engine.GenerateToday(ctx, d, "2026-09-22", now); err == nil {
			t.Fatal("sabotaged snapshot insert must fail")
		}
		assertNoPartialToday(t, ctx, d, "2026-09-22", actBefore, rotBefore)
	})
	t.Run("activation failure", func(t *testing.T) {
		d := openEngineDB(t)
		seedGov(t, ctx, d)
		seedDSA(t, ctx, d)
		sabotageSetup(t, ctx, d, now)
		var actBefore int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation`).Scan(&actBefore); err != nil {
			t.Fatal(err)
		}
		var rotBefore int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_rotation`).Scan(&rotBefore); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ExecContext(ctx, `CREATE TRIGGER sab_act BEFORE INSERT ON gov_subject_activation BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
			t.Fatal(err)
		}
		// Fresh subject activation is required by the lecture plan; the
		// abort must roll back snapshot, tasks and rotation together.
		if _, _, err := engine.GenerateToday(ctx, d, "2026-09-22", now); err == nil {
			t.Fatal("sabotaged activation must fail")
		}
		assertNoPartialToday(t, ctx, d, "2026-09-22", actBefore, rotBefore)
	})
	t.Run("rotation failure", func(t *testing.T) {
		d := openEngineDB(t)
		seedGov(t, ctx, d)
		seedDSA(t, ctx, d)
		sabotageSetup(t, ctx, d, now)
		var actBefore int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation`).Scan(&actBefore); err != nil {
			t.Fatal(err)
		}
		var rotBefore int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_rotation`).Scan(&rotBefore); err != nil {
			t.Fatal(err)
		}
		if _, err := d.ExecContext(ctx, `CREATE TRIGGER sab_rot BEFORE INSERT ON gov_practice_rotation BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
			t.Fatal(err)
		}
		if _, _, err := engine.GenerateToday(ctx, d, "2026-09-22", now); err == nil {
			t.Fatal("sabotaged rotation write must fail")
		}
		assertNoPartialToday(t, ctx, d, "2026-09-22", actBefore, rotBefore)
	})
}

// ---- Revision collision translates cleanly, never partial ----

func TestRevisionCollisionNoPartial(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	now := time.Now()
	// Poison the deterministic next ID: a foreign row owns the PK that the
	// next 2026-09-22 generation would use, forcing UNIQUE collisions on
	// both attempts.
	if _, err := d.ExecContext(ctx, `INSERT INTO today_snapshots (id, local_date, revision, generated_at) VALUES ('today-2026-09-22-r1', '2026-09-01', 1, ?)`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.GenerateToday(ctx, d, "2026-09-22", now); err == nil {
		t.Fatal("revision collision must surface, not duplicate")
	} else if !strings.Contains(strings.ToLower(err.Error()), "revision conflict") {
		t.Fatalf("collision must translate to a domain-level error, got %v", err)
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_tasks WHERE snapshot_id = 'today-2026-09-22-r1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("collided generation must persist no tasks, got %d", n)
	}
}

// ---- Calendar source of truth ----

func TestCalendarSettingsVerification(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := engine.VerifyCalendarSettings(ctx, d); err != nil {
		t.Fatalf("canonical settings must verify: %v", err)
	}
	if err := catalog.UpsertSettings(ctx, d, db.ForgeSettings{ID: 1, Timezone: "Asia/Kolkata", DayStartMinute: 600, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.VerifyCalendarSettings(ctx, d); !errors.Is(err, engine.ErrValidation) {
		t.Fatalf("non-midnight boundary must fail validation, got %v", err)
	}
	if err := catalog.UpsertSettings(ctx, d, db.ForgeSettings{ID: 1, Timezone: "UTC", DayStartMinute: 0, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.VerifyCalendarSettings(ctx, d); !errors.Is(err, engine.ErrValidation) {
		t.Fatalf("non-Kolkata timezone must fail validation, got %v", err)
	}
}

// ---- DSA NEW truthfulness includes repeat-state-only introduction ----

func TestDSARepeatStateOnlyStaleNew(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	now := time.Now()
	// dsa-2 introduced via scheduling state only (no attempts yet).
	if err := runtime.UpsertDSARepeatState(ctx, d, db.DSARepeatState{ProblemID: "dsa-2", RepeatStage: 1, CurrentIntervalDays: 3, NextReviewDate: "2026-09-21", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-2", Mode: db.DSAModeNew},
	}})
	dt, _ := db.UnmarshalDSATarget(raw)
	if err := engine.ValidateDSACurrentBundle(ctx, d, dt); !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("repeat-state-only problem labeled NEW must be stale, got %v", err)
	}
}

// ---- Weighted speed + empty-day semantics ----

func TestWeightedSpeedAndEmptyDay(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, cal.Location())
	savePractice := func(task, date string, att int, avg float64, at time.Time) {
		t.Helper()
		raw, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio"}})
		mkTaskDate(t, ctx, d, task, date, db.DomainGOV, db.SessionKindPractice, raw)
		sess, err := engine.StartTodayTask(ctx, d, db.ID(task), at)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.StopActiveSession(ctx, d, at.Add(10*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.SaveGovPracticeEvidence(ctx, d, db.ID(task), sess.ID,
			[]engine.GovPracticeInput{{TopicID: "topic-ratio", TimeSecs: 600, Attempted: att, Correct: att, AvgSpeed: &avg}}, at); err != nil {
			t.Fatal(err)
		}
	}
	// 10 questions x 10s + 100 questions x 20s -> weighted ~19.09s, not 15s.
	savePractice("w1", "2026-09-21", 10, 10.0, now)
	savePractice("w2", "2026-09-21", 100, 20.0, now.Add(time.Hour))
	rep, err := engine.BuildDailyReport(ctx, d, "2026-09-21")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Gov.AvgSpeed == nil {
		t.Fatal("weighted speed must be present")
	}
	if got := *rep.Gov.AvgSpeed; got < 19.0 || got > 19.2 {
		t.Fatalf("weighted speed=%v want ~19.09", got)
	}
	// Empty day: zero attempted, nil accuracy/speed (—), never 0.0%.
	empty, err := engine.BuildDailyReport(ctx, d, "2026-09-19")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Gov.QuestionsAttempted != 0 {
		t.Fatalf("attempted=%d want 0", empty.Gov.QuestionsAttempted)
	}
	if empty.Gov.Accuracy != nil || empty.Gov.AvgSpeed != nil || empty.Gov.HasSpeed {
		t.Fatal("no data must be nil/—, never 0")
	}
	if empty.TasksTotal != 0 || empty.TasksCompleted != 0 {
		t.Fatalf("empty day tasks=%d/%d want 0/0", empty.TasksCompleted, empty.TasksTotal)
	}
}
