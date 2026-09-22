package runtime

import (
	"context"
	"database/sql"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
)

// History aggregates are derived entirely from existing runtime evidence
// (sessions, today snapshots/tasks, lecture completions, practice records,
// DSA attempts, SWE executions). No invented values.

// DailySummary is the operational "what actually happened on this day" view.
type DailySummary struct {
	Date           string // YYYY-MM-DD
	Planned        int
	Actual         int
	Variance       int // actual - planned
	TasksTotal     int
	TasksCompleted int
	Sessions       int
	GovSeconds     int
	DSASeconds     int
	SWESeconds     int
	GovLectures    int
	GovAttempted   int
	GovCorrect     int
	GovAccuracy    float64
	DSAAttempts    int
	DSAPassed      int
	SWECompleted   int
	SWEPartial     int
	SWEBlocked     int
}

// dateOf formats a unix timestamp as YYYY-MM-DD UTC.
func dateOf(sec int64) string { return time.Unix(sec, 0).UTC().Format("2006-01-02") }

// CurrentWeekDays returns Mon..Sun dates for the week containing now (UTC).
func CurrentWeekDays(now time.Time) []time.Time {
	t := now.UTC()
	// time.Weekday: Sunday=0. Convert to Mon=0..Sun=6 offset.
	wd := int(t.Weekday())
	offset := (wd + 6) % 7
	monday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -offset)
	out := make([]time.Time, 0, 7)
	for i := 0; i < 7; i++ {
		out = append(out, monday.AddDate(0, 0, i))
	}
	return out
}

// CurrentWeekDates returns Mon..Sun YYYY-MM-DD strings for now.
func CurrentWeekDates(now time.Time) []string {
	days := CurrentWeekDays(now)
	out := make([]string, 0, len(days))
	for _, d := range days {
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

// ComputeDailySummary aggregates evidence for one local date.
func ComputeDailySummary(ctx context.Context, q db.DBTX, dateStr string) (DailySummary, error) {
	s := DailySummary{Date: dateStr}
	// Planned + task counts from the latest snapshot for the date.
	var snapID string
	var hasSnap bool
	var rev int
	_ = rev
	row := q.QueryRowContext(ctx, `SELECT id FROM today_snapshots WHERE local_date = ? ORDER BY revision DESC LIMIT 1`, dateStr)
	var sid string
	if err := row.Scan(&sid); err == nil {
		hasSnap = true
		snapID = sid
	}
	if hasSnap {
		trows, err := q.QueryContext(ctx, `SELECT id, planned_seconds FROM today_tasks WHERE snapshot_id = ?`, snapID)
		if err == nil {
			var taskIDs []string
			func() {
				defer trows.Close()
				for trows.Next() {
					var id string
					var planned int
					if err := trows.Scan(&id, &planned); err == nil {
						s.Planned += planned
						s.TasksTotal++
						taskIDs = append(taskIDs, id)
					}
				}
			}()
			// Tasks completed: snapshot tasks with persisted evidence.
			for _, tid := range taskIDs {
				if taskHasEvidence(ctx, q, db.ID(tid)) {
					s.TasksCompleted++
				}
			}
		}
	}
	// Sessions started on this date.
	type sessRow struct {
		id      string
		domain  string
		planned int
		started int64
		stopped sql.NullInt64
	}
	var sessRows []sessRow
	func() {
		srows, err := q.QueryContext(ctx, `SELECT id, domain, planned_seconds, started_at, stopped_at FROM sessions`)
		if err != nil {
			return
		}
		defer srows.Close()
		for srows.Next() {
			var r sessRow
			if err := srows.Scan(&r.id, &r.domain, &r.planned, &r.started, &r.stopped); err != nil {
				continue
			}
			sessRows = append(sessRows, r)
		}
	}()
	var sessIDs []string
	for _, r := range sessRows {
		if dateOf(r.started) != dateStr {
			continue
		}
		s.Sessions++
		sessIDs = append(sessIDs, r.id)
		if r.stopped.Valid {
			el := int(r.stopped.Int64 - r.started)
			if el < 0 {
				el = 0
			}
			s.Actual += el
			switch r.domain {
			case "GOV":
				s.GovSeconds += el
			case "DSA":
				s.DSASeconds += el
			case "SWE":
				s.SWESeconds += el
			}
		}
	}
	_ = sessIDs
	s.Variance = s.Actual - s.Planned
	// Gov lectures completed that day.
	{
		rows, err := q.QueryContext(ctx, `SELECT completed_at FROM gov_lecture_completions`)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var ts int64
					if rows.Scan(&ts) == nil && dateOf(ts) == dateStr {
						s.GovLectures++
					}
				}
			}()
		}
	}
	// Gov practice that day.
	{
		rows, err := q.QueryContext(ctx, `SELECT attempted, correct FROM gov_practice_records WHERE recorded_at >= ? AND recorded_at < ?`,
			dayStart(dateStr), dayStart(dateStr)+86400)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var att, corr int
					if rows.Scan(&att, &corr) == nil {
						s.GovAttempted += att
						s.GovCorrect += corr
					}
				}
			}()
		}
		if s.GovAttempted > 0 {
			s.GovAccuracy = float64(s.GovCorrect) / float64(s.GovAttempted)
		}
	}
	// DSA attempts that day.
	{
		rows, err := q.QueryContext(ctx, `SELECT independent, optimal, hint_used FROM dsa_attempts WHERE attempted_at >= ? AND attempted_at < ?`,
			dayStart(dateStr), dayStart(dateStr)+86400)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var indep, opt, hint int
					if rows.Scan(&indep, &opt, &hint) == nil {
						s.DSAAttempts++
						if indep == 1 && opt == 1 && hint == 0 {
							s.DSAPassed++
						}
					}
				}
			}()
		}
	}
	// SWE executions that day.
	{
		rows, err := q.QueryContext(ctx, `SELECT status FROM swe_executions WHERE recorded_at >= ? AND recorded_at < ?`,
			dayStart(dateStr), dayStart(dateStr)+86400)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var st string
					if rows.Scan(&st) == nil {
						switch st {
						case "COMPLETED":
							s.SWECompleted++
						case "PARTIAL":
							s.SWEPartial++
						case "BLOCKED":
							s.SWEBlocked++
						}
					}
				}
			}()
		}
	}
	return s, nil
}

