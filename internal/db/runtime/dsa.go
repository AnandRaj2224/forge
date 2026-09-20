package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "forge/internal/db"
)

// ---- DSA attempts ----

// InsertDSAAttempt inserts one attempt row.
// The partial unique index enforces a single NEW attempt per problem.
func InsertDSAAttempt(ctx context.Context, q db.DBTX, a db.DSAAttempt) error {
	var solve any
	if a.SolveTimeSeconds != nil {
		solve = *a.SolveTimeSeconds
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.SessionID, a.ProblemID, string(a.Mode), a.AttemptedAt.Unix(),
		boolToInt(a.Independent), boolToInt(a.Optimal), boolToInt(a.HintUsed), solve,
	); err != nil {
		return fmt.Errorf("runtime: insert dsa attempt: %w", err)
	}
	return nil
}

// GetDSAAttempts returns attempts for a problem ordered by attempted_at.
func GetDSAAttempts(ctx context.Context, q db.DBTX, problemID db.ID) ([]db.DSAAttempt, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds
FROM dsa_attempts WHERE problem_id = ? ORDER BY attempted_at`, problemID)
	if err != nil {
		return nil, fmt.Errorf("runtime: list dsa attempts: %w", err)
	}
	defer rows.Close()
	var out []db.DSAAttempt
	for rows.Next() {
		var a db.DSAAttempt
		var mode string
		var attempted int64
		var indep, opt, hint int
		var solve sql.NullInt64
		if err := rows.Scan(&a.ID, &a.SessionID, &a.ProblemID, &mode, &attempted, &indep, &opt, &hint, &solve); err != nil {
			return nil, fmt.Errorf("runtime: scan dsa attempt: %w", err)
		}
		a.Mode = db.DSAMode(mode)
		a.AttemptedAt = time.Unix(attempted, 0).UTC()
		a.Independent = indep == 1
		a.Optimal = opt == 1
		a.HintUsed = hint == 1
		if solve.Valid {
			v := int(solve.Int64)
			a.SolveTimeSeconds = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetDSAAttemptsBySession returns attempts for a session.
func GetDSAAttemptsBySession(ctx context.Context, q db.DBTX, sessionID db.ID) ([]db.DSAAttempt, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds
FROM dsa_attempts WHERE session_id = ? ORDER BY attempted_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runtime: dsa attempts by session: %w", err)
	}
	defer rows.Close()
	var out []db.DSAAttempt
	for rows.Next() {
		var a db.DSAAttempt
		var mode string
		var attempted int64
		var indep, opt, hint int
		var solve sql.NullInt64
		if err := rows.Scan(&a.ID, &a.SessionID, &a.ProblemID, &mode, &attempted, &indep, &opt, &hint, &solve); err != nil {
			return nil, fmt.Errorf("runtime: scan dsa attempt: %w", err)
		}
		a.Mode = db.DSAMode(mode)
		a.AttemptedAt = time.Unix(attempted, 0).UTC()
		a.Independent = indep == 1
		a.Optimal = opt == 1
		a.HintUsed = hint == 1
		if solve.Valid {
			v := int(solve.Int64)
			a.SolveTimeSeconds = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- DSA repeat state ----

// GetDSARepeatState returns current scheduling state for a problem.
func GetDSARepeatState(ctx context.Context, q db.DBTX, problemID db.ID) (db.DSARepeatState, error) {
	var s db.DSARepeatState
	var updated int64
	err := q.QueryRowContext(ctx, `SELECT problem_id, repeat_stage, current_interval_days, next_review_date, updated_at FROM dsa_repeat_state WHERE problem_id = ?`, problemID).Scan(
		&s.ProblemID, &s.RepeatStage, &s.CurrentIntervalDays, &s.NextReviewDate, &updated,
	)
	if err != nil {
		return db.DSARepeatState{}, fmt.Errorf("runtime: get repeat state: %w", err)
	}
	s.UpdatedAt = time.Unix(updated, 0).UTC()
	return s, nil
}

// UpsertDSARepeatState inserts or replaces scheduling state.
func UpsertDSARepeatState(ctx context.Context, q db.DBTX, s db.DSARepeatState) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(problem_id) DO UPDATE SET repeat_stage = excluded.repeat_stage, current_interval_days = excluded.current_interval_days, next_review_date = excluded.next_review_date, updated_at = excluded.updated_at`,
		s.ProblemID, s.RepeatStage, s.CurrentIntervalDays, s.NextReviewDate, s.UpdatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("runtime: upsert repeat state: %w", err)
	}
	return nil
}

// ListDSARepeatDue returns repeat states ordered by next_review_date.
func ListDSARepeatDue(ctx context.Context, q db.DBTX) ([]db.DSARepeatState, error) {
	rows, err := q.QueryContext(ctx, `SELECT problem_id, repeat_stage, current_interval_days, next_review_date, updated_at FROM dsa_repeat_state ORDER BY next_review_date`)
	if err != nil {
		return nil, fmt.Errorf("runtime: list repeat due: %w", err)
	}
	defer rows.Close()
	var out []db.DSARepeatState
	for rows.Next() {
		var s db.DSARepeatState
		var updated int64
		if err := rows.Scan(&s.ProblemID, &s.RepeatStage, &s.CurrentIntervalDays, &s.NextReviewDate, &updated); err != nil {
			return nil, fmt.Errorf("runtime: scan repeat state: %w", err)
		}
		s.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}
