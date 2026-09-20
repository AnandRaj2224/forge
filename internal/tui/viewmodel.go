package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// TaskStatus distinguishes presentation state of a Today task.
type TaskStatus string

const (
	TaskTodo      TaskStatus = "TODO"
	TaskActive    TaskStatus = "ACTIVE"
	TaskCompleted TaskStatus = "COMPLETED"
)

// FormatDuration renders seconds compactly: 3600->"1h", 5400->"1h30", 9000->"2h30".
func FormatDuration(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%02d", h, m)
}

// FormatHMS renders elapsed like 00:42:17.
func FormatHMS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
}

// FormatHM renders planned like 01:00:00 or 2h30 for headers.
func FormatHM(secs int) string { return FormatDuration(secs) }

// Bundle describes the concrete work items inside a Today task,
// resolved from target_json via the typed DB target structs.
type Bundle struct {
	Kind     string // LECTURE, PRACTICE, DSA, SWE
	Lecture  *LectureItem
	Practice []PracticeItem
	DSA      []DSAItem
	SWE      []SWEItem
	RawNote  string // non-empty when target_json missing/unparseable
}

type LectureItem struct {
	SubjectID db.ID
	TopicID   db.ID
	LectureID db.ID
	Subject   string
	Topic     string
	Lecture   string
}

type PracticeItem struct {
	TopicID db.ID
	Subject string
	Topic   string
}

type DSAItem struct {
	ProblemID db.ID
	Mode      db.DSAMode
	Title     string
	Pattern   string
}

type SWEItem struct {
	AreaID db.ID
	TaskID *db.ID
	Area   string
	Task   string
}

// ResolveBundle parses a Today task's target_json and resolves names via catalog.
// It never invents items: N items out reflect exactly N references in the target.
func ResolveBundle(ctx context.Context, q db.DBTX, t db.TodayTask) Bundle {
	var b Bundle
	b.Kind = string(t.Kind)
	if t.TargetJSON == nil || strings.TrimSpace(*t.TargetJSON) == "" {
		b.RawNote = "no target assigned"
		return b
	}
	raw := *t.TargetJSON
	switch t.Domain {
	case db.DomainGOV:
		if t.Kind == db.SessionKindLecture {
			lt, err := db.UnmarshalGovLectureTarget(raw)
			if err != nil {
				b.RawNote = "invalid lecture target"
				return b
			}
			li := LectureItem{SubjectID: lt.SubjectID, TopicID: lt.TopicID, LectureID: lt.LectureID}
			if s, err := catalog.GetGovSubject(ctx, q, lt.SubjectID); err == nil {
				li.Subject = s.Name
			} else {
				li.Subject = string(lt.SubjectID)
			}
			if tp, err := catalog.GetGovTopic(ctx, q, lt.TopicID); err == nil {
				li.Topic = tp.Name
			} else {
				li.Topic = string(lt.TopicID)
			}
			li.Lecture = string(lt.LectureID)
			if lecs, err := catalog.GetGovLectures(ctx, q, lt.TopicID); err == nil {
				for _, l := range lecs {
					if l.ID == lt.LectureID {
						li.Lecture = l.Title
					}
				}
			}
			b.Lecture = &li
			return b
		}
		pt, err := db.UnmarshalGovPracticeTarget(raw)
		if err != nil {
			b.RawNote = "invalid practice target"
			return b
		}
		subjName := string(pt.SubjectID)
		if s, err := catalog.GetGovSubject(ctx, q, pt.SubjectID); err == nil {
			subjName = s.Name
		}
		for _, tid := range pt.TopicIDs {
			pi := PracticeItem{TopicID: tid, Subject: subjName}
			if tp, err := catalog.GetGovTopic(ctx, q, tid); err == nil {
				pi.Topic = tp.Name
			} else {
				pi.Topic = string(tid)
			}
			b.Practice = append(b.Practice, pi)
		}
		return b
	case db.DomainDSA:
		dt, err := db.UnmarshalDSATarget(raw)
		if err != nil {
			b.RawNote = "invalid DSA target"
			return b
		}
		for _, item := range dt.Items() {
			di := DSAItem{ProblemID: item.ProblemID, Mode: item.Mode}
			if p, err := catalog.GetDSAProblem(ctx, q, item.ProblemID); err == nil {
				di.Title, di.Pattern = p.Title, p.Pattern
			} else {
				di.Title = string(item.ProblemID)
			}
			b.DSA = append(b.DSA, di)
		}
		if len(b.DSA) == 0 {
			b.RawNote = "empty DSA target"
		}
		return b
	case db.DomainSWE:
		st, err := db.UnmarshalSWETarget(raw)
		if err != nil {
			b.RawNote = "invalid SWE target"
			return b
		}
		areas, _ := catalog.GetSWEAreas(ctx, q)
		areaName := func(id db.ID) string {
			for _, a := range areas {
				if a.ID == id {
					return a.Name
				}
			}
			return string(id)
		}
		for _, item := range st.Items() {
			si := SWEItem{AreaID: item.AreaID, TaskID: item.TaskID, Area: areaName(item.AreaID)}
			if item.TaskID != nil {
				tasks, _ := catalog.GetSWETasks(ctx, q, item.AreaID)
				for _, tk := range tasks {
					if tk.ID == *item.TaskID {
						si.Task = tk.Title
					}
				}
				if si.Task == "" {
					si.Task = string(*item.TaskID)
				}
			}
			b.SWE = append(b.SWE, si)
		}
		if len(b.SWE) == 0 {
			b.RawNote = "empty SWE target"
		}
		return b
	}
	b.RawNote = "unknown domain"
	return b
}

