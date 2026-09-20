package runtime

import (
	"context"
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

// UpsertWeeklyReview inserts or replaces the row for a week.
func UpsertWeeklyReview(ctx context.Context, q db.DBTX, r db.WeeklyReview) error {
	if _, err := q.ExecContext(ctx, `
INSERT INTO weekly_reviews (week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json, reflection_worked, reflection_change)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(week) DO UPDATE SET
  week_start = excluded.week_start,
  week_end = excluded.week_end,
  generated_at = excluded.generated_at,
  updated_at = excluded.updated_at,
  planned_seconds = excluded.planned_seconds,
  elapsed_seconds = excluded.elapsed_seconds,
  overrun_seconds = excluded.overrun_seconds,
  report_json = excluded.report_json,
  reflection_worked = excluded.reflection_worked,
  reflection_change = excluded.reflection_change`,
		r.ID, r.WeekStart, r.WeekEnd, r.GeneratedAt.Unix(), r.UpdatedAt.Unix(),
		r.PlannedSeconds, r.ElapsedSeconds, r.OverrunSeconds,
		r.ReportJSON, r.ReflectionWorked, r.ReflectionChange,
	); err != nil {
		return fmt.Errorf("runtime: upsert weekly review: %w", err)
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
