package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "forge/internal/db"
)

// GetWeeklyReview returns the review for an ISO week (YYYY-Www).
func GetWeeklyReview(ctx context.Context, q db.DBTX, week string) (db.WeeklyReview, error) {
	var r db.WeeklyReview
	var generated, updated int64
	err := q.QueryRowContext(ctx, `
SELECT week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json, reflection_worked, reflection_change
FROM weekly_reviews WHERE week = ?`, week).Scan(
		&r.ID, &r.WeekStart, &r.WeekEnd, &generated, &updated,
		&r.PlannedSeconds, &r.ElapsedSeconds, &r.OverrunSeconds,
		&r.ReportJSON, &r.ReflectionWorked, &r.ReflectionChange,
	)
	if err != nil {
		return db.WeeklyReview{}, fmt.Errorf("runtime: get weekly review: %w", err)
	}
	r.GeneratedAt = time.Unix(generated, 0).UTC()
	r.UpdatedAt = time.Unix(updated, 0).UTC()
	return r, nil
}

// InsertWeeklyReviewOnce inserts the row for a week only when no row
// exists. On a conflicting week it returns the uniqueness error without
// touching the stored row, so competing snapshot saves are first-writer-
// wins: frozen metrics can never be silently overwritten.
func InsertWeeklyReviewOnce(ctx context.Context, q db.DBTX, r db.WeeklyReview) error {
	if _, err := q.ExecContext(ctx, `
INSERT INTO weekly_reviews (week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json, reflection_worked, reflection_change)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.WeekStart, r.WeekEnd, r.GeneratedAt.Unix(), r.UpdatedAt.Unix(),
		r.PlannedSeconds, r.ElapsedSeconds, r.OverrunSeconds,
		r.ReportJSON, r.ReflectionWorked, r.ReflectionChange,
	); err != nil {
		return fmt.Errorf("runtime: insert weekly review: %w", err)
	}
	return nil
}

// UpdateWeeklyReflections rewrites only the reflections (plus updated_at)
// of a stored review. Metric columns are not mentioned, so this path
// physically cannot replace a frozen snapshot. Returns an error wrapping
// sql.ErrNoRows when no row exists for the week.
func UpdateWeeklyReflections(ctx context.Context, q db.DBTX, weekID, worked, change string, updatedAt time.Time) error {
	res, err := q.ExecContext(ctx, `
UPDATE weekly_reviews SET reflection_worked = ?, reflection_change = ?, updated_at = ? WHERE week = ?`,
		worked, change, updatedAt.Unix(), string(weekID),
	)
	if err != nil {
		return fmt.Errorf("runtime: update weekly reflections: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("runtime: update weekly reflections rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("runtime: update weekly reflections: %w", sql.ErrNoRows)
	}
	return nil
}

// ListWeeklyReviews returns reviews ordered by week.
func ListWeeklyReviews(ctx context.Context, q db.DBTX) ([]db.WeeklyReview, error) {
	rows, err := q.QueryContext(ctx, `
SELECT week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json, reflection_worked, reflection_change
FROM weekly_reviews ORDER BY week`)
	if err != nil {
		return nil, fmt.Errorf("runtime: list weekly reviews: %w", err)
	}
	defer rows.Close()
	var out []db.WeeklyReview
	for rows.Next() {
		var r db.WeeklyReview
		var generated, updated int64
		if err := rows.Scan(&r.ID, &r.WeekStart, &r.WeekEnd, &generated, &updated,
			&r.PlannedSeconds, &r.ElapsedSeconds, &r.OverrunSeconds,
			&r.ReportJSON, &r.ReflectionWorked, &r.ReflectionChange); err != nil {
			return nil, fmt.Errorf("runtime: scan weekly review: %w", err)
		}
		r.GeneratedAt = time.Unix(generated, 0).UTC()
		r.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
