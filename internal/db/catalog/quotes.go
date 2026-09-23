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
// so the author column never holds an empty value.
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
	err := q.QueryRowContext(ctx, `SELECT id, quote, author FROM quotes ORDER BY RANDOM() LIMIT 1`).Scan(
		&quote.ID, &quote.Quote, &quote.Author,
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
	err := q.QueryRowContext(ctx, `SELECT id, quote, author FROM quotes WHERE id = ?`, id).Scan(
		&quote.ID, &quote.Quote, &quote.Author,
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
