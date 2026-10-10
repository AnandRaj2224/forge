package catalog

import (
	"context"
	"fmt"
	"time"

	db "forge/internal/db"
)

// ---- Exam profiles (V2 explicit lifecycle, independent of completion) ----

func GetGovExamProfiles(ctx context.Context, q db.DBTX) ([]db.GovExamProfile, error) {
	rows, err := q.QueryContext(ctx, `SELECT exam_id, status, is_main_goal, priority, target_date, updated_at FROM gov_exam_profiles ORDER BY priority, exam_id`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list exam profiles: %w", err)
	}
	defer rows.Close()
	var out []db.GovExamProfile
	for rows.Next() {
		var p db.GovExamProfile
		var main int
		var target *string
		var updated int64
		var examID string
		var status string
		if err := rows.Scan(&examID, &status, &main, &p.Priority, &target, &updated); err != nil {
			return nil, fmt.Errorf("catalog: scan exam profile: %w", err)
		}
		p.ExamID = db.ID(examID)
		p.Status = status
		p.IsMainGoal = main == 1
		p.TargetDate = target
		p.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: exam profile rows: %w", err)
	}
	return out, nil
}

func GetGovExamProfile(ctx context.Context, q db.DBTX, examID db.ID) (db.GovExamProfile, error) {
	var p db.GovExamProfile
	var main int
	var updated int64
	var examStr, status string
	err := q.QueryRowContext(ctx, `SELECT exam_id, status, is_main_goal, priority, target_date, updated_at FROM gov_exam_profiles WHERE exam_id = ?`, string(examID)).Scan(
		&examStr, &status, &main, &p.Priority, &p.TargetDate, &updated,
	)
	if err != nil {
		return db.GovExamProfile{}, fmt.Errorf("catalog: get exam profile: %w", err)
	}
	p.ExamID = db.ID(examStr)
	p.Status = status
	p.IsMainGoal = main == 1
	p.UpdatedAt = time.Unix(updated, 0).UTC()
	return p, nil
}

func UpsertGovExamProfile(ctx context.Context, q db.DBTX, p db.GovExamProfile) error {
	main := 0
	if p.IsMainGoal {
		main = 1
	}
	var target any
	if p.TargetDate != nil {
		target = *p.TargetDate
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO gov_exam_profiles (exam_id, status, is_main_goal, priority, target_date, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(exam_id) DO UPDATE SET status=excluded.status, is_main_goal=excluded.is_main_goal,
  priority=excluded.priority, target_date=excluded.target_date, updated_at=excluded.updated_at`,
		string(p.ExamID), p.Status, main, p.Priority, target, p.UpdatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("catalog: upsert exam profile: %w", err)
	}
	return nil
}

// ---- Focus state (singleton id=1) ----

func GetGovFocusState(ctx context.Context, q db.DBTX) (db.GovFocusState, error) {
	var s db.GovFocusState
	var focused *string
	var updated int64
	err := q.QueryRowContext(ctx, `SELECT id, focused_exam_id, updated_at FROM gov_focus_state WHERE id = 1`).Scan(&s.ID, &focused, &updated)
	if err != nil {
		return db.GovFocusState{}, fmt.Errorf("catalog: get focus state: %w", err)
	}
	if focused != nil {
		v := db.ID(*focused)
		s.FocusedExamID = &v
	}
	s.UpdatedAt = time.Unix(updated, 0).UTC()
	return s, nil
}

func SetGovFocusState(ctx context.Context, q db.DBTX, s db.GovFocusState) error {
	if s.ID == 0 {
		s.ID = 1
	}
	var focused any
	if s.FocusedExamID != nil {
		focused = string(*s.FocusedExamID)
	}
	if _, err := q.ExecContext(ctx, `
INSERT INTO gov_focus_state (id, focused_exam_id, updated_at) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET focused_exam_id=excluded.focused_exam_id, updated_at=excluded.updated_at`,
		s.ID, focused, s.UpdatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("catalog: set focus state: %w", err)
	}
	return nil
}

// ---- Practice rotation (per exam+subject cursor) ----

func GetGovPracticeRotation(ctx context.Context, q db.DBTX, examID, subjectID db.ID) (db.GovPracticeRotation, error) {
	var r db.GovPracticeRotation
	var updated int64
	var examStr, subjStr string
	err := q.QueryRowContext(ctx, `SELECT exam_id, subject_id, next_index, updated_at FROM gov_practice_rotation WHERE exam_id = ? AND subject_id = ?`,
		string(examID), string(subjectID)).Scan(&examStr, &subjStr, &r.NextIndex, &updated)
	if err != nil {
		return db.GovPracticeRotation{}, fmt.Errorf("catalog: get practice rotation: %w", err)
	}
	r.ExamID = db.ID(examStr)
	r.SubjectID = db.ID(subjStr)
	r.UpdatedAt = time.Unix(updated, 0).UTC()
	return r, nil
}

func SetGovPracticeRotation(ctx context.Context, q db.DBTX, r db.GovPracticeRotation) error {
	if _, err := q.ExecContext(ctx, `
INSERT INTO gov_practice_rotation (exam_id, subject_id, next_index, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT(exam_id, subject_id) DO UPDATE SET next_index=excluded.next_index, updated_at=excluded.updated_at`,
		string(r.ExamID), string(r.SubjectID), r.NextIndex, r.UpdatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("catalog: set practice rotation: %w", err)
	}
	return nil
}
