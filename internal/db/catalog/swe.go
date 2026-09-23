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

func GetSWEArea(ctx context.Context, q db.DBTX, id db.ID) (db.SWEArea, error) {
	var a db.SWEArea
	err := q.QueryRowContext(ctx, `SELECT id, name, sort_order FROM swe_areas WHERE id = ?`, id).Scan(
		&a.ID, &a.Name, &a.SortOrder,
	)
	if err != nil {
		return db.SWEArea{}, fmt.Errorf("catalog: get swe area: %w", err)
	}
	return a, nil
}

func InsertSWEArea(ctx context.Context, q db.DBTX, a db.SWEArea) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO swe_areas (id, name, sort_order) VALUES (?, ?, ?)`, a.ID, a.Name, a.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert swe area: %w", err)
	}
	return nil
}

// GetSWERoutine returns slots for a weekday ordered by position.
func GetSWERoutine(ctx context.Context, q db.DBTX, weekday int) ([]db.SWERoutineSlot, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, weekday, position, area_id, duration_seconds FROM swe_routine_slots WHERE weekday = ? ORDER BY position`, weekday)
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
	rows, err := q.QueryContext(ctx, `SELECT id, weekday, position, area_id, duration_seconds FROM swe_routine_slots ORDER BY weekday, position`)
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

// GetSWERoutineByArea returns slots for one area ordered by weekday/position.
func GetSWERoutineByArea(ctx context.Context, q db.DBTX, areaID db.ID) ([]db.SWERoutineSlot, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, weekday, position, area_id, duration_seconds FROM swe_routine_slots WHERE area_id = ? ORDER BY weekday, position`, areaID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list swe routine by area: %w", err)
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
	if _, err := q.ExecContext(ctx, `INSERT INTO swe_routine_slots (id, weekday, position, area_id, duration_seconds) VALUES (?, ?, ?, ?, ?)`,
		s.ID, s.Weekday, s.Position, s.AreaID, s.DurationSeconds); err != nil {
		return fmt.Errorf("catalog: insert swe routine slot: %w", err)
	}
	return nil
}
