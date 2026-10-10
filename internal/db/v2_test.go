package db_test

import (
	"context"
	"path/filepath"
	"testing"

	db "forge/internal/db"
)

// Fresh V2 initialization carries the v2 marker and focus tables.
func TestV2FreshInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var v string
	if err := database.QueryRowContext(ctx, `SELECT value FROM forge_meta WHERE key='schema_version'`).Scan(&v); err != nil || v != "v2" {
		t.Fatalf("schema_version=%q err=%v", v, err)
	}
	for _, tbl := range []string{"gov_focus_state", "gov_exam_profiles", "gov_practice_rotation", "today_snapshot_tasks"} {
		var n int
		if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s missing: %v %d", tbl, err, n)
		}
	}
	// Structural integrity on a fresh database.
	var integrity string
	if err := database.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check=%q err=%v", integrity, err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen preserves everything: migrate is idempotent, seeds are not
	// reapplied over user data, and the marker survives.
	reopened, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := reopened.QueryRowContext(ctx, `SELECT value FROM forge_meta WHERE key='schema_version'`).Scan(&v); err != nil || v != "v2" {
		t.Fatalf("reopen schema_version=%q err=%v", v, err)
	}
	var fkViolations int
	rows, err := reopened.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		fkViolations++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if fkViolations != 0 {
		t.Fatalf("foreign_key_check violations=%d", fkViolations)
	}
}

// Explicit V1 database fails closed instead of being reinterpreted.
func TestV2RejectsV1Database(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v1.db")
	v1, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Minimal V1 shape: exams without focus/meta tables.
	if _, err := v1.ExecContext(ctx, `CREATE TABLE gov_exams (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, sort_order INTEGER NOT NULL UNIQUE)`); err != nil {
		t.Fatal(err)
	}
	if _, err := v1.ExecContext(ctx, `INSERT INTO gov_exams (id, name, sort_order) VALUES ('e1','E1',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := v1.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := v1.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (11, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := v1.Close(); err != nil {
		t.Fatal(err)
	}
	v2, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	if err := v2.Migrate(); err == nil {
		t.Fatal("V1 database must fail closed")
	}
	// The rejected file is untouched: its data survives and no V2 objects
	// were created inside it.
	var name string
	if err := v2.QueryRowContext(ctx, `SELECT name FROM gov_exams WHERE id='e1'`).Scan(&name); err != nil || name != "E1" {
		t.Fatalf("V1 data must survive rejection: %q %v", name, err)
	}
	for _, tbl := range []string{"forge_meta", "today_snapshots", "gov_focus_state"} {
		var n int
		if err := v2.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("rejected database must gain no V2 table %s", tbl)
		}
	}
}
