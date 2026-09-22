package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
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

// SWEOutcomeInput is one task outcome for a SWE bundle save.
type SWEOutcomeInput struct {
	TaskID db.ID
	Status db.SWEExecutionStatus
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
	done, err := runtime.IsLectureComplete(ctx, database, lt.LectureID)
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

// ValidateGovPracticeInput validates one practice row: time>0, attempted>=0,
// 0<=correct<=attempted, avg speed >0 when present. New bundle saves require
// avg speed (engine rule); pass requireAvg=false for legacy-tolerant checks.
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
	avg, msg4 := ParseAvgSpeed(avgS)
	if msg4 != "" {
		return out, msg4
	}
	if requireAvg && avg == nil {
		return out, "average speed required"
	}
	out.AvgSpeed = avg
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
	for _, in := range inputs {
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
		if in.AvgSpeed == nil {
			return TaskEvaluation{}, wrapErr("practice save", ErrValidation, "average speed required")
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
	if err := ValidateDSATarget(ctx, database, dt); err != nil {
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
	cal := DefaultCalendar()
	localDate := cal.LocalDate(now)
	err = database.WithTx(ctx, func(q db.DBTX) error {
		for _, in := range inputs {
			mode := targetMode[in.ProblemID]
			existing, err := runtime.GetDSAAttempts(ctx, q, in.ProblemID)
			if err != nil {
				return fmt.Errorf("engine: dsa history: %w", err)
			}
			hasNew := false
			for _, a := range existing {
				if a.Mode == db.DSAModeNew {
					hasNew = true
					break
				}
			}
			saveMode := mode
			if mode == db.DSAModeNew && hasNew {
				saveMode = db.DSAModeRepeat
			}
			id := db.ID(fmt.Sprintf("datt-%d-%s", now.UnixNano(), string(in.ProblemID)))
			solve := in.SolveSecs
			if err := runtime.InsertDSAAttempt(ctx, q, db.DSAAttempt{
				ID: id, SessionID: sessionID, ProblemID: in.ProblemID, Mode: saveMode,
				AttemptedAt: now, Independent: in.Independent, Optimal: in.Optimal,
				HintUsed: in.HintUsed, SolveTimeSeconds: &solve,
			}); err != nil {
				return err
			}
			pass := in.Independent && in.Optimal && !in.HintUsed
			if err := applyRepeatProgress(ctx, q, in.ProblemID, saveMode, pass, localDate, now); err != nil {
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
	cur, err := runtime.GetDSARepeatState(ctx, q, problemID)
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

// SaveSWEEvidence validates the exact bundle, writes all outcomes in ONE
// transaction, and re-evaluates. Every target task needs exactly one valid
// outcome; BLOCKED/PARTIAL still complete the bundle as evidence.
func SaveSWEEvidence(ctx context.Context, database *db.DB, taskID, sessionID db.ID, inputs []SWEOutcomeInput, now time.Time) (TaskEvaluation, error) {
	task, err := runtime.GetTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load task: %w", err)
	}
	if task.TargetJSON == nil {
		return TaskEvaluation{}, wrapErr("swe save", ErrInvalidTarget, "no target")
	}
	st, err := db.UnmarshalSWETarget(*task.TargetJSON)
	if err != nil {
		return TaskEvaluation{}, wrapErr("swe save", ErrInvalidTarget, err.Error())
	}
	if err := ValidateSWETarget(ctx, database, st); err != nil {
		return TaskEvaluation{}, err
	}
	sess, err := runtime.GetSession(ctx, database, sessionID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load session: %w", err)
	}
	if sess.TodayTaskID != taskID {
		return TaskEvaluation{}, wrapErr("swe save", ErrValidation, "session does not belong to task")
	}
	if sess.Status != db.SessionStatusStopped {
		return TaskEvaluation{}, wrapErr("swe save", ErrValidation, "evidence requires a STOPPED session")
	}
	items := st.Items()
	targetSet := map[db.ID]db.SWETaskTarget{}
	for _, it := range items {
		if it.TaskID == nil {
			return TaskEvaluation{}, wrapErr("swe save", ErrInvalidTarget, "area-only targets require migration to task bundles for new saves")
		}
		targetSet[*it.TaskID] = it
	}
	if len(inputs) == 0 {
		return TaskEvaluation{}, wrapErr("swe save", ErrValidation, "no outcomes")
	}
	seen := map[db.ID]bool{}
	for _, in := range inputs {
		tgt, ok := targetSet[in.TaskID]
		if !ok {
			return TaskEvaluation{}, wrapErr("swe save", ErrValidation, fmt.Sprintf("task %q not in target", string(in.TaskID)))
		}
		if seen[in.TaskID] {
			return TaskEvaluation{}, wrapErr("swe save", ErrDuplicate, fmt.Sprintf("duplicate task %q", string(in.TaskID)))
		}
		seen[in.TaskID] = true
		if !validSWEStatus(in.Status) {
			return TaskEvaluation{}, wrapErr("swe save", ErrValidation, "invalid outcome")
		}
		// Verify area/task relationship.
		tasks, err := catalog.GetSWETasks(ctx, database, tgt.AreaID)
		if err != nil {
			return TaskEvaluation{}, fmt.Errorf("engine: swe tasks: %w", err)
		}
		found := false
		for _, t := range tasks {
			if t.ID == in.TaskID {
				found = true
				break
			}
		}
		if !found {
			return TaskEvaluation{}, wrapErr("swe save", ErrValidation, "area/task mismatch")
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
		if already[string(in.TaskID)] {
			return TaskEvaluation{}, wrapErr("swe save", ErrAlreadySatisfied, fmt.Sprintf("task %q already satisfied", string(in.TaskID)))
		}
	}
	err = database.WithTx(ctx, func(q db.DBTX) error {
		for _, in := range inputs {
			id := db.ID(fmt.Sprintf("swex-%d-%s", now.UnixNano(), string(in.TaskID)))
			if err := runtime.InsertSWEExecution(ctx, q, db.SWEExecution{
				ID: id, SessionID: sessionID, TaskID: in.TaskID, Status: in.Status, RecordedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: save swe: %w", err)
	}
	return EvaluateTodayTask(ctx, database, taskID)
}
