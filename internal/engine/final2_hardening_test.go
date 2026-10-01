package engine_test

// Second final-hardening regressions: repeat-state absence vs failure,
// weekly no-partial-data, lecture-completion failure propagation, review
// first-writer-wins, concurrent EnsureToday idempotence, strict DSA at the
// save boundary, exact day-end boundaries, and mock numeric tightening.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// ---- 1. Repeat state: no-row initializes, DB failure propagates ----

func TestRepeatStateFailurePropagates(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	now := time.Now()
	raw, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
	}})
	mkTaskDate(t, ctx, d, "rs-fail", "2026-09-20", db.DomainDSA, db.SessionKindDSA, raw)
	sess, err := engine.StartTodayTask(ctx, d, "rs-fail", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Genuinely failing repeat-state queries (not no-row).
	if _, err := d.ExecContext(ctx, `DROP TABLE dsa_repeat_state`); err != nil {
		t.Fatal(err)
	}
	_, err = engine.SaveDSAEvidence(ctx, d, "rs-fail", sess.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-4", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-2", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-3", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
	}, now)
	if err == nil {
		t.Fatal("repeat-state query failure must propagate, not initialize")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "repeat state") {
		t.Fatalf("failure must surface as repeat-state error, got %v", err)
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM dsa_attempts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("failed save must persist nothing, got %d attempts", n)
	}
}

// ---- 2/12. Weekly/daily reports fail whole, never partial ----

func TestReportAggregationFailureFails(t *testing.T) {
	newWeekDB := func(t *testing.T) *db.DB {
		t.Helper()
		d := openEngineDB(t)
		ctx := context.Background()
		seedGov(t, ctx, d)
		seedDSA(t, ctx, d)
		seedSWE(t, ctx, d)
		if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", time.Now()); err != nil {
			t.Fatal(err)
		}
		return d
	}
	ctx := context.Background()
	for _, table := range []string{"gov_lecture_completions", "gov_practice_records", "dsa_attempts"} {
		t.Run(table, func(t *testing.T) {
			d := newWeekDB(t)
			if _, err := d.ExecContext(ctx, `DROP TABLE `+table); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.BuildWeeklyDraft(ctx, d, "2026-W39", "2026-09-21", "2026-09-27"); err == nil {
				t.Fatalf("dropping %s must fail weekly generation, not partial data", table)
			}
		})
	}
	t.Run("daily broken db", func(t *testing.T) {
		broken, err := openBrokenDB(t)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.BuildDailyReport(ctx, broken, "2026-09-21"); err == nil {
			t.Fatal("daily report on broken DB must fail, not render empty")
		}
		if _, err := engine.BuildWeeklyDraft(ctx, broken, "2026-W39", "2026-09-21", "2026-09-27"); err == nil {
			t.Fatal("weekly draft on broken DB must fail, not render empty")
		}
	})
}

// ---- 3. Lecture completion: failure propagates, duplicates stay clean ----

func TestLectureCompletionFailurePropagation(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTaskDate(t, ctx, d, "lc1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	sess, err := engine.StartTodayTask(ctx, d, "lc1", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.RecordLectureCompletion(ctx, d, db.GovLectureCompletion{LectureID: "lec-1", SessionID: sess.ID, CompletedAt: now}); err != nil {
		t.Fatal(err)
	}
	// Duplicate stays a clean already-complete state, not a raw UNIQUE error.
	if err := runtime.RecordLectureCompletion(ctx, d, db.GovLectureCompletion{LectureID: "lec-1", SessionID: sess.ID, CompletedAt: now}); err == nil {
		t.Fatal("duplicate completion must be rejected")
	} else if strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate must not leak raw storage error: %v", err)
	}
	// Genuine DB failure propagates as a DB error, never "not complete".
	broken, err := openBrokenDB(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.RecordLectureCompletion(ctx, broken, db.GovLectureCompletion{LectureID: "lec-1", SessionID: sess.ID, CompletedAt: now}); err == nil {
		t.Fatal("pre-check query failure must propagate")
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, broken, "lc1", sess.ID, true, now); err == nil {
		t.Fatal("lecture save on broken DB must fail")
	}
}

// ---- 4. Review snapshot: first-writer-wins, reflections-only ----

