package engine

import (
	"context"
	"fmt"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// CompleteSWETask records an explicit YES for a SWE Today task: the stopped
// session is preserved and the task becomes COMPLETE (read-only). It is the
// only path to SWE completion; planned time never completes.
func CompleteSWETask(ctx context.Context, database *db.DB, taskID, sessionID db.ID, now time.Time) (TaskEvaluation, error) {
	task, err := runtime.GetTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load task: %w", err)
	}
	if task.Domain != db.DomainSWE {
		return TaskEvaluation{}, wrapErr("swe complete", ErrValidation, "not a SWE task")
	}
	if task.TargetJSON == nil || *task.TargetJSON == "" {
		return TaskEvaluation{}, wrapErr("swe complete", ErrInvalidTarget, "no target")
	}
	st, err := db.UnmarshalSWETarget(*task.TargetJSON)
	if err != nil {
		return TaskEvaluation{}, wrapErr("swe complete", ErrInvalidTarget, err.Error())
	}
	if err := ValidateSWETarget(ctx, database, st); err != nil {
		return TaskEvaluation{}, err
	}
	sess, err := runtime.GetSession(ctx, database, sessionID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load session: %w", err)
	}
	if sess.TodayTaskID != taskID {
		return TaskEvaluation{}, wrapErr("swe complete", ErrValidation, "session does not belong to task")
	}
	if sess.Status != db.SessionStatusStopped {
		return TaskEvaluation{}, wrapErr("swe complete", ErrValidation, "completion requires a STOPPED session")
	}
	ev, err := EvaluateTodayTask(ctx, database, taskID)
	if err != nil {
		return TaskEvaluation{}, err
	}
	if ev.State == StateCompleted {
		return TaskEvaluation{}, wrapErr("swe complete", ErrAlreadySatisfied, fmt.Sprintf("task %q already complete", string(taskID)))
	}
	if err := runtime.RecordSWECompletion(ctx, database, runtime.SWECompletion{
		TodayTaskID: taskID, SessionID: sessionID, CompletedAt: now,
	}); err != nil {
		return TaskEvaluation{}, err
	}
	return EvaluateTodayTask(ctx, database, taskID)
}

// IsSWEComplete reports whether a SWE task has an explicit completion row.
// Product notion wrapper over the runtime primitive; DB failures propagate.
func IsSWEComplete(ctx context.Context, q db.DBTX, taskID db.ID) (bool, error) {
	done, err := runtime.IsSWEComplete(ctx, q, taskID)
	if err != nil {
		return false, fmt.Errorf("engine: swe completion: %w", err)
	}
	return done, nil
}

// SWETotalSeconds sums SWE time spent for one task (stopped durations plus
// live active elapsed). Used for Actual metrics; never for completion.
func SWETotalSeconds(sessions []db.Session) int64 {
	return sweTotalSeconds(sessions, 0)
}
