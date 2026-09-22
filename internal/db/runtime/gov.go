package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
)

func unixToTime(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// isUniqueViolation reports a SQLite uniqueness conflict (PRIMARY KEY or
// UNIQUE index), used to translate persistence races into domain state
// instead of leaking raw storage errors.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "UNIQUE CONSTRAINT FAILED") || strings.Contains(msg, "PRIMARY KEY")
}

// ---- Gov subject activation ----

// IsGovSubjectActive reports whether a subject-activation row exists.
//
// Persistence primitive for the Engine: activation gating decisions live
// in engine planning; product code must not treat this as a business
// verdict on its own. DB failures are returned as errors, never silent
// false.
func IsGovSubjectActive(ctx context.Context, q db.DBTX, subjectID db.ID) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_subject_activation WHERE subject_id = ?`, subjectID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("runtime: is subject active: %w", err)
	}
	return count > 0, nil
}

// ActivateGovSubject marks a subject as active. Idempotent: a second
// activation is a no-op (never a UNIQUE collision through normal operation).
func ActivateGovSubject(ctx context.Context, q db.DBTX, subjectID db.ID, when time.Time) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_subject_activation (subject_id, activated_at) VALUES (?, ?)
ON CONFLICT(subject_id) DO NOTHING`, subjectID, when.Unix()); err != nil {
		return fmt.Errorf("runtime: activate subject: %w", err)
	}
	return nil
}

// GetGovSubjectActivation returns the activation row, or sql.ErrNoRows.
func GetGovSubjectActivation(ctx context.Context, q db.DBTX, subjectID db.ID) (db.GovSubjectActivation, error) {
	var a db.GovSubjectActivation
	var ts int64
	err := q.QueryRowContext(ctx, `SELECT subject_id, activated_at FROM gov_subject_activation WHERE subject_id = ?`, subjectID).Scan(&a.SubjectID, &ts)
	if err != nil {
		return db.GovSubjectActivation{}, fmt.Errorf("runtime: get subject activation: %w", err)
	}
	a.ActivatedAt = unixToTime(ts)
	return a, nil
}

// ---- Gov lecture completions ----

// IsLectureComplete reports whether a lecture has a completion row.
//
// Persistence primitive for the Engine: product code must use
// engine.IsLectureComplete (the authoritative product notion), never this
// helper directly. DB failures are returned as errors, never silent false.
func IsLectureComplete(ctx context.Context, q db.DBTX, lectureID db.ID) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_lecture_completions WHERE lecture_id = ?`, lectureID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("runtime: is lecture complete: %w", err)
	}
	return count > 0, nil
}

// RecordLectureCompletion records a one-time lecture completion.
// Lectures complete once: a duplicate completion is rejected here so a
// completed lecture can never reach the UNIQUE save path twice through
// normal operation (the Today CanStart rule blocks it first).
//
// The pre-check distinguishes absence from failure: a query failure
// propagates as a DB error and never reads as "not complete". The insert
// itself stays the persistence primitive: a uniqueness conflict there
// maps to the already-complete domain state, never a raw storage error.
func RecordLectureCompletion(ctx context.Context, q db.DBTX, c db.GovLectureCompletion) error {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_lecture_completions WHERE lecture_id = ?`, c.LectureID).Scan(&count); err != nil {
		return fmt.Errorf("runtime: lecture completion check: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("runtime: lecture %q already complete — result is read-only", c.LectureID)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES (?, ?, ?)`,
		c.LectureID, c.SessionID, c.CompletedAt.Unix()); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("runtime: lecture %q already complete — result is read-only", c.LectureID)
		}
		return fmt.Errorf("runtime: record lecture completion: %w", err)
	}
	return nil
}

