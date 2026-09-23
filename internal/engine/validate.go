package engine

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// Validation rules below distinguish two cases for every catalogue lookup:
//
//	subject/topic/lecture/problem genuinely absent (sql.ErrNoRows)
//	    → ErrInvalidTarget (the target itself is invalid)
//
//	actual database/query failure
//	    → plain wrapped error that propagates to the caller as a data
//	      error, never folded into an "invalid target" verdict.

// ValidateGovLectureTarget semantically validates a lecture target: non-empty
// IDs, subject/topic/lecture existence and relationship chain.
func ValidateGovLectureTarget(ctx context.Context, q db.DBTX, lt db.GovLectureTarget) error {
	if lt.SubjectID == "" || lt.TopicID == "" || lt.LectureID == "" {
		return wrapErr("lecture target", ErrInvalidTarget, "subject/topic/lecture required")
	}
	subj, err := catalog.GetGovSubject(ctx, q, lt.SubjectID)
	if err != nil {
		if isNotFoundErr(err) {
			return wrapErr("lecture target", ErrInvalidTarget, fmt.Sprintf("unknown subject %q", string(lt.SubjectID)))
		}
		return fmt.Errorf("engine: gov subject lookup: %w", err)
	}
	_ = subj
	tp, err := catalog.GetGovTopic(ctx, q, lt.TopicID)
	if err != nil {
		if isNotFoundErr(err) {
			return wrapErr("lecture target", ErrInvalidTarget, fmt.Sprintf("unknown topic %q", string(lt.TopicID)))
		}
		return fmt.Errorf("engine: gov topic lookup: %w", err)
	}
	if tp.SubjectID != lt.SubjectID {
		return wrapErr("lecture target", ErrInvalidTarget, "topic does not belong to subject")
	}
	lecs, err := catalog.GetGovLectures(ctx, q, lt.TopicID)
	if err != nil {
		return fmt.Errorf("engine: gov lectures lookup: %w", err)
	}
	for _, l := range lecs {
		if l.ID == lt.LectureID {
			return nil
		}
	}
	return wrapErr("lecture target", ErrInvalidTarget, fmt.Sprintf("unknown lecture %q for topic", string(lt.LectureID)))
}

// ValidateGovPracticeTarget validates a practice bundle: non-empty, no
// duplicates, subject exists, every topic exists and belongs to the subject.
func ValidateGovPracticeTarget(ctx context.Context, q db.DBTX, pt db.GovPracticeTarget) error {
	if pt.SubjectID == "" {
		return wrapErr("practice target", ErrInvalidTarget, "subject required")
	}
	if len(pt.TopicIDs) == 0 {
		return wrapErr("practice target", ErrInvalidTarget, "empty topic bundle")
	}
	if _, err := catalog.GetGovSubject(ctx, q, pt.SubjectID); err != nil {
		if isNotFoundErr(err) {
			return wrapErr("practice target", ErrInvalidTarget, fmt.Sprintf("unknown subject %q", string(pt.SubjectID)))
		}
		return fmt.Errorf("engine: gov subject lookup: %w", err)
	}
	seen := map[db.ID]bool{}
	for _, tid := range pt.TopicIDs {
		if tid == "" {
			return wrapErr("practice target", ErrInvalidTarget, "empty topic id")
		}
		if seen[tid] {
			return wrapErr("practice target", ErrInvalidTarget, fmt.Sprintf("duplicate topic %q", string(tid)))
		}
		seen[tid] = true
		tp, err := catalog.GetGovTopic(ctx, q, tid)
		if err != nil {
			if isNotFoundErr(err) {
				return wrapErr("practice target", ErrInvalidTarget, fmt.Sprintf("unknown topic %q", string(tid)))
			}
			return fmt.Errorf("engine: gov topic lookup: %w", err)
		}
		if tp.SubjectID != pt.SubjectID {
			return wrapErr("practice target", ErrInvalidTarget, fmt.Sprintf("topic %q not in subject", string(tid)))
		}
	}
	return nil
}

