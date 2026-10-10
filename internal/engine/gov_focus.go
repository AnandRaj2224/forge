package engine

import (
	"context"
	"fmt"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

// V2 explicit exam focus. The focus exam is stored, never derived from
// sort order or completion. Main-goal is separate from focus.

// GetFocusExam returns the explicitly selected focus exam, or (nil, nil)
// when no focus is set yet.
func GetFocusExam(ctx context.Context, q db.DBTX) (*db.GovExam, error) {
	st, err := catalog.GetGovFocusState(ctx, q)
	if err != nil {
		if isNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("engine: focus state: %w", err)
	}
	if st.FocusedExamID == nil || *st.FocusedExamID == "" {
		return nil, nil
	}
	e, err := catalog.GetGovExam(ctx, q, *st.FocusedExamID)
	if err != nil {
		if isNotFoundErr(err) {
			return nil, wrapErr("focus", ErrInvalidTarget, fmt.Sprintf("focused exam %q no longer exists", string(*st.FocusedExamID)))
		}
		return nil, fmt.Errorf("engine: focus exam lookup: %w", err)
	}
	c := e
	return &c, nil
}

// EnsureFocusExam returns the focus exam, establishing a default when none
// is set: the first ACTIVE-profiled exam in sort order, else the first exam.
// It persists the default so subsequent calls are stable. With no exams it
// returns (nil, nil).
func EnsureFocusExam(ctx context.Context, q db.DBTX, now time.Time) (*db.GovExam, error) {
	if cur, err := GetFocusExam(ctx, q); err != nil {
		return nil, err
	} else if cur != nil {
		return cur, nil
	}
	exams, err := catalog.GetGovExams(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("engine: gov exams: %w", err)
	}
	if len(exams) == 0 {
		return nil, nil
	}
	// Prefer ACTIVE profiles when they exist; otherwise sort order.
	profiles, _ := catalog.GetGovExamProfiles(ctx, q)
	statusOf := map[db.ID]string{}
	for _, p := range profiles {
		statusOf[p.ExamID] = p.Status
	}
	var pick *db.GovExam
	if len(profiles) > 0 {
		for _, e := range exams {
			if statusOf[e.ID] == db.ExamStatusActive {
				c := e
				pick = &c
				break
			}
		}
		// If profiles exist but none ACTIVE, do not invent focus.
		if pick == nil {
			return nil, nil
		}
	} else {
		c := exams[0]
		pick = &c
	}
	if err := catalog.SetGovFocusState(ctx, q, db.GovFocusState{ID: 1, FocusedExamID: &pick.ID, UpdatedAt: now}); err != nil {
		return nil, fmt.Errorf("engine: persist default focus: %w", err)
	}
	return pick, nil
}

// SetFocusExam validates and persists a new focus exam. Same-focus requests
// are no-ops returning the current exam. Archived exams cannot be focused.
func SetFocusExam(ctx context.Context, q db.DBTX, examID db.ID, now time.Time) (*db.GovExam, error) {
	if examID == "" {
		return nil, wrapErr("focus", ErrValidation, "exam required")
	}
	e, err := catalog.GetGovExam(ctx, q, examID)
	if err != nil {
		if isNotFoundErr(err) {
			return nil, wrapErr("focus", ErrInvalidTarget, fmt.Sprintf("unknown exam %q", string(examID)))
		}
		return nil, fmt.Errorf("engine: exam lookup: %w", err)
	}
	if prof, err := catalog.GetGovExamProfile(ctx, q, examID); err == nil {
		if prof.Status == db.ExamStatusArchived {
			return nil, wrapErr("focus", ErrValidation, "archived exams cannot be focused")
		}
	} else if !isNotFoundErr(err) {
		return nil, fmt.Errorf("engine: exam profile: %w", err)
	}
	cur, err := GetFocusExam(ctx, q)
	if err != nil {
		return nil, err
	}
	if cur != nil && cur.ID == examID {
		return cur, nil
	}
	if err := catalog.SetGovFocusState(ctx, q, db.GovFocusState{ID: 1, FocusedExamID: &examID, UpdatedAt: now}); err != nil {
		return nil, fmt.Errorf("engine: set focus: %w", err)
	}
	c := e
	return &c, nil
}

// GetExamProfiles returns all exam profiles (may be empty on fresh DBs).
func GetExamProfiles(ctx context.Context, q db.DBTX) ([]db.GovExamProfile, error) {
	return catalog.GetGovExamProfiles(ctx, q)
}

// UpsertExamProfile validates and stores an exam profile. Main-goal is
// exclusive: setting a new main goal clears the previous one in the same tx
// when q is a *db.DB (callers inside a tx pass the tx for atomicity).
func UpsertExamProfile(ctx context.Context, database *db.DB, p db.GovExamProfile, now time.Time) error {
	if p.ExamID == "" {
		return wrapErr("exam profile", ErrValidation, "exam required")
	}
	switch p.Status {
	case db.ExamStatusActive, db.ExamStatusPaused, db.ExamStatusArchived:
	default:
		return wrapErr("exam profile", ErrValidation, "status must be ACTIVE, PAUSED, or ARCHIVED")
	}
	if _, err := catalog.GetGovExam(ctx, database, p.ExamID); err != nil {
		if isNotFoundErr(err) {
			return wrapErr("exam profile", ErrInvalidTarget, fmt.Sprintf("unknown exam %q", string(p.ExamID)))
		}
		return fmt.Errorf("engine: exam lookup: %w", err)
	}
	if p.TargetDate != nil {
		if _, err := DefaultCalendar().StartOfDay(*p.TargetDate); err != nil {
			return wrapErr("exam profile", ErrValidation, "target date must be YYYY-MM-DD")
		}
	}
	p.UpdatedAt = now
	return database.WithTx(ctx, func(q db.DBTX) error {
		if p.IsMainGoal {
			profiles, err := catalog.GetGovExamProfiles(ctx, q)
			if err != nil {
				return err
			}
			for _, prev := range profiles {
				if prev.ExamID != p.ExamID && prev.IsMainGoal {
					prev.IsMainGoal = false
					prev.UpdatedAt = now
					if err := catalog.UpsertGovExamProfile(ctx, q, prev); err != nil {
						return err
					}
				}
			}
		}
		return catalog.UpsertGovExamProfile(ctx, q, p)
	})
}

// IsGovExamConfigured reports whether an exam has any syllabus topics.
// Unconfigured exams are never treated as complete.
func IsGovExamConfigured(ctx context.Context, q db.DBTX, examID db.ID) (bool, error) {
	syll, err := catalog.GetGovExamSyllabus(ctx, q, examID)
	if err != nil {
		return false, err
	}
	return len(syll) > 0, nil
}
