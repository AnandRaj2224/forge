package runtime

import (
	"context"
	"fmt"

	db "forge/internal/db"
)

// ---- V2 snapshot membership (today_snapshot_tasks) ----

// InsertSnapshotTask links one immutable task into a snapshot revision.
func InsertSnapshotTask(ctx context.Context, q db.DBTX, snapshotID, taskID db.ID, position int) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO today_snapshot_tasks (snapshot_id, task_id, position) VALUES (?, ?, ?)`,
		string(snapshotID), string(taskID), position); err != nil {
		return fmt.Errorf("runtime: insert snapshot task: %w", err)
	}
	return nil
}

// GetSnapshotTaskIDs returns task IDs for a snapshot in position order.
// Returns empty when no mapping rows exist (legacy rows fall back to
// today_tasks.snapshot_id). Never recurses into GetTodayTasks.
func GetSnapshotTaskIDs(ctx context.Context, q db.DBTX, snapshotID db.ID) ([]db.ID, error) {
	rows, err := q.QueryContext(ctx, `SELECT task_id FROM today_snapshot_tasks WHERE snapshot_id = ? ORDER BY position`, string(snapshotID))
	if err != nil {
		return nil, fmt.Errorf("runtime: snapshot task ids: %w", err)
	}
	var out []db.ID
	func() {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				out = append(out, db.ID(id))
			}
		}
	}()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("runtime: snapshot task rows: %w", err)
	}
	return out, nil
}