func dayStart(dateStr string) int64 {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	return t.UTC().Unix()
}

// taskHasEvidence reports whether any stopped session for the task has rows.
func taskHasEvidence(ctx context.Context, q db.DBTX, taskID db.ID) bool {
	rows, err := q.QueryContext(ctx, `SELECT id FROM sessions WHERE today_task_id = ? AND status = 'STOPPED'`, string(taskID))
	if err != nil {
		return false
	}
	var sids []string
	func() {
		defer rows.Close()
		for rows.Next() {
			var sid string
			if rows.Scan(&sid) == nil {
				sids = append(sids, sid)
			}
		}
	}()
	for _, sid := range sids {
		if sessionHasEvidenceRaw(ctx, q, sid) {
			return true
		}
	}
	return false
}

func sessionHasEvidenceRaw(ctx context.Context, q db.DBTX, sid string) bool {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_records WHERE session_id = ?`, sid).Scan(&n); err == nil && n > 0 {
		return true
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM dsa_attempts WHERE session_id = ?`, sid).Scan(&n); err == nil && n > 0 {
		return true
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM swe_executions WHERE session_id = ?`, sid).Scan(&n); err == nil && n > 0 {
		return true
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_lecture_completions WHERE session_id = ?`, sid).Scan(&n); err == nil && n > 0 {
		return true
	}
	return false
}

// WeeklyEvidence aggregates stopped-session evidence inside [weekStart, weekEnd].
type WeeklyEvidence struct {
	Planned      int
	Actual       int
	Sessions     int
	Tasks        int // distinct planned tasks with stopped-session evidence (DONE)
	TasksTotal   int // distinct planned tasks in range (completion denominator)
	GovSeconds   int
	DSASeconds   int
	SWESeconds   int
	GovLectures  int
	GovAttempted int
	GovCorrect   int
	GovAccuracy  float64
	DSAAttempts  int
	DSAPassed    int
	SWECompleted int
	SWEPartial   int
	SWEBlocked   int
}

// GovSubjectEvidence is one subject slice of GOV evidence: lectures plus
// practice questions with accuracy. Only subjects with actual evidence in
// the period are returned, ordered by subject sort order.
type GovSubjectEvidence struct {
	SubjectID db.ID
	Subject   string
	Lectures  int
	Attempted int
	Correct   int
	Accuracy  float64 // 0 when nothing attempted
}

