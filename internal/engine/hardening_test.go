package engine_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// openBrokenDB returns a closed handle: every query fails with a real DB
// error (never sql.ErrNoRows), so tests can prove failures propagate
// instead of being treated as missing state.
func openBrokenDB(t *testing.T) (*db.DB, error) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "broken.db"))
	if err != nil {
		return nil, err
	}
	if err := d.Migrate(); err != nil {
		_ = d.Close()
		return nil, err
	}
	if err := d.Close(); err != nil {
		return nil, err
	}
	return d, nil
}

// ---- GOV practice eligibility (locked rule) ----

func TestPracticeEligibilityBasics(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	// topic-perc: lec-1 incomplete -> excluded.
	elig, err := engine.PracticeEligibleTopics(ctx, d, "subj-quant")
	if err != nil {
		t.Fatal(err)
	}
	got := map[db.ID]bool{}
	for _, tp := range elig {
		got[tp.ID] = true
	}
	if got["topic-perc"] {
		t.Fatal("incomplete-lecture topic must be excluded")
	}
	// Zero-lecture topics are eligible.
	if !got["topic-ratio"] || !got["topic-ns"] {
		t.Fatalf("zero-lecture topics must be eligible, got %v", got)
	}
	// Reasoning series incomplete -> no eligible topics.
	elig, err = engine.PracticeEligibleTopics(ctx, d, "subj-reasoning")
	if err != nil {
		t.Fatal(err)
	}
	if len(elig) != 0 {
		t.Fatalf("incomplete track must yield none, got %v", elig)
	}
	// Complete lec-1 -> Percentage becomes eligible.
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "elig-lec", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	now := time.Now()
	sess, err := engine.StartTodayTask(ctx, d, "elig-lec", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "elig-lec", sess.ID, true, now); err != nil {
		t.Fatal(err)
	}
	elig, err = engine.PracticeEligibleTopics(ctx, d, "subj-quant")
	if err != nil {
		t.Fatal(err)
	}
	got = map[db.ID]bool{}
	for _, tp := range elig {
		got[tp.ID] = true
	}
	if !got["topic-perc"] {
		t.Fatal("completed-lecture topic must be eligible")
	}
}

func TestPracticeSlotSkippedWhenNoEligible(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	// Reasoning has no eligible topic -> no Reasoning practice task.
	for _, tk := range tasks {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindPractice {
			continue
		}
		pt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
		subj, _ := catalog.GetGovSubject(ctx, d, pt.SubjectID)
		if subj.Name == "Reasoning" {
			t.Fatal("subject with no eligible topic must be skipped")
		}
		if len(pt.TopicIDs) == 0 {
			t.Fatal("empty practice bundle generated")
		}
	}
}

func TestComputerAwarenessBlockedUntilActivation(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	// Inactive Computer Awareness with a zero-lecture topic. It is a member
	// of the current exam's subject set but must stay unselected until
	// activation.
	if err := catalog.InsertGovSubject(ctx, d, db.GovSubject{ID: "subj-ca", Name: "Computer Awareness", SortOrder: 9}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "topic-ca1", SubjectID: "subj-ca", Name: "Hardware", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: "exam-1", SubjectID: "subj-ca", SortOrder: 4}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			pt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if pt.SubjectID == "subj-ca" {
				t.Fatal("inactive subject must not appear before activation")
			}
		}
	}
	// After activation the zero-lecture topic is eligible, but V2 practice
	// uses fixed Quant/English/Reasoning slots: a non-fixed subject never
	// takes a practice slot.
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-ca", now); err != nil {
		t.Fatal(err)
	}
	_, tasks2, err := engine.GenerateToday(ctx, d, "2026-09-22", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks2 {
		if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindPractice {
			continue
		}
		pt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
		if pt.SubjectID == "subj-ca" {
			t.Fatal("fixed-slot practice must never include non-fixed subjects")
		}
	}
}

// ---- transactional generation ----

