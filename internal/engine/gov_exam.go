package engine

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// Exam-wise GOV progression.
//
// The global catalogue (subjects -> topics -> lectures) is the source of
// truth for learning objects; completion state is global (a lecture
// completed under one exam stays complete under every later exam that maps
// the same topic). The EXAM ORDER (gov_exams.sort_order) is the source of
// truth for which preparation track is currently worked through:
//
//	exams ordered by sort_order
//	    -> first exam whose mapped topics are not all lecture-complete
//	    -> CURRENT EXAM (derived, never stored)
//
// Within the current exam, its own subject order (gov_exam_subjects)
// determines the active subjects: the first three incomplete subjects feed
// the three 1-hour GOV lecture slots. A subject is incomplete while any of
// the exam's mapped topics under it is not lecture-complete; topics mapped
// only to other (later) exams never affect the current exam.
//
// There is no separate mutable "current exam" state and no exam-specific
// copies of subjects, topics, or lectures.

// GovExamSubjectProgress is the per-exam progress of one member subject:
// Done/Total count the exam's mapped topics under the subject that are
// lecture-complete (a topic with zero lectures counts as complete).
type GovExamSubjectProgress struct {
	Subject  db.GovSubject
	Done     int
	Total    int
	Complete bool
}

// maxGovLectureSubjects is the number of current/incomplete subjects the
// Today GOV lecture routine serves: 3 slots x 1 hour.
const maxGovLectureSubjects = 3

// IsGovTopicLectureComplete reports whether every lecture under a topic is
// complete. A topic with zero lectures counts as complete. DB failures
// propagate, never default.
func IsGovTopicLectureComplete(ctx context.Context, q db.DBTX, topicID db.ID) (bool, error) {
	lecs, err := catalog.GetGovLectures(ctx, q, topicID)
	if err != nil {
		return false, fmt.Errorf("engine: gov lectures: %w", err)
	}
	if len(lecs) == 0 {
		return true, nil
	}
	ids := make([]db.ID, 0, len(lecs))
	for _, l := range lecs {
		ids = append(ids, l.ID)
	}
	compMap, err := runtime.GetLectureCompletionMap(ctx, q, ids)
	if err != nil {
		return false, fmt.Errorf("engine: lecture completion map: %w", err)
	}
	for _, l := range lecs {
		if !compMap[l.ID] {
			return false, nil
		}
	}
	return true, nil
}

// examTopicCompletionMap resolves lecture-completeness for a bounded set of
// topics in batch (one lectures query + one completion query). Missing keys
// never occur: every input topic gets an entry (zero lectures counts as
// complete).
func examTopicCompletionMap(ctx context.Context, q db.DBTX, topicIDs []db.ID) (map[db.ID]bool, error) {
	out := make(map[db.ID]bool, len(topicIDs))
	for _, id := range topicIDs {
		out[id] = true
	}
	if len(topicIDs) == 0 {
		return out, nil
	}
	lecturesByTopic, err := catalog.GetGovLecturesByTopics(ctx, q, topicIDs)
	if err != nil {
		return nil, fmt.Errorf("engine: gov lectures by topics: %w", err)
	}
	var allIDs []db.ID
	for _, lecs := range lecturesByTopic {
		for _, l := range lecs {
			allIDs = append(allIDs, l.ID)
		}
	}
	compMap, err := runtime.GetLectureCompletionMap(ctx, q, allIDs)
	if err != nil {
		return nil, fmt.Errorf("engine: lecture completion map: %w", err)
	}
	for _, tid := range topicIDs {
		done := true
		for _, l := range lecturesByTopic[tid] {
			if !compMap[l.ID] {
				done = false
				break
			}
		}
		out[tid] = done
	}
	return out, nil
}

// examSyllabusTopicSet returns the topic IDs mapped to an exam.
func examSyllabusTopicSet(ctx context.Context, q db.DBTX, examID db.ID) (map[db.ID]bool, []db.ID, error) {
	syll, err := catalog.GetGovExamSyllabus(ctx, q, examID)
	if err != nil {
		return nil, nil, fmt.Errorf("engine: exam syllabus: %w", err)
	}
	set := make(map[db.ID]bool, len(syll))
	ids := make([]db.ID, 0, len(syll))
	for _, s := range syll {
		if !set[s.TopicID] {
			set[s.TopicID] = true
			ids = append(ids, s.TopicID)
		}
	}
	return set, ids, nil
}

// IsGovExamComplete reports whether every global topic mapped to the exam
// is lecture-complete (zero-lecture topics count as complete). GOV practice
// completion is not required for exam progression. An exam with no mapped
// topics is vacuously complete. DB failures propagate.
func IsGovExamComplete(ctx context.Context, q db.DBTX, examID db.ID) (bool, error) {
	_, ids, err := examSyllabusTopicSet(ctx, q, examID)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return true, nil
	}
	compMap, err := examTopicCompletionMap(ctx, q, ids)
	if err != nil {
		return false, err
	}
	for _, tid := range ids {
		if !compMap[tid] {
			return false, nil
		}
	}
	return true, nil
}

