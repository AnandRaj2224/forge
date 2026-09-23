package tui

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
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

// FormatDurationLong renders seconds as product metrics: 3600->"1h",
// 9300->"1h 35m", 300->"5m", 90->"1m 30s", 42->"42s". Sub-hour values keep
// second precision (1m 30s, not 1m); hour values truncate to minutes.
// Actual and planned stay visually distinct.
func FormatDurationLong(secs int) string {
	if secs <= 0 {
		if secs < 0 {
			secs = 0
		}
		if secs < 60 {
			return fmt.Sprintf("%ds", secs)
		}
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h == 0 {
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %02dm", h, m)
}

// ParseDurationSeconds parses human duration strings into seconds without
// changing the stored meaning (seconds). Accepts:
//   - plain integers as seconds: "300" -> 300 (back-compat)
//   - duration strings: "30s", "5m", "1h", "1h 15m", "1h15m", "1m 30s"
//
// Units are h/m/s, case-insensitive, spaces optional. Returns seconds or
// an error message fragment.
func ParseDurationSeconds(s string) (int, string) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, "required"
	}
	low := strings.ToLower(t)
	// Plain integer (optional surrounding spaces) = seconds for back-compat.
	if v, err := strconv.Atoi(strings.TrimSpace(low)); err == nil {
		if v < 0 {
			return 0, "must be >= 0"
		}
		return v, ""
	}
	// Duration tokens: sequence of <number><unit>.
	nospace := strings.ReplaceAll(low, " ", "")
	if nospace == "" {
		return 0, "required"
	}
	total := 0
	i := 0
	seen := false
	for i < len(nospace) {
		j := i
		for j < len(nospace) && nospace[j] >= '0' && nospace[j] <= '9' {
			j++
		}
		if j == i {
			return 0, "use like 30s, 5m, 1h 15m or seconds"
		}
		num, err := strconv.Atoi(nospace[i:j])
		if err != nil {
			return 0, "use like 30s, 5m, 1h 15m or seconds"
		}
		if j >= len(nospace) {
			return 0, "use like 30s, 5m, 1h 15m or seconds"
		}
		unit := nospace[j]
		switch unit {
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		default:
			return 0, "use like 30s, 5m, 1h 15m or seconds"
		}
		seen = true
		i = j + 1
	}
	if !seen {
		return 0, "use like 30s, 5m, 1h 15m or seconds"
	}
	return total, ""
}

// FormatDate renders UI dates as 21 Sep 2026 (numeric day, 3-letter
// month, 4-digit year). Storage formats are unchanged; this is display.
func FormatDate(t time.Time) string { return t.Format("02 Jan 2006") }

// FormatDateISO parses a stored YYYY-MM-DD date for display and falls
// back to the raw input when it does not parse.
func FormatDateISO(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return FormatDate(t)
	}
	return s
}

// FormatDateRange renders 24 Aug 2026 — 30 Aug 2026 for week ranges.
func FormatDateRange(a, b string) string { return FormatDateISO(a) + " — " + FormatDateISO(b) }

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
	Area   string
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
		if st.AreaID == "" {
			b.RawNote = "empty SWE target"
			return b
		}
		areaName := string(st.AreaID)
		if a, err := catalog.GetSWEArea(ctx, q, st.AreaID); err == nil {
			areaName = a.Name
		}
		b.SWE = append(b.SWE, SWEItem{AreaID: st.AreaID, Area: areaName})
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

// ComputeTaskStatus derives TODO/ACTIVE/COMPLETED for a Today task via the
// authoritative engine evaluator (exact bundle completion, never
// any-evidence). It is kept for test compatibility; the live Today workspace
// uses ComputeTaskStatusE so DB failures surface as data errors instead of
// silent TODO fallbacks.
func ComputeTaskStatus(ctx context.Context, q db.DBTX, taskID db.ID) TaskStatus {
	st, _ := ComputeTaskStatusE(ctx, q, taskID)
	return st
}

