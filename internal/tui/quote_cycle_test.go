package tui

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

func cycleDay(offset int) time.Time {
	return time.Date(2026, 9, 20+offset, 10, 0, 0, 0, time.UTC)
}

func cycleUsedFlag(t *testing.T, database *db.DB, id int) int {
	t.Helper()
	var used int
	if err := database.QueryRowContext(context.Background(), `SELECT used FROM quotes WHERE id = ?`, id).Scan(&used); err != nil {
		t.Fatalf("select used id=%d: %v", id, err)
	}
	return used
}

func cycleUsedTotal(t *testing.T, database *db.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM quotes WHERE used = 1`).Scan(&n); err != nil {
		t.Fatalf("count used: %v", err)
	}
	return n
}

func insertCycleQuotes(t *testing.T, database *db.DB, ids ...int) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		if err := catalog.InsertQuote(ctx, database, db.Quote{ID: id, Quote: "Cycle quote " + string(rune('A'+id)) + ".", Author: "Author " + string(rune('A'+id))}); err != nil {
			t.Fatalf("insert %d: %v", id, err)
		}
	}
}

func resetCycleState(t *testing.T, database *db.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `UPDATE quotes SET used = 0`); err != nil {
		t.Fatalf("reset used: %v", err)
	}
	// NewApp already performed a selection for the real wall-clock day.
	// Clear it so the fixed cycleDay selections below start from a clean
	// slate deterministically.
	if _, err := database.ExecContext(ctx, `DELETE FROM daily_quote_state`); err != nil {
		t.Fatalf("clear state: %v", err)
	}
}

func TestCycleFirstSelectionMarksUsed(t *testing.T) {
	database := openQuoteTestDB(t)
	insertCycleQuotes(t, database, 1, 2, 3)
	app := NewApp(database)
	resetCycleState(t, database)
	app.now = cycleDay(0)
	app.loadDailyQuote()
	if app.quoteID == nil {
		t.Fatal("expected a selected quote")
	}
	id := *app.quoteID
	if id != 1 && id != 2 && id != 3 {
		t.Fatalf("selected id=%d not a member", id)
	}
	if got := cycleUsedFlag(t, database, id); got != 1 {
		t.Fatalf("selected id=%d used=%d want 1", id, got)
	}
	if n := cycleUsedTotal(t, database); n != 1 {
		t.Fatalf("used total=%d want 1", n)
	}
}

func TestCycleSameDayReturnsSameQuote(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	insertCycleQuotes(t, database, 1, 2)
	app := NewApp(database)
	resetCycleState(t, database)
	app.now = cycleDay(0)
	app.loadDailyQuote()
	if app.quoteID == nil {
		t.Fatal("setup: no quote")
	}
	firstID := *app.quoteID
	firstText := app.quoteText
	usedBefore := cycleUsedTotal(t, database)
	app.ensureDailyQuote()
	if app.quoteID == nil || *app.quoteID != firstID {
		t.Fatalf("same day must keep id %d, got %v", firstID, app.quoteID)
	}
	if app.quoteText != firstText {
		t.Fatalf("same day must keep text %q, got %q", firstText, app.quoteText)
	}
	if n := cycleUsedTotal(t, database); n != usedBefore {
		t.Fatalf("same day must not mark another used: before=%d after=%d", usedBefore, n)
	}
	// Restart on the same day reuses the persisted quote even if the
	// quotes table is emptied (proves reuse, not reselection).
	if _, err := database.ExecContext(ctx, `DELETE FROM quotes`); err != nil {
		t.Fatal(err)
	}
	app2 := NewApp(database)
	app2.now = cycleDay(0)
	app2.loadDailyQuote()
	if app2.quoteText != firstText {
		t.Fatalf("restart same day must reuse %q, got %q", firstText, app2.quoteText)
	}
}

func TestCycleNewDaySelectsOnlyUnused(t *testing.T) {
	database := openQuoteTestDB(t)
	insertCycleQuotes(t, database, 1, 2)
	app := NewApp(database)
	resetCycleState(t, database)
	app.now = cycleDay(0)
	app.loadDailyQuote()
	if app.quoteID == nil {
		t.Fatal("setup: no quote")
	}
	first := *app.quoteID
	app.now = cycleDay(1)
	app.ensureDailyQuote()
	if app.quoteID == nil {
		t.Fatal("new day must select a quote")
	}
	second := *app.quoteID
	if second == first {
		t.Fatalf("new day must select from unused: repeated id=%d", first)
	}
	if second != 1 && second != 2 {
		t.Fatalf("new day id=%d not a member", second)
	}
	if n := cycleUsedTotal(t, database); n != 2 {
		t.Fatalf("both quotes must be used after two days, got %d", n)
	}
}

func TestCycleNoRepeatWhileUnusedRemain(t *testing.T) {
	database := openQuoteTestDB(t)
	insertCycleQuotes(t, database, 1, 2, 3)
	app := NewApp(database)
	resetCycleState(t, database)
	seen := map[int]bool{}
	for i := 0; i < 3; i++ {
		app.now = cycleDay(i)
		if i == 0 {
			app.loadDailyQuote()
		} else {
			app.ensureDailyQuote()
		}
		if app.quoteID == nil {
			t.Fatalf("day %d: no quote", i)
		}
		if seen[*app.quoteID] {
			t.Fatalf("repeat id=%d on day %d while unused remain", *app.quoteID, i)
		}
		seen[*app.quoteID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 unique, got %v", seen)
	}
}

func TestCycleResetSelectsOneAndMarksUsed(t *testing.T) {
	database := openQuoteTestDB(t)
	insertCycleQuotes(t, database, 1, 2)
	app := NewApp(database)
	resetCycleState(t, database)
	app.now = cycleDay(0)
	app.loadDailyQuote()
	app.now = cycleDay(1)
	app.ensureDailyQuote()
	if n := cycleUsedTotal(t, database); n != 2 {
		t.Fatalf("setup: used=%d want 2 exhausted", n)
	}
	app.now = cycleDay(2)
	app.ensureDailyQuote()
	if app.quoteID == nil {
		t.Fatal("reset day must select a quote")
	}
	if *app.quoteID != 1 && *app.quoteID != 2 {
		t.Fatalf("reset id=%d not a member", *app.quoteID)
	}
	if n := cycleUsedTotal(t, database); n != 1 {
		t.Fatalf("after reset exactly one used=1, got %d", n)
	}
}

func TestCycleAfterResetOthersRemainAvailable(t *testing.T) {
	database := openQuoteTestDB(t)
	insertCycleQuotes(t, database, 1, 2)
	app := NewApp(database)
	resetCycleState(t, database)
	app.now = cycleDay(0)
	app.loadDailyQuote()
	app.now = cycleDay(1)
	app.ensureDailyQuote()
	app.now = cycleDay(2)
	app.ensureDailyQuote()
	if app.quoteID == nil {
		t.Fatal("setup: reset day no quote")
	}
	resetID := *app.quoteID
	app.now = cycleDay(3)
	app.ensureDailyQuote()
	if app.quoteID == nil {
		t.Fatal("day after reset must select a quote")
	}
	nextID := *app.quoteID
	if nextID == resetID {
		t.Fatalf("day after reset must use the remaining unused quote, repeated %d", resetID)
	}
	if n := cycleUsedTotal(t, database); n != 2 {
		t.Fatalf("after completing new cycle used=%d want 2", n)
	}
}

func TestCycleEmptyStillFallback(t *testing.T) {
	database := openQuoteTestDB(t)
	app := NewApp(database)
	app.now = cycleDay(0)
	app.loadDailyQuote()
	if app.quoteText != "" {
		t.Fatalf("empty must keep empty state for fallback, got %q", app.quoteText)
	}
	text, attr := app.quoteDisplay()
	if !strings.Contains(text, "Everyone sees") || !strings.Contains(attr, "Machiavelli") {
		t.Fatalf("fallback broken: %q %q", text, attr)
	}
	if n := quoteCount(t, database); n != 0 {
		t.Fatalf("fallback must not insert, count=%d", n)
	}
	if _, err := catalog.GetDailyQuoteState(context.Background(), database); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty must not persist state, err=%v", err)
	}
}

func TestCycleTickAndViewDoNotSelect(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	insertCycleQuotes(t, database, 1)
	app := NewApp(database)
	resetCycleState(t, database)
	app.now = cycleDay(0)
	app.width, app.height = 120, 36
	app.loadDailyQuote()
	before := app.quoteText
	if before == "" {
		t.Fatal("setup: quote not loaded")
	}
	beforeView := app.View().Content
	beforeHeader := app.headerView(150)
	_ = database.Close()
	// Same-day tick must not need the DB and must keep the quote.
	_, _ = app.Update(TickMsg(cycleDay(0).Add(time.Minute)))
	if app.quoteText != before {
		t.Fatalf("same-day tick must keep %q, got %q", before, app.quoteText)
	}
	// View/header render purely from state with the DB closed.
	afterView := app.View().Content
	afterHeader := app.headerView(150)
	if afterView != beforeView {
		t.Fatal("View changed after DB close (must not query DB)")
	}
	if afterHeader != beforeHeader {
		t.Fatal("header changed after DB close (must not query DB)")
	}
	_ = ctx
}

func TestCycleAtomicOnFailure(t *testing.T) {
	database := openQuoteTestDB(t)
	ctx := context.Background()
	insertCycleQuotes(t, database, 1, 2)
	app := NewApp(database)
	resetCycleState(t, database)
	app.now = cycleDay(0)
	app.loadDailyQuote()
	if app.quoteID == nil {
		t.Fatal("setup: no quote")
	}
	if n := cycleUsedTotal(t, database); n != 1 {
		t.Fatalf("setup used=%d want 1", n)
	}
	// Break persistence so the next day's atomic update fails.
	if _, err := database.ExecContext(ctx, `DROP TABLE daily_quote_state`); err != nil {
		t.Fatal(err)
	}
	app.now = cycleDay(1)
	app.ensureDailyQuote()
	// Failed selection falls back in memory without partial marking.
	if app.quoteText != "" {
		t.Fatalf("failed select must fall back to empty state, got %q", app.quoteText)
	}
	if n := cycleUsedTotal(t, database); n != 1 {
		t.Fatalf("atomicity violated: used=%d want 1 after failure", n)
	}
}
