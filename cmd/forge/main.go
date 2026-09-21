package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/tui"
)

func main() {
	dbPath := flag.String("db", "", "path to forge sqlite db (default dev/forge-dev.db)")
	flag.Parse()
	path := *dbPath
	if path == "" {
		path = os.Getenv("FORGE_DB")
	}
	if path == "" {
		path = filepath.Join("dev", "forge-dev.db")
		if _, err := os.Stat(path); err != nil {
			// Fallback when launched from another cwd: walk up.
			abs, _ := filepath.Abs(".")
			for d := abs; d != "/" && d != "."; d = filepath.Dir(d) {
				cand := filepath.Join(d, "dev", "forge-dev.db")
				if _, err := os.Stat(cand); err == nil {
					path = cand
					break
				}
			}
		}
	}
	database, err := db.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	app := tui.NewApp(database)
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
}