func TestReviewSnapshotFirstWriterWins(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	draft := mustWeeklyDraft(t, ctx, d, "2026-W38", "2026-09-14", "2026-09-20")
	js, err := engine.MarshalWeeklySnapshot(draft)
	if err != nil {
		t.Fatal(err)
	}
	rowA := db.WeeklyReview{ID: "2026-W38", WeekStart: "2026-09-14", WeekEnd: "2026-09-20",
		PlannedSeconds: draft.Planned, ElapsedSeconds: draft.Actual,
		OverrunSeconds: draft.Actual - draft.Planned, ReportJSON: js}
	storedA, err := engine.SaveWeeklyReviewSnapshot(ctx, d, rowA, now)
	if err != nil {
		t.Fatal(err)
	}
	// Competing save with different metrics must NOT overwrite the frozen
	// snapshot; the authoritative first snapshot is returned instead.
	draftB := draft
	draftB.Planned += 100
	jsB, err := engine.MarshalWeeklySnapshot(draftB)
	if err != nil {
		t.Fatal(err)
	}
	rowB := rowA
	rowB.PlannedSeconds = draftB.Planned
	rowB.OverrunSeconds = rowB.ElapsedSeconds - rowB.PlannedSeconds
	rowB.ReportJSON = jsB
	got, err := engine.SaveWeeklyReviewSnapshot(ctx, d, rowB, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.PlannedSeconds != storedA.PlannedSeconds || got.ReportJSON != storedA.ReportJSON {
		t.Fatal("competing save must not replace frozen metrics")
	}
	raw, err := runtime.GetWeeklyReview(ctx, d, "2026-W38")
	if err != nil {
		t.Fatal(err)
	}
	if raw.PlannedSeconds != storedA.PlannedSeconds || raw.ReportJSON != storedA.ReportJSON {
		t.Fatal("stored row must keep first-writer metrics")
	}
	// Reflection edits still work and still preserve metrics.
	updated, err := engine.SaveReviewReflections(ctx, d, raw, "good week", "sleep more", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if updated.ReflectionWorked != "good week" || updated.ReflectionChange != "sleep more" {
		t.Fatalf("reflections not saved: %+v", updated)
	}
	if updated.PlannedSeconds != storedA.PlannedSeconds || updated.ReportJSON != storedA.ReportJSON {
		t.Fatal("reflection edit must preserve frozen metrics")
	}
	// Draft-path reflections freeze the snapshot on first write.
	draft2 := mustWeeklyDraft(t, ctx, d, "2026-W39", "2026-09-21", "2026-09-27")
	js2, err := engine.MarshalWeeklySnapshot(draft2)
	if err != nil {
		t.Fatal(err)
	}
	row2 := db.WeeklyReview{ID: "2026-W39", WeekStart: "2026-09-21", WeekEnd: "2026-09-27",
		PlannedSeconds: draft2.Planned, ElapsedSeconds: draft2.Actual,
		OverrunSeconds: draft2.Actual - draft2.Planned, ReportJSON: js2}
	via, err := engine.SaveReviewReflections(ctx, d, row2, "w", "c", now)
	if err != nil {
		t.Fatal(err)
	}
	if via.PlannedSeconds != row2.PlannedSeconds || via.ReportJSON != row2.ReportJSON {
		t.Fatal("draft reflection save must freeze the validated snapshot")
	}
}

// ---- 5. Concurrent EnsureToday converges on one snapshot ----

func TestEnsureTodayConcurrentIdempotent(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	const callers = 8
	ids := make([]db.ID, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			snap, _, err := engine.EnsureToday(ctx, d, "2026-09-22", now)
			if err == nil {
				ids[i] = snap.ID
			}
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("caller %d failed: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Fatalf("caller %d got %q want %q (same snapshot)", i, ids[i], ids[0])
		}
	}
	var n, revs int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT revision) FROM today_snapshots WHERE local_date = '2026-09-22'`).Scan(&n, &revs); err != nil {
		t.Fatal(err)
	}
	if n != 1 || revs != 1 {
		t.Fatalf("one logical snapshot required, rows=%d revs=%d", n, revs)
	}
}

// ---- 6/7. Strict DSA at save; legacy singular stays read-only ----

func TestDSASaveRejectsLegacySingular(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	now := time.Now()
	mkTaskDate(t, ctx, d, "sing", "2026-09-20", db.DomainDSA, db.SessionKindDSA, mustDSATarget(t, "dsa-1", db.DSAModeNew))
	sess, err := engine.StartTodayTask(ctx, d, "sing", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, err = engine.SaveDSAEvidence(ctx, d, "sing", sess.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
	}, now)
	if !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("legacy singular save must be ErrInvalidTarget, got %v", err)
	}
	// Historical read compatibility is preserved: evaluation still reads
	// the singular shape instead of erroring.
	if _, err := engine.EvaluateTodayTask(ctx, d, "sing"); err != nil {
		t.Fatalf("singular target must stay readable: %v", err)
	}
}

func TestDSABootstrapSaveSucceeds(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	now := time.Now()
	raw, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeNew},
		{ProblemID: "dsa-2", Mode: db.DSAModeNew},
		{ProblemID: "dsa-3", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
	}})
	mkTaskDate(t, ctx, d, "boot", "2026-09-20", db.DomainDSA, db.SessionKindDSA, raw)
	sess, err := engine.StartTodayTask(ctx, d, "boot", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ev, err := engine.SaveDSAEvidence(ctx, d, "boot", sess.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-2", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-3", Independent: false, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-4", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 120},
	}, now)
	if err != nil {
		t.Fatalf("truthful 4xNEW bootstrap save must succeed: %v", err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("bootstrap bundle must complete, got %s", ev.State)
	}
}

func TestDSAStaleNewRepeatStateOnlyAtSave(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	now := time.Now()
	// dsa-1 introduced via scheduling state only.
	if err := runtime.UpsertDSARepeatState(ctx, d, db.DSARepeatState{ProblemID: "dsa-1", RepeatStage: 1, CurrentIntervalDays: 3, NextReviewDate: "2026-09-21", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
	}})
	mkTaskDate(t, ctx, d, "stale-rs", "2026-09-21", db.DomainDSA, db.SessionKindDSA, raw)
	sess, err := engine.StartTodayTask(ctx, d, "stale-rs", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, err = engine.SaveDSAEvidence(ctx, d, "stale-rs", sess.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-4", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-2", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
		{ProblemID: "dsa-3", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
	}, now)
	if !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("repeat-state-only stale NEW must be ErrInvalidTarget, got %v", err)
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM dsa_attempts WHERE session_id = ?`, string(sess.ID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("stale save must persist nothing, got %d", n)
	}
}

// ---- Reviews: DB failure propagates (never validation verdicts) ----

func TestReviewDBFailurePropagates(t *testing.T) {
	ctx := context.Background()
	broken, err := openBrokenDB(t)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	row := db.WeeklyReview{ID: "2026-W38", WeekStart: "2026-09-14", WeekEnd: "2026-09-20",
		ReportJSON: `{"week_id":"2026-W38","week_start":"2026-09-14","week_end":"2026-09-20"}`}
	if _, err := engine.SaveWeeklyReviewSnapshot(ctx, broken, row, now); err == nil {
		t.Fatal("snapshot save on broken DB must fail")
	}
	if _, err := engine.SaveReviewReflections(ctx, broken, row, "w", "c", now); err == nil {
		t.Fatal("reflection save on broken DB must fail")
	}
}

// ---- 13. Exact day-end boundary: [start, end) ----

func TestDayEndBoundaryExact(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTaskDate(t, ctx, d, "edge", "2026-09-22", db.DomainGOV, db.SessionKindLecture, lec)
	// Session Sep 22 23:00 IST -> Sep 23 00:00 IST exactly.
	start := time.Date(2026, 9, 22, 23, 0, 0, 0, cal.Location())
	end := time.Date(2026, 9, 23, 0, 0, 0, 0, cal.Location())
	sess, err := engine.StartTodayTask(ctx, d, "edge", start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, end); err != nil {
		t.Fatal(err)
	}
	rep, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Activities) != 1 {
		t.Fatalf("activities=%d want 1", len(rep.Activities))
	}
	// Stopped exactly at dayEnd: completed interval, NOT active at boundary.
	if rep.Activities[0].State == engine.StateActive {
		t.Fatal("session stopped exactly at dayEnd must not be ACTIVE for that day")
	}
	if rep.Activities[0].Actual != 3600 {
		t.Fatalf("task actual=%d want 3600", rep.Activities[0].Actual)
	}
	next, err := engine.BuildDailyReport(ctx, d, "2026-09-23")
	if err != nil {
		t.Fatal(err)
	}
	if next.Sessions != 0 || next.Actual != 0 {
		t.Fatalf("boundary-stopped session must not leak into next day: %+v", next)
	}
	// Evidence recorded exactly at the boundary belongs to the next day.
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "edge", sess.ID, true, end); err != nil {
		t.Fatal(err)
	}
	again, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if again.Gov.LecturesCompleted != 0 {
		t.Fatal("boundary evidence must not count for the previous day")
	}
	next2, err := engine.BuildDailyReport(ctx, d, "2026-09-23")
	if err != nil {
		t.Fatal(err)
	}
	if next2.Gov.LecturesCompleted != 1 {
		t.Fatalf("boundary evidence belongs to next day, got %d", next2.Gov.LecturesCompleted)
	}
}

