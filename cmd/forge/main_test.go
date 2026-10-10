package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
