package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/engine"
)

func TestApplySeedAndEnsureTodayWithPracticeTestOrders(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seeded.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	seedPath := filepath.Join("..", "..", "seed.sql")
	if err := applySeed(database, seedPath); err != nil {
		t.Fatalf("applySeed: %v", err)
	}

	ctx := context.Background()
	var numSessions, numTests, numTracks int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_sessions`).Scan(&numSessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_tests`).Scan(&numTests); err != nil {
		t.Fatalf("count tests: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_tracks`).Scan(&numTracks); err != nil {
		t.Fatalf("count tracks: %v", err)
	}

	if numSessions != 391 {
		t.Errorf("sessions = %d, want 391", numSessions)
	}
	if numTests != 1062 {
		t.Errorf("tests = %d, want 1062", numTests)
	}
	if numTracks != 3 {
		t.Errorf("tracks = %d, want 3", numTracks)
	}

	// Now verify engine.EnsureToday plans 1 session each from Quant, English, and Reasoning
	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	snap, tasks, err := engine.EnsureToday(ctx, database, "2026-10-31", now)
	if err != nil {
		t.Fatalf("EnsureToday: %v", err)
	}
	if snap.ID == "" {
		t.Fatal("empty snapshot ID")
	}

	var practiceTasks []db.TodayTask
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			practiceTasks = append(practiceTasks, tk)
		}
	}
	if len(practiceTasks) != 3 {
		t.Fatalf("expected 3 practice tasks, got %d", len(practiceTasks))
	}

	// Task 1: Quant practice ses_1
	qTgt, err := db.UnmarshalGovPracticeTarget(*practiceTasks[0].TargetJSON)
	if err != nil {
		t.Fatalf("unmarshal quant target: %v", err)
	}
	if qTgt.SubjectID != db.PracticeSubjectQuant || qTgt.SessionCode != "ses_1" || len(qTgt.TopicIDs) != 3 {
		t.Fatalf("quant target mismatch: %+v", qTgt)
	}
	if !strings.Contains(practiceTasks[0].Title, "Number System - Beginner Level") {
		t.Fatalf("quant title mismatch: %q", practiceTasks[0].Title)
	}

	// Task 2: English practice ses_1
	eTgt, err := db.UnmarshalGovPracticeTarget(*practiceTasks[1].TargetJSON)
	if err != nil {
		t.Fatalf("unmarshal english target: %v", err)
	}
	if eTgt.SubjectID != db.PracticeSubjectEnglish || eTgt.SessionCode != "ses_1" || len(eTgt.TopicIDs) != 2 {
		t.Fatalf("english target mismatch: %+v", eTgt)
	}

	// Task 3: Reasoning practice ses_1
	rTgt, err := db.UnmarshalGovPracticeTarget(*practiceTasks[2].TargetJSON)
	if err != nil {
		t.Fatalf("unmarshal reasoning target: %v", err)
	}
	if rTgt.SubjectID != db.PracticeSubjectReasoning || rTgt.SessionCode != "ses_1" || len(rTgt.TopicIDs) != 3 {
		t.Fatalf("reasoning target mismatch: %+v", rTgt)
	}
}

func TestDefaultDBPathUsesXDGLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := defaultDBPath()
	if err != nil {
		t.Fatalf("defaultDBPath: %v", err)
	}
	want := filepath.Join(home, ".local", "share", "forge", "forge-v2.db")
	if got != want {
		t.Fatalf("defaultDBPath = %q, want %q", got, want)
	}
}

func TestResolveDBPathPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	def, err := defaultDBPath()
	if err != nil {
		t.Fatalf("defaultDBPath: %v", err)
	}

	// Flag wins over env and default.
	got, err := resolveDBPath("/tmp/flag.db", "/tmp/env.db")
	if err != nil {
		t.Fatalf("resolveDBPath flag: %v", err)
	}
	if got != "/tmp/flag.db" {
		t.Fatalf("flag precedence: got %q, want %q", got, "/tmp/flag.db")
	}

	// Env wins over default.
	got, err = resolveDBPath("", "/tmp/env.db")
	if err != nil {
		t.Fatalf("resolveDBPath env: %v", err)
	}
	if got != "/tmp/env.db" {
		t.Fatalf("env precedence: got %q, want %q", got, "/tmp/env.db")
	}

	// Empty flag+env falls back to the XDG default, never dev/forge-dev.db.
	got, err = resolveDBPath("", "")
	if err != nil {
		t.Fatalf("resolveDBPath default: %v", err)
	}
	if got != def {
		t.Fatalf("default precedence: got %q, want %q", got, def)
	}
	if got == filepath.Join("dev", "forge-dev.db") {
		t.Fatalf("default must not be dev/forge-dev.db")
	}
}

func TestEnsureDBParentDirCreatesWithParents(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, ".local", "share", "forge", "forge.db")

	if err := ensureDBParentDir(target); err != nil {
		t.Fatalf("ensureDBParentDir: %v", err)
	}
	info, err := os.Stat(filepath.Dir(target))
	if err != nil {
		t.Fatalf("parent dir missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("parent is not a dir: %q", filepath.Dir(target))
	}

	// Memory DSNs are no-ops.
	if err := ensureDBParentDir(":memory:"); err != nil {
		t.Fatalf("ensureDBParentDir :memory:: %v", err)
	}
}
