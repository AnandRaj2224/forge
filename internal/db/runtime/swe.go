package runtime

import (
	"context"
	"fmt"
	"time"

	db "forge/internal/db"
)

// InsertSWEExecution preserves one execution outcome.
func InsertSWEExecution(ctx context.Context, q db.DBTX, e db.SWEExecution) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES (?, ?, ?, ?, ?)`,
		e.ID, e.SessionID, e.TaskID, string(e.Status), e.RecordedAt.Unix(),
	); err != nil {
		return fmt.Errorf("runtime: insert swe execution: %w", err)
	}
	return nil
}

// GetSWEExecutionHistory returns executions for a task ordered by recorded_at.
func GetSWEExecutionHistory(ctx context.Context, q db.DBTX, taskID db.ID) ([]db.SWEExecution, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, session_id, task_id, status, recorded_at FROM swe_executions WHERE task_id = ? ORDER BY recorded_at`, taskID)
	if err != nil {
		return nil, fmt.Errorf("runtime: swe history: %w", err)
	}
	defer rows.Close()
	var out []db.SWEExecution
	for rows.Next() {
		var e db.SWEExecution
		var status string
		var recorded int64
		if err := rows.Scan(&e.ID, &e.SessionID, &e.TaskID, &status, &recorded); err != nil {
			return nil, fmt.Errorf("runtime: scan swe execution: %w", err)
		}
		e.Status = db.SWEExecutionStatus(status)
		e.RecordedAt = time.Unix(recorded, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetSWEExecutionsBySession returns executions for a session.
func GetSWEExecutionsBySession(ctx context.Context, q db.DBTX, sessionID db.ID) ([]db.SWEExecution, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, session_id, task_id, status, recorded_at FROM swe_executions WHERE session_id = ? ORDER BY recorded_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runtime: swe by session: %w", err)
	}
	defer rows.Close()
	var out []db.SWEExecution
	for rows.Next() {
		var e db.SWEExecution
		var status string
		var recorded int64
		if err := rows.Scan(&e.ID, &e.SessionID, &e.TaskID, &status, &recorded); err != nil {
			return nil, fmt.Errorf("runtime: scan swe execution: %w", err)
		}
		e.Status = db.SWEExecutionStatus(status)
		e.RecordedAt = time.Unix(recorded, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// IsSWETaskComplete reports whether a COMPLETED execution row exists.
//
// Persistence primitive for the Engine: product code must use
// engine.IsSWETaskComplete (the authoritative product notion), never this
// helper directly. DB failures are returned as errors, never silent false.
func IsSWETaskComplete(ctx context.Context, q db.DBTX, taskID db.ID) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM swe_executions WHERE task_id = ? AND status = 'COMPLETED'`, taskID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("runtime: is swe task complete: %w", err)
	}
	return count > 0, nil
}
