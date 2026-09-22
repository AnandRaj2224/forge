package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// TaskActivity is one actual Today task from the selected day.
type TaskActivity struct {
	Position   int
	TaskID     db.ID
	Domain     string
	Kind       string
	Title      string
	Detail     string // bundle detail: topics/problems/tasks
	Planned    int
	Actual     int
	State      TaskState
}

// GovDailyEvidence answers what GOV produced that day.
type GovDailyEvidence struct {
	LecturesCompleted  int
	QuestionsAttempted int
	QuestionsCorrect   int
	QuestionsIncorrect int
	Accuracy           *float64 // nil when nothing attempted
	AvgSpeed           *float64 // weighted, nil when no speed data
	HasSpeed           bool
}

// DSADailyEvidence answers DSA outcomes.
type DSADailyEvidence struct {
	Attempted int
	Passed    int
	NotPassed int
}

// SWEDailyEvidence answers SWE outcomes.
type SWEDailyEvidence struct {
	Completed int
	Partial   int
	Blocked   int
}

// DailyReport is the structured "what actually happened on this day" DTO.
// The engine generates it; the UI only renders it.
type DailyReport struct {
	Date           string // YYYY-MM-DD
	Planned        int
	Actual         int
	Variance       int
	TasksTotal     int
	TasksCompleted int
	Sessions       int
	GovSeconds     int
	DSASeconds     int
	SWESeconds     int
	Activities     []TaskActivity
	Gov            GovDailyEvidence
	DSA            DSADailyEvidence
	SWE            SWEDailyEvidence
	Unfinished     []TaskActivity
}

// BuildDailyReport generates the report for one Forge local date.
func BuildDailyReport(ctx context.Context, q db.DBTX, localDate string) (DailyReport, error) {
	cal := DefaultCalendar()
	if _, err := cal.StartOfDay(localDate); err != nil {
		return DailyReport{}, wrapErr("daily report", ErrValidation, "bad date")
	}
	rep := DailyReport{Date: localDate}
	dayStart, dayEnd, err := cal.DayBounds(localDate)
	if err != nil {
		return DailyReport{}, err
	}
	// Planned + tasks from LATEST snapshot for the date (never all revisions).
	// A genuine no-row result means "no plan that day"; a real DB failure
	// propagates instead of masquerading as an empty day.
	snap, err := runtime.GetLatestTodaySnapshot(ctx, q, localDate)
	hasSnap := err == nil
	if err != nil && !isNotFoundErr(err) {
		return DailyReport{}, fmt.Errorf("engine: latest today snapshot: %w", err)
	}
	var tasks []db.TodayTask
	if hasSnap {
		tasks, err = runtime.GetTodayTasks(ctx, q, snap.ID)
		if err != nil {
			return DailyReport{}, fmt.Errorf("engine: today tasks: %w", err)
		}
		for _, t := range tasks {
			rep.Planned += t.PlannedSeconds
		}
		rep.TasksTotal = len(tasks)
	}
	// Sessions: split actual elapsed at local day boundaries.
	allSessions, err := runtime.ListSessions(ctx, q)
	if err != nil {
		return DailyReport{}, fmt.Errorf("engine: list sessions: %w", err)
	}
	actualByTask := map[db.ID]int{}
	for _, s := range allSessions {
		if s.Status != db.SessionStatusStopped || s.StoppedAt == nil {
			continue
		}
		parts := cal.SplitIntervalAtDayBoundaries(s.StartedAt.Unix(), s.StoppedAt.Unix())
		if secs, ok := parts[localDate]; ok {
			rep.Actual += int(secs)
			switch s.Domain {
			case db.DomainGOV:
				rep.GovSeconds += int(secs)
			case db.DomainDSA:
				rep.DSASeconds += int(secs)
			case db.DomainSWE:
				rep.SWESeconds += int(secs)
			}
			// Task-level actual respects the same split: a session
			// spanning midnight attributes each portion to its own day.
			actualByTask[s.TodayTaskID] += int(secs)
		}
		// Count sessions touching this day (started or stopped within, or spanning).
		if s.StartedAt.Unix() < dayEnd && s.StoppedAt.Unix() > dayStart {
			rep.Sessions++
		}
	}
	rep.Variance = rep.Actual - rep.Planned
	// Task activities from the day's snapshot evaluated as of end of day:
	// evidence recorded later must not rewrite this day's history.
	for i, t := range tasks {
		ev, err := EvaluateTodayTaskAt(ctx, q, t.ID, dayEnd)
		if err != nil {
			return DailyReport{}, err
		}
		detail := taskDetail(ctx, q, t)
		act := TaskActivity{
			Position: i + 1, TaskID: t.ID, Domain: string(t.Domain), Kind: string(t.Kind),
			Title: t.Title, Detail: detail, Planned: t.PlannedSeconds,
			Actual: actualByTask[t.ID], State: ev.State,
		}
		rep.Activities = append(rep.Activities, act)
		if ev.State == StateCompleted {
			rep.TasksCompleted++
		} else {
			rep.Unfinished = append(rep.Unfinished, act)
		}
	}
	// Domain evidence for the day (recorded_at within day bounds).
	if err := fillDailyGov(ctx, q, &rep, dayStart, dayEnd); err != nil {
		return DailyReport{}, err
	}
	if err := fillDailyDSA(ctx, q, &rep, dayStart, dayEnd); err != nil {
		return DailyReport{}, err
	}
	if err := fillDailySWE(ctx, q, &rep, dayStart, dayEnd); err != nil {
		return DailyReport{}, err
	}
	return rep, nil
}

