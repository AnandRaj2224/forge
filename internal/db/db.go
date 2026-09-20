package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB is the Forge database handle.
// It wraps database/sql with FK enforcement and WAL for single-user use.
type DB struct {
	sqlDB *sql.DB
	path  string
}

// DBTX is the minimal query interface satisfied by *DB and *sql.Tx.
// Repository functions accept DBTX so callers can pass either a
// direct DB handle or a transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var _ DBTX = (*DB)(nil)

// dsnForPath builds a modernc.org/sqlite DSN that enables foreign keys
// (and WAL for file-backed DBs) on every pooled connection.
func dsnForPath(path string) string {
	if path == "" {
		return ""
	}
	if path == ":memory:" {
		return "file::memory:?cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}
	if strings.HasPrefix(path, "file:") {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		if strings.Contains(path, "_pragma=foreign_keys") {
			return path
		}
		return path + sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}
	return "file:" + path + "?cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// Open opens the SQLite database at path and configures it.
// path is filesystem-driven; ":memory:" is supported for tests.
func Open(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("db: empty path")
	}
	dsn := dsnForPath(path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %q: %w", path, err)
	}
	// Single-user application: serialize writes to avoid SQLITE_BUSY.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: ping %q: %w", path, err)
	}
	// Belt-and-suspenders: ensure FK enforcement on the pooled connection.
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: enable foreign_keys: %w", err)
	}
	// WAL for file-backed DBs; harmless/no-op attempt for memory.
	if path != ":memory:" && !strings.HasPrefix(path, "file::memory:") {
		if _, err := sqlDB.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("db: set WAL mode: %w", err)
		}
		if _, err := sqlDB.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("db: set busy_timeout: %w", err)
		}
	}
	// Verify FK enforcement is actually on.
	var fkOn int
	if err := sqlDB.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fkOn); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: query foreign_keys pragma: %w", err)
	}
	if fkOn != 1 {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: foreign_keys pragma not enabled")
	}
	return &DB{sqlDB: sqlDB, path: path}, nil
}

// SQL returns the underlying *sql.DB for tooling that needs it.
func (d *DB) SQL() *sql.DB { return d.sqlDB }

// Path returns the path this DB was opened with.
func (d *DB) Path() string { return d.path }

// Close closes the database.
func (d *DB) Close() error {
	if d == nil || d.sqlDB == nil {
		return nil
	}
	return d.sqlDB.Close()
}

// ExecContext delegates to the underlying pool.
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.sqlDB.ExecContext(ctx, query, args...)
}

// QueryContext delegates to the underlying pool.
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.sqlDB.QueryContext(ctx, query, args...)
}

// QueryRowContext delegates to the underlying pool.
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.sqlDB.QueryRowContext(ctx, query, args...)
}

// Migrate applies all embedded migrations. See migrate.go.
func (d *DB) Migrate() error {
	return d.MigrateContext(context.Background())
}

// MigrateContext applies migrations with a caller context.
func (d *DB) MigrateContext(ctx context.Context) error {
	return runMigrations(ctx, d.sqlDB)
}

// WithTx runs fn inside a transaction. fn receives the *sql.Tx which
// satisfies DBTX. Rolls back on error, commits on success.
func (d *DB) WithTx(ctx context.Context, fn func(DBTX) error) error {
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit tx: %w", err)
	}
	return nil
}
