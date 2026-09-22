package catalog

import (
	"context"
	"fmt"

	db "forge/internal/db"
)

func GetSWEAreas(ctx context.Context, q db.DBTX) ([]db.SWEArea, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, sort_order FROM swe_areas ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list swe areas: %w", err)
	}
	defer rows.Close()
	var out []db.SWEArea
	for rows.Next() {
		var a db.SWEArea
		if err := rows.Scan(&a.ID, &a.Name, &a.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan swe area: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func InsertSWEArea(ctx context.Context, q db.DBTX, a db.SWEArea) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO swe_areas (id, name, sort_order) VALUES (?, ?, ?)`, a.ID, a.Name, a.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert swe area: %w", err)
	}
	return nil
}

func GetSWETask(ctx context.Context, q db.DBTX, id db.ID) (db.SWETask, error) {
	var t db.SWETask
	err := q.QueryRowContext(ctx, `SELECT id, area_id, title, sort_order FROM swe_tasks WHERE id = ?`, id).Scan(
		&t.ID, &t.AreaID, &t.Title, &t.SortOrder,
	)
	if err != nil {
		return db.SWETask{}, fmt.Errorf("catalog: get swe task: %w", err)
	}
	return t, nil
}

func GetSWETasks(ctx context.Context, q db.DBTX, areaID db.ID) ([]db.SWETask, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, area_id, title, sort_order FROM swe_tasks WHERE area_id = ? ORDER BY sort_order`, areaID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list swe tasks: %w", err)
	}
	defer rows.Close()
	var out []db.SWETask
	for rows.Next() {
		var t db.SWETask
		if err := rows.Scan(&t.ID, &t.AreaID, &t.Title, &t.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan swe task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func InsertSWETask(ctx context.Context, q db.DBTX, t db.SWETask) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES (?, ?, ?, ?)`, t.ID, t.AreaID, t.Title, t.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert swe task: %w", err)
	}
	return nil
}

// GetSWERoutine returns slots for a weekday ordered by position.
func GetSWERoutine(ctx context.Context, q db.DBTX, weekday int) ([]db.SWERoutineSlot, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, weekday, position, area_id, duration_seconds FROM swe_routine WHERE weekday = ? ORDER BY position`, weekday)
	if err != nil {
		return nil, fmt.Errorf("catalog: list swe routine: %w", err)
	}
	defer rows.Close()
	var out []db.SWERoutineSlot
	for rows.Next() {
		var s db.SWERoutineSlot
		if err := rows.Scan(&s.ID, &s.Weekday, &s.Position, &s.AreaID, &s.DurationSeconds); err != nil {
			return nil, fmt.Errorf("catalog: scan swe routine: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetSWERoutineAll returns the full weekly routine ordered by weekday/position.
func GetSWERoutineAll(ctx context.Context, q db.DBTX) ([]db.SWERoutineSlot, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, weekday, position, area_id, duration_seconds FROM swe_routine ORDER BY weekday, position`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list swe routine all: %w", err)
	}
	defer rows.Close()
	var out []db.SWERoutineSlot
	for rows.Next() {
		var s db.SWERoutineSlot
		if err := rows.Scan(&s.ID, &s.Weekday, &s.Position, &s.AreaID, &s.DurationSeconds); err != nil {
			return nil, fmt.Errorf("catalog: scan swe routine: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func InsertSWERoutineSlot(ctx context.Context, q db.DBTX, s db.SWERoutineSlot) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES (?, ?, ?, ?, ?)`,
		s.ID, s.Weekday, s.Position, s.AreaID, s.DurationSeconds); err != nil {
		return fmt.Errorf("catalog: insert swe routine slot: %w", err)
	}
	return nil
}
