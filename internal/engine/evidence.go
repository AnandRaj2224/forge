package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// GovPracticeInput is one validated topic row for a practice bundle save.
type GovPracticeInput struct {
	TopicID   db.ID
	TimeSecs  int
	Attempted int
	Correct   int
	AvgSpeed  *float64
}

// DSAAttemptInput is one problem result for a DSA bundle save.
type DSAAttemptInput struct {
	ProblemID   db.ID
	Independent bool
	Optimal     bool
	HintUsed    bool
	SolveSecs   int
}

// SaveGovLectureEvidence records a lecture YES. NO inserts nothing and leaves
// the task TODO. Duplicate completion is detected first (clean domain error,
// never a raw SQLite UNIQUE error through normal UI).
func SaveGovLectureEvidence(ctx context.Context, database *db.DB, taskID, sessionID db.ID, yes bool, now time.Time) (TaskEvaluation, error) {
	task, err := runtime.GetTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load task: %w", err)
	}
	if task.Domain != db.DomainGOV || task.Kind != db.SessionKindLecture {
		return TaskEvaluation{}, wrapErr("lecture save", ErrValidation, "not a lecture task")
	}
	if task.TargetJSON == nil {
		return TaskEvaluation{}, wrapErr("lecture save", ErrInvalidTarget, "no target")
	}
	lt, err := db.UnmarshalGovLectureTarget(*task.TargetJSON)
	if err != nil {
		return TaskEvaluation{}, wrapErr("lecture save", ErrInvalidTarget, err.Error())
	}
	if err := ValidateGovLectureTarget(ctx, database, lt); err != nil {
		return TaskEvaluation{}, err
	}
	sess, err := runtime.GetSession(ctx, database, sessionID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load session: %w", err)
	}
	if sess.TodayTaskID != taskID {
		return TaskEvaluation{}, wrapErr("lecture save", ErrValidation, "session does not belong to task")
	}
	if sess.Status != db.SessionStatusStopped {
		return TaskEvaluation{}, wrapErr("lecture save", ErrValidation, "evidence requires a STOPPED session")
	}
	if !yes {
		// NO means NOT completed: insert nothing, task stays TODO.
		return EvaluateTodayTask(ctx, database, taskID)
	}
	done, err := IsLectureComplete(ctx, database, lt.LectureID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: lecture check: %w", err)
	}
	if done {
		return TaskEvaluation{}, wrapErr("lecture save", ErrAlreadySatisfied, fmt.Sprintf("lecture %q already complete", string(lt.LectureID)))
	}
	if err := runtime.RecordLectureCompletion(ctx, database, db.GovLectureCompletion{
		LectureID: lt.LectureID, SessionID: sessionID, CompletedAt: now,
	}); err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: record lecture: %w", err)
	}
	return EvaluateTodayTask(ctx, database, taskID)
}

// DerivePracticeAvgSpeed calculates avg time per question from raw values:
// time_spent_seconds / attempted. Returns nil when attempted == 0 (no
// meaningful average). Never invents a value when time is 0; callers keep
// the existing time>0 validation.
func DerivePracticeAvgSpeed(timeSecs, attempted int) *float64 {
	if attempted <= 0 {
		return nil
	}
	v := float64(timeSecs) / float64(attempted)
	if v <= 0 {
		return nil
	}
	return &v
}

// ValidateGovPracticeInput validates one practice row from RAW facts only:
// time>0, attempted>=0, 0<=correct<=attempted. Avg speed is DERIVED as
// time/attempted (nil when attempted==0), never user-entered. The avgS
// parameter is kept for backward compatibility with old records/tests:
// when non-empty it is validated but the derived value takes precedence
// for new saves; when empty the derived value is used directly.
func ValidateGovPracticeInput(topicName, timeS, attS, corrS, avgS string, requireAvg bool) (GovPracticeInput, string) {
	var out GovPracticeInput
	tSecs, msg := ParseDurationSeconds(timeS)
	if msg != "" {
		if msg == "required" {
			return out, "time required"
		}
		return out, "time " + msg
	}
	if tSecs <= 0 {
		return out, "time must be > 0"
	}
	out.TimeSecs = tSecs
	att, msg2 := parseNonNegInt("attempted", attS)
	if msg2 != "" {
		return out, msg2
	}
	corr, msg3 := parseNonNegInt("correct", corrS)
	if msg3 != "" {
		return out, msg3
	}
	if corr > att {
		return out, "correct > attempted"
	}
	out.Attempted = att
	out.Correct = corr
	// Backward compat: validate a provided avg when present, but always
	// derive the stored value from raw facts.
	if avgS != "" {
		if _, msg4 := ParseAvgSpeed(avgS); msg4 != "" {
			return out, msg4
		}
	}
	_ = requireAvg
	out.AvgSpeed = DerivePracticeAvgSpeed(tSecs, att)
	_ = topicName
	return out, ""
}