// ComputeWeeklyGovSubjects aggregates GOV lecture completions and practice
// records per subject inside [weekStart, weekEnd] using existing tables:
// completions join lectures to topics to subjects, records carry topic IDs.
// NOTE: the DB pool allows a single open connection, so every rows result
// is fully collected (and closed) before any follow-up lookup runs.
// Nesting a query inside an open rows loop deadlocks permanently.
func ComputeWeeklyGovSubjects(ctx context.Context, q db.DBTX, weekStart, weekEnd string) []GovSubjectEvidence {
	lo := dayStart(weekStart)
	hi := dayStart(weekEnd) + 86400
	type agg struct {
		subjectID db.ID
		lectures  int
		attempted int
		correct   int
	}
	bySubject := map[string]*agg{}
	accumulate := func(subjectID db.ID, lectures, attempted, correct int) {
		a, ok := bySubject[string(subjectID)]
		if !ok {
			a = &agg{subjectID: subjectID}
			bySubject[string(subjectID)] = a
		}
		a.lectures += lectures
		a.attempted += attempted
		a.correct += correct
	}
	subjectOfTopic := func(topicID string) (db.ID, bool) {
		var subjectID string
		if err := q.QueryRowContext(ctx, `SELECT subject_id FROM gov_topics WHERE id = ?`, topicID).Scan(&subjectID); err != nil {
			return "", false
		}
		return db.ID(subjectID), true
	}
	// Lecture completions in range: collect IDs first, then map through
	// lectures/topics with no open rows.
	var lectureIDs []string
	if rows, err := q.QueryContext(ctx, `SELECT lecture_id FROM gov_lecture_completions WHERE completed_at >= ? AND completed_at < ?`, lo, hi); err == nil {
		func() {
			defer rows.Close()
			for rows.Next() {
				var lectureID string
				if rows.Scan(&lectureID) == nil {
					lectureIDs = append(lectureIDs, lectureID)
				}
			}
		}()
	}
	for _, lectureID := range lectureIDs {
		var topicID string
		if err := q.QueryRowContext(ctx, `SELECT topic_id FROM gov_lectures WHERE id = ?`, lectureID).Scan(&topicID); err != nil {
			continue
		}
		if subjectID, ok := subjectOfTopic(topicID); ok {
			accumulate(subjectID, 1, 0, 0)
		}
	}
	// Practice records in range: collect rows first, then map topics.
	type pracRow struct {
		topicID string
		att     int
		corr    int
	}
	var pracRows []pracRow
	if rows, err := q.QueryContext(ctx, `SELECT topic_id, attempted, correct FROM gov_practice_records WHERE recorded_at >= ? AND recorded_at < ?`, lo, hi); err == nil {
		func() {
			defer rows.Close()
			for rows.Next() {
				var r pracRow
				if rows.Scan(&r.topicID, &r.att, &r.corr) == nil {
					pracRows = append(pracRows, r)
				}
			}
		}()
	}
	for _, r := range pracRows {
		if subjectID, ok := subjectOfTopic(r.topicID); ok {
			accumulate(subjectID, 0, r.att, r.corr)
		}
	}
	// Order by subject sort order; resolve display names via catalog.
	subjects, err := catalog.GetGovSubjects(ctx, q)
	if err != nil {
		return nil
	}
	out := make([]GovSubjectEvidence, 0, len(bySubject))
	for _, s := range subjects {
		a, ok := bySubject[string(s.ID)]
		if !ok || (a.lectures == 0 && a.attempted == 0) {
			continue
		}
		ev := GovSubjectEvidence{SubjectID: s.ID, Subject: s.Name, Lectures: a.lectures, Attempted: a.attempted, Correct: a.correct}
		if a.attempted > 0 {
			ev.Accuracy = float64(a.correct) / float64(a.attempted)
		}
		out = append(out, ev)
	}
	return out
}