// ---- Cross-midnight task-level actual time ----

func TestCrossMidnightTaskActualSplit(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	raw, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio"}})
	mkTaskDate(t, ctx, d, "split", "2026-09-21", db.DomainGOV, db.SessionKindPractice, raw)
	start := time.Date(2026, 9, 21, 23, 50, 0, 0, cal.Location())
	end := time.Date(2026, 9, 22, 0, 20, 0, 0, cal.Location())
	if _, err := engine.StartTodayTask(ctx, d, "split", start); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, end); err != nil {
		t.Fatal(err)
	}
	rep21, err := engine.BuildDailyReport(ctx, d, "2026-09-21")
	if err != nil {
		t.Fatal(err)
	}
	rep22, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	taskActual := func(rep engine.DailyReport) int {
		for _, a := range rep.Activities {
			if string(a.TaskID) == "split" {
				return a.Actual
			}
		}
		return -1
	}
	if got := taskActual(rep21); got != 600 {
		t.Fatalf("sep21 task actual=%d want 600", got)
	}
	// The task row belongs to its plan day (Sep 21); Sep 22 carries only
	// its 20m portion in the day totals, never a duplicated task row.
	if got := taskActual(rep22); got != -1 {
		t.Fatalf("sep22 must not duplicate the task row, got actual=%d", got)
	}
	if rep22.Actual != 1200 || rep22.GovSeconds != 1200 {
		t.Fatalf("sep22 totals must carry the 20m portion: %+v", rep22)
	}
}

