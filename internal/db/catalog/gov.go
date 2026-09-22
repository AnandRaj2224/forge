package catalog

import (
	"context"
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
	var all []db.GovLecture
	func() {
		defer rows.Close()
		for rows.Next() {
			var l db.GovLecture
			if err := rows.Scan(&l.ID, &l.TopicID, &l.Title, &l.SortOrder); err == nil {
				all = append(all, l)
			}
		}
	}()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: scan gov lectures by topics: %w", err)
	}
	for _, l := range all {
		out[l.TopicID] = append(out[l.TopicID], l)
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

// ---- Gov exam syllabus ----

func GetGovExamSyllabus(ctx context.Context, q db.DBTX, examID db.ID) ([]db.GovExamSyllabus, error) {
	rows, err := q.QueryContext(ctx, `SELECT exam_id, topic_id, sort_order FROM gov_exam_syllabus WHERE exam_id = ? ORDER BY sort_order`, examID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list exam syllabus: %w", err)
	}
	defer rows.Close()
	var out []db.GovExamSyllabus
	for rows.Next() {
		var s db.GovExamSyllabus
		if err := rows.Scan(&s.ExamID, &s.TopicID, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("catalog: scan exam syllabus: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func InsertGovExamSyllabus(ctx context.Context, q db.DBTX, s db.GovExamSyllabus) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES (?, ?, ?)`, s.ExamID, s.TopicID, s.SortOrder); err != nil {
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
	if _, err := q.ExecContext(ctx, `INSERT INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (?, ?)`, s.SlotOrder, s.DurationSeconds); err != nil {
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
