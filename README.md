# Forge

Single-user local study planner (Go + SQLite + Bubble Tea TUI): GOV exam prep, DSA practice, and SWE time allocation.

## Prerequisites

- Go 1.27+
- `sqlite3` CLI (only for manual DB inspection; not required to run)

## Build / Run / Test

```sh
go build -o /tmp/forge ./cmd/forge
go run ./cmd/forge -db /tmp/forge-test.db
go test -count=1 ./...
go vet ./...
go run ./cmd/dbsmoke
```

Opt-in large-scale stress test (isolated temp DBs, skipped by default):

```sh
STRESS25K=1 go test ./internal/engine -run TestStressScale -v -count=1
```

## Database

- Default: `~/.local/share/forge/forge-v2.db`
- Override: `-db <path>` > `FORGE_DB` env > default
- Fresh databases initialize from `internal/db/schema/001_initial.sql` via `database.Migrate()`. Never point Forge at the old V1 database; V1 files are rejected without modification.