func taskDetail(ctx context.Context, q db.DBTX, t db.TodayTask) string {
	if t.TargetJSON == nil {
		return t.Title
	}
	switch t.Domain {
	case db.DomainGOV:
		if t.Kind == db.SessionKindLecture {
			lt, err := db.UnmarshalGovLectureTarget(*t.TargetJSON)
			if err != nil {
				return t.Title
			}
			tp, err := catalog.GetGovTopic(ctx, q, lt.TopicID)
			if err != nil {
				return t.Title
			}
			return tp.Name
		}
		pt, err := db.UnmarshalGovPracticeTarget(*t.TargetJSON)
		if err != nil {
			return t.Title
		}
		var names []string
		for _, tid := range pt.TopicIDs {
			if tp, err := catalog.GetGovTopic(ctx, q, tid); err == nil {
				names = append(names, tp.Name)
			}
		}
		return joinPlus(names)
	case db.DomainDSA:
		dt, err := db.UnmarshalDSATarget(*t.TargetJSON)
		if err != nil {
			return t.Title
		}
		var names []string
		for _, it := range dt.Items() {
			if p, err := catalog.GetDSAProblem(ctx, q, it.ProblemID); err == nil {
				names = append(names, p.Title)
			}
		}
		return joinPlus(names)
	case db.DomainSWE:
		st, err := db.UnmarshalSWETarget(*t.TargetJSON)
		if err != nil {
			return t.Title
		}
		var names []string
		for _, it := range st.Items() {
			if it.TaskID != nil {
				tasks, err := catalog.GetSWETasks(ctx, q, it.AreaID)
				if err != nil {
					// Read-only enrichment only: a catalogue failure
					// degrades this detail line to the title rather than
					// failing the whole report. Domain state is decided
					// by the evaluator, never here.
					continue
				}
				for _, tk := range tasks {
					if tk.ID == *it.TaskID {
						names = append(names, tk.Title)
					}
				}
			}
		}
		return joinPlus(names)
	}
	return t.Title
}

