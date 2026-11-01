# Forge

Single-user local study planner (Go + SQLite + Bubble Tea TUI): GOV exam prep and DSA practice.

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

## Data pipeline

`data/*.yaml` and `data/quotes.json` are the **source of truth**. `seed.sql` is
generated from them and is what populates a database.

```sh
# 1. regenerate seed.sql from the canonical data files
go run ./cmd/datagen -data data -out seed.sql

# 2. populate a fresh database
rm -f ~/.local/share/forge/forge-v2.db*
go run ./cmd/forge -db ~/.local/share/forge/forge-v2.db -seed seed.sql
```

`cmd/datagen` is dependency-free: it implements the small YAML subset the data
files use rather than adding a YAML library to `go.mod`.

### Naming conventions

- Subject names and exam names are UPPER CASE.
- Topic names and lecture names are Title Case. The policy is conservative:
  acronyms (`DI`, `PM`, `NBFC`, `OSI`), file extensions (`.docx`), `etc.`, a
  capitalized `The` before a proper noun, and brand fragments (`e-Drive`) are
  preserved as written.

### Progress markers

A lecture or DSA problem is complete when it appears at or before the
`# done till here` comment in its file. The marker is **cumulative**: it means
"everything up to and including this line is done", and everything after it is
not started. `cmd/datagen` reads the markers and emits the matching
`gov_lecture_completions` and `dsa_attempts` rows.

`go test ./cmd/datagen` guards the data files: it fails on a dangling topic
reference, an unreachable topic, a duplicated name, a casing violation, a
non-cumulative marker, or a change in catalogue size.
