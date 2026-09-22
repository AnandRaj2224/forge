package engine

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

// ValidateGovLectureTarget semantically validates a lecture target: non-empty
// IDs, subject/topic/lecture existence and relationship chain.
func ValidateGovLectureTarget(ctx context.Context, q db.DBTX, lt db.GovLectureTarget) error {
	if lt.SubjectID == "" || lt.TopicID == "" || lt.LectureID == "" {
		return wrapErr("lecture target", ErrInvalidTarget, "subject/topic/lecture required")
	}
	subj, err := catalog.GetGovSubject(ctx, q, lt.SubjectID)
	if err != nil {
		return wrapErr("lecture target", ErrInvalidTarget, fmt.Sprintf("unknown subject %q", string(lt.SubjectID)))
	}
	_ = subj
	tp, err := catalog.GetGovTopic(ctx, q, lt.TopicID)
	if err != nil {
		return wrapErr("lecture target", ErrInvalidTarget, fmt.Sprintf("unknown topic %q", string(lt.TopicID)))
	}
	if tp.SubjectID != lt.SubjectID {
		return wrapErr("lecture target", ErrInvalidTarget, "topic does not belong to subject")
	}
	lecs, err := catalog.GetGovLectures(ctx, q, lt.TopicID)
	if err != nil {
		return wrapErr("lecture target", ErrInvalidTarget, "lecture lookup failed")
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
		return wrapErr("practice target", ErrInvalidTarget, fmt.Sprintf("unknown subject %q", string(pt.SubjectID)))
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
			return wrapErr("practice target", ErrInvalidTarget, fmt.Sprintf("unknown topic %q", string(tid)))
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
			return wrapErr("dsa target", ErrInvalidTarget, fmt.Sprintf("unknown problem %q", string(it.ProblemID)))
		}
	}
	return nil
}

// ValidateSWETarget validates a SWE bundle: non-empty, no duplicate tasks,
// every area exists, every task exists and belongs to its area.
func ValidateSWETarget(ctx context.Context, q db.DBTX, st db.SWETarget) error {
	items := st.Items()
	if len(items) == 0 {
		return wrapErr("swe target", ErrInvalidTarget, "empty task bundle")
	}
	areas, err := catalog.GetSWEAreas(ctx, q)
	if err != nil {
		return fmt.Errorf("engine: list swe areas: %w", err)
	}
	areaOK := map[db.ID]bool{}
	for _, a := range areas {
		areaOK[a.ID] = true
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.AreaID == "" {
			return wrapErr("swe target", ErrInvalidTarget, "area required")
		}
		if !areaOK[it.AreaID] {
			return wrapErr("swe target", ErrInvalidTarget, fmt.Sprintf("unknown area %q", string(it.AreaID)))
		}
		if it.TaskID == nil {
			// Legacy area-only: allowed for historical snapshots.
			key := "area:" + string(it.AreaID)
			if seen[key] {
				return wrapErr("swe target", ErrInvalidTarget, "duplicate area target")
			}
			seen[key] = true
			continue
		}
		key := "task:" + string(*it.TaskID)
		if seen[key] {
			return wrapErr("swe target", ErrInvalidTarget, fmt.Sprintf("duplicate task %q", string(*it.TaskID)))
		}
		seen[key] = true
		tasks, err := catalog.GetSWETasks(ctx, q, it.AreaID)
		if err != nil {
			return fmt.Errorf("engine: list swe tasks: %w", err)
		}
		found := false
		for _, t := range tasks {
			if t.ID == *it.TaskID {
				found = true
				break
			}
		}
		if !found {
			return wrapErr("swe target", ErrInvalidTarget, fmt.Sprintf("task %q not in area %q", string(*it.TaskID), string(it.AreaID)))
		}
	}
	return nil
}
