package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	db "forge/internal/db"
)

func openConstraintDB(t *testing.T) (*db.DB, context.Context) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "constraints.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	return database, context.Background()
}

func seedSessionParents(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	if _, err := d.ExecContext(ctx, `INSERT INTO today_snapshots (id, local_date, revision, generated_at) VALUES ('s1', '2026-09-21', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO today_tasks (id, snapshot_id, position, domain, kind, title, planned_seconds) VALUES ('t1', 's1', 1, 'GOV', 'LECTURE', 'T', 3600)`); err != nil {
		t.Fatal(err)
	}
}

// Sessions must plan positive work.
func Test21_SessionPlannedPositive(t *testing.T) {
	d, ctx := openConstraintDB(t)
	seedSessionParents(t, ctx, d)
	now := time.Now().Unix()
	if _, err := d.ExecContext(ctx, `INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, planned_seconds, started_at, status) VALUES ('a', 't1', 'GOV', 'LECTURE', 'T', 0, ?, 'ACTIVE')`, now); err == nil {
		t.Fatal("planned_seconds = 0 must fail")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, planned_seconds, started_at, status) VALUES ('b', 't1', 'GOV', 'LECTURE', 'T', -5, ?, 'ACTIVE')`, now); err == nil {
		t.Fatal("planned_seconds < 0 must fail")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, planned_seconds, started_at, status) VALUES ('c', 't1', 'GOV', 'LECTURE', 'T', 60, ?, 'ACTIVE')`, now); err != nil {
		t.Fatalf("positive planned must pass: %v", err)
	}
}

// A stopped session cannot end before it started.
func Test22_SessionStopOrder(t *testing.T) {
	d, ctx := openConstraintDB(t)
	seedSessionParents(t, ctx, d)
	now := time.Now().Unix()
	if _, err := d.ExecContext(ctx, `INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, planned_seconds, started_at, status) VALUES ('a', 't1', 'GOV', 'LECTURE', 'T', 60, ?, 'ACTIVE')`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE sessions SET status = 'STOPPED', stopped_at = ? WHERE id = 'a'`, now-60); err == nil {
		t.Fatal("stopped_at < started_at must fail")
	}
	if _, err := d.ExecContext(ctx, `UPDATE sessions SET status = 'STOPPED', stopped_at = ? WHERE id = 'a'`, now+60); err != nil {
		t.Fatalf("ordered stop must pass: %v", err)
	}
}
