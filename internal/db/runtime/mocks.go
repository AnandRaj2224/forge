package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
)

// ---- Mocks: actual completed mock/exam attempts ----

func scanMock(row *sql.Row, m *db.Mock) error {
	var examID, name, mockDate, platform string
	var score, totalScore, rankVal, attempted, totalQ, correct, incorrect, unattempted sql.NullInt64
	var percentile, accuracy, avgSpeed sql.NullFloat64
	var created int64
	var id db.ID
	if err := row.Scan(&id, &examID, &name, &mockDate, &platform,
		&score, &totalScore, &rankVal, &percentile, &accuracy, &avgSpeed,
		&attempted, &totalQ, &correct, &incorrect, &unattempted, &created); err != nil {
		return err
	}
	m.ID = id
	m.ExamID = db.ID(examID)
	m.Name = name
	m.MockDate = mockDate
	m.Platform = platform
	if score.Valid {
		v := int(score.Int64)
		m.Score = &v
	}
	if totalScore.Valid {
		v := int(totalScore.Int64)
		m.TotalScore = &v
	}
	if rankVal.Valid {
		v := int(rankVal.Int64)
		m.RankValue = &v
	}
	if percentile.Valid {
		v := percentile.Float64
		m.Percentile = &v
	}
	if accuracy.Valid {
		v := accuracy.Float64
		m.Accuracy = &v
	}
	if avgSpeed.Valid {
		v := avgSpeed.Float64
		m.AvgSpeedSeconds = &v
	}
	if attempted.Valid {
		v := int(attempted.Int64)
		m.Attempted = &v
	}
	if totalQ.Valid {
		v := int(totalQ.Int64)
		m.TotalQuestions = &v
	}
	if correct.Valid {
		v := int(correct.Int64)
		m.Correct = &v
	}
	if incorrect.Valid {
		v := int(incorrect.Int64)
		m.Incorrect = &v
	}
	if unattempted.Valid {
		v := int(unattempted.Int64)
		m.Unattempted = &v
	}
	m.CreatedAt = time.Unix(created, 0).UTC()
	return nil
}

func scanMockRows(rows *sql.Rows) ([]db.Mock, error) {
	var out []db.Mock
	for rows.Next() {
		var m db.Mock
		var examID, name, mockDate, platform string
		var score, totalScore, rankVal, attempted, totalQ, correct, incorrect, unattempted sql.NullInt64
		var percentile, accuracy, avgSpeed sql.NullFloat64
		var created int64
		var id db.ID
		if err := rows.Scan(&id, &examID, &name, &mockDate, &platform,
			&score, &totalScore, &rankVal, &percentile, &accuracy, &avgSpeed,
			&attempted, &totalQ, &correct, &incorrect, &unattempted, &created); err != nil {
			return nil, fmt.Errorf("runtime: scan mock: %w", err)
		}
		m.ID = id
		m.ExamID = db.ID(examID)
		m.Name = name
		m.MockDate = mockDate
		m.Platform = platform
		if score.Valid {
			v := int(score.Int64)
			m.Score = &v
		}
		if totalScore.Valid {
			v := int(totalScore.Int64)
			m.TotalScore = &v
		}
		if rankVal.Valid {
			v := int(rankVal.Int64)
			m.RankValue = &v
		}
		if percentile.Valid {
			v := percentile.Float64
			m.Percentile = &v
		}
		if accuracy.Valid {
			v := accuracy.Float64
			m.Accuracy = &v
		}
		if avgSpeed.Valid {
			v := avgSpeed.Float64
			m.AvgSpeedSeconds = &v
		}
		if attempted.Valid {
			v := int(attempted.Int64)
			m.Attempted = &v
		}
		if totalQ.Valid {
			v := int(totalQ.Int64)
			m.TotalQuestions = &v
		}
		if correct.Valid {
			v := int(correct.Int64)
			m.Correct = &v
		}
		if incorrect.Valid {
			v := int(incorrect.Int64)
			m.Incorrect = &v
		}
		if unattempted.Valid {
			v := int(unattempted.Int64)
			m.Unattempted = &v
		}
		m.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, m)
	}
	return out, nil
}

