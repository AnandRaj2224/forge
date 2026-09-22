package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// MockSectionInput is one user-entered section with explicit GOV subject mapping.
type MockSectionInput struct {
	Name         string
	GovSubjectID db.ID
	WeakTopics   []db.ID // ordered, 0-3, rank = index+1
}

// ValidateMockMetrics checks score/total/correct/attempted consistency for the
// provided fields. Nil fields are skipped (missing metrics are never
// fabricated). Negative values are rejected for every count-like field
// before any DB write; percentile/accuracy stay 0..100; avg speed stays
// positive where supplied.
func ValidateMockMetrics(m db.Mock) error {
	for name, v := range map[string]*int{
		"score": m.Score, "total_score": m.TotalScore, "rank": m.RankValue,
		"attempted": m.Attempted, "total_questions": m.TotalQuestions,
		"correct": m.Correct, "incorrect": m.Incorrect, "unattempted": m.Unattempted,
	} {
		if v != nil && *v < 0 {
			return wrapErr("mock", ErrValidation, name+" must be >= 0")
		}
	}
	if m.Score != nil && m.TotalScore != nil && *m.Score > *m.TotalScore {
		return wrapErr("mock", ErrValidation, "score <= total_score required")
	}
	if m.Correct != nil && m.Attempted != nil && *m.Correct > *m.Attempted {
		return wrapErr("mock", ErrValidation, "correct <= attempted required")
	}
	if m.Incorrect != nil && m.Attempted != nil && *m.Incorrect > *m.Attempted {
		return wrapErr("mock", ErrValidation, "incorrect <= attempted required")
	}
	if m.Unattempted != nil && *m.Unattempted < 0 {
		return wrapErr("mock", ErrValidation, "unattempted >= 0 required")
	}
	if m.Attempted != nil && m.TotalQuestions != nil && m.Unattempted != nil {
		if *m.Attempted+*m.Unattempted != *m.TotalQuestions {
			return wrapErr("mock", ErrValidation, "attempted + unattempted must equal total_questions")
		}
	}
	if m.Correct != nil && m.Incorrect != nil && m.Attempted != nil {
		if *m.Correct+*m.Incorrect != *m.Attempted {
			return wrapErr("mock", ErrValidation, "correct + incorrect must equal attempted")
		}
	}
	if m.Percentile != nil && (*m.Percentile < 0 || *m.Percentile > 100) {
		return wrapErr("mock", ErrValidation, "percentile must be 0..100")
	}
	if m.Accuracy != nil && (*m.Accuracy < 0 || *m.Accuracy > 100) {
		return wrapErr("mock", ErrValidation, "accuracy must be 0..100")
	}
	if m.AvgSpeedSeconds != nil && *m.AvgSpeedSeconds <= 0 {
		return wrapErr("mock", ErrValidation, "avg speed must be > 0")
	}
	return nil
}