func fillDailyGov(ctx context.Context, q db.DBTX, rep *DailyReport, lo, hi int64) error {
	// Lectures completed that day.
	rows, err := q.QueryContext(ctx, `SELECT completed_at FROM gov_lecture_completions WHERE completed_at >= ? AND completed_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: lecture completions: %w", err)
	}
	n := 0
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			rows.Close()
			return fmt.Errorf("engine: scan completion: %w", err)
		}
		n++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("engine: lecture rows: %w", err)
	}
	rep.Gov.LecturesCompleted = n
	// Practice with weighted speed.
	prows, err := q.QueryContext(ctx, `SELECT attempted, correct, avg_question_speed_seconds FROM gov_practice_records WHERE recorded_at >= ? AND recorded_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: practice records: %w", err)
	}
	defer prows.Close()
	var att, corr int
	var weightedSum float64
	var weightedN int
	for prows.Next() {
		var a, c int
		var av sql.NullFloat64
		if err := prows.Scan(&a, &c, &av); err != nil {
			return fmt.Errorf("engine: scan practice: %w", err)
		}
		att += a
		corr += c
		if av.Valid {
			weightedSum += av.Float64 * float64(a)
			weightedN += a
		}
	}
	if err := prows.Err(); err != nil {
		return fmt.Errorf("engine: practice rows: %w", err)
	}
	rep.Gov.QuestionsAttempted = att
	rep.Gov.QuestionsCorrect = corr
	rep.Gov.QuestionsIncorrect = att - corr
	if att > 0 {
		acc := float64(corr) / float64(att)
		rep.Gov.Accuracy = &acc
	}
	if weightedN > 0 {
		v := weightedSum / float64(weightedN)
		rep.Gov.AvgSpeed = &v
		rep.Gov.HasSpeed = true
	}
	return nil
}

// sqlNullFloat mirrors sql.NullFloat64 for report scanning.
type sqlNullFloat = sql.NullFloat64

func fillDailyDSA(ctx context.Context, q db.DBTX, rep *DailyReport, lo, hi int64) error {
	rows, err := q.QueryContext(ctx, `SELECT independent, optimal, hint_used FROM dsa_attempts WHERE attempted_at >= ? AND attempted_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: dsa attempts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var indep, opt, hint int
		if err := rows.Scan(&indep, &opt, &hint); err != nil {
			return fmt.Errorf("engine: scan dsa: %w", err)
		}
		rep.DSA.Attempted++
		// Single authoritative pass rule (never a duplicated condition).
		if IsDSAPass(db.DSAAttempt{Independent: indep == 1, Optimal: opt == 1, HintUsed: hint == 1}) {
			rep.DSA.Passed++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("engine: dsa rows: %w", err)
	}
	rep.DSA.NotPassed = rep.DSA.Attempted - rep.DSA.Passed
	if rep.DSA.NotPassed < 0 {
		rep.DSA.NotPassed = 0
	}
	return nil
}

func fillDailySWE(ctx context.Context, q db.DBTX, rep *DailyReport, lo, hi int64) error {
	rows, err := q.QueryContext(ctx, `SELECT status FROM swe_executions WHERE recorded_at >= ? AND recorded_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: swe executions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			return fmt.Errorf("engine: scan swe: %w", err)
		}
		switch st {
		case "COMPLETED":
			rep.SWE.Completed++
		case "PARTIAL":
			rep.SWE.Partial++
		case "BLOCKED":
			rep.SWE.Blocked++
		}
	}
	return rows.Err()
}

// WeightedAvgSpeed computes sum(avg*attempted)/sum(attempted) for overall
// question-speed metrics. Never simple-average averages.
func WeightedAvgSpeed(rows []struct {
	Attempted int
	AvgSpeed  *float64
}) *float64 {
	var sum float64
	var n int
	for _, r := range rows {
		if r.AvgSpeed != nil && r.Attempted > 0 {
			sum += *r.AvgSpeed * float64(r.Attempted)
			n += r.Attempted
		}
	}
	if n == 0 {
		return nil
	}
	v := sum / float64(n)
	return &v
}

// ---- weekly ----

// GovSubjectWeekly is one subject slice of weekly GOV performance.
type GovSubjectWeekly struct {
	SubjectID db.ID
	Subject   string
	Lectures  int
	Attempted int
	Correct   int
	Incorrect int
	Accuracy  *float64
	AvgSpeed  *float64
}

// WeeklyReport is the structured weekly snapshot DTO. Stored reviews persist
// the complete snapshot; drafts are generated live by the engine.
type WeeklyReport struct {
	WeekID         string // YYYY-Www
	WeekStart      string
	WeekEnd        string
	Planned        int
	Actual         int
	Variance       int
	CompletionPct  *float64 // nil when nothing planned
	TasksTotal     int
	TasksCompleted int
	Sessions       int
	GovSeconds     int
	DSASeconds     int
	SWESeconds     int
	GovSubjects    []GovSubjectWeekly
	GovLectures    int
	GovAttempted   int
	GovCorrect     int
	GovAccuracy    *float64
	DSAAttempted   int
	DSAPassed      int
	DSANotPassed   int
	SWECompleted   int
	SWEPartial     int
	SWEBlocked     int
}