// ---- Mock schema contract: failure propagates, never unmapped partials ----

func dropGovSubjectIDColumn(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	if _, err := d.ExecContext(ctx, `DROP INDEX IF EXISTS idx_mock_sections_subject`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `ALTER TABLE mock_sections DROP COLUMN gov_subject_id`); err != nil {
		t.Fatal(err)
	}
}

func TestMockSchemaFailureNoPartial(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	// Current schema: normal creation with explicit mapping works.
	mk := db.Mock{ID: "m-ok", ExamID: "exam-1", Name: "OK", MockDate: "2026-09-21", CreatedAt: now}
	if _, err := engine.CreateMockAtomic(ctx, d, mk, []engine.MockSectionInput{
		{Name: "QA", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-perc"}},
	}, now); err != nil {
		t.Fatal(err)
	}
	// Break the schema contract the engine relies on.
	dropGovSubjectIDColumn(t, ctx, d)
	// Creation must fail outright, never persist an unmapped section.
	mk2 := db.Mock{ID: "m-bad", ExamID: "exam-1", Name: "BAD", MockDate: "2026-09-21", CreatedAt: now}
	if _, err := engine.CreateMockAtomic(ctx, d, mk2, []engine.MockSectionInput{
		{Name: "QA", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-perc"}},
	}, now); err == nil {
		t.Fatal("schema failure must propagate, not create unmapped sections")
	}
	var mocks, secs, weak int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM mocks WHERE id = 'm-bad'`).Scan(&mocks); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM mock_sections WHERE mock_id = 'm-bad'`).Scan(&secs); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM mock_weak_topics`).Scan(&weak); err != nil {
		t.Fatal(err)
	}
	if mocks != 0 || secs != 0 || weak != 1 {
		t.Fatalf("no partial state allowed: mocks=%d secs=%d weak=%d (want 0/0/1)", mocks, secs, weak)
	}
	// Weak-topic replacement must fail the same way, leaving prior state.
	var secID string
	if err := d.QueryRowContext(ctx, `SELECT id FROM mock_sections WHERE mock_id = 'm-ok'`).Scan(&secID); err != nil {
		t.Fatal(err)
	}
	if err := engine.ReplaceWeakTopicsAtomic(ctx, d, "m-ok", db.ID(secID), "subj-quant", []db.ID{"topic-ratio"}); err == nil {
		t.Fatal("schema failure on replace must propagate")
	}
	var kept int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM mock_weak_topics WHERE section_id = ?`, secID).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("failed replace must keep prior weak topics, got %d", kept)
	}
}

// ---- History enrichment: catalog failure is a data error, never silent ----

