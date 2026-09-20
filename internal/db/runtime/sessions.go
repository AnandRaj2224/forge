package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "forge/internal/db"
)

// CreateSession creates one session for a Today task.
func CreateSession(ctx context.Context, q db.DBTX, s db.Session) error {
	var stopped any
	if s.StoppedAt != nil {
		stopped = s.StoppedAt.Unix()
	}
	var target any
	if s.TargetSnapshotJSON != nil {
		target = *s.TargetSnapshotJSON
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.TodayTaskID, string(s.Domain), string(s.Kind), s.TitleSnapshot, target,
		s.PlannedSeconds, s.StartedAt.Unix(), stopped, string(s.Status),
	); err != nil {
		return fmt.Errorf("runtime: create session: %w", err)
	}
	return nil
}

// GetSession returns a session by id.
func GetSession(ctx context.Context, q db.DBTX, sessionID db.ID) (db.Session, error) {
	return scanSession(q.QueryRowContext(ctx, `
SELECT id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status
FROM sessions WHERE id = ?`, sessionID))
}

// GetActiveSession returns the single ACTIVE session, or sql.ErrNoRows.
func GetActiveSession(ctx context.Context, q db.DBTX) (db.Session, error) {
	return scanSession(q.QueryRowContext(ctx, `
SELECT id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status
FROM sessions WHERE status = 'ACTIVE' LIMIT 1`))
}

// StopSession transitions an ACTIVE session to STOPPED.
func StopSession(ctx context.Context, q db.DBTX, sessionID db.ID, stoppedAt time.Time) error {
	res, err := q.ExecContext(ctx, `UPDATE sessions SET status = 'STOPPED', stopped_at = ? WHERE id = ? AND status = 'ACTIVE'`, stoppedAt.Unix(), sessionID)
	if err != nil {
		return fmt.Errorf("runtime: stop session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("runtime: stop session rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("runtime: stop session: no active session %q", sessionID)
	}
	return nil
}

// ListSessions returns sessions ordered by started_at.
func ListSessions(ctx context.Context, q db.DBTX) ([]db.Session, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status
FROM sessions ORDER BY started_at`)
	if err != nil {
		return nil, fmt.Errorf("runtime: list sessions: %w", err)
	}
	defer rows.Close()
	var out []db.Session
	for rows.Next() {
		var s db.Session
		var domain, kind, status string
		var target sql.NullString
		var started int64
		var stopped sql.NullInt64
		if err := rows.Scan(&s.ID, &s.TodayTaskID, &domain, &kind, &s.TitleSnapshot, &target, &s.PlannedSeconds, &started, &stopped, &status); err != nil {
			return nil, fmt.Errorf("runtime: scan session: %w", err)
		}
		s.Domain = db.Domain(domain)
		s.Kind = db.SessionKind(kind)
		s.Status = db.SessionStatus(status)
		s.StartedAt = time.Unix(started, 0).UTC()
		if target.Valid {
			v := target.String
			s.TargetSnapshotJSON = &v
		}
		if stopped.Valid {
			v := time.Unix(stopped.Int64, 0).UTC()
			s.StoppedAt = &v
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanSession(row *sql.Row) (db.Session, error) {
	var s db.Session
	var domain, kind, status string
	var target sql.NullString
	var started int64
	var stopped sql.NullInt64
	if err := row.Scan(&s.ID, &s.TodayTaskID, &domain, &kind, &s.TitleSnapshot, &target, &s.PlannedSeconds, &started, &stopped, &status); err != nil {
		return db.Session{}, fmt.Errorf("runtime: scan session: %w", err)
	}
	s.Domain = db.Domain(domain)
	s.Kind = db.SessionKind(kind)
	s.Status = db.SessionStatus(status)
	s.StartedAt = time.Unix(started, 0).UTC()
	if target.Valid {
		v := target.String
		s.TargetSnapshotJSON = &v
	}
	if stopped.Valid {
		v := time.Unix(stopped.Int64, 0).UTC()
		s.StoppedAt = &v
	}
	return s, nil
}
