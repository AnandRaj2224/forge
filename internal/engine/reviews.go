package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// Review persistence is Engine-owned: the TUI never writes weekly_reviews
// rows directly. Runtime performs SQL only.
//
// Semantics (preserved):
//
//	live draft    = engine-generated metrics, never stored
//	SAVE          = freeze the metric snapshot (planned/actual/report_json)
//	after save    = metrics never silently recalculate
//	reflections   = the only mutable fields on a stored review
//	refresh       = explicit regeneration of drafts only

// validateWeeklySnapshot checks the metric snapshot structure before any
// write: week identity, date coherence, and that the stored scalars match
// the structured report (no placeholder report_json that recomputes later).
func validateWeeklySnapshot(r db.WeeklyReview) error {
	if strings.TrimSpace(r.ID) == "" {
		return wrapErr("review save", ErrValidation, "week id required")
	}
	cal := DefaultCalendar()
	start, err := cal.StartOfDay(r.WeekStart)
	if err != nil {
		return wrapErr("review save", ErrValidation, "bad week start")
	}
	end, err := cal.StartOfDay(r.WeekEnd)
	if err != nil {
		return wrapErr("review save", ErrValidation, "bad week end")
	}
	if end.Before(start) {
		return wrapErr("review save", ErrValidation, "week end before week start")
	}
	if cal.ISOWeekID(r.WeekStart) != r.ID {
		return wrapErr("review save", ErrValidation, "week id does not match week start")
	}
	if strings.TrimSpace(r.ReportJSON) == "" || strings.TrimSpace(r.ReportJSON) == "{}" {
		return wrapErr("review save", ErrValidation, "structured report snapshot required")
	}
	rep, err := UnmarshalWeeklySnapshot(r.ReportJSON)
	if err != nil {
		return wrapErr("review save", ErrValidation, "report snapshot unparseable: "+err.Error())
	}
	if rep.WeekID != r.ID || rep.WeekStart != r.WeekStart || rep.WeekEnd != r.WeekEnd {
		return wrapErr("review save", ErrValidation, "report snapshot does not match week")
	}
	if rep.Planned != r.PlannedSeconds || rep.Actual != r.ElapsedSeconds {
		return wrapErr("review save", ErrValidation, "stored metrics do not match report snapshot")
	}
	if r.OverrunSeconds != r.ElapsedSeconds-r.PlannedSeconds {
		return wrapErr("review save", ErrValidation, "overrun must equal elapsed - planned")
	}
	return nil
}

// SaveWeeklyReviewSnapshot freezes a live draft into a stored review. The
// structured report_json is stored as-is (never a placeholder); later reads
// render the persisted snapshot.
//
// First-writer-wins: the insert succeeds only when no row exists for the
// week. A competing save that loses the race returns the already-stored
// authoritative snapshot instead of overwriting frozen metrics.
func SaveWeeklyReviewSnapshot(ctx context.Context, database *db.DB, r db.WeeklyReview, now time.Time) (db.WeeklyReview, error) {
	if err := validateWeeklySnapshot(r); err != nil {
		return db.WeeklyReview{}, err
	}
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = now
	}
	r.UpdatedAt = now
	if r.UpdatedAt.Before(r.GeneratedAt) {
		r.UpdatedAt = r.GeneratedAt
	}
	if err := runtime.InsertWeeklyReviewOnce(ctx, database, r); err != nil {
		if !isUniqueConflict(err) {
			return db.WeeklyReview{}, fmt.Errorf("engine: save weekly review: %w", err)
		}
		// Lost the race (or a snapshot already exists): the stored row
		// stays authoritative; return it unchanged.
		stored, rerr := runtime.GetWeeklyReview(ctx, database, r.ID)
		if rerr != nil {
			return db.WeeklyReview{}, fmt.Errorf("engine: load weekly review: %w", rerr)
		}
		return stored, nil
	}
	stored, err := runtime.GetWeeklyReview(ctx, database, r.ID)
	if err != nil {
		return db.WeeklyReview{}, fmt.Errorf("engine: reload weekly review: %w", err)
	}
	return stored, nil
}

// SaveReviewReflections persists only reflection edits. When a stored row
// exists, its metric snapshot (planned/actual/overrun/report_json,
// generated_at, week bounds) is preserved untouched via a reflections-only
// UPDATE; only reflections and updated_at change. When nothing is stored
// yet (draft), the provided snapshot is validated and frozen together with
// the reflections; a lost insert race falls back to a reflections-only
// update on the winner's frozen metrics.
func SaveReviewReflections(ctx context.Context, database *db.DB, r db.WeeklyReview, worked, change string, now time.Time) (db.WeeklyReview, error) {
	if strings.TrimSpace(r.ID) == "" {
		return db.WeeklyReview{}, wrapErr("review save", ErrValidation, "week id required")
	}
	worked = strings.TrimSpace(worked)
	change = strings.TrimSpace(change)
	if _, err := runtime.GetWeeklyReview(ctx, database, r.ID); err == nil {
		if err := runtime.UpdateWeeklyReflections(ctx, database, r.ID, worked, change, now); err != nil {
			return db.WeeklyReview{}, fmt.Errorf("engine: save review reflections: %w", err)
		}
	} else if !isNotFoundErr(err) {
		return db.WeeklyReview{}, fmt.Errorf("engine: load weekly review: %w", err)
	} else {
		// No stored row: the draft snapshot must validate before it can
		// be frozen with reflections.
		if err := validateWeeklySnapshot(r); err != nil {
			return db.WeeklyReview{}, err
		}
		if r.GeneratedAt.IsZero() {
			r.GeneratedAt = now
		}
		r.UpdatedAt = now
		if r.UpdatedAt.Before(r.GeneratedAt) {
			r.UpdatedAt = r.GeneratedAt
		}
		r.ReflectionWorked = worked
		r.ReflectionChange = change
		if err := runtime.InsertWeeklyReviewOnce(ctx, database, r); err != nil {
			if !isUniqueConflict(err) {
				return db.WeeklyReview{}, fmt.Errorf("engine: save weekly review: %w", err)
			}
			// Lost the insert race: frozen metrics belong to the
			// winner; apply only our reflections onto them.
			if err := runtime.UpdateWeeklyReflections(ctx, database, r.ID, worked, change, now); err != nil {
				return db.WeeklyReview{}, fmt.Errorf("engine: save review reflections: %w", err)
			}
		}
		stored, err := runtime.GetWeeklyReview(ctx, database, r.ID)
		if err != nil {
			return db.WeeklyReview{}, fmt.Errorf("engine: reload weekly review: %w", err)
		}
		return stored, nil
	}
	stored, err := runtime.GetWeeklyReview(ctx, database, r.ID)
	if err != nil {
		return db.WeeklyReview{}, fmt.Errorf("engine: reload weekly review: %w", err)
	}
	return stored, nil
}