// ResolveSessionBundle parses a session's target snapshot the same way,
// falling back to the Today task bundle when the snapshot is absent.
func ResolveSessionBundle(ctx context.Context, q db.DBTX, s db.Session, task db.TodayTask) Bundle {
	if s.TargetSnapshotJSON != nil && strings.TrimSpace(*s.TargetSnapshotJSON) != "" {
		proxy := db.TodayTask{ID: task.ID, Domain: s.Domain, Kind: s.Kind, TargetJSON: s.TargetSnapshotJSON}
		return ResolveBundle(ctx, q, proxy)
	}
	return ResolveBundle(ctx, q, task)
}

// ComputeTaskStatus derives TODO/ACTIVE/COMPLETED for a Today task.
// ACTIVE wins; COMPLETED requires stopped-session evidence; else TODO.
// Stopping without evidence stays TODO (stopping is not completion).
func ComputeTaskStatus(ctx context.Context, q db.DBTX, taskID db.ID) TaskStatus {
	sessList, err := runtime.ListSessions(ctx, q)
	if err != nil {
		return TaskTodo
	}
	var hasActive, hasStopped bool
	var stoppedIDs []db.ID
	for _, s := range sessList {
		if s.TodayTaskID != taskID {
			continue
		}
		if s.Status == db.SessionStatusActive {
			hasActive = true
		} else {
			hasStopped = true
			stoppedIDs = append(stoppedIDs, s.ID)
		}
	}
	if hasActive {
		return TaskActive
	}
	if !hasStopped {
		return TaskTodo
	}
	for _, sid := range stoppedIDs {
		if sessionHasEvidence(ctx, q, sid) {
			return TaskCompleted
		}
	}
	return TaskTodo
}

func sessionHasEvidence(ctx context.Context, q db.DBTX, sid db.ID) bool {
	if pr, err := runtime.GetGovPracticeRecordsBySession(ctx, q, sid); err == nil && len(pr) > 0 {
		return true
	}
	if da, err := runtime.GetDSAAttemptsBySession(ctx, q, sid); err == nil && len(da) > 0 {
		return true
	}
	if se, err := runtime.GetSWEExecutionsBySession(ctx, q, sid); err == nil && len(se) > 0 {
		return true
	}
	// Lecture completions are keyed by lecture; scan via sessions list is not
	// directly supported, so check the completions table for this session.
	rows, err := q.QueryContext(ctx, `SELECT COUNT(*) FROM gov_lecture_completions WHERE session_id = ?`, string(sid))
	if err != nil {
		return false
	}
	defer rows.Close()
	var n int
	if rows.Next() && rows.Scan(&n) == nil && n > 0 {
		return true
	}
	return false
}

// ExamCoverage is pure syllabus (lecture) coverage: topics mapped to the exam
// where every lecture is complete (zero lectures counts as complete).
type SubjectCoverage struct {
	SubjectID db.ID
	Subject   string
	Done      int
	Total     int
}

