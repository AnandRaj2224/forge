package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
)

func unixToTime(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// ---- Gov subjects ----

func GetGovSubjects(ctx context.Context, q db.DBTX) ([]db.GovSubject, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, sort_order FROM gov_subjects ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list gov subjects: %w", err)
	}
	defer rows.Close()
	var out []db.GovSubject
	for rows.Next() {
		var s db.GovSubject
		if err := rows.Scan(&s.ID, &s.Name, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan gov subject: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func GetGovSubject(ctx context.Context, q db.DBTX, id db.ID) (db.GovSubject, error) {
	var s db.GovSubject
	err := q.QueryRowContext(ctx, `SELECT id, name, sort_order FROM gov_subjects WHERE id = ?`, id).Scan(&s.ID, &s.Name, &s.SortOrder)
	if err != nil {
		return db.GovSubject{}, fmt.Errorf("catalog: get gov subject: %w", err)
	}
	return s, nil
}

func InsertGovSubject(ctx context.Context, q db.DBTX, s db.GovSubject) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_subjects (id, name, sort_order) VALUES (?, ?, ?)`, s.ID, s.Name, s.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert gov subject: %w", err)
	}
	return nil
}

// ---- Gov topics ----

func GetGovTopics(ctx context.Context, q db.DBTX, subjectID db.ID) ([]db.GovTopic, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, subject_id, name, sort_order FROM gov_topics WHERE subject_id = ? ORDER BY sort_order`, subjectID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list gov topics: %w", err)
	}
	defer rows.Close()
	var out []db.GovTopic
	for rows.Next() {
		var t db.GovTopic
		if err := rows.Scan(&t.ID, &t.SubjectID, &t.Name, &t.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan gov topic: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func GetGovTopic(ctx context.Context, q db.DBTX, id db.ID) (db.GovTopic, error) {
	var t db.GovTopic
	err := q.QueryRowContext(ctx, `SELECT id, subject_id, name, sort_order FROM gov_topics WHERE id = ?`, id).Scan(&t.ID, &t.SubjectID, &t.Name, &t.SortOrder)
	if err != nil {
		return db.GovTopic{}, fmt.Errorf("catalog: get gov topic: %w", err)
	}
	return t, nil
}

func InsertGovTopic(ctx context.Context, q db.DBTX, t db.GovTopic) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES (?, ?, ?, ?)`, t.ID, t.SubjectID, t.Name, t.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert gov topic: %w", err)
	}
	return nil
}

// ---- Gov lectures ----

func GetGovLectures(ctx context.Context, q db.DBTX, topicID db.ID) ([]db.GovLecture, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, topic_id, title, sort_order FROM gov_lectures WHERE topic_id = ? ORDER BY sort_order`, topicID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list gov lectures: %w", err)
	}
	defer rows.Close()
	var out []db.GovLecture
	for rows.Next() {
		var l db.GovLecture
		if err := rows.Scan(&l.ID, &l.TopicID, &l.Title, &l.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan gov lecture: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetGovLecturesByTopics returns lectures for a bounded set of topics in ONE
// query, grouped by topic ID. It preserves the rows-consume-then-query
// invariant (rows fully consumed and closed before any follow-up query).
func GetGovLecturesByTopics(ctx context.Context, q db.DBTX, topicIDs []db.ID) (map[db.ID][]db.GovLecture, error) {
	out := make(map[db.ID][]db.GovLecture, len(topicIDs))
	for _, id := range topicIDs {
		out[id] = nil
	}
	if len(topicIDs) == 0 {
		return out, nil
	}
	place := make([]string, 0, len(topicIDs))
	args := make([]any, 0, len(topicIDs))
	for _, id := range topicIDs {
		place = append(place, "?")
		args = append(args, string(id))
	}
	rows, err := q.QueryContext(ctx, `SELECT id, topic_id, title, sort_order FROM gov_lectures WHERE topic_id IN (`+strings.Join(place, ",")+`) ORDER BY topic_id, sort_order`, args...)
	if err != nil {
		return nil, fmt.Errorf("catalog: list gov lectures by topics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l db.GovLecture
		if err := rows.Scan(&l.ID, &l.TopicID, &l.Title, &l.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan gov lectures by topics: %w", err)
		}
		out[l.TopicID] = append(out[l.TopicID], l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: gov lectures by topics rows: %w", err)
	}
	return out, nil
}

func InsertGovLecture(ctx context.Context, q db.DBTX, l db.GovLecture) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES (?, ?, ?, ?)`, l.ID, l.TopicID, l.Title, l.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert gov lecture: %w", err)
	}
	return nil
}

// ---- Gov exams ----

func GetGovExams(ctx context.Context, q db.DBTX) ([]db.GovExam, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, sort_order FROM gov_exams ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list gov exams: %w", err)
	}
	defer rows.Close()
	var out []db.GovExam
	for rows.Next() {
		var e db.GovExam
		if err := rows.Scan(&e.ID, &e.Name, &e.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan gov exam: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func InsertGovExam(ctx context.Context, q db.DBTX, e db.GovExam) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_exams (id, name, sort_order) VALUES (?, ?, ?)`, e.ID, e.Name, e.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert gov exam: %w", err)
	}
	return nil
}

func GetGovExam(ctx context.Context, q db.DBTX, id db.ID) (db.GovExam, error) {
	var e db.GovExam
	err := q.QueryRowContext(ctx, `SELECT id, name, sort_order FROM gov_exams WHERE id = ?`, id).Scan(&e.ID, &e.Name, &e.SortOrder)
	if err != nil {
		return db.GovExam{}, fmt.Errorf("catalog: get gov exam: %w", err)
	}
	return e, nil
}

// ---- Gov exam subjects (explicit per-exam membership + order) ----

// GetGovExamSubjects returns the global subjects belonging to an exam in
// exam-specific order. The global catalogue rows are shared, never copied.
func GetGovExamSubjects(ctx context.Context, q db.DBTX, examID db.ID) ([]db.GovExamSubject, error) {
	rows, err := q.QueryContext(ctx, `SELECT exam_id, subject_id, sort_order FROM gov_exam_subjects WHERE exam_id = ? ORDER BY sort_order`, examID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list exam subjects: %w", err)
	}
	defer rows.Close()
	var out []db.GovExamSubject
	for rows.Next() {
		var s db.GovExamSubject
		if err := rows.Scan(&s.ExamID, &s.SubjectID, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan exam subject: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func InsertGovExamSubject(ctx context.Context, q db.DBTX, s db.GovExamSubject) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_exam_subjects (exam_id, subject_id, sort_order) VALUES (?, ?, ?)`, s.ExamID, s.SubjectID, s.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert exam subject: %w", err)
	}
	return nil
}

// ---- Gov exam syllabus ----

// GetGovExamSyllabus returns the authoritative (exam, subject, topic) rows
// in syllabus order. Rows seeded without subject_id resolve it from the
// canonical topic.
func GetGovExamSyllabus(ctx context.Context, q db.DBTX, examID db.ID) ([]db.GovExamSyllabus, error) {
	rows, err := q.QueryContext(ctx, `SELECT exam_id, subject_id, topic_id, sort_order FROM gov_exam_syllabus WHERE exam_id = ? ORDER BY sort_order`, examID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list exam syllabus: %w", err)
	}
	defer rows.Close()
	var out []db.GovExamSyllabus
	for rows.Next() {
		var s db.GovExamSyllabus
		var subj sql.NullString
		if err := rows.Scan(&s.ExamID, &subj, &s.TopicID, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan exam syllabus: %w", err)
		}
		if subj.Valid {
			s.SubjectID = db.ID(subj.String)
		} else {
			// Row seeded without subject: resolve from canonical topic.
			tp, err := GetGovTopic(ctx, q, s.TopicID)
			if err != nil {
				return nil, fmt.Errorf("catalog: syllabus topic subject: %w", err)
			}
			s.SubjectID = tp.SubjectID
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: exam syllabus rows: %w", err)
	}
	return out, nil
}

// GetGovExamSyllabusForSubject returns topics for one (exam, subject) in order.
// This is the single authoritative answer to "which topics belong to this
// subject in this exam".
func GetGovExamSyllabusForSubject(ctx context.Context, q db.DBTX, examID, subjectID db.ID) ([]db.GovExamSyllabus, error) {
	syll, err := GetGovExamSyllabus(ctx, q, examID)
	if err != nil {
		return nil, err
	}
	var out []db.GovExamSyllabus
	for _, s := range syll {
		if s.SubjectID == subjectID {
			out = append(out, s)
		}
	}
	return out, nil
}

func InsertGovExamSyllabus(ctx context.Context, q db.DBTX, s db.GovExamSyllabus) error {
	if s.SubjectID == "" {
		// Resolve from canonical topic for callers that only set exam+topic.
		tp, err := GetGovTopic(ctx, q, s.TopicID)
		if err != nil {
			return fmt.Errorf("catalog: insert syllabus topic lookup: %w", err)
		}
		s.SubjectID = tp.SubjectID
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_exam_syllabus (exam_id, subject_id, topic_id, sort_order) VALUES (?, ?, ?, ?)`, s.ExamID, s.SubjectID, s.TopicID, s.SortOrder); err != nil {
		return fmt.Errorf("catalog: insert exam syllabus: %w", err)
	}
	return nil
}

// ---- Gov routines ----

func GetGovLectureRoutine(ctx context.Context, q db.DBTX) ([]db.GovLectureRoutineSlot, error) {
	rows, err := q.QueryContext(ctx, `SELECT slot_order, duration_seconds FROM gov_lecture_routine ORDER BY slot_order`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list lecture routine: %w", err)
	}
	defer rows.Close()
	var out []db.GovLectureRoutineSlot
	for rows.Next() {
		var s db.GovLectureRoutineSlot
		if err := rows.Scan(&s.SlotOrder, &s.DurationSeconds); err != nil {
			return nil, fmt.Errorf("catalog: scan lecture routine: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func InsertGovLectureRoutineSlot(ctx context.Context, q db.DBTX, s db.GovLectureRoutineSlot) error {
	if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (?, ?)`, s.SlotOrder, s.DurationSeconds); err != nil {
		return fmt.Errorf("catalog: insert lecture routine slot: %w", err)
	}
	return nil
}

func GetGovPracticeRoutine(ctx context.Context, q db.DBTX) ([]db.GovPracticeRoutineSlot, error) {
	rows, err := q.QueryContext(ctx, `SELECT slot_order, duration_seconds, selection_mode, fixed_subject_id FROM gov_practice_routine ORDER BY slot_order`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list practice routine: %w", err)
	}
	defer rows.Close()
	var out []db.GovPracticeRoutineSlot
	for rows.Next() {
		var s db.GovPracticeRoutineSlot
		var mode string
		var fixed *string
		if err := rows.Scan(&s.SlotOrder, &s.DurationSeconds, &mode, &fixed); err != nil {
			return nil, fmt.Errorf("catalog: scan practice routine: %w", err)
		}
		s.SelectionMode = db.GovPracticeSelectionMode(mode)
		if fixed != nil {
			id := db.ID(*fixed)
			s.FixedSubjectID = &id
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func InsertGovPracticeRoutineSlot(ctx context.Context, q db.DBTX, s db.GovPracticeRoutineSlot) error {
	var fixed any
	if s.FixedSubjectID != nil {
		fixed = string(*s.FixedSubjectID)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_practice_routine (slot_order, duration_seconds, selection_mode, fixed_subject_id) VALUES (?, ?, ?, ?)`,
		s.SlotOrder, s.DurationSeconds, string(s.SelectionMode), fixed); err != nil {
		return fmt.Errorf("catalog: insert practice routine slot: %w", err)
	}
	return nil
}

// ---- Gov practice test orders and tracks ----

func GetGovPracticeSessionByOrder(ctx context.Context, q db.DBTX, subjectID db.ID, order int) (db.GovPracticeSession, error) {
	var s db.GovPracticeSession
	var subID, sID string
	err := q.QueryRowContext(ctx, `
SELECT id, subject_id, session_code, session_order
FROM gov_practice_sessions
WHERE subject_id = ? AND session_order = ?`, string(subjectID), order).Scan(&sID, &subID, &s.SessionCode, &s.SessionOrder)
	if err != nil {
		return db.GovPracticeSession{}, fmt.Errorf("catalog: get practice session by order: %w", err)
	}
	s.ID = db.ID(sID)
	s.SubjectID = db.ID(subID)
	return s, nil
}

func GetGovPracticeSessionByCode(ctx context.Context, q db.DBTX, subjectID db.ID, code string) (db.GovPracticeSession, error) {
	var s db.GovPracticeSession
	var subID, sID string
	err := q.QueryRowContext(ctx, `
SELECT id, subject_id, session_code, session_order
FROM gov_practice_sessions
WHERE subject_id = ? AND session_code = ?`, string(subjectID), code).Scan(&sID, &subID, &s.SessionCode, &s.SessionOrder)
	if err != nil {
		return db.GovPracticeSession{}, fmt.Errorf("catalog: get practice session by code: %w", err)
	}
	s.ID = db.ID(sID)
	s.SubjectID = db.ID(subID)
	return s, nil
}

func GetGovPracticeSessionCount(ctx context.Context, q db.DBTX, subjectID db.ID) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_sessions WHERE subject_id = ?`, string(subjectID)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("catalog: count practice sessions: %w", err)
	}
	return n, nil
}

func GetGovPracticeTests(ctx context.Context, q db.DBTX, sessionID db.ID) ([]db.GovPracticeTest, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, session_id, subject_id, title, sort_order
FROM gov_practice_tests
WHERE session_id = ?
ORDER BY sort_order`, string(sessionID))
	if err != nil {
		return nil, fmt.Errorf("catalog: list practice tests: %w", err)
	}
	defer rows.Close()
	var out []db.GovPracticeTest
	for rows.Next() {
		var t db.GovPracticeTest
		var tID, sesID, subID string
		if err := rows.Scan(&tID, &sesID, &subID, &t.Title, &t.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan practice test: %w", err)
		}
		t.ID = db.ID(tID)
		t.SessionID = db.ID(sesID)
		t.SubjectID = db.ID(subID)
		out = append(out, t)
	}
	return out, rows.Err()
}

func GetGovPracticeTest(ctx context.Context, q db.DBTX, testID db.ID) (db.GovPracticeTest, error) {
	var t db.GovPracticeTest
	var tID, sesID, subID string
	err := q.QueryRowContext(ctx, `
SELECT id, session_id, subject_id, title, sort_order
FROM gov_practice_tests
WHERE id = ?`, string(testID)).Scan(&tID, &sesID, &subID, &t.Title, &t.SortOrder)
	if err != nil {
		return db.GovPracticeTest{}, fmt.Errorf("catalog: get practice test: %w", err)
	}
	t.ID = db.ID(tID)
	t.SessionID = db.ID(sesID)
	t.SubjectID = db.ID(subID)
	return t, nil
}

func GetGovPracticeTrack(ctx context.Context, q db.DBTX, subjectID db.ID) (db.GovPracticeTrack, error) {
	var tr db.GovPracticeTrack
	var subID string
	var updated int64
	err := q.QueryRowContext(ctx, `
SELECT subject_id, current_session_order, updated_at
FROM gov_practice_tracks
WHERE subject_id = ?`, string(subjectID)).Scan(&subID, &tr.CurrentSessionOrder, &updated)
	if err != nil {
		return db.GovPracticeTrack{}, fmt.Errorf("catalog: get practice track: %w", err)
	}
	tr.SubjectID = db.ID(subID)
	tr.UpdatedAt = time.Unix(updated, 0).UTC()
	return tr, nil
}

func SetGovPracticeTrack(ctx context.Context, q db.DBTX, tr db.GovPracticeTrack) error {
	_, err := q.ExecContext(ctx, `
INSERT INTO gov_practice_tracks (subject_id, current_session_order, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(subject_id) DO UPDATE SET current_session_order = excluded.current_session_order, updated_at = excluded.updated_at`,
		string(tr.SubjectID), tr.CurrentSessionOrder, tr.UpdatedAt.Unix())
	if err != nil {
		return fmt.Errorf("catalog: set practice track: %w", err)
	}
	return nil
}

func AdvanceGovPracticeTrack(ctx context.Context, q db.DBTX, subjectID db.ID, completedOrder int, now time.Time) error {
	total, err := GetGovPracticeSessionCount(ctx, q, subjectID)
	if err != nil {
		return err
	}
	next := completedOrder + 1
	if total > 0 && next > total {
		next = 1
	}
	track, err := GetGovPracticeTrack(ctx, q, subjectID)
	if err != nil {
		// Insert if missing
		return SetGovPracticeTrack(ctx, q, db.GovPracticeTrack{
			SubjectID:           subjectID,
			CurrentSessionOrder: next,
			UpdatedAt:           now,
		})
	}
	if track.CurrentSessionOrder <= completedOrder {
		track.CurrentSessionOrder = next
		track.UpdatedAt = now
		return SetGovPracticeTrack(ctx, q, track)
	}
	return nil
}

func InsertGovPracticeSession(ctx context.Context, q db.DBTX, s db.GovPracticeSession) error {
	_, err := q.ExecContext(ctx, `
INSERT INTO gov_practice_sessions (id, subject_id, session_code, session_order)
VALUES (?, ?, ?, ?)`, string(s.ID), string(s.SubjectID), s.SessionCode, s.SessionOrder)
	if err != nil {
		return fmt.Errorf("catalog: insert practice session: %w", err)
	}
	return nil
}

func InsertGovPracticeTest(ctx context.Context, q db.DBTX, t db.GovPracticeTest) error {
	_, err := q.ExecContext(ctx, `
INSERT INTO gov_practice_tests (id, session_id, subject_id, title, sort_order)
VALUES (?, ?, ?, ?, ?)`, string(t.ID), string(t.SessionID), string(t.SubjectID), t.Title, t.SortOrder)
	if err != nil {
		return fmt.Errorf("catalog: insert practice test: %w", err)
	}
	return nil
}
