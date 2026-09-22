package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "forge/internal/db"
)

// CreateTodaySnapshot inserts a snapshot revision.
func CreateTodaySnapshot(ctx context.Context, q db.DBTX, s db.TodaySnapshot) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO today_snapshots (id, local_date, revision, generated_at) VALUES (?, ?, ?, ?)`,
		s.ID, s.LocalDate, s.Revision, s.GeneratedAt.Unix(),
	); err != nil {
		return fmt.Errorf("runtime: create today snapshot: %w", err)
	}
	return nil
}

// InsertTodayTask inserts one task row inside a snapshot.
func InsertTodayTask(ctx context.Context, q db.DBTX, t db.TodayTask) error {
	var sched any
	if t.ScheduledStartMinute != nil {
		sched = *t.ScheduledStartMinute
	}
	var target any
	if t.TargetJSON != nil {
		target = *t.TargetJSON
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SnapshotID, t.Position, sched, string(t.Domain), string(t.Kind), t.Title, t.PlannedSeconds, target,
	); err != nil {
		return fmt.Errorf("runtime: insert today task: %w", err)
	}
	return nil
}

// GetLatestTodaySnapshot returns the highest-revision snapshot for a date.
func GetLatestTodaySnapshot(ctx context.Context, q db.DBTX, localDate string) (db.TodaySnapshot, error) {
	var s db.TodaySnapshot
	var generated int64
	err := q.QueryRowContext(ctx, `SELECT id, local_date, revision, generated_at FROM today_snapshots WHERE local_date = ? ORDER BY revision DESC LIMIT 1`, localDate).Scan(
		&s.ID, &s.LocalDate, &s.Revision, &generated,
	)
	if err != nil {
		return db.TodaySnapshot{}, fmt.Errorf("runtime: latest today snapshot: %w", err)
	}
	s.GeneratedAt = time.Unix(generated, 0).UTC()
	return s, nil
}

// GetTodayTasks returns tasks for a snapshot ordered by position.
func GetTodayTasks(ctx context.Context, q db.DBTX, snapshotID db.ID) ([]db.TodayTask, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json
FROM today_tasks WHERE snapshot_id = ? ORDER BY position`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("runtime: list today tasks: %w", err)
	}
	defer rows.Close()
	var out []db.TodayTask
	for rows.Next() {
		var t db.TodayTask
		var sched sql.NullInt64
		var domain, kind string
		var target sql.NullString
		if err := rows.Scan(&t.ID, &t.SnapshotID, &t.Position, &sched, &domain, &kind, &t.Title, &t.PlannedSeconds, &target); err != nil {
			return nil, fmt.Errorf("runtime: scan today task: %w", err)
		}
		if sched.Valid {
			v := int(sched.Int64)
			t.ScheduledStartMinute = &v
		}
		t.Domain = db.Domain(domain)
		t.Kind = db.SessionKind(kind)
		if target.Valid {
			v := target.String
			t.TargetJSON = &v
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTodayTask returns a single today task.
func GetTodayTask(ctx context.Context, q db.DBTX, id db.ID) (db.TodayTask, error) {
	var t db.TodayTask
	var sched sql.NullInt64
	var domain, kind string
	var target sql.NullString
	err := q.QueryRowContext(ctx, `
SELECT id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json
FROM today_tasks WHERE id = ?`, id).Scan(
		&t.ID, &t.SnapshotID, &t.Position, &sched, &domain, &kind, &t.Title, &t.PlannedSeconds, &target,
	)
	if err != nil {
		return db.TodayTask{}, fmt.Errorf("runtime: get today task: %w", err)
	}
	if sched.Valid {
		v := int(sched.Int64)
		t.ScheduledStartMinute = &v
	}
	t.Domain = db.Domain(domain)
	t.Kind = db.SessionKind(kind)
	if target.Valid {
		v := target.String
		t.TargetJSON = &v
	}
	return t, nil
}

// CanStartTodayTask is a legacy any-evidence approximation kept for the
// existing UI test.
//
// Deprecated: non-authoritative. The authoritative rule is
// engine.EvaluateTodayTask (exact bundle completion) enforced by
// engine.StartTodayTask, which the TUI uses. New code must consult the
// engine, not this helper.
// It returns false with a reason when starting is forbidden. The UI must
// reflect this (no Start action) but must never be the only enforcement:
// callers creating sessions must consult this first.
func CanStartTodayTask(ctx context.Context, q db.DBTX, taskID db.ID) (bool, string) {
	if taskHasEvidence(ctx, q, taskID) {
		return false, "task already complete — result is read-only"
	}
	return true, ""
}
