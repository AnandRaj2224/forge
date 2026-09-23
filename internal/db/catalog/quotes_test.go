package catalog_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

func openQuotesTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "quotes.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func TestQuotesMigrationCreatesTable(t *testing.T) {
	database := openQuotesTestDB(t)
	ctx := context.Background()
	var name string
	if err := database.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='quotes'`).Scan(&name); err != nil {
		t.Fatalf("quotes table missing: %v", err)
	}
	rows, err := database.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='quotes'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ddl string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		if s.Valid {
			ddl = s.String
		}
	}
	upper := strings.ToUpper(ddl)
	if !strings.Contains(upper, "QUOTE TEXT NOT NULL") {
		t.Fatalf("quotes.quote must be NOT NULL, ddl=%q", ddl)
	}
	if !strings.Contains(upper, "AUTHOR TEXT NOT NULL") {
		t.Fatalf("quotes.author must be NOT NULL, ddl=%q", ddl)
	}
	// Author must never be NULL: raw NULL insert must fail.
	if _, err := database.ExecContext(ctx, `INSERT INTO quotes (id, quote, author) VALUES (999, 'x', NULL)`); err == nil {
		t.Fatal("author NULL must be rejected")
	}
}

func TestRandomQuoteRetrieval(t *testing.T) {
	database := openQuotesTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Stay hungry.", Author: "A1"}); err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 2, Quote: "Keep going.", Author: "A2"}); err != nil {
		t.Fatalf("insert 2: %v", err)
	}
	got, err := catalog.GetRandomQuote(ctx, database)
	if err != nil {
		t.Fatalf("random: %v", err)
	}
	if got.Quote != "Stay hungry." && got.Quote != "Keep going." {
		t.Fatalf("unexpected random quote %q", got.Quote)
	}
	byID, err := catalog.GetQuoteByID(ctx, database, 1)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if byID.Quote != "Stay hungry." || byID.Author != "A1" {
		t.Fatalf("by id mismatch: %+v", byID)
	}
}

func TestEmptyQuoteTableHandling(t *testing.T) {
	database := openQuotesTestDB(t)
	ctx := context.Background()
	if _, err := catalog.GetRandomQuote(ctx, database); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty random must be ErrNoRows, got %v", err)
	}
	if _, err := catalog.GetQuoteByID(ctx, database, 1); err == nil {
		t.Fatal("missing id must error")
	}
	// Empty table must not break migration/idempotency.
	if err := database.Migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

func TestQuoteAuthorUnknownNormalization(t *testing.T) {
	database := openQuotesTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "No author yet.", Author: ""}); err != nil {
		t.Fatalf("insert empty author: %v", err)
	}
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 2, Quote: "Spaces.", Author: "   "}); err != nil {
		t.Fatalf("insert blank author: %v", err)
	}
	for _, id := range []int{1, 2} {
		got, err := catalog.GetQuoteByID(ctx, database, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Author != "Unknown" {
			t.Fatalf("id %d author=%q want %q", id, got.Author, "Unknown")
		}
	}
	if got := catalog.NormalizeQuoteAuthor(""); got != "Unknown" {
		t.Fatalf("normalize empty=%q", got)
	}
}