// ValidateMockSection checks one section: user-entered name, explicit existing
// GOV subject mapping, weak topics belong to that subject AND the exam syllabus,
// max 3, no duplicates. Never infers the subject from the section name.
func ValidateMockSection(ctx context.Context, q db.DBTX, examID db.ID, sec MockSectionInput) error {
	if strings.TrimSpace(sec.Name) == "" {
		return wrapErr("mock section", ErrValidation, "actual section name required")
	}
	if sec.GovSubjectID == "" {
		return wrapErr("mock section", ErrValidation, "GOV subject mapping required")
	}
	if _, err := catalog.GetGovSubject(ctx, q, sec.GovSubjectID); err != nil {
		if isNotFoundErr(err) {
			return wrapErr("mock section", ErrValidation, fmt.Sprintf("unknown GOV subject %q", string(sec.GovSubjectID)))
		}
		return fmt.Errorf("engine: gov subject lookup: %w", err)
	}
	if len(sec.WeakTopics) > 3 {
		return wrapErr("mock section", ErrValidation, "at most 3 weak topics per section")
	}
	seen := map[db.ID]bool{}
	for _, tid := range sec.WeakTopics {
		if tid == "" {
			return wrapErr("mock section", ErrValidation, "empty topic id")
		}
		if seen[tid] {
			return wrapErr("mock section", ErrValidation, fmt.Sprintf("duplicate weak topic %q", string(tid)))
		}
		seen[tid] = true
		tp, err := catalog.GetGovTopic(ctx, q, tid)
		if err != nil {
			if isNotFoundErr(err) {
				return wrapErr("mock section", ErrValidation, fmt.Sprintf("unknown topic %q", string(tid)))
			}
			return fmt.Errorf("engine: gov topic lookup: %w", err)
		}
		if tp.SubjectID != sec.GovSubjectID {
			return wrapErr("mock section", ErrValidation, fmt.Sprintf("topic %q does not belong to mapped subject", string(tid)))
		}
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_exam_syllabus WHERE exam_id = ? AND topic_id = ?`, string(examID), string(tid)).Scan(&n); err != nil {
			return fmt.Errorf("engine: exam/topic check: %w", err)
		}
		if n == 0 {
			return wrapErr("mock section", ErrValidation, fmt.Sprintf("topic %q not in exam syllabus", string(tid)))
		}
	}
	return nil
}

// CreateMockAtomic creates mock + sections + weak topics in ONE transaction.
// Either everything succeeds or nothing is created.
func CreateMockAtomic(ctx context.Context, database *db.DB, m db.Mock, sections []MockSectionInput, now time.Time) (db.Mock, error) {
	if strings.TrimSpace(m.Name) == "" {
		return db.Mock{}, wrapErr("mock", ErrValidation, "mock name required")
	}
	if strings.TrimSpace(string(m.ExamID)) == "" {
		return db.Mock{}, wrapErr("mock", ErrValidation, "mock exam required")
	}
	if _, err := time.Parse("2006-01-02", m.MockDate); err != nil {
		return db.Mock{}, wrapErr("mock", ErrValidation, "mock date must be YYYY-MM-DD")
	}
	var n int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_exams WHERE id = ?`, string(m.ExamID)).Scan(&n); err != nil {
		return db.Mock{}, fmt.Errorf("engine: check exam: %w", err)
	}
	if n == 0 {
		return db.Mock{}, wrapErr("mock", ErrValidation, fmt.Sprintf("unknown exam %q", string(m.ExamID)))
	}
	if err := ValidateMockMetrics(m); err != nil {
		return db.Mock{}, err
	}
	for _, sec := range sections {
		if err := ValidateMockSection(ctx, database, m.ExamID, sec); err != nil {
			return db.Mock{}, err
		}
	}
	// Duplicate section names / ranks checked transactionally below.
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	err := database.WithTx(ctx, func(q db.DBTX) error {
		if err := runtime.CreateMock(ctx, q, m); err != nil {
			return err
		}
		seenNames := map[string]bool{}
		for i, sec := range sections {
			lower := strings.ToLower(strings.TrimSpace(sec.Name))
			if seenNames[lower] {
				return wrapErr("mock", ErrDuplicate, fmt.Sprintf("duplicate section %q", sec.Name))
			}
			seenNames[lower] = true
			sid := db.ID(fmt.Sprintf("%s-sec-%d-%d", string(m.ID), i, now.UnixNano()%100000))
			// Current schema contract: mock_sections.gov_subject_id is
			// required (migration 003). No legacy fallback here: a
			// schema/DB failure propagates and rolls the transaction
			// back instead of persisting an unmapped section.
			if _, err := q.ExecContext(ctx, `INSERT INTO mock_sections (id, mock_id, name, sort_order, gov_subject_id) VALUES (?, ?, ?, ?, ?)`,
				string(sid), string(m.ID), strings.TrimSpace(sec.Name), i+1, string(sec.GovSubjectID)); err != nil {
				return fmt.Errorf("engine: create mock section: %w", err)
			}
			for rank, tid := range sec.WeakTopics {
				if _, err := q.ExecContext(ctx, `INSERT INTO mock_weak_topics (section_id, topic_id, rank_order) VALUES (?, ?, ?)`,
					string(sid), string(tid), rank+1); err != nil {
					return fmt.Errorf("engine: insert weak topic: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return db.Mock{}, err
	}
	return m, nil
}

// DeleteMock deletes a mock (sections/weak topics cascade). It is the only
// normal path to mock deletion; the TUI must use it.
func DeleteMock(ctx context.Context, database *db.DB, mockID db.ID) error {
	if _, err := runtime.GetMock(ctx, database, mockID); err != nil {
		if isNotFoundErr(err) {
			return wrapErr("mock delete", ErrNotFound, fmt.Sprintf("unknown mock %q", string(mockID)))
		}
		return fmt.Errorf("engine: get mock: %w", err)
	}
	if err := runtime.DeleteMock(ctx, database, mockID); err != nil {
		return fmt.Errorf("engine: delete mock: %w", err)
	}
	return nil
}

// ReplaceWeakTopicsAtomic replaces a section's weak topics in ONE transaction
// with full validation (subject + syllabus + max 3 + ranks).
func ReplaceWeakTopicsAtomic(ctx context.Context, database *db.DB, mockID, sectionID db.ID, govSubjectID db.ID, topicIDs []db.ID) error {
	var examID db.ID
	var mock db.Mock
	m, err := runtime.GetMock(ctx, database, mockID)
	if err != nil {
		return fmt.Errorf("engine: get mock: %w", err)
	}
	mock = m
	examID = mock.ExamID
	_ = examID
	if err := ValidateMockSection(ctx, database, mock.ExamID, MockSectionInput{
		Name: "x", GovSubjectID: govSubjectID, WeakTopics: topicIDs,
	}); err != nil {
		// Name check would fail on "x"? No, "x" is non-empty so fine.
		return err
	}
	// Verify section belongs to mock.
	var cnt int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM mock_sections WHERE id = ? AND mock_id = ?`, string(sectionID), string(mockID)).Scan(&cnt); err != nil {
		return fmt.Errorf("engine: section lookup: %w", err)
	}
	if cnt == 0 {
		return wrapErr("mock", ErrValidation, "section does not belong to mock")
	}
	return database.WithTx(ctx, func(q db.DBTX) error {
		if _, err := q.ExecContext(ctx, `DELETE FROM mock_weak_topics WHERE section_id = ?`, string(sectionID)); err != nil {
			return fmt.Errorf("engine: clear weak topics: %w", err)
		}
		for i, tid := range topicIDs {
			if _, err := q.ExecContext(ctx, `INSERT INTO mock_weak_topics (section_id, topic_id, rank_order) VALUES (?, ?, ?)`,
				string(sectionID), string(tid), i+1); err != nil {
				return fmt.Errorf("engine: insert weak topic: %w", err)
			}
		}
		// Current schema contract: the explicit subject mapping is
		// persisted unconditionally (migration 003). A schema/DB
		// failure propagates and rolls back instead of leaving weak
		// topics on an unmapped section.
		if _, err := q.ExecContext(ctx, `UPDATE mock_sections SET gov_subject_id = ? WHERE id = ?`, string(govSubjectID), string(sectionID)); err != nil {
			return fmt.Errorf("engine: update section subject: %w", err)
		}
		return nil
	})
}