// ComputeTaskStatusE is the error-propagating evaluator used by live UI:
// DB failures return an error (never a default business state).
func ComputeTaskStatusE(ctx context.Context, q db.DBTX, taskID db.ID) (TaskStatus, error) {
	ev, err := engine.EvaluateTodayTask(ctx, q, taskID)
	if err != nil {
		return TaskTodo, err
	}
	switch ev.State {
	case engine.StateCompleted:
		return TaskCompleted, nil
	case engine.StateActive:
		return TaskActive, nil
	default:
		return TaskTodo, nil
	}
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
// Lecture completion is resolved in batch: one lectures query for all
// syllabus topics plus one completion-map query, then progress is computed
// in memory (no per-lecture completion polling).
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
	// Topic subjects: one lookup per syllabus topic (bounded, syllabus-sized).
	type topicInfo struct {
		subjectID db.ID
	}
	topicSubject := make(map[db.ID]db.ID, len(syll))
	var topicIDs []db.ID
	for _, s := range syll {
		tp, err := catalog.GetGovTopic(ctx, q, s.TopicID)
		if err != nil {
			continue
		}
		topicSubject[s.TopicID] = tp.SubjectID
		topicIDs = append(topicIDs, s.TopicID)
	}
	// Batch lectures + batch completions (2 queries total for progress).
	lecturesByTopic, err := catalog.GetGovLecturesByTopics(ctx, q, topicIDs)
	if err != nil {
		return ExamCoverage{}, err
	}
	var allLectureIDs []db.ID
	for _, tids := range lecturesByTopic {
		for _, l := range tids {
			allLectureIDs = append(allLectureIDs, l.ID)
		}
	}
	compMap, err := runtime.GetLectureCompletionMap(ctx, q, allLectureIDs)
	if err != nil {
		return ExamCoverage{}, err
	}
	bySubject := map[db.ID]*SubjectCoverage{}
	order := []db.ID{}
	for _, s := range syll {
		subjID, ok := topicSubject[s.TopicID]
		if !ok {
			continue
		}
		sc, ok := bySubject[subjID]
		if !ok {
			name := string(subjID)
			if subj, err := catalog.GetGovSubject(ctx, q, subjID); err == nil {
				name = subj.Name
			}
			sc = &SubjectCoverage{SubjectID: subjID, Subject: name}
			bySubject[subjID] = sc
			order = append(order, subjID)
		}
		sc.Total++
		lecs := lecturesByTopic[s.TopicID]
		done := true
		if len(lecs) > 0 {
			for _, l := range lecs {
				if !compMap[l.ID] {
					done = false
					break
				}
			}
		} else {
			done = true // zero lectures: track satisfied
		}
		if done {
			cov.Done++
			sc.Done++
		}
	}
	_ = topicInfo{}
	// Order subjects by the exam's own subject order when membership rows
	// exist; otherwise keep syllabus first-appearance order (legacy display
	// for exams without membership).
	if members, err := catalog.GetGovExamSubjects(ctx, q, examID); err == nil && len(members) > 0 {
		rank := make(map[db.ID]int, len(members))
		for i, m := range members {
			rank[m.SubjectID] = i
		}
		sort.SliceStable(order, func(i, j int) bool {
			ri, iok := rank[order[i]]
			rj, jok := rank[order[j]]
			if iok && jok {
				return ri < rj
			}
			if iok {
				return true
			}
			if jok {
				return false
			}
			return false
		})
	}
	for _, id := range order {
		cov.Subjects = append(cov.Subjects, *bySubject[id])
	}
	return cov, nil
}

// topicLectureCompleteE reports whether every lecture of a topic is
// complete (zero lectures counts as complete). DB failures propagate;
// callers must surface them as data errors instead of defaulting.
// Implemented via one lectures query + one completion-map query.
func topicLectureCompleteE(ctx context.Context, q db.DBTX, topicID db.ID) (bool, error) {
	lecs, err := catalog.GetGovLectures(ctx, q, topicID)
	if err != nil {
		return false, err
	}
	if len(lecs) == 0 {
		return true, nil // zero lectures: track satisfied
	}
	ids := make([]db.ID, 0, len(lecs))
	for _, l := range lecs {
		ids = append(ids, l.ID)
	}
	compMap, err := runtime.GetLectureCompletionMap(ctx, q, ids)
	if err != nil {
		return false, err
	}
	for _, l := range lecs {
		if !compMap[l.ID] {
			return false, nil
		}
	}
	return true, nil
}

// TopicLectureProgressE returns done/total plus per-lecture done flags.
// DB failures propagate; callers must surface them as data errors.
// Implemented via one lectures query + one completion-map query.
func TopicLectureProgressE(ctx context.Context, q db.DBTX, topicID db.ID) (done, total int, flags []bool, lectures []db.GovLecture, err error) {
	lecs, err := catalog.GetGovLectures(ctx, q, topicID)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	total = len(lecs)
	if total == 0 {
		return 0, 0, nil, lecs, nil
	}
	ids := make([]db.ID, 0, len(lecs))
	for _, l := range lecs {
		ids = append(ids, l.ID)
	}
	compMap, err := runtime.GetLectureCompletionMap(ctx, q, ids)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	for _, l := range lecs {
		ok := compMap[l.ID]
		flags = append(flags, ok)
		if ok {
			done++
		}
	}
	return done, total, flags, lecs, nil
}