func TestGenerationRollback(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	// Make English eligible for slot 3 so the plan carries an activation
	// batch and a rotation write.
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-english", now); err != nil {
		t.Fatal(err)
	}
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-english", TopicID: "topic-eng1", LectureID: "lec-eng-1"})
	mkTask(t, ctx, d, "rb-eng", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	sess, err := engine.StartTodayTask(ctx, d, "rb-eng", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "rb-eng", sess.ID, true, now); err != nil {
		t.Fatal(err)
	}
	var actBefore int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation`).Scan(&actBefore); err != nil {
		t.Fatal(err)
	}
	var rotBefore int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_rotation`).Scan(&rotBefore); err != nil {
		t.Fatal(err)
	}
	// Sabotage the final transaction: plan reads still work, but the
	// snapshot/task commit must fail and roll back everything. An abort
	// trigger fails the write phase without disturbing any plan read.
	if _, err := d.ExecContext(ctx, `CREATE TRIGGER rb_fail BEFORE INSERT ON today_tasks BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.GenerateToday(ctx, d, "2026-09-22", now); err == nil {
		t.Fatal("sabotaged generation must fail")
	}
	// No partial state: no snapshot, no new activations, no rotation row.
	if _, err := runtime.GetLatestTodaySnapshot(ctx, d, "2026-09-22"); err == nil {
		t.Fatal("partial snapshot must roll back")
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

// ---- EnsureToday not-found vs DB failure ----

func TestEnsureTodayNotFoundVsDBFailure(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	// Genuine no-row: generates.
	snap, _, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Revision != 1 {
		t.Fatalf("rev=%d", snap.Revision)
	}
	// Second call: latest snapshot returned, no new revision.
	snap2, _, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if snap2.ID != snap.ID || snap2.Revision != 1 {
		t.Fatalf("EnsureToday must not duplicate: %+v", snap2)
	}
	// Real DB failure must propagate, never trigger generation.
	broken, err := openBrokenDB(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.EnsureToday(ctx, broken, "2026-09-21", now); err == nil {
		t.Fatal("DB failure must propagate, not generate")
	}
}

func TestLatestSnapshotPerDaySemantics(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", now); err != nil {
		t.Fatal(err)
	}
	snap2, _, err := engine.GenerateToday(ctx, d, "2026-09-21", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if snap2.Revision != 2 {
		t.Fatalf("rev=%d", snap2.Revision)
	}
	got, _, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != snap2.ID {
		t.Fatal("latest revision must win")
	}
}

// ---- sessions ----

func TestCompletedCannotStart(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "done1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	now := time.Now()
	sess, err := engine.StartTodayTask(ctx, d, "done1", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "done1", sess.ID, true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StartTodayTask(ctx, d, "done1", now.Add(2*time.Hour)); !errors.Is(err, engine.ErrTaskCompleted) {
		t.Fatalf("completed start must return ErrTaskCompleted, got %v", err)
	}
}

func TestStopThenNewSessionLeavesOldUnchanged(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "sess1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	now := time.Now()
	s1, err := engine.StartTodayTask(ctx, d, "sess1", now)
	if err != nil {
		t.Fatal(err)
	}
	stopAt := now.Add(time.Hour)
	closed, err := engine.StopActiveSession(ctx, d, stopAt)
	if err != nil {
		t.Fatal(err)
	}
	if closed.ID != s1.ID {
		t.Fatal("stop must close the same session")
	}
	// Stopped sessions never resume: a later start creates a NEW session.
	s2, err := engine.StartTodayTask(ctx, d, "sess1", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if s2.ID == s1.ID {
		t.Fatal("later start must create a new session")
	}
	old, err := runtime.GetSession(ctx, d, s1.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Storage keeps Unix seconds: compare truncated timestamps.
	if old.StartedAt.Unix() != s1.StartedAt.Unix() || old.StoppedAt == nil || old.StoppedAt.Unix() != stopAt.Unix() {
		t.Fatalf("old session mutated: %+v", old)
	}
	if _, err := engine.StopActiveSession(ctx, d, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestRestartWithActiveSession(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "restart1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	now := time.Now()
	s1, err := engine.StartTodayTask(ctx, d, "restart1", now)
	if err != nil {
		t.Fatal(err)
	}
	// Application restart: load persisted ACTIVE state, never duplicate.
	loaded, err := engine.LoadActiveSession(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.ID != s1.ID {
		t.Fatalf("restart must return same session, got %+v", loaded)
	}
	// Storage keeps Unix seconds: compare truncated timestamps.
	if loaded.StartedAt.Unix() != s1.StartedAt.Unix() {
		t.Fatal("restart must not reset started_at")
	}
	loaded2, err := engine.LoadActiveSession(ctx, d)
	if err != nil || loaded2 == nil || loaded2.ID != s1.ID {
		t.Fatal("second load must return the same session")
	}
	all, err := runtime.ListSessions(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("no duplicate session may be created, got %d", len(all))
	}
	// No active session -> (nil, nil), never an error.
	if _, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	none, err := engine.LoadActiveSession(ctx, d)
	if err != nil || none != nil {
		t.Fatalf("no active must be (nil,nil), got %+v %v", none, err)
	}
}

func TestIdempotentActivation(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-quant", now); err != nil {
		t.Fatal(err)
	}
	if err := engine.EnsureGovSubjectActive(ctx, d, "subj-quant", now); err != nil {
		t.Fatalf("repeated activation must be idempotent: %v", err)
	}
	var n int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation WHERE subject_id = 'subj-quant'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("activation rows=%d", n)
	}
}

// ---- evidence membership ----

func TestPracticeTargetMembership(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-perc", "topic-ratio"}})
	mkTask(t, ctx, d, "memb1", "2026-09-20", db.DomainGOV, db.SessionKindPractice, prac)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "memb1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	avg := 20.0
	// Off-target topic must fail with nothing persisted.
	_, err := engine.SaveGovPracticeEvidence(ctx, d, "memb1", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-series", TimeSecs: 100, Attempted: 5, Correct: 3, AvgSpeed: &avg},
	}, now)
	if err == nil {
		t.Fatal("off-target evidence must fail")
	}
	var n int
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_records WHERE session_id = ?`, string(sess.ID)).Scan(&n)
	if n != 0 {
		t.Fatalf("failed save must persist nothing, got %d", n)
	}
}

