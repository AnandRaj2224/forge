package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
	return scanSWEExecutions(rows)
}

// GetSWEExecutionsBySessions returns executions for a bounded set of
// sessions in ONE query (batch alternative to per-session lookups).
func GetSWEExecutionsBySessions(ctx context.Context, q db.DBTX, sessionIDs []db.ID) ([]db.SWEExecution, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	place := make([]string, 0, len(sessionIDs))
	args := make([]any, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		place = append(place, "?")
		args = append(args, string(id))
	}
	rows, err := q.QueryContext(ctx, `SELECT id, session_id, task_id, status, recorded_at FROM swe_executions WHERE session_id IN (`+strings.Join(place, ",")+`) ORDER BY recorded_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("runtime: swe by sessions: %w", err)
	}
	defer rows.Close()
	return scanSWEExecutions(rows)
}

func scanSWEExecutions(rows *sql.Rows) ([]db.SWEExecution, error) {
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

// GetSWETaskCompletionMap returns COMPLETED presence for a bounded set of
// task IDs in ONE query: task IDs → one completion query → map[taskID]bool.
func GetSWETaskCompletionMap(ctx context.Context, q db.DBTX, taskIDs []db.ID) (map[db.ID]bool, error) {
	out := make(map[db.ID]bool, len(taskIDs))
	for _, id := range taskIDs {
		out[id] = false
	}
	if len(taskIDs) == 0 {
		return out, nil
	}
	place := make([]string, 0, len(taskIDs))
	args := make([]any, 0, len(taskIDs))
	for _, id := range taskIDs {
		place = append(place, "?")
		args = append(args, string(id))
	}
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT task_id FROM swe_executions WHERE status = 'COMPLETED' AND task_id IN (`+strings.Join(place, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("runtime: swe completion map: %w", err)
	}
	var found []string
	func() {
		defer rows.Close()
		for rows.Next() {
			var tid string
			if err := rows.Scan(&tid); err == nil {
				found = append(found, tid)
			}
		}
	}()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("runtime: swe completion map rows: %w", err)
	}
	for _, tid := range found {
		out[db.ID(tid)] = true
	}
	return out, nil
}
