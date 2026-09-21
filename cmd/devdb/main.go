package main

import (
	"fmt"
	"os"
	"os/exec"

	db "forge/internal/db"
)

func main() {
	const path = "dev/forge-dev.db"
	const fixture = "dev/dev_data.sql"

	_ = os.Remove(path)

	database, err := db.Open(path)
	if err != nil {
		panic(err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		panic(err)
	}
	database.Close()

	// Load the development fixture (static catalogue + current-week history
	// + mock evidence) so the TUI renders dense History/Mocks screens.
	// SQLite remains the canonical runtime storage; no JSON/YAML truth.
	if _, err := os.Stat(fixture); err == nil {
		cmd := exec.Command("sqlite3", path, ".read "+fixture)
		// sqlite3 CLI reads dot-commands from stdin; pass via args file.
		cmd = exec.Command("sqlite3", path)
		in, err := os.Open(fixture)
		if err != nil {
			panic(err)
		}
		defer in.Close()
		cmd.Stdin = in
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			panic(fmt.Sprintf("load fixture: %v", err))
		}
	}

	fmt.Println("dev db ready:", path)
}