func parseNonNegInt(field, s string) (int, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, field + " required"
	}
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil || v < 0 {
		// Fall back to strict atoi for trailing garbage detection.
		return 0, field + " must be integer >= 0"
	}
	// Reject "12abc".
	trimmed := strings.TrimSpace(s)
	for _, r := range trimmed {
		if (r < '0' || r > '9') && !(len(trimmed) > 0 && trimmed[0] == '0') {
			if r == '+' || r == '-' {
				return 0, field + " must be integer >= 0"
			}
			if r < '0' || r > '9' {
				return 0, field + " must be integer >= 0"
			}
		}
	}
	return v, ""
}

// SaveGovPracticeEvidence validates the exact task target, rejects duplicates
// of already-satisfied topics, and writes all rows in ONE transaction.
// Partial bundles stay TODO; exact bundles become COMPLETED.
func SaveGovPracticeEvidence(ctx context.Context, database *db.DB, taskID, sessionID db.ID, inputs []GovPracticeInput, now time.Time) (TaskEvaluation, error) {
	task, err := runtime.GetTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load task: %w", err)
	}
	if task.TargetJSON == nil {
		return TaskEvaluation{}, wrapErr("practice save", ErrInvalidTarget, "no target")
	}
	pt, err := db.UnmarshalGovPracticeTarget(*task.TargetJSON)
	if err != nil {
		return TaskEvaluation{}, wrapErr("practice save", ErrInvalidTarget, err.Error())
	}
	if err := ValidateGovPracticeTarget(ctx, database, pt); err != nil {
		return TaskEvaluation{}, err
	}
	sess, err := runtime.GetSession(ctx, database, sessionID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load session: %w", err)
	}
	if sess.TodayTaskID != taskID {
		return TaskEvaluation{}, wrapErr("practice save", ErrValidation, "session does not belong to task")
	}
	if sess.Status != db.SessionStatusStopped {
		return TaskEvaluation{}, wrapErr("practice save", ErrValidation, "evidence requires a STOPPED session")
	}
	targetSet := map[db.ID]bool{}
	for _, tid := range pt.TopicIDs {
		targetSet[tid] = true
	}
	if len(inputs) == 0 {
		return TaskEvaluation{}, wrapErr("practice save", ErrValidation, "no evidence items")
	}
	seen := map[db.ID]bool{}
	for i, in := range inputs {
		if !targetSet[in.TopicID] {
			return TaskEvaluation{}, wrapErr("practice save", ErrValidation, fmt.Sprintf("topic %q not in target", string(in.TopicID)))
		}
		if seen[in.TopicID] {
			return TaskEvaluation{}, wrapErr("practice save", ErrDuplicate, fmt.Sprintf("duplicate topic %q", string(in.TopicID)))
		}
		seen[in.TopicID] = true
		if in.TimeSecs <= 0 {
			return TaskEvaluation{}, wrapErr("practice save", ErrValidation, "time must be > 0")
		}
		if in.Correct > in.Attempted || in.Attempted < 0 || in.Correct < 0 {
			return TaskEvaluation{}, wrapErr("practice save", ErrValidation, "correct/attempted invalid")
		}
		if in.AvgSpeed != nil && *in.AvgSpeed <= 0 {
			return TaskEvaluation{}, wrapErr("practice save", ErrValidation, "avg speed must be > 0")
		}
		// Derive avg time/question from raw values when not provided
		// (new form never asks for it). Old records carrying an explicit
		// avg remain readable and accepted as-is.
		if in.AvgSpeed == nil {
			inputs[i].AvgSpeed = DerivePracticeAvgSpeed(in.TimeSecs, in.Attempted)
		}
	}
	// Reject already-satisfied target items (no duplicate completion evidence).
	ev, err := EvaluateTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, err
	}
	already := map[string]bool{}
	for _, c := range ev.Completed {
		already[c] = true
	}
	for _, in := range inputs {
		if already[string(in.TopicID)] {
			return TaskEvaluation{}, wrapErr("practice save", ErrAlreadySatisfied, fmt.Sprintf("topic %q already satisfied", string(in.TopicID)))
		}
	}
	err = database.WithTx(ctx, func(q db.DBTX) error {
		for _, in := range inputs {
			id := db.ID(fmt.Sprintf("gpr-%d-%s", now.UnixNano(), string(in.TopicID)))
			if err := runtime.InsertGovPracticeRecord(ctx, q, db.GovPracticeRecord{
				ID: id, SessionID: sessionID, TopicID: in.TopicID,
				TimeSpentSeconds: in.TimeSecs, Attempted: in.Attempted, Correct: in.Correct,
				AvgQuestionSpeedSeconds: in.AvgSpeed, RecordedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: save practice: %w", err)
	}
	return EvaluateTodayTask(ctx, database, taskID)
}

// SaveDSAEvidence validates the exact 2-repeat+1-new bundle target, writes all
// attempts plus repeat-state updates in ONE transaction, and re-evaluates.
// All target problems require a recorded attempt (solve>0); pass/fail affects
// progression but any recorded result satisfies bundle completion.
func SaveDSAEvidence(ctx context.Context, database *db.DB, taskID, sessionID db.ID, inputs []DSAAttemptInput, now time.Time) (TaskEvaluation, error) {
	task, err := runtime.GetTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load task: %w", err)
	}
	if task.TargetJSON == nil {
		return TaskEvaluation{}, wrapErr("dsa save", ErrInvalidTarget, "no target")
	}
	dt, err := db.UnmarshalDSATarget(*task.TargetJSON)
	if err != nil {
		return TaskEvaluation{}, wrapErr("dsa save", ErrInvalidTarget, err.Error())
	}
	// Current Today tasks save under strict current-target semantics
	// (exactly 3 distinct problems: 2 REPEAT + 1 NEW, or the truthful
	// 3×NEW bootstrap) — never the generic legacy validator alone.
	if err := ValidateDSACurrentBundle(ctx, database, dt); err != nil {
		return TaskEvaluation{}, err
	}
	sess, err := runtime.GetSession(ctx, database, sessionID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load session: %w", err)
	}
	if sess.TodayTaskID != taskID {
		return TaskEvaluation{}, wrapErr("dsa save", ErrValidation, "session does not belong to task")
	}
	if sess.Status != db.SessionStatusStopped {
		return TaskEvaluation{}, wrapErr("dsa save", ErrValidation, "evidence requires a STOPPED session")
	}
	items := dt.Items()
	targetMode := map[db.ID]db.DSAMode{}
	for _, it := range items {
		targetMode[it.ProblemID] = it.Mode
	}
	if len(inputs) == 0 {
		return TaskEvaluation{}, wrapErr("dsa save", ErrValidation, "no attempt results")
	}
	seen := map[db.ID]bool{}
	for _, in := range inputs {
		mode, ok := targetMode[in.ProblemID]
		if !ok {
			return TaskEvaluation{}, wrapErr("dsa save", ErrValidation, fmt.Sprintf("problem %q not in target", string(in.ProblemID)))
		}
		_ = mode
		if seen[in.ProblemID] {
			return TaskEvaluation{}, wrapErr("dsa save", ErrDuplicate, fmt.Sprintf("duplicate problem %q", string(in.ProblemID)))
		}
		seen[in.ProblemID] = true
		if in.SolveSecs <= 0 {
			return TaskEvaluation{}, wrapErr("dsa save", ErrValidation, "solve time must be > 0")
		}
	}
	ev, err := EvaluateTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, err
	}
	already := map[string]bool{}
	for _, c := range ev.Completed {
		already[c] = true
	}
	for _, in := range inputs {
		if already[string(in.ProblemID)] {
			return TaskEvaluation{}, wrapErr("dsa save", ErrAlreadySatisfied, fmt.Sprintf("problem %q already satisfied", string(in.ProblemID)))
		}
	}
	// Strict mode truthfulness: a persisted target that says NEW for an
	// already-introduced problem is stale. Introduction means a NEW
	// attempt OR repeat scheduling state (repeat-state-only introduction
	// counts). Never mutate the plan silently into REPEAT; surface the
	// stale target instead. Planned semantics are immutable.
	for _, in := range inputs {
		if targetMode[in.ProblemID] != db.DSAModeNew {
			continue
		}
		existing, err := runtime.GetDSAAttempts(ctx, database, in.ProblemID)
		if err != nil {
			return TaskEvaluation{}, fmt.Errorf("engine: dsa history: %w", err)
		}
		for _, a := range existing {
			if a.Mode == db.DSAModeNew {
				return TaskEvaluation{}, wrapErr("dsa save", ErrInvalidTarget, fmt.Sprintf("stale target: problem %q labeled NEW but already introduced", string(in.ProblemID)))
			}
		}
		if _, err := runtime.GetDSARepeatState(ctx, database, in.ProblemID); err == nil {
			return TaskEvaluation{}, wrapErr("dsa save", ErrInvalidTarget, fmt.Sprintf("stale target: problem %q labeled NEW but already introduced", string(in.ProblemID)))
		} else if !isNotFoundErr(err) {
			return TaskEvaluation{}, fmt.Errorf("engine: repeat state: %w", err)
		}
	}
	cal := DefaultCalendar()
	localDate := cal.LocalDate(now)
	err = database.WithTx(ctx, func(q db.DBTX) error {
		for _, in := range inputs {
			mode := targetMode[in.ProblemID]
			id := db.ID(fmt.Sprintf("datt-%d-%s", now.UnixNano(), string(in.ProblemID)))
			solve := in.SolveSecs
			if err := runtime.InsertDSAAttempt(ctx, q, db.DSAAttempt{
				ID: id, SessionID: sessionID, ProblemID: in.ProblemID, Mode: mode,
				AttemptedAt: now, Independent: in.Independent, Optimal: in.Optimal,
				HintUsed: in.HintUsed, SolveTimeSeconds: &solve,
			}); err != nil {
				return err
			}
			pass := IsDSAPass(db.DSAAttempt{Independent: in.Independent, Optimal: in.Optimal, HintUsed: in.HintUsed})
			if err := applyRepeatProgress(ctx, q, in.ProblemID, mode, pass, localDate, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: save dsa: %w", err)
	}
	return EvaluateTodayTask(ctx, database, taskID)
}

func applyRepeatProgress(ctx context.Context, q db.DBTX, problemID db.ID, mode db.DSAMode, pass bool, localDate string, now time.Time) error {
	// A genuine no-row result means "no scheduling state yet" (initialize
	// below); any other DB/query failure propagates instead of being
	// treated as nonexistent state.
	cur, err := runtime.GetDSARepeatState(ctx, q, problemID)
	if err != nil && !isNotFoundErr(err) {
		return fmt.Errorf("engine: repeat state: %w", err)
	}
	curExists := err == nil
	var stage, interval int
	if mode == db.DSAModeNew {
		// NEW attempt: initialize/refresh per progression, enter scheduling.
		if pass {
			stage = 1
		} else {
			stage = 0
		}
		interval = IntervalForStage(stage)
	} else {
		if !curExists {
			curStage := 0
			stage, interval = NextRepeatOnRepeat(curStage, pass)
		} else {
			stage, interval = NextRepeatOnRepeat(cur.RepeatStage, pass)
		}
	}
	nextReview, err := addDaysLocal(localDate, interval)
	if err != nil {
		return err
	}
	return runtime.UpsertDSARepeatState(ctx, q, db.DSARepeatState{
		ProblemID: problemID, RepeatStage: stage, CurrentIntervalDays: interval,
		NextReviewDate: nextReview, UpdatedAt: now,
	})
}

func addDaysLocal(localDate string, days int) (string, error) {
	cal := DefaultCalendar()
	s, err := cal.StartOfDay(localDate)
	if err != nil {
		return "", err
	}
	return s.AddDate(0, 0, days).Format("2006-01-02"), nil
}
