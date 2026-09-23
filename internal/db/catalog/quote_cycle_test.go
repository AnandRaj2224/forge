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

func openCycleTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "cycle.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func cycleUsed(t *testing.T, database *db.DB, id int) int {
	t.Helper()
	var used int
	if err := database.QueryRowContext(context.Background(), `SELECT used FROM quotes WHERE id = ?`, id).Scan(&used); err != nil {
		t.Fatalf("select used id=%d: %v", id, err)
	}
	return used
}

func cycleUsedCount(t *testing.T, database *db.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM quotes WHERE used = 1`).Scan(&n); err != nil {
		t.Fatalf("count used: %v", err)
	}
	return n
}

func cycleUnusedCount(t *testing.T, database *db.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM quotes WHERE used = 0`).Scan(&n); err != nil {
		t.Fatalf("count unused: %v", err)
	}
	return n
}

func mustInsertCycleQuotes(t *testing.T, database *db.DB, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 1; i <= n; i++ {
		q := db.Quote{ID: i, Quote: "Quote number " + itoaCycle(i) + ".", Author: "Author " + itoaCycle(i)}
		if err := catalog.InsertQuote(ctx, database, q); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
}

func itoaCycle(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(b[pos:])
}

func TestCycleSchemaHasUsedColumn(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
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
	if !strings.Contains(upper, "USED INTEGER NOT NULL DEFAULT 0") {
		t.Fatalf("quotes.used must be INTEGER NOT NULL DEFAULT 0, ddl=%q", ddl)
	}
	if !strings.Contains(upper, "CHECK") || !strings.Contains(upper, "USED IN (0,1)") && !strings.Contains(upper, "USED IN (0, 1)") {
		t.Fatalf("quotes.used must have CHECK (used IN (0,1)), ddl=%q", ddl)
	}
	// New rows default to unused.
	if err := catalog.InsertQuote(ctx, database, db.Quote{ID: 1, Quote: "Default used.", Author: "A"}); err != nil {
		t.Fatal(err)
	}
	if got := cycleUsed(t, database, 1); got != 0 {
		t.Fatalf("default used=%d want 0", got)
	}
	// Invalid values must be rejected.
	if _, err := database.ExecContext(ctx, `INSERT INTO quotes (id, quote, author, used) VALUES (99, 'x', 'y', 2)`); err == nil {
		t.Fatal("used=2 must be rejected")
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO quotes (id, quote, author, used) VALUES (100, 'x', 'y', NULL)`); err == nil {
		t.Fatal("used NULL must be rejected")
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO quotes (id, quote, author, used) VALUES (101, 'x', 'y', 1)`); err != nil {
		t.Fatalf("used=1 must be accepted: %v", err)
	}
}

func TestCycleFirstSelectionMarksUsed(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
	mustInsertCycleQuotes(t, database, 3)
	got, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-20")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	// Membership, not a particular random quote.
	if got.ID < 1 || got.ID > 3 {
		t.Fatalf("selected id=%d not a member of {1,2,3}", got.ID)
	}
	if got.Used != 1 {
		t.Fatalf("returned Used=%d want 1", got.Used)
	}
	if u := cycleUsed(t, database, got.ID); u != 1 {
		t.Fatalf("db used for %d = %d want 1", got.ID, u)
	}
	if n := cycleUsedCount(t, database); n != 1 {
		t.Fatalf("used count=%d want 1", n)
	}
	// Daily state persisted atomically with the selection.
	st, err := catalog.GetDailyQuoteState(ctx, database)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if st.LocalDate != "2026-09-20" {
		t.Fatalf("state date=%q", st.LocalDate)
	}
	if st.QuoteID == nil || *st.QuoteID != got.ID {
		t.Fatalf("state quote_id=%v want %d", st.QuoteID, got.ID)
	}
	if st.QuoteText != got.Quote || st.Author != got.Author {
		t.Fatalf("state text/author mismatch: %+v vs %+v", st, got)
	}
}

func TestCycleNewDaySelectsOnlyUnused(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
	mustInsertCycleQuotes(t, database, 3)
	// Exhaust all but one deterministically: mark 1 and 2 used.
	if _, err := database.ExecContext(ctx, `UPDATE quotes SET used = 1 WHERE id IN (1, 2)`); err != nil {
		t.Fatal(err)
	}
	got, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-21")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if got.ID != 3 {
		t.Fatalf("must select the only unused quote 3, got %d", got.ID)
	}
	if u := cycleUsed(t, database, 3); u != 1 {
		t.Fatalf("selected must be marked used, got %d", u)
	}
}