type ExamCoverage struct {
	ExamID   db.ID
	Exam     string
	Done     int
	Total    int
	Subjects []SubjectCoverage
}

func (e ExamCoverage) Percent() float64 {
	if e.Total == 0 {
		return 0
	}
	return float64(e.Done) / float64(e.Total)
}

// ComputeExamCoverage aggregates only existing DB rows; no invented formulas.
func ComputeExamCoverage(ctx context.Context, q db.DBTX, examID db.ID) (ExamCoverage, error) {
	exams, err := catalog.GetGovExams(ctx, q)
	if err != nil {
		return ExamCoverage{}, err
	}
	var cov ExamCoverage
	cov.ExamID = examID
	for _, e := range exams {
		if e.ID == examID {
			cov.Exam = e.Name
		}
	}
	syll, err := catalog.GetGovExamSyllabus(ctx, q, examID)
	if err != nil {
		return ExamCoverage{}, err
	}
	cov.Total = len(syll)
	bySubject := map[db.ID]*SubjectCoverage{}
	order := []db.ID{}
	for _, s := range syll {
		tp, err := catalog.GetGovTopic(ctx, q, s.TopicID)
		if err != nil {
			continue
		}
		sc, ok := bySubject[tp.SubjectID]
		if !ok {
			name := string(tp.SubjectID)
			if subj, err := catalog.GetGovSubject(ctx, q, tp.SubjectID); err == nil {
				name = subj.Name
			}
			sc = &SubjectCoverage{SubjectID: tp.SubjectID, Subject: name}
			bySubject[tp.SubjectID] = sc
			order = append(order, tp.SubjectID)
		}
		sc.Total++
		if topicLectureComplete(ctx, q, s.TopicID) {
			cov.Done++
			sc.Done++
		}
	}
	for _, id := range order {
		cov.Subjects = append(cov.Subjects, *bySubject[id])
	}
	return cov, nil
}

func topicLectureComplete(ctx context.Context, q db.DBTX, topicID db.ID) bool {
	lecs, err := catalog.GetGovLectures(ctx, q, topicID)
	if err != nil || len(lecs) == 0 {
		return true // zero lectures: track satisfied
	}
	for _, l := range lecs {
		ok, err := runtime.IsLectureComplete(ctx, q, l.ID)
		if err != nil || !ok {
			return false
		}
	}
	return true
}

// TopicLectureProgress returns done/total plus per-lecture done flags.
func TopicLectureProgress(ctx context.Context, q db.DBTX, topicID db.ID) (done, total int, flags []bool, lectures []db.GovLecture) {
	lecs, err := catalog.GetGovLectures(ctx, q, topicID)
	if err != nil {
		return 0, 0, nil, nil
	}
	total = len(lecs)
	for _, l := range lecs {
		ok, _ := runtime.IsLectureComplete(ctx, q, l.ID)
		flags = append(flags, ok)
		if ok {
			done++
		}
	}
	return done, total, flags, lecs
}

// TopicPracticeStats aggregates raw practice rows; no progression formula.
type TopicPracticeStats struct {
	Rows      int
	Attempted int
	Correct   int
	Incorrect int
	Accuracy  float64 // 0 when no attempts
	AvgSpeed  float64 // mean of non-null speeds, 0 when none
	HasSpeed  bool
}

func ComputeTopicPracticeStats(ctx context.Context, q db.DBTX, topicID db.ID) TopicPracticeStats {
	hist, err := runtime.GetGovPracticeHistory(ctx, q, topicID)
	if err != nil {
		return TopicPracticeStats{}
	}
	var st TopicPracticeStats
	var speedSum float64
	var speedN int
	for _, r := range hist {
		st.Rows++
		st.Attempted += r.Attempted
		st.Correct += r.Correct
		if r.AvgQuestionSpeedSeconds != nil {
			speedSum += *r.AvgQuestionSpeedSeconds
			speedN++
		}
	}
	st.Incorrect = st.Attempted - st.Correct
	if st.Attempted > 0 {
		st.Accuracy = float64(st.Correct) / float64(st.Attempted)
	}
	if speedN > 0 {
		st.AvgSpeed = speedSum / float64(speedN)
		st.HasSpeed = true
	}
	return st
}