const mockCols = `id, exam_id, name, mock_date, platform, score, total_score, rank_value, percentile, accuracy, avg_speed_seconds, attempted, total_questions, correct, incorrect, unattempted, created_at`

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// ValidateMock checks exam existence and date shape before insert.
func ValidateMock(ctx context.Context, q db.DBTX, m db.Mock) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("runtime: mock name required")
	}
	if strings.TrimSpace(string(m.ExamID)) == "" {
		return fmt.Errorf("runtime: mock exam required")
	}
	if _, err := time.Parse("2006-01-02", m.MockDate); err != nil {
		return fmt.Errorf("runtime: mock date must be YYYY-MM-DD")
	}
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_exams WHERE id = ?`, string(m.ExamID)).Scan(&n); err != nil {
		return fmt.Errorf("runtime: check mock exam: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("runtime: unknown exam %q", string(m.ExamID))
	}
	return nil
}

// CreateMock inserts one mock attempt.
func CreateMock(ctx context.Context, q db.DBTX, m db.Mock) error {
	if err := ValidateMock(ctx, q, m); err != nil {
		return err
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO mocks (id, exam_id, name, mock_date, platform, score, total_score, rank_value, percentile, accuracy, avg_speed_seconds, attempted, total_questions, correct, incorrect, unattempted, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(m.ID), string(m.ExamID), m.Name, m.MockDate, m.Platform,
		nullInt(m.Score), nullInt(m.TotalScore), nullInt(m.RankValue),
		nullFloat(m.Percentile), nullFloat(m.Accuracy), nullFloat(m.AvgSpeedSeconds),
		nullInt(m.Attempted), nullInt(m.TotalQuestions), nullInt(m.Correct),
		nullInt(m.Incorrect), nullInt(m.Unattempted), m.CreatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("runtime: create mock: %w", err)
	}
	return nil
}

// ListMocks returns mocks newest-first by mock_date.
func ListMocks(ctx context.Context, q db.DBTX) ([]db.Mock, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+mockCols+` FROM mocks ORDER BY mock_date DESC, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("runtime: list mocks: %w", err)
	}
	defer rows.Close()
	out, err := scanMockRows(rows)
	if err != nil {
		return nil, err
	}
	return out, rows.Err()
}

// GetMock returns one mock.
func GetMock(ctx context.Context, q db.DBTX, id db.ID) (db.Mock, error) {
	var m db.Mock
	row := q.QueryRowContext(ctx, `SELECT `+mockCols+` FROM mocks WHERE id = ?`, string(id))
	if err := scanMock(row, &m); err != nil {
		return db.Mock{}, fmt.Errorf("runtime: get mock: %w", err)
	}
	return m, nil
}

// DeleteMock removes a mock (sections + weak topics cascade).
func DeleteMock(ctx context.Context, q db.DBTX, id db.ID) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM mocks WHERE id = ?`, string(id)); err != nil {
		return fmt.Errorf("runtime: delete mock: %w", err)
	}
	return nil
}

// ---- Mock sections ----

// CreateMockSection inserts one section for a mock. When gov_subject_id
// column exists it is persisted; legacy handles omit it.
func CreateMockSection(ctx context.Context, q db.DBTX, s db.MockSection) error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("runtime: section name required")
	}
	var maxOrder sql.NullInt64
	_ = q.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM mock_sections WHERE mock_id = ?`, string(s.MockID)).Scan(&maxOrder)
	order := s.SortOrder
	if order <= 0 {
		if maxOrder.Valid {
			order = int(maxOrder.Int64) + 1
		} else {
			order = 1
		}
	}
	if hasMockSubjectCol(ctx, q) {
		var subj any
		if s.GovSubjectID != nil {
			subj = string(*s.GovSubjectID)
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO mock_sections (id, mock_id, name, sort_order, gov_subject_id) VALUES (?, ?, ?, ?, ?)`,
			string(s.ID), string(s.MockID), s.Name, order, subj); err != nil {
			return fmt.Errorf("runtime: create mock section: %w", err)
		}
		return nil
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO mock_sections (id, mock_id, name, sort_order) VALUES (?, ?, ?, ?)`,
		string(s.ID), string(s.MockID), s.Name, order); err != nil {
		return fmt.Errorf("runtime: create mock section: %w", err)
	}
	return nil
}

