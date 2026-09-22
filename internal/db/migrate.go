package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed schema/*.sql
var schemaFS embed.FS

// runMigrations applies embedded SQL migrations deterministically.
// Each migration runs in its own transaction and is recorded in
// schema_migrations. Errors fail loudly.
func runMigrations(ctx context.Context, sqlDB *sql.DB) error {
	if _, err := sqlDB.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
)`); err != nil {
		return fmt.Errorf("db: create schema_migrations: %w", err)
	}

	entries, err := schemaFS.ReadDir("schema")
	if err != nil {
		return fmt.Errorf("db: read schema dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version := migrationVersion(name)
		var exists int
		err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("db: check migration %s: %w", name, err)
		}
		if exists > 0 {
			continue
		}
		body, err := schemaFS.ReadFile("schema/" + name)
		if err != nil {
			return fmt.Errorf("db: read migration %s: %w", name, err)
		}
		if err := applyMigration(ctx, sqlDB, version, string(body)); err != nil {
			return fmt.Errorf("db: apply migration %s: %w", name, err)
		}
	}
	return nil
}

func migrationVersion(name string) int {
	// "001_initial.sql" -> 1
	var v int
	_, _ = fmt.Sscanf(name, "%d_", &v)
	return v
}

// applyMigration executes one migration file transactionally.
func applyMigration(ctx context.Context, sqlDB *sql.DB, version int, body string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range splitStatements(body) {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec statement %.80q: %w", stmt, err)
		}
	}
	appliedAt := time.Now().UTC().Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, appliedAt); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// splitStatements splits a migration file on semicolons.
// The schema contains no semicolons inside string literals, so a naive
// split is deterministic and sufficient — except trigger bodies, which
// legitimately contain semicolons between BEGIN and END. Chunks belonging
// to a CREATE TRIGGER statement are therefore rejoined until the closing
// END before splitting.
func splitStatements(body string) []string {
	// Strip line comments starting with -- to avoid stray semicolons there.
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if idx := strings.Index(ln, "--"); idx >= 0 {
			lines[i] = ln[:idx]
		}
	}
	cleaned := strings.Join(lines, "\n")
	parts := strings.Split(cleaned, ";")
	out := make([]string, 0, len(parts))
	var pending string
	inTrigger := false
	flush := func(s string) {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	for _, p := range parts {
		if !inTrigger {
			trimmed := strings.TrimSpace(p)
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "CREATE TRIGGER") || strings.HasPrefix(upper, "CREATE TEMP TRIGGER") {
				// A trigger body follows; rejoin until its END.
				inTrigger = true
				pending = p
				if isTriggerEnd(pending) {
					inTrigger = false
					flush(pending)
					pending = ""
				}
				continue
			}
			flush(p)
			continue
		}
		pending += ";" + p
		if isTriggerEnd(pending) {
			inTrigger = false
			flush(pending)
			pending = ""
		}
	}
	flush(pending)
	return out
}

// isTriggerEnd reports whether a CREATE TRIGGER chunk has reached its
// closing END (the last significant token).
func isTriggerEnd(chunk string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(chunk)))
	if len(fields) == 0 {
		return false
	}
	return fields[len(fields)-1] == "END"
}
