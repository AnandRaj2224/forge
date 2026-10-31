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

// GetLectureCompletionMap returns completion presence for a bounded set of
// lecture IDs in ONE query: lecture IDs → one completion query →
// map[lectureID]bool. Missing keys mean incomplete. It preserves the
// rows-consume-then-query invariant (rows fully consumed and closed before
// any follow-up query).
func GetLectureCompletionMap(ctx context.Context, q db.DBTX, lectureIDs []db.ID) (map[db.ID]bool, error) {
	out := make(map[db.ID]bool, len(lectureIDs))
	for _, id := range lectureIDs {
		out[id] = false
	}
	if len(lectureIDs) == 0 {
		return out, nil
	}
	place := make([]string, 0, len(lectureIDs))
	args := make([]any, 0, len(lectureIDs))
	for _, id := range lectureIDs {
		place = append(place, "?")
		args = append(args, string(id))
	}
	rows, err := q.QueryContext(ctx, `SELECT lecture_id FROM gov_lecture_completions WHERE lecture_id IN (`+strings.Join(place, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("runtime: lecture completion map: %w", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var lid string
		if err := rows.Scan(&lid); err != nil {
			return nil, fmt.Errorf("runtime: lecture completion map scan: %w", err)
		}
		found = append(found, lid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("runtime: lecture completion map rows: %w", err)
	}
	for _, lid := range found {
		out[db.ID(lid)] = true
	}
	return out, nil
}

// ---- Gov practice records ----

// InsertGovPracticeRecord inserts one topic-practiced row.
// ExamID attributes the result to the focused exam; nil preserves legacy
// global rows. Callers should pass the focused exam for new saves.
func InsertGovPracticeRecord(ctx context.Context, q db.DBTX, r db.GovPracticeRecord) error {
	var avg any
	if r.AvgQuestionSpeedSeconds != nil {
		avg = *r.AvgQuestionSpeedSeconds
	}
	var exam any
	if r.ExamID != nil {
		exam = string(*r.ExamID)
	}
	var topicID, testID any
	if r.TestID != nil {
		testID = string(*r.TestID)
	} else if strings.HasPrefix(string(r.TopicID), "test-") {
		testID = string(r.TopicID)
	}
	if testID == nil && r.TopicID != "" {
		topicID = string(r.TopicID)
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO gov_practice_records (id, session_id, topic_id, test_id, exam_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, topicID, testID, exam, r.TimeSpentSeconds, r.Attempted, r.Correct, avg, r.RecordedAt.Unix(),
	); err != nil {
		return fmt.Errorf("runtime: insert practice record: %w", err)
	}
	return nil
}

// GetGovPracticeHistory returns practice rows for a topic ordered by recorded_at.
func GetGovPracticeHistory(ctx context.Context, q db.DBTX, topicID db.ID) ([]db.GovPracticeRecord, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, topic_id, test_id, exam_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at
FROM gov_practice_records WHERE topic_id = ? OR test_id = ? ORDER BY recorded_at`, string(topicID), string(topicID))
	if err != nil {
		return nil, fmt.Errorf("runtime: practice history: %w", err)
	}
	defer rows.Close()
	return scanPracticeRecords(rows)
}

// GetGovPracticeRecordsBySession returns practice rows for a session.
func GetGovPracticeRecordsBySession(ctx context.Context, q db.DBTX, sessionID db.ID) ([]db.GovPracticeRecord, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, topic_id, test_id, exam_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at
FROM gov_practice_records WHERE session_id = ? ORDER BY COALESCE(test_id, topic_id)`, sessionID)
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
		var topicID, testID sql.NullString
		var avg sql.NullFloat64
		var exam sql.NullString
		var recorded int64
		if err := rows.Scan(&r.ID, &r.SessionID, &topicID, &testID, &exam, &r.TimeSpentSeconds, &r.Attempted, &r.Correct, &avg, &recorded); err != nil {
			return nil, fmt.Errorf("runtime: scan practice record: %w", err)
		}
		if testID.Valid && testID.String != "" {
			tID := db.ID(testID.String)
			r.TestID = &tID
			r.TopicID = tID
		} else if topicID.Valid {
			r.TopicID = db.ID(topicID.String)
		}
		if avg.Valid {
			v := avg.Float64
			r.AvgQuestionSpeedSeconds = &v
		}
		if exam.Valid {
			v := db.ID(exam.String)
			r.ExamID = &v
		}
		r.RecordedAt = unixToTime(recorded)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetGovPracticeRecordsBySessions returns practice rows for a bounded set of
// sessions in ONE query (batch alternative to per-session
// GetGovPracticeRecordsBySession). Rows are fully consumed before return;
// callers must not nest queries inside the scan.
func GetGovPracticeRecordsBySessions(ctx context.Context, q db.DBTX, sessionIDs []db.ID) ([]db.GovPracticeRecord, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	place := make([]string, 0, len(sessionIDs))
	args := make([]any, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		place = append(place, "?")
		args = append(args, string(id))
	}
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, topic_id, test_id, exam_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at
FROM gov_practice_records WHERE session_id IN (`+strings.Join(place, ",")+`) ORDER BY COALESCE(test_id, topic_id)`, args...)
	if err != nil {
		return nil, fmt.Errorf("runtime: practice by sessions: %w", err)
	}
	defer rows.Close()
	return scanPracticeRecords(rows)
}
