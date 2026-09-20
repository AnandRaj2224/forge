package catalog

import (
	"context"
	"fmt"

	db "forge/internal/db"
)

func GetDSAProblems(ctx context.Context, q db.DBTX) ([]db.DSAProblem, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, title, pattern, difficulty, sort_order FROM dsa_problems ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list dsa problems: %w", err)
	}
	defer rows.Close()
	var out []db.DSAProblem
	for rows.Next() {
		var p db.DSAProblem
		if err := rows.Scan(&p.ID, &p.Title, &p.Pattern, &p.Difficulty, &p.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan dsa problem: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func GetDSAProblem(ctx context.Context, q db.DBTX, id db.ID) (db.DSAProblem, error) {
	var p db.DSAProblem
	err := q.QueryRowContext(ctx, `SELECT id, title, pattern, difficulty, sort_order FROM dsa_problems WHERE id = ?`, id).Scan(
		&p.ID, &p.Title, &p.Pattern, &p.Difficulty, &p.SortOrder,
	)
	if err != nil {
		return db.DSAProblem{}, fmt.Errorf("catalog: get dsa problem: %w", err)
	}
	return p, nil
}

func InsertDSAProblem(ctx context.Context, q db.DBTX, p db.DSAProblem) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Title, p.Pattern, p.Difficulty, p.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert dsa problem: %w", err)
	}
	return nil
}

func GetDSARoutine(ctx context.Context, q db.DBTX) (db.DSARoutine, error) {
	var r db.DSARoutine
	err := q.QueryRowContext(ctx, `SELECT id, planned_seconds FROM dsa_routine WHERE id = 1`).Scan(&r.ID, &r.PlannedSeconds)
	if err != nil {
		return db.DSARoutine{}, fmt.Errorf("catalog: get dsa routine: %w", err)
	}
	return r, nil
}

func UpsertDSARoutine(ctx context.Context, q db.DBTX, r db.DSARoutine) error {
	if r.ID == 0 {
		r.ID = 1
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO dsa_routine (id, planned_seconds) VALUES (?, ?)
ON CONFLICT(id) DO UPDATE SET planned_seconds = excluded.planned_seconds`, r.ID, r.PlannedSeconds); err != nil {
		return fmt.Errorf("catalog: upsert dsa routine: %w", err)
	}
	return nil
}