func TestSWEStaysTodoWithoutEvidenceForm(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWE(t, ctx, d)
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	// Partial time stays TODO: mkTask plans 3600, stop after 30m (1800).
	mkTask(t, ctx, d, "swep", "2026-09-20", db.DomainSWE, db.SessionKindSWE, raw)
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "swep", now); err != nil {
		t.Fatal(err)
	}
	closed, err := engine.StopActiveSession(ctx, d, now.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if closed.ElapsedSeconds() != 1800 {
		t.Fatalf("elapsed=%d want 1800", closed.ElapsedSeconds())
	}
	// No evidence form exists and time is partial: stays TODO by design.
	ev, err := engine.EvaluateTodayTask(ctx, d, "swep")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("partial SWE must stay TODO, got %s", ev.State)
	}
	// Second session reaches the target cumulatively: 1800 + 1800 = 3600.
	// Explicit completion: time never completes, so still TODO.
	sess2, err := engine.StartTodayTask(ctx, d, "swep", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = engine.StopActiveSession(ctx, d, now.Add(2*time.Hour).Add(30*time.Minute))
	ev, err = engine.EvaluateTodayTask(ctx, d, "swep")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("cumulative SWE without YES must stay TODO, got %s", ev.State)
	}
	_ = sess2
}

func TestSWESessionTimeRecorded(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWE(t, ctx, d)
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-sql"})
	mkTask(t, ctx, d, "swea", "2026-09-20", db.DomainSWE, db.SessionKindSWE, raw)
	now := time.Now()
	sess, err := engine.StartTodayTask(ctx, d, "swea", now)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := engine.StopActiveSession(ctx, d, now.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if closed.ElapsedSeconds() != 1800 {
		t.Fatalf("elapsed=%d want 1800", closed.ElapsedSeconds())
	}
	if closed.PlannedSeconds != 3600 {
		t.Fatalf("planned=%d want 3600", closed.PlannedSeconds)
	}
	_ = sess
}

// ---- DSA strictness ----

func TestDSAExactBundleGeneration(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	// Two due repeats + two genuinely new problems.
	for _, pid := range []db.ID{"dsa-1", "dsa-2"} {
		if err := runtime.UpsertDSARepeatState(ctx, d, db.DSARepeatState{
			ProblemID: pid, RepeatStage: 1, CurrentIntervalDays: 3,
			NextReviewDate: "2026-09-21", UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tk := range tasks {
		if tk.Domain != db.DomainDSA {
			continue
		}
		found = true
		dt, _ := db.UnmarshalDSATarget(*tk.TargetJSON)
		items := dt.Items()
		if len(items) != 4 {
			t.Fatalf("DSA bundle=%d", len(items))
		}
		var repeats, news int
		seen := map[db.ID]bool{}
		for _, it := range items {
			if seen[it.ProblemID] {
				t.Fatal("duplicate problem in bundle")
			}
			seen[it.ProblemID] = true
			switch it.Mode {
			case db.DSAModeRepeat:
				repeats++
			case db.DSAModeNew:
				news++
			}
		}
		if repeats != 2 || news != 2 {
			t.Fatalf("want 2 repeat + 2 new, got %d/%d", repeats, news)
		}
		if err := engine.ValidateDSACurrentBundle(ctx, d, dt); err != nil {
			t.Fatalf("generated bundle must validate: %v", err)
		}
	}
	if !found {
		t.Fatal("DSA block missing")
	}
}

func TestDSAStrictValidation(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	// Two items: rejected.
	bad2, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeNew},
	}})
	dt2, _ := db.UnmarshalDSATarget(bad2)
	if err := engine.ValidateDSACurrentBundle(ctx, d, dt2); err == nil {
		t.Fatal("2-item bundle must fail strict validation")
	}
	// Four repeats: rejected (right length, wrong shape).
	bad3, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-4", Mode: db.DSAModeRepeat},
	}})
	dt3, _ := db.UnmarshalDSATarget(bad3)
	if err := engine.ValidateDSACurrentBundle(ctx, d, dt3); err == nil {
		t.Fatal("4-repeat bundle must fail strict validation")
	}
	// Fake NEW (problem already introduced) rejected, even in 2R+2N shape.
	// dsa-1 starts genuinely new here (no attempts, no repeat state).
	now := time.Now()
	// Introduce dsa-1 through a strict 2R+2N bundle (partial save of the
	// NEW item only: current saves require strict targets).
	intro3, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
	}})
	mkTask(t, ctx, d, "intro", "2026-09-20", db.DomainDSA, db.SessionKindDSA, intro3)
	sess, _ := engine.StartTodayTask(ctx, d, "intro", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	if _, err := engine.SaveDSAEvidence(ctx, d, "intro", sess.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
	}, now); err != nil {
		t.Fatal(err)
	}
	fake, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-1", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
	}})
	dtf, _ := db.UnmarshalDSATarget(fake)
	if err := engine.ValidateDSACurrentBundle(ctx, d, dtf); err == nil {
		t.Fatal("fake NEW must fail strict validation")
	}
	// Valid steady-state shape accepted.
	_ = runtime.UpsertDSARepeatState(ctx, d, db.DSARepeatState{ProblemID: "dsa-2", RepeatStage: 0, CurrentIntervalDays: 1, NextReviewDate: "2026-09-21", UpdatedAt: now})
	okb, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
	}})
	dto, _ := db.UnmarshalDSATarget(okb)
	if err := engine.ValidateDSACurrentBundle(ctx, d, dto); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
}