// BuildWeeklyDraft generates a live draft for [weekStart, weekEnd] using the
// latest snapshot per day for planned totals and actual session intervals.
//
// Historical semantics: task completion is evaluated as of the week-end
// boundary (exclusive), so work completed after the week never rewrites the
// week's completion. Evidence inclusion stays bounded by recorded timestamps
// in range. Session counts use interval overlap: any stopped session whose
// [started, stopped) interval overlaps the week counts, with its actual time
// split at local day boundaries into the week.
func BuildWeeklyDraft(ctx context.Context, q db.DBTX, weekID, weekStart, weekEnd string) (WeeklyReport, error) {
	rep := WeeklyReport{WeekID: weekID, WeekStart: weekStart, WeekEnd: weekEnd}
	cal := DefaultCalendar()
	start, err := cal.StartOfDay(weekStart)
	if err != nil {
		return WeeklyReport{}, wrapErr("weekly", ErrValidation, "bad week start")
	}
	end, err := cal.StartOfDay(weekEnd)
	if err != nil {
		return WeeklyReport{}, wrapErr("weekly", ErrValidation, "bad week end")
	}
	// Exclusive week-end cutoff (start of the day after weekEnd). Task state
	// and evidence strictly before this bound belong to the week; anything
	// at or after it does not.
	weekEndCutoff := end.AddDate(0, 0, 1).Unix()
	// Planned: latest snapshot per day in range. A genuine no-row result
	// means "no plan that day" (skip); a real DB failure propagates.
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		snap, err := runtime.GetLatestTodaySnapshot(ctx, q, ds)
		if err != nil {
			if isNotFoundErr(err) {
				continue
			}
			return WeeklyReport{}, fmt.Errorf("engine: weekly snapshot: %w", err)
		}
		tasks, err := runtime.GetTodayTasks(ctx, q, snap.ID)
		if err != nil {
			return WeeklyReport{}, fmt.Errorf("engine: weekly tasks: %w", err)
		}
		for _, t := range tasks {
			rep.Planned += t.PlannedSeconds
			rep.TasksTotal++
		}
		// Completion via authoritative evaluation as of the week-end
		// cutoff (never today's state).
		for _, t := range tasks {
			ev, err := EvaluateTodayTaskAt(ctx, q, t.ID, weekEndCutoff)
			if err != nil {
				return WeeklyReport{}, err
			}
			if ev.State == StateCompleted {
				rep.TasksCompleted++
			}
		}
	}
	if rep.TasksTotal > 0 {
		p := float64(rep.TasksCompleted) / float64(rep.TasksTotal) * 100
		rep.CompletionPct = &p
	}
	// Actual: sessions overlapping the week, split at day boundaries within week.
	allSessions, err := runtime.ListSessions(ctx, q)
	if err != nil {
		return WeeklyReport{}, fmt.Errorf("engine: sessions: %w", err)
	}
	lo := start.Unix()
	hi := end.AddDate(0, 0, 1).Unix()
	for _, s := range allSessions {
		if s.Status != db.SessionStatusStopped || s.StoppedAt == nil {
			continue
		}
		if s.StartedAt.Unix() >= hi || s.StoppedAt.Unix() <= lo {
			continue
		}
		parts := cal.SplitIntervalAtDayBoundaries(s.StartedAt.Unix(), s.StoppedAt.Unix())
		for date, secs := range parts {
			if date < weekStart || date > weekEnd {
				continue
			}
			rep.Actual += int(secs)
			switch s.Domain {
			case db.DomainGOV:
				rep.GovSeconds += int(secs)
			case db.DomainDSA:
				rep.DSASeconds += int(secs)
			case db.DomainSWE:
				rep.SWESeconds += int(secs)
			}
		}
		// Session counts when its time interval overlaps the week (same
		// overlap semantics as the actual-time split above): a session
		// spanning the week boundary counts for the week it touches.
		if s.StartedAt.Unix() < hi && s.StoppedAt.Unix() > lo {
			rep.Sessions++
		}
	}
	rep.Variance = rep.Actual - rep.Planned
	// Evidence in range.
	loB, hiB := lo, hi
	if err := fillWeeklyEvidence(ctx, q, &rep, loB, hiB); err != nil {
		return WeeklyReport{}, err
	}
	return rep, nil
}

