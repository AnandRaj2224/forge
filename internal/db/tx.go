package db

import (
	"context"
	"database/sql"
	"fmt"
)

// BeginTx exposes the raw transaction for callers that need *sql.Tx behavior.
func (d *DB) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("db: begin tx: %w", err)
	}
	return tx, nil
}