func TestDSAStaleNewRejectedOnSave(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	now := time.Now()
	// Introduce dsa-1 as NEW through a strict 2R+2N bundle (partial save
	// of the NEW item only: current saves require strict targets).
	new3, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
	}})
	mkTask(t, ctx, d, "new1", "2026-09-20", db.DomainDSA, db.SessionKindDSA, new3)
	s1, _ := engine.StartTodayTask(ctx, d, "new1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	if _, err := engine.SaveDSAEvidence(ctx, d, "new1", s1.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 100},
	}, now); err != nil {
		t.Fatal(err)
	}
	// A stale persisted target still saying NEW must surface, never convert.
	stale3, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeNew},
		{ProblemID: "dsa-4", Mode: db.DSAModeNew},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeRepeat},
	}})
	mkTask(t, ctx, d, "new2", "2026-09-21", db.DomainDSA, db.SessionKindDSA, stale3)
	s2, err := engine.StartTodayTask(ctx, d, "new2", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = engine.StopActiveSession(ctx, d, now.Add(3*time.Hour))
	_, err = engine.SaveDSAEvidence(ctx, d, "new2", s2.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 120},
	}, now)
	if !errors.Is(err, engine.ErrInvalidTarget) {
		t.Fatalf("stale NEW must return ErrInvalidTarget, got %v", err)
	}
	var n int
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM dsa_attempts WHERE session_id = ?`, string(s2.ID)).Scan(&n)
	if n != 0 {
		t.Fatalf("stale save must persist nothing, got %d", n)
	}
}

func TestDSARepeatDueRespectsDate(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	now := time.Now()
	_ = runtime.UpsertDSARepeatState(ctx, d, db.DSARepeatState{ProblemID: "dsa-1", RepeatStage: 1, CurrentIntervalDays: 3, NextReviewDate: "2026-09-20", UpdatedAt: now})
	_ = runtime.UpsertDSARepeatState(ctx, d, db.DSARepeatState{ProblemID: "dsa-2", RepeatStage: 1, CurrentIntervalDays: 3, NextReviewDate: "2026-09-30", UpdatedAt: now})
	due, err := runtime.ListDSARepeatDue(ctx, d, "2026-09-21")
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ProblemID != "dsa-1" {
		t.Fatalf("due query must respect date, got %+v", due)
	}
	all, err := runtime.ListDSARepeatDue(ctx, d, "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all due by 09-30, got %+v", all)
	}
}

// ---- calendar / history ----

func TestCrossMidnightDailyReport(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-ratio"}})
	mkTask(t, ctx, d, "xmid", "2026-09-21", db.DomainGOV, db.SessionKindPractice, prac)
	// Session Sep 21 23:50 IST -> Sep 22 00:20 IST.
	start := time.Date(2026, 9, 21, 23, 50, 0, 0, cal.Location())
	end := time.Date(2026, 9, 22, 0, 20, 0, 0, cal.Location())
	sess, err := engine.StartTodayTask(ctx, d, "xmid", start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, end); err != nil {
		t.Fatal(err)
	}
	avg := 20.0
	if _, err := engine.SaveGovPracticeEvidence(ctx, d, "xmid", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-ratio", TimeSecs: 1800, Attempted: 20, Correct: 15, AvgSpeed: &avg},
	}, end); err != nil {
		t.Fatal(err)
	}
	rep21, err := engine.BuildDailyReport(ctx, d, "2026-09-21")
	if err != nil {
		t.Fatal(err)
	}
	if rep21.Actual != 600 {
		t.Fatalf("sep21 actual=%d want 600", rep21.Actual)
	}
	if len(rep21.Activities) != 1 || rep21.Activities[0].Actual != 600 {
		t.Fatalf("sep21 task actual=%+v", rep21.Activities)
	}
	// Evidence recorded Sep 22: Sep 21 history stays TODO.
	if rep21.TasksCompleted != 0 {
		t.Fatal("sep21 must stay incomplete (evidence recorded sep22)")
	}
	if rep21.Gov.QuestionsAttempted != 0 {
		t.Fatalf("sep21 questions must be 0, got %d", rep21.Gov.QuestionsAttempted)
	}
	if rep21.Gov.Accuracy != nil {
		t.Fatal("sep21 accuracy must be missing (—), not 0%")
	}
	rep22, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if rep22.Actual != 1200 {
		t.Fatalf("sep22 actual=%d want 1200", rep22.Actual)
	}
	if len(rep22.Activities) != 0 {
		// No snapshot on sep22 in this fixture: activities empty, but
		// evidence still attributes to the day.
		t.Fatalf("sep22 activities=%d want 0 (no snapshot)", len(rep22.Activities))
	}
	if rep22.Gov.QuestionsAttempted != 20 || rep22.Gov.QuestionsCorrect != 15 {
		t.Fatalf("sep22 gov=%+v", rep22.Gov)
	}
	if rep22.Gov.Accuracy == nil {
		t.Fatal("sep22 accuracy must be set")
	}
}

func TestHistoricalCutoffLecture(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	cal := engine.DefaultCalendar()
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "hist-lec", "2026-09-22", db.DomainGOV, db.SessionKindLecture, lec)
	day1 := time.Date(2026, 9, 22, 10, 0, 0, 0, cal.Location())
	day2 := time.Date(2026, 9, 23, 10, 0, 0, 0, cal.Location())
	s1, err := engine.StartTodayTask(ctx, d, "hist-lec", day1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, day1.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// NO on day 1: nothing recorded.
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "hist-lec", s1.ID, false, day1); err != nil {
		t.Fatal(err)
	}
	rep, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if rep.TasksCompleted != 0 {
		t.Fatal("day1 must be incomplete")
	}
	// YES on day 2: day-2 completes, day-1 history unchanged.
	s2, err := engine.StartTodayTask(ctx, d, "hist-lec", day2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.StopActiveSession(ctx, d, day2.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SaveGovLectureEvidence(ctx, d, "hist-lec", s2.ID, true, day2); err != nil {
		t.Fatal(err)
	}
	rep1, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if rep1.TasksCompleted != 0 {
		t.Fatal("sep22 history must NOT flip when evidence lands sep23")
	}
	rep2, err := engine.BuildDailyReport(ctx, d, "2026-09-23")
	if err != nil {
		t.Fatal(err)
	}
	_ = rep2
	ev, err := engine.EvaluateTodayTask(ctx, d, "hist-lec")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("current state must be COMPLETED, got %s", ev.State)
	}
}

func TestWeeklyLatestSnapshotPerDay(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	// Two revisions same day: rev1 planned 3600, rev2 planned 7200.
	_ = runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: "w-r1", LocalDate: "2026-09-22", Revision: 1, GeneratedAt: now})
	tj := `{"subject_id":"subj-quant","topic_id":"topic-perc","lecture_id":"lec-1"}`
	_ = runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: "w-r1-t1", SnapshotID: "w-r1", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "A", PlannedSeconds: 3600, TargetJSON: &tj})
	_ = runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: "w-r2", LocalDate: "2026-09-22", Revision: 2, GeneratedAt: now})
	_ = runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: "w-r2-t1", SnapshotID: "w-r2", Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "B", PlannedSeconds: 7200, TargetJSON: &tj})
	rep, err := engine.BuildWeeklyDraft(ctx, d, "2026-W39", "2026-09-22", "2026-09-28")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Planned != 7200 {
		t.Fatalf("weekly planned=%d want 7200 (latest revision only)", rep.Planned)
	}
	if rep.TasksTotal != 1 {
		t.Fatalf("weekly tasks=%d want 1", rep.TasksTotal)
	}
}

func TestEmptyDaySemantics(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	rep, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Planned != 0 || rep.Actual != 0 || rep.TasksTotal != 0 {
		t.Fatalf("empty day must be zero, got %+v", rep)
	}
	if rep.Gov.Accuracy != nil {
		t.Fatal("no evidence must mean missing accuracy (—), not 0%")
	}
}

// ---- mocks ----

func TestMockWeakTopicRules(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	// 4 weak topics rejected.
	m := db.Mock{ID: "mr-1", ExamID: "exam-1", Name: "M", MockDate: "2026-09-18", CreatedAt: now}
	if _, err := engine.CreateMockAtomic(ctx, d, m, []engine.MockSectionInput{
		{Name: "QA", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-perc", "topic-ratio", "topic-ns", "topic-series"}},
	}, now); err == nil {
		t.Fatal("4 weak topics must fail")
	}
	// Duplicates rejected.
	m2 := db.Mock{ID: "mr-2", ExamID: "exam-1", Name: "M2", MockDate: "2026-09-18", CreatedAt: now}
	if _, err := engine.CreateMockAtomic(ctx, d, m2, []engine.MockSectionInput{
		{Name: "QA", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-perc", "topic-perc"}},
	}, now); err == nil {
		t.Fatal("duplicate weak topics must fail")
	}
	// Off-subject rejected (series belongs to Reasoning, not Quant).
	m3 := db.Mock{ID: "mr-3", ExamID: "exam-1", Name: "M3", MockDate: "2026-09-18", CreatedAt: now}
	if _, err := engine.CreateMockAtomic(ctx, d, m3, []engine.MockSectionInput{
		{Name: "QA", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-series"}},
	}, now); err == nil {
		t.Fatal("off-subject weak topic must fail")
	}
	for _, id := range []db.ID{"mr-1", "mr-2", "mr-3"} {
		if _, err := runtime.GetMock(ctx, d, id); err == nil {
			t.Fatalf("failed mock %s must roll back", id)
		}
	}
}

func TestMockMetricCoherence(t *testing.T) {
	att, tot, un := 80, 100, 30
	m := db.Mock{ID: "mm-1", ExamID: "e", Name: "M", MockDate: "2026-09-18", Attempted: &att, TotalQuestions: &tot, Unattempted: &un}
	if err := engine.ValidateMockMetrics(m); err == nil {
		t.Fatal("attempted + unattempted != total must fail")
	}
	p := 101.0
	m2 := db.Mock{ID: "mm-2", ExamID: "e", Name: "M", MockDate: "2026-09-18", Percentile: &p}
	if err := engine.ValidateMockMetrics(m2); err == nil {
		t.Fatal("percentile > 100 must fail")
	}
	z := 0.0
	m3 := db.Mock{ID: "mm-3", ExamID: "e", Name: "M", MockDate: "2026-09-18", AvgSpeedSeconds: &z}
	if err := engine.ValidateMockMetrics(m3); err == nil {
		t.Fatal("avg speed 0 must fail")
	}
	// Zero attempted with missing accuracy stays missing (not 0%).
	if err := engine.ValidateMockMetrics(db.Mock{ID: "x", ExamID: "e", Name: "M", MockDate: "2026-09-18"}); err != nil {
		t.Fatalf("omitted metrics must pass: %v", err)
	}
}

func TestRepeatProgressionTable(t *testing.T) {
	if got := engine.IntervalForStage(0); got != 1 {
		t.Fatalf("stage0=%d", got)
	}
	if got := engine.IntervalForStage(1); got != 3 {
		t.Fatalf("stage1=%d", got)
	}
	if got := engine.IntervalForStage(2); got != 7 {
		t.Fatalf("stage2=%d", got)
	}
	st, _ := engine.NextRepeatOnRepeat(2, true)
	if st != 3 {
		t.Fatalf("pass must advance, got %d", st)
	}
	st, _ = engine.NextRepeatOnRepeat(2, false)
	if st != 0 {
		t.Fatalf("fail must reset, got %d", st)
	}
	st, iv, nr := engine.NextRepeatOnNew(true, "2026-09-21")
	if st != 1 || iv != 3 || nr != "2026-09-24" {
		t.Fatalf("new pass: %d %d %s", st, iv, nr)
	}
	st, iv, nr = engine.NextRepeatOnNew(false, "2026-09-21")
	if st != 0 || iv != 1 || nr != "2026-09-22" {
		t.Fatalf("new fail: %d %d %s", st, iv, nr)
	}
}