// GetLectureCompletion returns the completion row for a lecture.
func GetLectureCompletion(ctx context.Context, q db.DBTX, lectureID db.ID) (db.GovLectureCompletion, error) {
	var c db.GovLectureCompletion
	var ts int64
	err := q.QueryRowContext(ctx, `SELECT lecture_id, session_id, completed_at FROM gov_lecture_completions WHERE lecture_id = ?`, lectureID).Scan(&c.LectureID, &c.SessionID, &ts)
	if err != nil {
		return db.GovLectureCompletion{}, fmt.Errorf("runtime: get lecture completion: %w", err)
	}
	c.CompletedAt = unixToTime(ts)
	return c, nil
}

// ---- Gov practice records ----

// InsertGovPracticeRecord inserts one topic-practiced row.
func InsertGovPracticeRecord(ctx context.Context, q db.DBTX, r db.GovPracticeRecord) error {
	var avg any
	if r.AvgQuestionSpeedSeconds != nil {
		avg = *r.AvgQuestionSpeedSeconds
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, r.TopicID, r.TimeSpentSeconds, r.Attempted, r.Correct, avg, r.RecordedAt.Unix(),
	); err != nil {
		return fmt.Errorf("runtime: insert practice record: %w", err)
	}
	return nil
}

// GetGovPracticeHistory returns practice rows for a topic ordered by recorded_at.
func GetGovPracticeHistory(ctx context.Context, q db.DBTX, topicID db.ID) ([]db.GovPracticeRecord, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at
FROM gov_practice_records WHERE topic_id = ? ORDER BY recorded_at`, topicID)
	if err != nil {
		return nil, fmt.Errorf("runtime: practice history: %w", err)
	}
	defer rows.Close()
	return scanPracticeRecords(rows)
}

// GetGovPracticeRecordsBySession returns practice rows for a session.
func GetGovPracticeRecordsBySession(ctx context.Context, q db.DBTX, sessionID db.ID) ([]db.GovPracticeRecord, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at
FROM gov_practice_records WHERE session_id = ? ORDER BY topic_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runtime: practice by session: %w", err)
	}
	defer rows.Close()
	return scanPracticeRecords(rows)
}

func scanPracticeRecords(rows *sql.Rows) ([]db.GovPracticeRecord, error) {
	var out []db.GovPracticeRecord
	for rows.Next() {
		var r db.GovPracticeRecord
		var avg sql.NullFloat64
		var recorded int64
		if err := rows.Scan(&r.ID, &r.SessionID, &r.TopicID, &r.TimeSpentSeconds, &r.Attempted, &r.Correct, &avg, &recorded); err != nil {
			return nil, fmt.Errorf("runtime: scan practice record: %w", err)
		}
		if avg.Valid {
			v := avg.Float64
			r.AvgQuestionSpeedSeconds = &v
		}
		r.RecordedAt = unixToTime(recorded)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- Gov practice rotation state ----

// GetGovPracticeRotationState returns the singleton rotation state.
// Returns sql.ErrNoRows wrapped when the row does not exist yet.
func GetGovPracticeRotationState(ctx context.Context, q db.DBTX) (db.GovPracticeRotationState, error) {
	var s db.GovPracticeRotationState
	var last *string
	var updated int64
	err := q.QueryRowContext(ctx, `SELECT id, last_subject_id, updated_at FROM gov_practice_rotation_state WHERE id = 1`).Scan(&s.ID, &last, &updated)
	if err != nil {
		return db.GovPracticeRotationState{}, fmt.Errorf("runtime: get rotation state: %w", err)
	}
	if last != nil {
		id := db.ID(*last)
		s.LastSubjectID = &id
	}
	s.UpdatedAt = unixToTime(updated)
	return s, nil
}

// SetGovPracticeRotationState upserts the singleton rotation state.
func SetGovPracticeRotationState(ctx context.Context, q db.DBTX, s db.GovPracticeRotationState) error {
	if s.ID == 0 {
		s.ID = 1
	}
	var last any
	if s.LastSubjectID != nil {
		last = string(*s.LastSubjectID)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_practice_rotation_state (id, last_subject_id, updated_at) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET last_subject_id = excluded.last_subject_id, updated_at = excluded.updated_at`,
		s.ID, last, s.UpdatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("runtime: set rotation state: %w", err)
	}
	return nil
}