func TestHistoryEnrichmentFailureIsDataError(t *testing.T) {
	ctx := context.Background()
	t.Run("practice topic", func(t *testing.T) {
		d := openEngineDB(t)
		seedGov(t, ctx, d)
		// Dangling topic reference: evaluator reports ConfigError, but
		// enrichment must fail the report instead of rendering one name
		// as if the bundle were complete.
		raw, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio", "topic-NOPE"}})
		mkTaskDate(t, ctx, d, "enr-p", "2026-09-22", db.DomainGOV, db.SessionKindPractice, raw)
		if _, err := engine.BuildDailyReport(ctx, d, "2026-09-22"); err == nil {
			t.Fatal("unresolvable practice topic must fail the report, not render partial names")
		}
	})
	t.Run("dsa problem", func(t *testing.T) {
		d := openEngineDB(t)
		seedDSA(t, ctx, d)
		raw, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
			{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
			{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
			{ProblemID: "dsa-NOPE", Mode: db.DSAModeNew},
		}})
		mkTaskDate(t, ctx, d, "enr-d", "2026-09-22", db.DomainDSA, db.SessionKindDSA, raw)
		if _, err := engine.BuildDailyReport(ctx, d, "2026-09-22"); err == nil {
			t.Fatal("unresolvable DSA problem must fail the report, not render partial names")
		}
	})
	t.Run("swe area", func(t *testing.T) {
		d := openEngineDB(t)
		seedSWE(t, ctx, d)
		raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "no-area"})
		mkTaskDate(t, ctx, d, "enr-s", "2026-09-22", db.DomainSWE, db.SessionKindSWE, raw)
		if _, err := engine.BuildDailyReport(ctx, d, "2026-09-22"); err == nil {
			t.Fatal("unresolvable SWE area must fail the report, not render the title as detail")
		}
	})
	t.Run("healthy enrichment renders names", func(t *testing.T) {
		d := openEngineDB(t)
		seedGov(t, ctx, d)
		seedDSA(t, ctx, d)
		seedSWE(t, ctx, d)
		// Both tasks in ONE snapshot: the report renders the latest
		// snapshot's tasks with resolved bundle names.
		if err := runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: "snap-ok", LocalDate: "2026-09-22", Revision: 1, GeneratedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		praw, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio"}})
		if err := runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: "ok-p", SnapshotID: "snap-ok", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "P", PlannedSeconds: 3600, TargetJSON: &praw}); err != nil {
			t.Fatal(err)
		}
		draw, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
			{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
			{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
			{ProblemID: "dsa-3", Mode: db.DSAModeNew},
		}})
		if err := runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: "ok-d", SnapshotID: "snap-ok", Position: 2, Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "D", PlannedSeconds: 3600, TargetJSON: &draw}); err != nil {
			t.Fatal(err)
		}
		rep, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
		if err != nil {
			t.Fatal(err)
		}
		details := map[string]string{}
		for _, a := range rep.Activities {
			details[string(a.TaskID)] = a.Detail
		}
		if !strings.Contains(details["ok-p"], "Ratio") {
			t.Fatalf("practice detail must name topics, got %q", details["ok-p"])
		}
		if !strings.Contains(details["ok-d"], "Two Sum") {
			t.Fatalf("dsa detail must name problems, got %q", details["ok-d"])
		}
	})
}

// ---- 14. Mock negatives rejected before writes ----

func TestMockNegativeMetricsRejected(t *testing.T) {
	neg := -5
	for _, m := range []db.Mock{
		{Score: &neg},
		{Attempted: &neg},
		{Correct: &neg},
		{Incorrect: &neg},
		{Unattempted: &neg},
		{TotalScore: &neg},
		{TotalQuestions: &neg},
		{RankValue: &neg},
	} {
		if err := engine.ValidateMockMetrics(m); !errors.Is(err, engine.ErrValidation) {
			t.Fatalf("negative metric must be ErrValidation, got %v (%+v)", err, m)
		}
	}
	// Atomic creation path rejects negatives without writing.
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d) // includes exam-1
	now := time.Now()
	score := -1
	if _, err := engine.CreateMockAtomic(ctx, d, db.Mock{ID: "m-neg", ExamID: "exam-1", Name: "N", MockDate: "2026-09-21", CreatedAt: now, Score: &score},
		[]engine.MockSectionInput{{Name: "S", GovSubjectID: "subj-quant"}}, now); !errors.Is(err, engine.ErrValidation) {
		t.Fatalf("atomic create must reject negative score, got %v", err)
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM mocks WHERE id = 'm-neg'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("rejected mock must not be created")
	}
}
