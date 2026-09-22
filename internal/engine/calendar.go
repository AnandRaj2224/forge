package engine

import (
	"context"
	"fmt"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

// ForgeCalendar is the single authoritative calendar abstraction.
//
//	TIMEZONE = Asia/Kolkata
//	DayStart = midnight (00:00 local)
//	2026-09-21 23:59 IST = Sep 21
//	2026-09-22 00:00 IST = Sep 22
//
// Stored timestamps remain Unix seconds (UTC). All date/week calculations
// convert through this calendar. Never use UTC calendar dates directly for
// Today/History/Reviews.
type ForgeCalendar struct {
	loc *time.Location
}

// DefaultCalendar returns the Forge calendar (Asia/Kolkata, midnight start).
// Falls back to a fixed +05:30 zone when the tz database is unavailable.
func DefaultCalendar() ForgeCalendar {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("Asia/Kolkata", 5*60*60+30*60)
	}
	return ForgeCalendar{loc: loc}
}

// VerifyCalendarSettings is the single reconciliation point between the
// stored forge_settings row and the fixed Forge calendar. Forge is
// intentionally fixed to Asia/Kolkata + midnight; the settings row is NOT
// an independently configurable timezone. This check makes that explicit:
// it loads the row (DB failures propagate) and rejects a silently
// diverging configuration instead of maintaining two sources of truth.
// Callers that need calendar assurance (e.g. Today generation entry
// points) use the returned calendar; a mismatch is a hard error.
func VerifyCalendarSettings(ctx context.Context, q db.DBTX) (ForgeCalendar, error) {
	cal := DefaultCalendar()
	s, err := catalog.GetSettings(ctx, q)
	if err != nil {
		return ForgeCalendar{}, fmt.Errorf("engine: load calendar settings: %w", err)
	}
	if s.Timezone != "Asia/Kolkata" {
		return ForgeCalendar{}, wrapErr("calendar", ErrValidation,
			fmt.Sprintf("unsupported timezone %q: Forge is fixed to Asia/Kolkata", s.Timezone))
	}
	if s.DayStartMinute != 0 {
		return ForgeCalendar{}, wrapErr("calendar", ErrValidation,
			fmt.Sprintf("unsupported day_start_minute %d: Forge day boundary is fixed to midnight", s.DayStartMinute))
	}
	return cal, nil
}

// Location exposes the underlying location (for tests/formatting).
func (c ForgeCalendar) Location() *time.Location { return c.loc }

// LocalDate converts an instant to its Forge local date (YYYY-MM-DD).
func (c ForgeCalendar) LocalDate(t time.Time) string {
	return t.In(c.loc).Format("2006-01-02")
}

// StartOfDay returns midnight IST starting the given local date.
func (c ForgeCalendar) StartOfDay(localDate string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", localDate, c.loc)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// StartOfNextDay returns midnight IST starting the day after localDate.
func (c ForgeCalendar) StartOfNextDay(localDate string) (time.Time, error) {
	s, err := c.StartOfDay(localDate)
	if err != nil {
		return time.Time{}, err
	}
	return s.AddDate(0, 0, 1), nil
}

// DayBounds returns [start, end) Unix seconds for a local date.
func (c ForgeCalendar) DayBounds(localDate string) (int64, int64, error) {
	s, err := c.StartOfDay(localDate)
	if err != nil {
		return 0, 0, err
	}
	return s.Unix(), s.AddDate(0, 0, 1).Unix(), nil
}

// LocalDateOfUnix converts Unix seconds to a Forge local date.
func (c ForgeCalendar) LocalDateOfUnix(sec int64) string {
	return time.Unix(sec, 0).In(c.loc).Format("2006-01-02")
}

// CurrentWeek returns Mon..Sun local dates containing now.
func (c ForgeCalendar) CurrentWeek(now time.Time) []string {
	local := now.In(c.loc)
	wd := int(local.Weekday()) // Sunday=0
	offset := (wd + 6) % 7     // Mon=0..Sun=6
	monday := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, c.loc).AddDate(0, 0, -offset)
	out := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		out = append(out, monday.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return out
}

// WeekStartEnd returns Monday and Sunday local dates for the week containing now.
func (c ForgeCalendar) WeekStartEnd(now time.Time) (string, string) {
	w := c.CurrentWeek(now)
	return w[0], w[6]
}

// ISOWeekID returns YYYY-Www for a local date's Monday (matches existing weekly_reviews PK).
func (c ForgeCalendar) ISOWeekID(localDate string) string {
	t, err := time.ParseInLocation("2006-01-02", localDate, c.loc)
	if err != nil {
		return ""
	}
	y, w := t.ISOWeek()
	return formatWeekID(y, w)
}

func formatWeekID(year, week int) string {
	// YYYY-Www with zero-padded week.
	ws := itoa2(week)
	ys := itoa(year)
	return ys + "-W" + ws
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	pos := len(b)
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func itoa2(n int) string {
	s := itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// SplitIntervalAtDayBoundaries splits [startUnix, endUnix) into per-local-date
// second counts, attributing elapsed time to the actual interval. Used for
// cross-midnight session accounting: Sep 21 23:50 -> Sep 22 00:20 counts
// 10m on Sep 21 and 20m on Sep 22.
func (c ForgeCalendar) SplitIntervalAtDayBoundaries(startUnix, endUnix int64) map[string]int64 {
	out := map[string]int64{}
	if endUnix <= startUnix {
		return out
	}
	cur := startUnix
	for cur < endUnix {
		local := time.Unix(cur, 0).In(c.loc)
		dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, c.loc).Unix()
		dayEnd := dayStart + 86400
		segEnd := endUnix
		if dayEnd < segEnd {
			segEnd = dayEnd
		}
		date := time.Unix(cur, 0).In(c.loc).Format("2006-01-02")
		out[date] += segEnd - cur
		cur = segEnd
	}
	return out
}
