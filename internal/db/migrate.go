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

// runMigrations initializes the V2 schema and applies pending migrations.
// The V2 schema is one consolidated migration (schema/001_initial.sql);
// future V2 changes add incremental files picked up here in filename order.
// Each migration runs in its own transaction and is recorded in
// schema_migrations. Errors fail loudly.
//
// Safety: an explicitly supplied database with an incompatible (V1) schema
// is never auto-migrated. It fails closed with an actionable error so old
// data stays untouched.
func runMigrations(ctx context.Context, sqlDB *sql.DB) error {
	// Verify before modifying: an incompatible database fails closed here,
	// before any table is created, so old files stay untouched.
	if err := rejectIncompatibleV1(ctx, sqlDB); err != nil {
		return err
	}

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
	// Post-apply verification: the database must carry the V2 marker and
	// every core table. Later launches re-verify without recreating or
	// reseeding anything (migrations above are skipped when recorded).
	return verifyV2Schema(ctx, sqlDB)
}

// verifyV2Schema checks the schema marker plus all core V2 tables.
// It never writes; reseeds live inside migrations (OR IGNORE) and run once.
func verifyV2Schema(ctx context.Context, sqlDB *sql.DB) error {
	var marker string
	if err := sqlDB.QueryRowContext(ctx, `SELECT value FROM forge_meta WHERE key='schema_version'`).Scan(&marker); err != nil || marker != "v2" {
		return fmt.Errorf("db: V2 schema marker missing (forge_meta.schema_version=v2)")
	}
	for _, tbl := range []string{
		"forge_settings", "gov_subjects", "gov_topics", "gov_lectures",
		"gov_exams", "gov_exam_subjects", "gov_exam_syllabus",
		"gov_exam_profiles", "gov_focus_state",
		"gov_lecture_routine", "gov_practice_routine",
		"gov_practice_sessions", "gov_practice_tests", "gov_practice_tracks",
		"dsa_problems", "dsa_routine", "swe_areas", "swe_routine_slots",
		"today_snapshots", "today_tasks", "today_snapshot_tasks",
		"sessions", "gov_subject_activation", "gov_lecture_completions",
		"gov_practice_records", "gov_practice_rotation",
		"dsa_attempts", "dsa_repeat_state", "swe_task_completions",
		"mocks", "mock_sections", "mock_weak_topics",
		"weekly_reviews", "quotes", "daily_quote_state",
	} {
		var n int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil {
			return fmt.Errorf("db: verify table %s: %w", tbl, err)
		}
		if n != 1 {
			return fmt.Errorf("db: required V2 table %s missing", tbl)
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

// rejectIncompatibleV1 fails closed when the selected database carries a
// non-V2 schema (V1 tables without the forge_meta v2 marker and the V2
// focus table). Fresh/empty databases and genuine V2 databases pass.
// Memory/file: URIs used by tests start empty and pass through.
func rejectIncompatibleV1(ctx context.Context, sqlDB *sql.DB) error {
	hasTable := func(name string) bool {
		var n int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
			return false
		}
		return n > 0
	}
	hasGovExams := hasTable("gov_exams")
	hasFocus := hasTable("gov_focus_state")
	hasMeta := hasTable("forge_meta")
	if !hasGovExams {
		return nil
	}
	if hasFocus && hasMeta {
		return nil
	}
	var metaVal string
	if hasMeta {
		if err := sqlDB.QueryRowContext(ctx, `SELECT value FROM forge_meta WHERE key='schema_version'`).Scan(&metaVal); err == nil && metaVal == "v2" {
			return nil
		}
	}
	return fmt.Errorf("db: incompatible V1 database: this Forge V2 build uses a separate database (default ~/.local/share/forge/forge-v2.db). The V1 file is preserved untouched; choose an empty -db path or set FORGE_DB to a new V2 path")
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
