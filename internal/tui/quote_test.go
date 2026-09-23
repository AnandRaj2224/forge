package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

// openQuoteTestDB opens a migrated temp DB for quote tests.
func openQuoteTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "quote.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	return database
}

func quoteCount(t *testing.T, database *db.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM quotes`).Scan(&n); err != nil {
		t.Fatalf("count quotes: %v", err)
	}
	return n
}

// Fixed IST-friendly times: 10:00 UTC = 15:30 IST, safely inside the day.
func quoteDay1() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) }
func quoteDay2() time.Time { return time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC) }

func TestCurrentQuoteLoadsIntoState(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Test quote one.", Author: "Tester"}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	app.now = quoteDay1()
	app.loadDailyQuote()
	if app.quoteText != "Test quote one." {
		t.Fatalf("quoteText=%q want DB quote", app.quoteText)
	}
	if app.quoteAuthor != "Tester" {
		t.Fatalf("quoteAuthor=%q", app.quoteAuthor)
	}
	if app.quoteDate != app.currentLocalDate() {
		t.Fatalf("quoteDate=%q want %q", app.quoteDate, app.currentLocalDate())
	}
}

func TestEmptyDBUsesFallbackWithoutInsert(t *testing.T) {
	database := openQuoteTestDB(t)
	app := NewApp(database)
	app.now = quoteDay1()
	app.loadDailyQuote()
	if app.quoteText != "" {
		t.Fatalf("empty DB must keep empty state for fallback, got %q", app.quoteText)
	}
	text, attr := app.quoteDisplay()
	if !strings.Contains(text, "Everyone sees") {
		t.Fatalf("fallback text broken: %q", text)
	}
	if !strings.Contains(attr, "Machiavelli") {
		t.Fatalf("fallback attr broken: %q", attr)
	}
	if n := quoteCount(t, database); n != 0 {
		t.Fatalf("fallback must not be inserted, count=%d", n)
	}
}

func TestSameDayDoesNotSelectAnotherQuote(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Same day quote.", Author: "Same"}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	app.now = quoteDay1()
	app.loadDailyQuote()
	before := app.quoteText
	if before == "" {
		t.Fatal("setup: quote not loaded")
	}
	// Empty the table: a same-day refresh would fall back, proving a query.
	if _, err := database.ExecContext(ctx, `DELETE FROM quotes`); err != nil {
		t.Fatal(err)
	}
	app.ensureDailyQuote()
	if app.quoteText != before {
		t.Fatalf("same day must keep quote %q, got %q", before, app.quoteText)
	}
	// Fallback case: empty DB stays fallback even after a quote arrives same day.
	database2 := openQuoteTestDB(t)
	app2 := NewApp(database2)
	app2.now = quoteDay1()
	app2.loadDailyQuote()
	if app2.quoteText != "" {
		t.Fatalf("setup: expected fallback, got %q", app2.quoteText)
	}
	if err := catalog.InsertQuote(context.Background(), database2, db.Quote{ID: 9, Quote: "Arrived late.", Author: "Late"}); err != nil {
		t.Fatal(err)
	}
	app2.ensureDailyQuote()
	if app2.quoteText != "" {
		t.Fatalf("same day must not pick newly arrived quote, got %q", app2.quoteText)
	}
}

func TestDayChangeSelectsNewQuote(t *testing.T) {
	database := openQuoteTestDB(t)
	app := NewApp(database)
	app.now = quoteDay1()
	app.loadDailyQuote()
	if app.quoteText != "" {
		t.Fatalf("setup: expected fallback on empty DB, got %q", app.quoteText)
	}
	if err := catalog.InsertQuote(context.Background(), database, db.Quote{ID: 1, Quote: "New day quote.", Author: "Newbie"}); err != nil {
		t.Fatal(err)
	}
	app.now = quoteDay2()
	app.ensureDailyQuote()
	if app.quoteText != "New day quote." {
		t.Fatalf("day change must select DB quote, got %q", app.quoteText)
	}
	if app.quoteDate != app.currentLocalDate() {
		t.Fatalf("quoteDate=%q want %q", app.quoteDate, app.currentLocalDate())
	}
}

func TestQuotePersistsAcrossRestartSameDay(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Persisted quote.", Author: "Keeper"}); err != nil {
		t.Fatal(err)
	}
	app1 := NewApp(database)
	app1.now = quoteDay1()
	app1.loadDailyQuote()
	if app1.quoteText != "Persisted quote." {
		t.Fatalf("app1 quote=%q", app1.quoteText)
	}
	// Simulate restart with the quotes table emptied: persisted state must win.
	if _, err := database.ExecContext(ctx, `DELETE FROM quotes`); err != nil {
		t.Fatal(err)
	}
	app2 := NewApp(database)
	app2.now = quoteDay1()
	app2.loadDailyQuote()
	if app2.quoteText != "Persisted quote." {
		t.Fatalf("restart same day must reuse persisted quote, got %q", app2.quoteText)
	}
}

func TestHeaderRendersCurrentQuote(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Header quote here.", Author: "Header Author"}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	app.now = quoteDay1()
	app.width, app.height = 150, 40
	app.loadDailyQuote()
	h := stripANSIForTest(app.headerView(150))
	if !strings.Contains(h, "Header quote here.") {
		t.Fatalf("header missing quote text:\n%s", h)
	}
	if !strings.Contains(h, "— Header Author") {
		t.Fatalf("header missing author:\n%s", h)
	}
	// Unknown author formatting.
	if _, err := database.ExecContext(ctx, `DELETE FROM quotes`); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 2, Quote: "Mystery quote.", Author: ""}); err != nil {
		t.Fatal(err)
	}
	app.now = quoteDay2()
	app.ensureDailyQuote()
	h2 := stripANSIForTest(app.headerView(150))
	if !strings.Contains(h2, "Mystery quote.") {
		t.Fatalf("header missing mystery quote:\n%s", h2)
	}
	if !strings.Contains(h2, "— Unknown") {
		t.Fatalf("unknown author must render as — Unknown:\n%s", h2)
	}
}

func TestHeaderFallbackWhenEmpty(t *testing.T) {
	database := openQuoteTestDB(t)
	app := NewApp(database)
	app.now = quoteDay1()
	app.width, app.height = 150, 40
	app.loadDailyQuote()
	h := stripANSIForTest(app.headerView(150))
	if !strings.Contains(h, "Everyone sees") || !strings.Contains(h, "Machiavelli") {
		t.Fatalf("empty DB header must use fallback:\n%s", h)
	}
}

func TestViewDoesNotQueryDBForQuote(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Stable quote.", Author: "Stable"}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	app.now = quoteDay1()
	app.width, app.height = 150, 40
	app.loadDailyQuote()
	before := app.View().Content
	beforeHeader := app.headerView(150)
	_ = database.Close()
	// View and header must render purely from state with the DB closed.
	after := app.View().Content
	afterHeader := app.headerView(150)
	if after != before {
		t.Fatal("View changed after DB close (must not query DB for quote)")
	}
	if afterHeader != beforeHeader {
		t.Fatal("header changed after DB close (must not query DB for quote)")
	}
	if !strings.Contains(stripANSIForTest(after), "Stable quote.") {
		t.Fatal("View lost the current quote after DB close")
	}
}

func TestTickSameDayKeepsQuoteWithoutDB(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Tick quote.", Author: "Ticker"}); err != nil {
		t.Fatal(err)
	}
	app := NewApp(database)
	app.now = quoteDay1()
	app.width, app.height = 120, 36
	app.loadDailyQuote()
	before := app.quoteText
	_ = database.Close()
	// Same-day tick must not need the DB.
	_, _ = app.Update(TickMsg(quoteDay1().Add(time.Minute)))
	if app.quoteText != before {
		t.Fatalf("same-day tick must keep quote %q, got %q", before, app.quoteText)
	}
}