// ExamsForTopic returns exam names mapping a global topic (derived grouping).
func ExamsForTopic(ctx context.Context, q db.DBTX, topicID db.ID) []string {
	exams, err := catalog.GetGovExams(ctx, q)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range exams {
		syll, err := catalog.GetGovExamSyllabus(ctx, q, e.ID)
		if err != nil {
			continue
		}
		for _, s := range syll {
			if s.TopicID == topicID {
				out = append(out, e.Name)
				break
			}
		}
	}
	return out
}

// DSAPattern groups problems preserving catalogue order.
type DSAPattern struct {
	Name     string
	Problems []db.DSAProblem
}

func GroupDSAPatterns(problems []db.DSAProblem) []DSAPattern {
	idx := map[string]int{}
	var out []DSAPattern
	for _, p := range problems {
		if i, ok := idx[p.Pattern]; ok {
			out[i].Problems = append(out[i].Problems, p)
		} else {
			idx[p.Pattern] = len(out)
			out = append(out, DSAPattern{Name: p.Pattern, Problems: []db.DSAProblem{p}})
		}
	}
	return out
}

// DSAPassUI reports whether any attempt passes (independent&&optimal&&!hint).
// This is a display aggregation over raw evidence, not the future engine.
func DSAPassUI(attempts []db.DSAAttempt) bool {
	for _, a := range attempts {
		if a.Independent && a.Optimal && !a.HintUsed {
			return true
		}
	}
	return false
}

func LastAttempt(attempts []db.DSAAttempt) *db.DSAAttempt {
	if len(attempts) == 0 {
		return nil
	}
	best := attempts[0]
	for _, a := range attempts[1:] {
		if !a.AttemptedAt.Before(best.AttemptedAt) {
			best = a
		}
	}
	return &best
}

// SWEAreaProgress: done = tasks with a COMPLETED execution (derived rule).
type SWEAreaProgress struct {
	Area  db.SWEArea
	Total int
	Done  int
}

func ComputeSWEProgress(ctx context.Context, q db.DBTX, area db.SWEArea) SWEAreaProgress {
	tasks, err := catalog.GetSWETasks(ctx, q, area.ID)
	if err != nil {
		return SWEAreaProgress{Area: area}
	}
	pr := SWEAreaProgress{Area: area, Total: len(tasks)}
	for _, t := range tasks {
		if done, _ := runtime.IsSWETaskComplete(ctx, q, t.ID); done {
			pr.Done++
		}
	}
	return pr
}

// ---- Form validation (pure, unit-tested) ----

// ValidatePracticeRow validates one practice topic row of text inputs.
func ValidatePracticeRow(timeS, attS, corrS, avgS string) (tSecs, att, corr int, avg *float64, errMsg string) {
	tSecs, errMsg = parseNonNegInt("time", timeS)
	if errMsg != "" {
		return
	}
	att, errMsg = parseNonNegInt("attempted", attS)
	if errMsg != "" {
		return
	}
	corr, errMsg = parseNonNegInt("correct", corrS)
	if errMsg != "" {
		return
	}
	if corr > att {
		return 0, 0, 0, nil, "correct > attempted"
	}
	avgS = strings.TrimSpace(avgS)
	if avgS != "" {
		v, err := strconv.ParseFloat(avgS, 64)
		if err != nil || v <= 0 {
			return 0, 0, 0, nil, "avg speed must be > 0"
		}
		avg = &v
	}
	return tSecs, att, corr, avg, ""
}

func parseNonNegInt(field, s string) (int, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, field + " required"
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 0, field + " must be integer >= 0"
	}
	return v, ""
}

// ValidateSolveTime validates optional solve-time seconds (required by DSA form).
func ValidateSolveTime(s string) (int, *int, string) {
	v, msg := parseNonNegInt("solve time", s)
	if msg != "" {
		return 0, nil, msg
	}
	return v, &v, ""
}

// DecideDSAAttemptMode picks NEW only when the target says NEW and no NEW
// attempt exists yet (preserves the partial-unique invariant); else REPEAT.
func DecideDSAAttemptMode(existing []db.DSAAttempt, target db.DSAMode) db.DSAMode {
	if target == db.DSAModeNew {
		for _, a := range existing {
			if a.Mode == db.DSAModeNew {
				return db.DSAModeRepeat
			}
		}
		return db.DSAModeNew
	}
	return db.DSAModeRepeat
}

// SortReviews sorts weekly reviews ascending by week id.
func SortReviews(rs []db.WeeklyReview) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
}