// ListMockSections returns sections for a mock in order.
func ListMockSections(ctx context.Context, q db.DBTX, mockID db.ID) ([]db.MockSection, error) {
	if hasMockSubjectCol(ctx, q) {
		rows, err := q.QueryContext(ctx, `SELECT id, mock_id, name, sort_order, gov_subject_id FROM mock_sections WHERE mock_id = ? ORDER BY sort_order`, string(mockID))
		if err != nil {
			return nil, fmt.Errorf("runtime: list mock sections: %w", err)
		}
		defer rows.Close()
		var out []db.MockSection
		for rows.Next() {
			var s db.MockSection
			var subj sql.NullString
			if err := rows.Scan(&s.ID, &s.MockID, &s.Name, &s.SortOrder, &subj); err != nil {
				return nil, fmt.Errorf("runtime: scan mock section: %w", err)
			}
			if subj.Valid {
				v := db.ID(subj.String)
				s.GovSubjectID = &v
			}
			out = append(out, s)
		}
		return out, rows.Err()
	}
	rows, err := q.QueryContext(ctx, `SELECT id, mock_id, name, sort_order FROM mock_sections WHERE mock_id = ? ORDER BY sort_order`, string(mockID))
	if err != nil {
		return nil, fmt.Errorf("runtime: list mock sections: %w", err)
	}
	defer rows.Close()
	var out []db.MockSection
	for rows.Next() {
		var s db.MockSection
		if err := rows.Scan(&s.ID, &s.MockID, &s.Name, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("runtime: scan mock section: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// hasMockSubjectCol reports whether mock_sections.gov_subject_id exists
// (post-migration 003). Keeps runtime working on pre-migration DBs.
func hasMockSubjectCol(ctx context.Context, q db.DBTX) bool {
	rows, err := q.QueryContext(ctx, `SELECT name FROM pragma_table_info('mock_sections')`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && name == "gov_subject_id" {
			return true
		}
	}
	return false
}

// DeleteMockSection removes a section (weak topics cascade).
func DeleteMockSection(ctx context.Context, q db.DBTX, id db.ID) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM mock_sections WHERE id = ?`, string(id)); err != nil {
		return fmt.Errorf("runtime: delete mock section: %w", err)
	}
	return nil
}

// ---- Weak topics: ordered gov_topic_id references, max 3 per section ----

// ListWeakTopics returns ordered weak topics for a section.
func ListWeakTopics(ctx context.Context, q db.DBTX, sectionID db.ID) ([]db.MockWeakTopic, error) {
	rows, err := q.QueryContext(ctx, `SELECT section_id, topic_id, rank_order FROM mock_weak_topics WHERE section_id = ? ORDER BY rank_order`, string(sectionID))
	if err != nil {
		return nil, fmt.Errorf("runtime: list weak topics: %w", err)
	}
	defer rows.Close()
	var out []db.MockWeakTopic
	for rows.Next() {
		var w db.MockWeakTopic
		if err := rows.Scan(&w.SectionID, &w.TopicID, &w.RankOrder); err != nil {
			return nil, fmt.Errorf("runtime: scan weak topic: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetWeakTopics replaces the ordered weak-topic list for a section.
// topicIDs are ordered: index 0 = strongest weakness (rank 1).
// Enforces: max 3, no duplicates, every topic exists and belongs to the
// mock's exam syllabus (no impossible exam/topic combos).
//
// NOTE: the product path is engine.ReplaceWeakTopicsAtomic, which also
// validates the explicit GOV subject mapping. New product code must use
// the engine; this stays as the low-level validated primitive.
func SetWeakTopics(ctx context.Context, q db.DBTX, mockID, sectionID db.ID, topicIDs []db.ID) error {
	if len(topicIDs) > 3 {
		return fmt.Errorf("runtime: at most 3 weak topics per section")
	}
	seen := map[db.ID]bool{}
	for _, t := range topicIDs {
		if t == "" {
			return fmt.Errorf("runtime: empty topic id")
		}
		if seen[t] {
			return fmt.Errorf("runtime: duplicate weak topic %q", string(t))
		}
		seen[t] = true
	}
	// Resolve the mock's exam.
	var examID string
	if err := q.QueryRowContext(ctx, `SELECT exam_id FROM mocks WHERE id = ?`, string(mockID)).Scan(&examID); err != nil {
		return fmt.Errorf("runtime: weak topics mock lookup: %w", err)
	}
	// Verify the section belongs to the mock.
	var cnt int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM mock_sections WHERE id = ? AND mock_id = ?`, string(sectionID), string(mockID)).Scan(&cnt); err != nil {
		return fmt.Errorf("runtime: weak topics section lookup: %w", err)
	}
	if cnt == 0 {
		return fmt.Errorf("runtime: section does not belong to mock")
	}
	// Verify every topic exists and is mapped to the exam.
	for _, tid := range topicIDs {
		var tc int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_topics WHERE id = ?`, string(tid)).Scan(&tc); err != nil {
			return fmt.Errorf("runtime: check weak topic: %w", err)
		}
		if tc == 0 {
			return fmt.Errorf("runtime: unknown topic %q", string(tid))
		}
		var mc int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_exam_syllabus WHERE exam_id = ? AND topic_id = ?`, examID, string(tid)).Scan(&mc); err != nil {
			return fmt.Errorf("runtime: check exam/topic map: %w", err)
		}
		if mc == 0 {
			return fmt.Errorf("runtime: topic %q is not in this exam's syllabus", string(tid))
		}
	}
	// Replace atomically.
	if _, err := q.ExecContext(ctx, `DELETE FROM mock_weak_topics WHERE section_id = ?`, string(sectionID)); err != nil {
		return fmt.Errorf("runtime: clear weak topics: %w", err)
	}
	for i, tid := range topicIDs {
		if _, err := q.ExecContext(ctx, `INSERT INTO mock_weak_topics (section_id, topic_id, rank_order) VALUES (?, ?, ?)`,
			string(sectionID), string(tid), i+1); err != nil {
			return fmt.Errorf("runtime: insert weak topic: %w", err)
		}
	}
	return nil
}

// ExamTopicChoices returns global gov topics belonging to an exam, ordered
// by subject then topic sort, for the weak-topic selector.
func ExamTopicChoices(ctx context.Context, q db.DBTX, examID db.ID) ([]db.GovTopic, error) {
	rows, err := q.QueryContext(ctx, `
SELECT t.id, t.subject_id, t.name, t.sort_order
FROM gov_exam_syllabus x
JOIN gov_topics t ON t.id = x.topic_id
JOIN gov_subjects s ON s.id = t.subject_id
WHERE x.exam_id = ?
ORDER BY s.sort_order, t.sort_order`, string(examID))
	if err != nil {
		return nil, fmt.Errorf("runtime: exam topic choices: %w", err)
	}
	defer rows.Close()
	var out []db.GovTopic
	for rows.Next() {
		var t db.GovTopic
		if err := rows.Scan(&t.ID, &t.SubjectID, &t.Name, &t.SortOrder); err != nil {
			return nil, fmt.Errorf("runtime: scan exam topic: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