func fillWeeklyEvidence(ctx context.Context, q db.DBTX, rep *WeeklyReport, lo, hi int64) error {
	// Gov lectures.
	rows, err := q.QueryContext(ctx, `SELECT completed_at FROM gov_lecture_completions WHERE completed_at >= ? AND completed_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: weekly lectures: %w", err)
	}
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			rows.Close()
			return fmt.Errorf("engine: scan: %w", err)
		}
		rep.GovLectures++
	}
	rows.Close()
	// Gov practice + per-subject. NOTE: single-conn pool deadlocks if we
	// query inside an open rows loop, so collect first, close, then map.
	prows, err := q.QueryContext(ctx, `SELECT topic_id, attempted, correct, avg_question_speed_seconds FROM gov_practice_records WHERE recorded_at >= ? AND recorded_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: weekly practice: %w", err)
	}
	type prow struct {
		topicID string
		att     int
		corr    int
		avg     sql.NullFloat64
	}
	var collected []prow
	var scanErr error
	func() {
		defer prows.Close()
		for prows.Next() {
			var topicID string
			var a, c int
			var av sql.NullFloat64
			if err := prows.Scan(&topicID, &a, &c, &av); err != nil {
				scanErr = fmt.Errorf("engine: scan practice: %w", err)
				return
			}
			collected = append(collected, prow{topicID: topicID, att: a, corr: c, avg: av})
		}
	}()
	if scanErr != nil {
		return scanErr
	}
	if err := prows.Err(); err != nil {
		return fmt.Errorf("engine: practice rows: %w", err)
	}
	type agg struct {
		att, corr int
		wsum      float64
		wn        int
	}
	bySubject := map[string]*agg{}
	subjectOf := map[string]db.ID{}
	var totalWsum float64
	var totalWn int
	_ = subjectOf
	for _, pr := range collected {
		a, c, av := pr.att, pr.corr, pr.avg
		topicID := pr.topicID
		rep.GovAttempted += a
		rep.GovCorrect += c
		if av.Valid {
			totalWsum += av.Float64 * float64(a)
			totalWn += a
		}
		sid, err := topicSubject(ctx, q, topicID)
		if err != nil {
			return fmt.Errorf("engine: topic subject: %w", err)
		}
		subjectOf[topicID] = sid
		k := string(sid)
		ag, ok := bySubject[k]
		if !ok {
			ag = &agg{}
			bySubject[k] = ag
		}
		ag.att += a
		ag.corr += c
		if av.Valid {
			ag.wsum += av.Float64 * float64(a)
			ag.wn += a
		}
	}
	if rep.GovAttempted > 0 {
		v := float64(rep.GovCorrect) / float64(rep.GovAttempted)
		rep.GovAccuracy = &v
	}
	// Per-subject lectures + names in global order. Every lookup failure
	// propagates: a weekly report is either complete or a data error,
	// never a partially correct-looking snapshot with quietly dropped
	// lectures.
	subjects, err := catalog.GetGovSubjects(ctx, q)
	if err != nil {
		return fmt.Errorf("engine: subjects: %w", err)
	}
	lecBySubject := map[string]int{}
	if err := collectWeeklyLectureSubjects(ctx, q, lo, hi, lecBySubject); err != nil {
		return err
	}
	for _, s := range subjects {
		ag, ok := bySubject[string(s.ID)]
		lecs := lecBySubject[string(s.ID)]
		if !ok && lecs == 0 {
			continue
		}
		g := GovSubjectWeekly{SubjectID: s.ID, Subject: s.Name, Lectures: lecs}
		if ok {
			g.Attempted = ag.att
			g.Correct = ag.corr
			g.Incorrect = ag.att - ag.corr
			if ag.att > 0 {
				v := float64(ag.corr) / float64(ag.att)
				g.Accuracy = &v
			}
			if ag.wn > 0 {
				v := ag.wsum / float64(ag.wn)
				g.AvgSpeed = &v
			}
		}
		rep.GovSubjects = append(rep.GovSubjects, g)
	}
	// DSA.
	drows, err := q.QueryContext(ctx, `SELECT independent, optimal, hint_used FROM dsa_attempts WHERE attempted_at >= ? AND attempted_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: weekly dsa: %w", err)
	}
	for drows.Next() {
		var i, o, h int
		if err := drows.Scan(&i, &o, &h); err != nil {
			drows.Close()
			return fmt.Errorf("engine: scan dsa: %w", err)
		}
		rep.DSAAttempted++
		// Single authoritative pass rule (never a duplicated condition).
		if IsDSAPass(db.DSAAttempt{Independent: i == 1, Optimal: o == 1, HintUsed: h == 1}) {
			rep.DSAPassed++
		}
	}
	drows.Close()
	if err := drows.Err(); err != nil {
		return fmt.Errorf("engine: dsa rows: %w", err)
	}
	rep.DSANotPassed = rep.DSAAttempted - rep.DSAPassed
	if rep.DSANotPassed < 0 {
		rep.DSANotPassed = 0
	}
	// SWE.
	srows, err := q.QueryContext(ctx, `SELECT status FROM swe_executions WHERE recorded_at >= ? AND recorded_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: weekly swe: %w", err)
	}
	for srows.Next() {
		var st string
		if err := srows.Scan(&st); err != nil {
			srows.Close()
			return fmt.Errorf("engine: scan swe: %w", err)
		}
		switch st {
		case "COMPLETED":
			rep.SWECompleted++
		case "PARTIAL":
			rep.SWEPartial++
		case "BLOCKED":
			rep.SWEBlocked++
		}
	}
	srows.Close()
	return srows.Err()
}

