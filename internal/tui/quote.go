package tui

import (
	"context"
	"strings"
	"time"

	"forge/internal/db/catalog"
	"forge/internal/engine"
)

// Fallback header quote used while the quotes table is empty.
// It is never inserted into the database; once real quote data is loaded
// the database quote is used instead.
const (
	fallbackQuoteText = "“Everyone sees what you appear to be, few experience what you really are.”"
	fallbackQuoteAttr = "— Niccolò Machiavelli, The Prince"
)

// currentLocalDate returns today's Forge local date (Asia/Kolkata,
// midnight boundary) for the app clock.
func (a *App) currentLocalDate() string {
	cal := engine.DefaultCalendar()
	n := a.now
	if n.IsZero() {
		n = time.Now()
	}
	return cal.LocalDate(n)
}

// quoteDisplay returns the header quote content from application state.
// When no quote has been selected yet (empty DB), it returns the temporary
// hardcoded fallback. Author formatting stays "— <author>".
func (a *App) quoteDisplay() (text, attr string) {
	if strings.TrimSpace(a.quoteText) == "" {
		return fallbackQuoteText, fallbackQuoteAttr
	}
	author := strings.TrimSpace(a.quoteAuthor)
	if author == "" {
		author = "Unknown"
	}
	return a.quoteText, "— " + author
}

// loadDailyQuote loads the current quote on application startup.
// It reuses the persisted state when it is for today, otherwise selects a
// new quote from the non-repeat cycle. Empty tables and DB failures fall
// back silently.
func (a *App) loadDailyQuote() {
	today := a.currentLocalDate()
	if a.DB == nil {
		a.quoteDate = today
		a.quoteText = ""
		a.quoteAuthor = ""
		a.quoteID = nil
		return
	}
	ctx := context.Background()
	if st, err := catalog.GetDailyQuoteState(ctx, a.DB); err == nil {
		if st.LocalDate == today && strings.TrimSpace(st.QuoteText) != "" {
			a.quoteDate = st.LocalDate
			a.quoteText = st.QuoteText
			a.quoteAuthor = st.Author
			if st.QuoteID != nil {
				v := *st.QuoteID
				a.quoteID = &v
			} else {
				a.quoteID = nil
			}
			return
		}
	}
	a.refreshDailyQuoteForDate(today)
}

// refreshDailyQuoteForDate selects a new quote for the given local date
// using the non-repeat cycle (never repeats until every quote has been
// used once) and persists it atomically with the daily state. When the
// table is empty (or the DB fails), it keeps a fallback in memory without
// inserting anything.
func (a *App) refreshDailyQuoteForDate(today string) {
	if a.DB == nil {
		a.quoteDate = today
		a.quoteText = ""
		a.quoteAuthor = ""
		a.quoteID = nil
		return
	}
	ctx := context.Background()
	quote, err := catalog.SelectDailyQuoteForDate(ctx, a.DB, today)
	if err != nil {
		a.quoteDate = today
		a.quoteText = ""
		a.quoteAuthor = ""
		a.quoteID = nil
		return
	}
	a.quoteDate = today
	a.quoteText = quote.Quote
	a.quoteAuthor = quote.Author
	v := quote.ID
	a.quoteID = &v
}

// ensureDailyQuote keeps one quote for one day: same-day calls are a no-op
// (no DB query), a new local day selects the next quote from the cycle.
func (a *App) ensureDailyQuote() {
	today := a.currentLocalDate()
	if a.quoteDate == today {
		return
	}
	a.refreshDailyQuoteForDate(today)
}