// TopicLectureProgressMap returns done/total for a bounded set of topics in
// batch: one lectures query + one completion query, progress computed in
// memory. Used by GOV reload paths to avoid per-topic N+1 polling.
func TopicLectureProgressMap(ctx context.Context, q db.DBTX, topicIDs []db.ID) (map[db.ID]struct {
	Done  int
	Total int
}, error) {
	out := make(map[db.ID]struct {
		Done  int
		Total int
	}, len(topicIDs))
	for _, id := range topicIDs {
		out[id] = struct {
			Done  int
			Total int
		}{}
	}
	if len(topicIDs) == 0 {
		return out, nil
	}
	lecturesByTopic, err := catalog.GetGovLecturesByTopics(ctx, q, topicIDs)
	if err != nil {
		return nil, err
	}
	var allIDs []db.ID
	for _, lecs := range lecturesByTopic {
		for _, l := range lecs {
			allIDs = append(allIDs, l.ID)
		}
	}
	compMap, err := runtime.GetLectureCompletionMap(ctx, q, allIDs)
	if err != nil {
		return nil, err
	}
	for _, tid := range topicIDs {
		lecs := lecturesByTopic[tid]
		if len(lecs) == 0 {
			out[tid] = struct {
				Done  int
				Total int
			}{Done: 0, Total: 0}
			continue
		}
		done := 0
		for _, l := range lecs {
			if compMap[l.ID] {
				done++
			}
		}
		out[tid] = struct {
			Done  int
			Total int
		}{Done: done, Total: len(lecs)}
	}
	return out, nil
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

func ComputeTopicPracticeStatsE(ctx context.Context, q db.DBTX, topicID db.ID) (TopicPracticeStats, error) {
	hist, err := runtime.GetGovPracticeHistory(ctx, q, topicID)
	if err != nil {
		return TopicPracticeStats{}, err
	}
	var st TopicPracticeStats
	var weightedSum float64
	var weightedN int
	for _, r := range hist {
		st.Rows++
		st.Attempted += r.Attempted
		st.Correct += r.Correct
		if r.AvgQuestionSpeedSeconds != nil {
			weightedSum += *r.AvgQuestionSpeedSeconds * float64(r.Attempted)
			weightedN += r.Attempted
		}
	}
	st.Incorrect = st.Attempted - st.Correct
	if st.Attempted > 0 {
		st.Accuracy = float64(st.Correct) / float64(st.Attempted)
	}
	if weightedN > 0 {
		st.AvgSpeed = weightedSum / float64(weightedN)
		st.HasSpeed = true
	}
	return st, nil
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

// DSAPassUI reports whether any attempt passes via the authoritative engine
// rule (independent AND optimal AND NOT hint_used).
func DSAPassUI(attempts []db.DSAAttempt) bool { return engine.AnyPass(attempts) }

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

// ---- Form validation (pure, unit-tested) ----

// ValidatePracticeRow validates one practice topic row of text inputs.
// Time accepts plain seconds ("1380") or durations ("23m", "20m 30s").
func ValidatePracticeRow(timeS, attS, corrS, avgS string) (tSecs, att, corr int, avg *float64, errMsg string) {
	if v, msg := ParseDurationSeconds(timeS); msg != "" {
		if msg == "required" {
			return 0, 0, 0, nil, "time required"
		}
		return 0, 0, 0, nil, "time " + msg
	} else {
		tSecs = v
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
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return 0, 0, 0, nil, "average speed must be > 0"
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

// ValidateSolveTime validates solve-time input (required by DSA form).
// Accepts plain seconds ("300") or durations ("5m", "30s", "1h 15m").
// Stored meaning stays seconds.
func ValidateSolveTime(s string) (int, *int, string) {
	v, msg := ParseDurationSeconds(s)
	if msg != "" {
		if msg == "required" {
			return 0, nil, "solve time required"
		}
		return 0, nil, "solve time must be like 30s, 5m, 1h 15m or seconds"
	}
	return v, &v, ""
}

// DecideDSAAttemptMode is legacy.
//
// Deprecated: non-authoritative. It silently converts a NEW target into
// REPEAT when a NEW attempt already exists. The engine never converts:
// engine.SaveDSAEvidence surfaces a stale target (ErrInvalidTarget)
// instead. Kept only for the existing unit test; product code must not
// decide attempt modes outside the engine.
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