func topicSubject(ctx context.Context, q db.DBTX, topicID string) (db.ID, error) {
	var sid string
	if err := q.QueryRowContext(ctx, `SELECT subject_id FROM gov_topics WHERE id = ?`, topicID).Scan(&sid); err != nil {
		return "", err
	}
	return db.ID(sid), nil
}

// collectWeeklyLectureSubjects maps in-week lecture completions to subject
// IDs for the per-subject breakdown. Single-connection discipline: rows
// are fully collected (and closed) before follow-up lookups run. Every
// scan/lookup failure propagates — a dropped lecture here would silently
// undercount a subject in a stored review.
func collectWeeklyLectureSubjects(ctx context.Context, q db.DBTX, lo, hi int64, lecBySubject map[string]int) error {
	lrows, err := q.QueryContext(ctx, `SELECT lecture_id FROM gov_lecture_completions WHERE completed_at >= ? AND completed_at < ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("engine: weekly lecture ids: %w", err)
	}
	var lids []string
	var scanErr error
	func() {
		defer lrows.Close()
		for lrows.Next() {
			var lid string
			if err := lrows.Scan(&lid); err != nil {
				scanErr = fmt.Errorf("engine: scan weekly lecture id: %w", err)
				return
			}
			lids = append(lids, lid)
		}
	}()
	if scanErr != nil {
		return scanErr
	}
	if err := lrows.Err(); err != nil {
		return fmt.Errorf("engine: weekly lecture rows: %w", err)
	}
	for _, lid := range lids {
		var topicID string
		if err := q.QueryRowContext(ctx, `SELECT topic_id FROM gov_lectures WHERE id = ?`, lid).Scan(&topicID); err != nil {
			return fmt.Errorf("engine: weekly lecture topic: %w", err)
		}
		sid, err := topicSubject(ctx, q, topicID)
		if err != nil {
			return fmt.Errorf("engine: weekly topic subject: %w", err)
		}
		lecBySubject[string(sid)]++
	}
	return nil
}

// MarshalWeeklySnapshot serializes the structured weekly snapshot for storage.
func MarshalWeeklySnapshot(rep WeeklyReport) (string, error) {
	buf, err := json.Marshal(rep)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// UnmarshalWeeklySnapshot parses a stored snapshot.
func UnmarshalWeeklySnapshot(raw string) (WeeklyReport, error) {
	var rep WeeklyReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		return WeeklyReport{}, err
	}
	return rep, nil
}
