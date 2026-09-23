package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	db "forge/internal/db"
)

// NormalizeQuoteAuthor maps empty/missing upstream authors to exactly
// "Unknown". Non-empty authors are trimmed and returned as-is.
func NormalizeQuoteAuthor(author string) string {
	if strings.TrimSpace(author) == "" {
		return "Unknown"
	}
	return strings.TrimSpace(author)
}

// InsertQuote inserts one quote row. Empty authors are stored as "Unknown"
// so the author column never holds an empty value. New rows always start
// the cycle as unused (used = 0 via the column default).
func InsertQuote(ctx context.Context, q db.DBTX, quote db.Quote) error {
	author := NormalizeQuoteAuthor(quote.Author)
	if _, err := q.ExecContext(ctx, `INSERT INTO quotes (id, quote, author) VALUES (?, ?, ?)`,
		quote.ID, quote.Quote, author); err != nil {
		return fmt.Errorf("catalog: insert quote: %w", err)
	}
	return nil
}

// GetRandomQuote returns one random quote using SQLite's simple
// ORDER BY RANDOM() LIMIT 1. It returns sql.ErrNoRows (wrapped) when the
// quotes table is empty; callers must fall back without surfacing an error.
func GetRandomQuote(ctx context.Context, q db.DBTX) (db.Quote, error) {
	var quote db.Quote
	err := q.QueryRowContext(ctx, `SELECT id, quote, author, used FROM quotes ORDER BY RANDOM() LIMIT 1`).Scan(
		&quote.ID, &quote.Quote, &quote.Author, &quote.Used,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return db.Quote{}, fmt.Errorf("catalog: random quote: %w", err)
		}
		return db.Quote{}, fmt.Errorf("catalog: random quote: %w", err)
	}
	return quote, nil
}

// GetQuoteByID returns one quote by ID for application-state use.
// It returns sql.ErrNoRows (wrapped) when the ID does not exist.
func GetQuoteByID(ctx context.Context, q db.DBTX, id int) (db.Quote, error) {
	var quote db.Quote
	err := q.QueryRowContext(ctx, `SELECT id, quote, author, used FROM quotes WHERE id = ?`, id).Scan(
		&quote.ID, &quote.Quote, &quote.Author, &quote.Used,
	)
	if err != nil {
		return db.Quote{}, fmt.Errorf("catalog: get quote: %w", err)
	}
	return quote, nil
}

// GetDailyQuoteState returns the singleton daily-quote state row (id = 1).
// It returns sql.ErrNoRows (wrapped) when no state has been persisted yet.
func GetDailyQuoteState(ctx context.Context, q db.DBTX) (db.DailyQuoteState, error) {
	var st db.DailyQuoteState
	var quoteID sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT id, local_date, quote_id, quote_text, author FROM daily_quote_state WHERE id = 1`).Scan(
		&st.ID, &st.LocalDate, &quoteID, &st.QuoteText, &st.Author,
	)
	if err != nil {
		return db.DailyQuoteState{}, fmt.Errorf("catalog: get daily quote state: %w", err)
	}
	if quoteID.Valid {
		v := int(quoteID.Int64)
		st.QuoteID = &v
	}
	return st, nil
}

// UpsertDailyQuoteState persists the singleton daily-quote state row.
func UpsertDailyQuoteState(ctx context.Context, q db.DBTX, st db.DailyQuoteState) error {
	if st.ID == 0 {
		st.ID = 1
	}
	var quoteID any
	if st.QuoteID != nil {
		quoteID = *st.QuoteID
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO daily_quote_state (id, local_date, quote_id, quote_text, author)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET local_date = excluded.local_date, quote_id = excluded.quote_id, quote_text = excluded.quote_text, author = excluded.author`,
		st.ID, st.LocalDate, quoteID, st.QuoteText, st.Author,
	); err != nil {
		return fmt.Errorf("catalog: upsert daily quote state: %w", err)
	}
	return nil
}

// SelectDailyQuoteForDate selects the quote for a new local day with a
// non-repeating cycle: a quote is never selected again until every quote
// has been used once.
//
//  1. Select one random row with used = 0, mark it used = 1, persist it as
//     today's daily_quote_state.
//  2. If no unused row exists and the table is non-empty, reset the cycle
//     (UPDATE quotes SET used = 0), then select one random row, mark it
//     used = 1, and persist it.
//  3. If the table is empty, return sql.ErrNoRows (wrapped); callers must
//     fall back without inserting anything.
//
// Selection + marking used + updating daily_quote_state happen atomically
// in a single transaction via the existing DB transaction infrastructure.
func SelectDailyQuoteForDate(ctx context.Context, database *db.DB, localDate string) (db.Quote, error) {
	var selected db.Quote
	err := database.WithTx(ctx, func(q db.DBTX) error {
		var cand db.Quote
		err := q.QueryRowContext(ctx, `SELECT id, quote, author FROM quotes WHERE used = 0 ORDER BY RANDOM() LIMIT 1`).Scan(
			&cand.ID, &cand.Quote, &cand.Author,
		)
		if err == nil {
			if _, err := q.ExecContext(ctx, `UPDATE quotes SET used = 1 WHERE id = ?`, cand.ID); err != nil {
				return fmt.Errorf("catalog: mark quote used: %w", err)
			}
			id := cand.ID
			if err := UpsertDailyQuoteState(ctx, q, db.DailyQuoteState{
				ID: 1, LocalDate: localDate, QuoteID: &id, QuoteText: cand.Quote, Author: cand.Author,
			}); err != nil {
				return err
			}
			cand.Used = 1
			selected = cand
			return nil
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("catalog: select unused quote: %w", err)
		}
		var total int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM quotes`).Scan(&total); err != nil {
			return fmt.Errorf("catalog: count quotes: %w", err)
		}
		if total == 0 {
			return fmt.Errorf("catalog: random quote: %w", sql.ErrNoRows)
		}
		if _, err := q.ExecContext(ctx, `UPDATE quotes SET used = 0`); err != nil {
			return fmt.Errorf("catalog: reset quote cycle: %w", err)
		}
		var next db.Quote
		if err := q.QueryRowContext(ctx, `SELECT id, quote, author FROM quotes ORDER BY RANDOM() LIMIT 1`).Scan(
			&next.ID, &next.Quote, &next.Author,
		); err != nil {
			return fmt.Errorf("catalog: select reset quote: %w", err)
		}
		if _, err := q.ExecContext(ctx, `UPDATE quotes SET used = 1 WHERE id = ?`, next.ID); err != nil {
			return fmt.Errorf("catalog: mark quote used: %w", err)
		}
		id := next.ID
		if err := UpsertDailyQuoteState(ctx, q, db.DailyQuoteState{
			ID: 1, LocalDate: localDate, QuoteID: &id, QuoteText: next.Quote, Author: next.Author,
		}); err != nil {
			return err
		}
		next.Used = 1
		selected = next
		return nil
	})
	if err != nil {
		return db.Quote{}, err
	}
	return selected, nil
}