// ValidateDSATarget validates a DSA bundle: non-empty, no duplicate problems,
// every problem exists, modes valid (NEW/REPEAT).
func ValidateDSATarget(ctx context.Context, q db.DBTX, dt db.DSATarget) error {
	items := dt.Items()
	if len(items) == 0 {
		return wrapErr("dsa target", ErrInvalidTarget, "empty problem bundle")
	}
	seen := map[db.ID]bool{}
	for _, it := range items {
		if it.ProblemID == "" {
			return wrapErr("dsa target", ErrInvalidTarget, "empty problem id")
		}
		if seen[it.ProblemID] {
			return wrapErr("dsa target", ErrInvalidTarget, fmt.Sprintf("duplicate problem %q", string(it.ProblemID)))
		}
		seen[it.ProblemID] = true
		if it.Mode != db.DSAModeNew && it.Mode != db.DSAModeRepeat {
			return wrapErr("dsa target", ErrInvalidTarget, fmt.Sprintf("invalid mode %q", string(it.Mode)))
		}
		if _, err := catalog.GetDSAProblem(ctx, q, it.ProblemID); err != nil {
			if isNotFoundErr(err) {
				return wrapErr("dsa target", ErrInvalidTarget, fmt.Sprintf("unknown problem %q", string(it.ProblemID)))
			}
			return fmt.Errorf("engine: dsa problem lookup: %w", err)
		}
	}
	return nil
}

// ValidateDSACurrentBundle strictly validates a newly generated DSA block
// target: exactly 3 problems, all distinct, valid modes, every problem
// exists, truthful labeling (a NEW-labeled problem must be genuinely new:
// no prior NEW attempt and no repeat state), and the steady-state shape of
// exactly 2 REPEAT + 1 NEW. An all-NEW triple is accepted only as the
// truthful bootstrap when all three problems are genuinely new (fresh DB).
// Historical singular/legacy shapes stay readable via ValidateDSATarget;
// this strict rule governs NEW/current targets. DB failures propagate.
func ValidateDSACurrentBundle(ctx context.Context, q db.DBTX, dt db.DSATarget) error {
	if err := ValidateDSATarget(ctx, q, dt); err != nil {
		return err
	}
	items := dt.Items()
	if len(items) != 3 {
		return wrapErr("dsa target", ErrInvalidTarget, "current DSA bundle must be exactly 3 problems")
	}
	var repeats, news int
	for _, it := range items {
		switch it.Mode {
		case db.DSAModeRepeat:
			repeats++
		case db.DSAModeNew:
			news++
			if err := requireGenuinelyNew(ctx, q, it.ProblemID); err != nil {
				return err
			}
		}
	}
	if repeats == 2 && news == 1 {
		return nil
	}
	if news == 3 {
		// Truthful bootstrap: all three genuinely new (checked above).
		return nil
	}
	return wrapErr("dsa target", ErrInvalidTarget, "current DSA bundle must be 2 repeat + 1 new")
}

// requireGenuinelyNew rejects a NEW label for an already-introduced
// problem instead of silently converting it to REPEAT at save time. A
// problem is introduced once it has a NEW attempt OR repeat state; either
// makes a persisted NEW label stale. Catalogue/DB failures propagate and
// never read as "genuinely new".
func requireGenuinelyNew(ctx context.Context, q db.DBTX, problemID db.ID) error {
	atts, err := runtime.GetDSAAttempts(ctx, q, problemID)
	if err != nil {
		return fmt.Errorf("engine: dsa attempts: %w", err)
	}
	for _, a := range atts {
		if a.Mode == db.DSAModeNew {
			return wrapErr("dsa target", ErrInvalidTarget, fmt.Sprintf("stale target: problem %q labeled NEW but already introduced", string(problemID)))
		}
	}
	if _, err := runtime.GetDSARepeatState(ctx, q, problemID); err == nil {
		return wrapErr("dsa target", ErrInvalidTarget, fmt.Sprintf("stale target: problem %q labeled NEW but already introduced", string(problemID)))
	} else if !isNotFoundErr(err) {
		return fmt.Errorf("engine: repeat state: %w", err)
	}
	return nil
}

// ValidateSWETarget validates a SWE area target: the area must exist.
// SWE is a time-allocation system: routine slot -> area -> Today block.
// There are no tasks, bundles, or completion states.
func ValidateSWETarget(ctx context.Context, q db.DBTX, st db.SWETarget) error {
	if st.AreaID == "" {
		return wrapErr("swe target", ErrInvalidTarget, "area required")
	}
	if _, err := catalog.GetSWEArea(ctx, q, st.AreaID); err != nil {
		if isNotFoundErr(err) {
			return wrapErr("swe target", ErrInvalidTarget, fmt.Sprintf("unknown area %q", string(st.AreaID)))
		}
		return fmt.Errorf("engine: swe area lookup: %w", err)
	}
	return nil
}