// ComputeWeeklyEvidence aggregates evidence for an inclusive date range.
func ComputeWeeklyEvidence(ctx context.Context, q db.DBTX, weekStart, weekEnd string) WeeklyEvidence {
	var e WeeklyEvidence
	start, err1 := time.Parse("2006-01-02", weekStart)
	end, err2 := time.Parse("2006-01-02", weekEnd)
	if err1 != nil || err2 != nil {
		return e
	}
	inRange := func(dateStr string) bool { return dateStr >= weekStart && dateStr <= weekEnd }
	_ = start
	_ = end
	// Planned from snapshots in range.
	rows, err := q.QueryContext(ctx, `SELECT s.id FROM today_snapshots s WHERE s.local_date >= ? AND s.local_date <= ?`, weekStart, weekEnd)
	if err == nil {
		var snapIDs []string
		func() {
			defer rows.Close()
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					snapIDs = append(snapIDs, id)
				}
			}
		}()
		seenTask := map[string]bool{}
		for _, sid := range snapIDs {
			trows, err := q.QueryContext(ctx, `SELECT id, planned_seconds FROM today_tasks WHERE snapshot_id = ?`, sid)
			if err != nil {
				continue
			}
			func() {
				defer trows.Close()
				for trows.Next() {
					var id string
					var planned int
					if trows.Scan(&id, &planned) == nil && !seenTask[id] {
						seenTask[id] = true
						e.Planned += planned
						e.TasksTotal++
					}
				}
			}()
		}
	}
	// Sessions in range by start date.
	type wSess struct {
		id      string
		taskID  string
		domain  string
		started int64
		stopped sql.NullInt64
	}
	var wRows []wSess
	func() {
		srows, err := q.QueryContext(ctx, `SELECT id, today_task_id, domain, planned_seconds, started_at, stopped_at FROM sessions`)
		if err != nil {
			return
		}
		defer srows.Close()
		for srows.Next() {
			var id, taskID, domain string
			var planned int
			var started int64
			var stopped sql.NullInt64
			if srows.Scan(&id, &taskID, &domain, &planned, &started, &stopped) != nil {
				continue
			}
			wRows = append(wRows, wSess{id: id, taskID: taskID, domain: domain, started: started, stopped: stopped})
		}
	}()
	seenTasks := map[string]bool{}
	for _, r := range wRows {
		if !inRange(dateOf(r.started)) {
			continue
		}
		if !r.stopped.Valid {
			continue
		}
		el := int(r.stopped.Int64 - r.started)
		if el < 0 {
			el = 0
		}
		e.Sessions++
		e.Actual += el
		switch r.domain {
		case "GOV":
			e.GovSeconds += el
		case "DSA":
			e.DSASeconds += el
		case "SWE":
			e.SWESeconds += el
		}
		if sessionHasEvidenceRaw(ctx, q, r.id) && !seenTasks[r.taskID] {
			seenTasks[r.taskID] = true
			e.Tasks++
		}
	}
	lo := dayStart(weekStart)
	hi := dayStart(weekEnd) + 86400
	// Gov lectures.
	{
		rows, err := q.QueryContext(ctx, `SELECT completed_at FROM gov_lecture_completions WHERE completed_at >= ? AND completed_at < ?`, lo, hi)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var ts int64
					if rows.Scan(&ts) == nil {
						e.GovLectures++
					}
				}
			}()
		}
	}
	// Gov practice.
	{
		rows, err := q.QueryContext(ctx, `SELECT attempted, correct FROM gov_practice_records WHERE recorded_at >= ? AND recorded_at < ?`, lo, hi)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var att, corr int
					if rows.Scan(&att, &corr) == nil {
						e.GovAttempted += att
						e.GovCorrect += corr
					}
				}
			}()
		}
		if e.GovAttempted > 0 {
			e.GovAccuracy = float64(e.GovCorrect) / float64(e.GovAttempted)
		}
	}
	// DSA.
	{
		rows, err := q.QueryContext(ctx, `SELECT independent, optimal, hint_used FROM dsa_attempts WHERE attempted_at >= ? AND attempted_at < ?`, lo, hi)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var indep, opt, hint int
					if rows.Scan(&indep, &opt, &hint) == nil {
						e.DSAAttempts++
						if indep == 1 && opt == 1 && hint == 0 {
							e.DSAPassed++
						}
					}
				}
			}()
		}
	}
	// SWE.
	{
		rows, err := q.QueryContext(ctx, `SELECT status FROM swe_executions WHERE recorded_at >= ? AND recorded_at < ?`, lo, hi)
		if err == nil {
			func() {
				defer rows.Close()
				for rows.Next() {
					var st string
					if rows.Scan(&st) == nil {
						switch st {
						case "COMPLETED":
							e.SWECompleted++
						case "PARTIAL":
							e.SWEPartial++
						case "BLOCKED":
							e.SWEBlocked++
						}
					}
				}
			}()
		}
	}
	return e
}
