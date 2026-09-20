package catalog

import (
	"context"
	"fmt"

	db "forge/internal/db"
)

// GetSettings returns the singleton forge_settings row.
func GetSettings(ctx context.Context, q db.DBTX) (db.ForgeSettings, error) {
	var s db.ForgeSettings
	var createdAt int64
	err := q.QueryRowContext(ctx, `SELECT id, timezone, day_start_minute, created_at FROM forge_settings WHERE id = 1`).Scan(
		&s.ID, &s.Timezone, &s.DayStartMinute, &createdAt,
	)
	if err != nil {
		return db.ForgeSettings{}, fmt.Errorf("catalog: get settings: %w", err)
	}
	s.CreatedAt = unixToTime(createdAt)
	return s, nil
}

// UpsertSettings inserts or replaces the singleton settings row.
// Used by seed/test harnesses; runtime Forge only reads it.
func UpsertSettings(ctx context.Context, q db.DBTX, s db.ForgeSettings) error {
	if s.ID == 0 {
		s.ID = 1
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO forge_settings (id, timezone, day_start_minute, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET timezone = excluded.timezone, day_start_minute = excluded.day_start_minute, created_at = excluded.created_at`,
		s.ID, s.Timezone, s.DayStartMinute, s.CreatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("catalog: upsert settings: %w", err)
	}
	return nil
}
