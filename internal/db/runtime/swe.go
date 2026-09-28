package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	db "forge/internal/db"
)

// SWECompletion is an explicit task-level completion for a SWE Today task.
// Planned time never completes a task; only an explicit YES persists a row.
type SWECompletion struct {
	TodayTaskID db.ID
	SessionID   db.ID
	CompletedAt time.Time
}

// RecordSWECompletion persists an explicit SWE completion (one per task).
// Duplicate completion is rejected as already-complete (read-only), never a
// raw UNIQUE error. The stopped session is preserved; completion only adds
// the marker row.
func RecordSWECompletion(ctx context.Context, q db.DBTX, c SWECompletion) error {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM swe_task_completions WHERE today_task_id = ?`, string(c.TodayTaskID)).Scan(&count); err != nil {
		return fmt.Errorf("runtime: swe completion check: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("runtime: swe task %q already complete — result is read-only", string(c.TodayTaskID))
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO swe_task_completions (today_task_id, session_id, completed_at) VALUES (?, ?, ?)`,
		string(c.TodayTaskID), string(c.SessionID), c.CompletedAt.Unix()); err != nil {
		return fmt.Errorf("runtime: record swe completion: %w", err)
	}
	return nil
}

// GetSWECompletion returns the explicit completion for a task, or
// sql.ErrNoRows when the task is not explicitly completed.
func GetSWECompletion(ctx context.Context, q db.DBTX, taskID db.ID) (SWECompletion, error) {
	var c SWECompletion
	var completed int64
	var taskIDStr, sessIDStr string
	err := q.QueryRowContext(ctx, `SELECT today_task_id, session_id, completed_at FROM swe_task_completions WHERE today_task_id = ?`, string(taskID)).Scan(&taskIDStr, &sessIDStr, &completed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SWECompletion{}, err
		}
		return SWECompletion{}, fmt.Errorf("runtime: get swe completion: %w", err)
	}
	c.TodayTaskID = db.ID(taskIDStr)
	c.SessionID = db.ID(sessIDStr)
	c.CompletedAt = time.Unix(completed, 0).UTC()
	return c, nil
}

// IsSWEComplete reports whether an explicit completion row exists.
// DB failures propagate, never silent false.
func IsSWEComplete(ctx context.Context, q db.DBTX, taskID db.ID) (bool, error) {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM swe_task_completions WHERE today_task_id = ?`, string(taskID)).Scan(&count); err != nil {
		return false, fmt.Errorf("runtime: is swe complete: %w", err)
	}
	return count > 0, nil
}