func TestCycleNoRepeatWhileUnusedRemain(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
	mustInsertCycleQuotes(t, database, 3)
	seen := map[int]bool{}
	for i, date := range []string{"2026-09-20", "2026-09-21", "2026-09-22"} {
		got, err := catalog.SelectDailyQuoteForDate(ctx, database, date)
		if err != nil {
			t.Fatalf("day %d select: %v", i, err)
		}
		if seen[got.ID] {
			t.Fatalf("repeat id=%d on day %d while unused remain", got.ID, i)
		}
		seen[got.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 unique selections, got %v", seen)
	}
	if n := cycleUsedCount(t, database); n != 3 {
		t.Fatalf("after exhausting 3, used=%d want 3", n)
	}
}

func TestCycleResetSelectsOneAndMarksUsed(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
	mustInsertCycleQuotes(t, database, 2)
	if _, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-20"); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-21"); err != nil {
		t.Fatal(err)
	}
	if n := cycleUsedCount(t, database); n != 2 {
		t.Fatalf("setup: used=%d want 2 (exhausted)", n)
	}
	got, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-22")
	if err != nil {
		t.Fatalf("reset select: %v", err)
	}
	if got.ID != 1 && got.ID != 2 {
		t.Fatalf("reset selected id=%d not a member", got.ID)
	}
	if u := cycleUsed(t, database, got.ID); u != 1 {
		t.Fatalf("reset selected must be marked used")
	}
	if n := cycleUsedCount(t, database); n != 1 {
		t.Fatalf("after reset exactly one used=1, got %d", n)
	}
	if n := cycleUnusedCount(t, database); n != 1 {
		t.Fatalf("after reset exactly one unused, got %d", n)
	}
	st, err := catalog.GetDailyQuoteState(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if st.LocalDate != "2026-09-22" || st.QuoteID == nil || *st.QuoteID != got.ID {
		t.Fatalf("state after reset mismatch: %+v vs %d", st, got.ID)
	}
}

func TestCycleAfterResetOthersRemainAvailable(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
	mustInsertCycleQuotes(t, database, 3)
	// Exhaust first cycle.
	for _, d := range []string{"2026-09-20", "2026-09-21", "2026-09-22"} {
		if _, err := catalog.SelectDailyQuoteForDate(ctx, database, d); err != nil {
			t.Fatal(err)
		}
	}
	// Second cycle: 3 selections must again cover all 3 without repeat.
	seen := map[int]bool{}
	for _, d := range []string{"2026-09-23", "2026-09-24", "2026-09-25"} {
		got, err := catalog.SelectDailyQuoteForDate(ctx, database, d)
		if err != nil {
			t.Fatalf("second cycle %s: %v", d, err)
		}
		if seen[got.ID] {
			t.Fatalf("second cycle repeat id=%d", got.ID)
		}
		seen[got.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("second cycle must cover all 3, got %v", seen)
	}
}

func TestCycleEmptyTableUsesFallback(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
	if _, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-20"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty select must be ErrNoRows, got %v", err)
	}
	// No state row must be created for the empty case.
	if _, err := catalog.GetDailyQuoteState(ctx, database); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty select must not persist state, get state err=%v", err)
	}
	var n int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM quotes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty select must not insert fallback, count=%d", n)
	}
}

func TestCycleAtomicOnFailure(t *testing.T) {
	database := openCycleTestDB(t)
	ctx := context.Background()
	mustInsertCycleQuotes(t, database, 2)
	first, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-20")
	if err != nil {
		t.Fatal(err)
	}
	if n := cycleUsedCount(t, database); n != 1 {
		t.Fatalf("setup used=%d want 1", n)
	}
	// Break the state table so the next selection's upsert fails after
	// the used-marking step. A correct transaction must roll back the mark.
	if _, err := database.ExecContext(ctx, `DROP TABLE daily_quote_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.SelectDailyQuoteForDate(ctx, database, "2026-09-21"); err == nil {
		t.Fatal("expected failure when state table is missing")
	}
	// The failed selection must not have marked another quote used.
	if n := cycleUsedCount(t, database); n != 1 {
		t.Fatalf("atomicity violated: used=%d want 1 after failed select", n)
	}
	if u := cycleUsed(t, database, first.ID); u != 1 {
		t.Fatalf("original used row lost: id=%d used=%d", first.ID, u)
	}
	other := 1
	if first.ID == 1 {
		other = 2
	}
	if u := cycleUsed(t, database, other); u != 0 {
		t.Fatalf("failed select partially marked id=%d used=%d want 0", other, u)
	}
}