// CurrentGovExam derives the current GOV exam: the first incomplete exam in
// sort_order. When every exam is complete (or no exams exist) it returns
// (nil, nil): GOV lecture/practice planning must then produce no exam work
// rather than inventing work. A later-added exam with incomplete topics
// naturally becomes current. DB failures propagate.
func CurrentGovExam(ctx context.Context, q db.DBTX) (*db.GovExam, error) {
	exams, err := catalog.GetGovExams(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("engine: gov exams: %w", err)
	}
	for _, e := range exams {
		done, err := IsGovExamComplete(ctx, q, e.ID)
		if err != nil {
			return nil, err
		}
		if !done {
			c := e
			return &c, nil
		}
	}
	return nil, nil
}

// ExamSubjectProgress returns per-subject progress for an exam in the exam's
// own subject order. Only topics mapped to the exam count (in each
// subject's global topic order); global topics mapped solely to other exams
// never affect this exam's subjects. Subjects with no mapped topics are
// vacuously complete. DB failures propagate.
func ExamSubjectProgress(ctx context.Context, q db.DBTX, examID db.ID) ([]GovExamSubjectProgress, error) {
	members, err := catalog.GetGovExamSubjects(ctx, q, examID)
	if err != nil {
		return nil, fmt.Errorf("engine: exam subjects: %w", err)
	}
	if len(members) == 0 {
		return nil, nil
	}
	inExam, _, err := examSyllabusTopicSet(ctx, q, examID)
	if err != nil {
		return nil, err
	}
	type subjTopics struct {
		subj   db.GovSubject
		topics []db.ID
	}
	ordered := make([]subjTopics, 0, len(members))
	var allTopicIDs []db.ID
	for _, m := range members {
		subj, err := catalog.GetGovSubject(ctx, q, m.SubjectID)
		if err != nil {
			if isNotFoundErr(err) {
				return nil, fmt.Errorf("engine: exam subject %q: %w", string(m.SubjectID), wrapErr("exam subjects", ErrInvalidTarget, "unknown subject"))
			}
			return nil, fmt.Errorf("engine: gov subject lookup: %w", err)
		}
		topics, err := catalog.GetGovTopics(ctx, q, m.SubjectID)
		if err != nil {
			return nil, fmt.Errorf("engine: gov topics: %w", err)
		}
		st := subjTopics{subj: subj}
		for _, tp := range topics {
			if !inExam[tp.ID] {
				continue
			}
			st.topics = append(st.topics, tp.ID)
			allTopicIDs = append(allTopicIDs, tp.ID)
		}
		ordered = append(ordered, st)
	}
	compMap, err := examTopicCompletionMap(ctx, q, allTopicIDs)
	if err != nil {
		return nil, err
	}
	out := make([]GovExamSubjectProgress, 0, len(ordered))
	for _, st := range ordered {
		p := GovExamSubjectProgress{Subject: st.subj, Total: len(st.topics), Complete: true}
		for _, tid := range st.topics {
			if compMap[tid] {
				p.Done++
			} else {
				p.Complete = false
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// CurrentGovSubjects derives the current exam plus its first up-to-three
// incomplete subjects in exam subject order. Completing a subject replaces
// it with the next incomplete subject; no slot state is stored. Returns
// (nil, nil, nil) when no exam is current. DB failures propagate.
func CurrentGovSubjects(ctx context.Context, q db.DBTX) (*db.GovExam, []GovExamSubjectProgress, error) {
	cur, err := CurrentGovExam(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	if cur == nil {
		return nil, nil, nil
	}
	progress, err := ExamSubjectProgress(ctx, q, cur.ID)
	if err != nil {
		return nil, nil, err
	}
	var out []GovExamSubjectProgress
	for _, p := range progress {
		if p.Complete {
			continue
		}
		out = append(out, p)
		if len(out) >= maxGovLectureSubjects {
			break
		}
	}
	return cur, out, nil
}

// firstIncompleteExamLecture returns the first incomplete topic (in the
// subject's global topic order, restricted to topics mapped to the exam)
// and its first incomplete lecture. It returns (nil, nil, nil) when the
// subject has no incomplete exam-mapped topic. DB failures propagate.
func firstIncompleteExamLecture(ctx context.Context, q db.DBTX, examID, subjectID db.ID) (*db.GovTopic, *db.GovLecture, error) {
	inExam, _, err := examSyllabusTopicSet(ctx, q, examID)
	if err != nil {
		return nil, nil, err
	}
	topics, err := catalog.GetGovTopics(ctx, q, subjectID)
	if err != nil {
		return nil, nil, fmt.Errorf("engine: gov topics: %w", err)
	}
	var candidates []db.GovTopic
	var candidateIDs []db.ID
	for _, tp := range topics {
		if !inExam[tp.ID] {
			continue
		}
		candidates = append(candidates, tp)
		candidateIDs = append(candidateIDs, tp.ID)
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	compMap, err := examTopicCompletionMap(ctx, q, candidateIDs)
	if err != nil {
		return nil, nil, err
	}
	for _, tp := range candidates {
		if compMap[tp.ID] {
			continue
		}
		lecs, err := catalog.GetGovLectures(ctx, q, tp.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: gov lectures: %w", err)
		}
		if len(lecs) == 0 {
			continue
		}
		ids := make([]db.ID, 0, len(lecs))
		for _, l := range lecs {
			ids = append(ids, l.ID)
		}
		lecDone, err := runtime.GetLectureCompletionMap(ctx, q, ids)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: lecture completion map: %w", err)
		}
		for _, l := range lecs {
			if !lecDone[l.ID] {
				tc := tp
				lc := l
				return &tc, &lc, nil
			}
		}
	}
	return nil, nil, nil
}
