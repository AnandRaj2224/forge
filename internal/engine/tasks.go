package engine

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// TaskState is the authoritative Today task state.
type TaskState string

const (
	StateTodo      TaskState = "TODO"
	StateActive    TaskState = "ACTIVE"
	StateCompleted TaskState = "COMPLETED"
)

// TaskEvaluation is the authoritative evaluator result for one Today task.
// Errors are returned separately via error return; never folded into state.
type TaskEvaluation struct {
	TaskID      db.ID
	State       TaskState
	Total       int      // required target items
	Done        int      // satisfied target items
	Completed   []string // satisfied target item IDs (topic/problem/task/lecture)
	Missing     []string // still-required target item IDs
	Active      *db.Session
	ConfigError string // target validation/configuration problem (not a DB error)
}

// EvaluateTodayTask loads the task, validates its target, loads all sessions
// belonging to it, loads domain evidence across those sessions, and compares
// target items against actual evidence. Bundle completion requires ALL
// required target items to have valid evidence.
//
// DB failures propagate as error (never TODO/COMPLETE fallbacks).
func EvaluateTodayTask(ctx context.Context, q db.DBTX, taskID db.ID) (TaskEvaluation, error) {
	return evaluateAt(ctx, q, taskID, 0)
}

// EvaluateTodayTaskAt is the historical evaluator: identical to
// EvaluateTodayTask but only evidence strictly before cutoffUnix (a Forge
// day-end Unix bound, exclusive) counts, and ACTIVE is determined as of the
// cutoff. A historical day therefore never changes when later evidence is
// recorded. cutoffUnix == 0 means current state (no cutoff).
func EvaluateTodayTaskAt(ctx context.Context, q db.DBTX, taskID db.ID, cutoffUnix int64) (TaskEvaluation, error) {
	return evaluateAt(ctx, q, taskID, cutoffUnix)
}

func evaluateAt(ctx context.Context, q db.DBTX, taskID db.ID, cutoff int64) (TaskEvaluation, error) {
	ev := TaskEvaluation{TaskID: taskID, State: StateTodo}
	task, err := runtime.GetTodayTask(ctx, q, taskID)
	if err != nil {
		return TaskEvaluation{}, fmt.Errorf("engine: load today task: %w", err)
	}
	sessions, err := sessionsForTask(ctx, q, taskID)
	if err != nil {
		return TaskEvaluation{}, err
	}
	for i := range sessions {
		if isActiveAsOf(sessions[i], cutoff) {
			c := sessions[i]
			ev.Active = &c
			ev.State = StateActive
			break
		}
	}
	// ACTIVE wins; still compute progress for the form (missing items).
	switch task.Domain {
	case db.DomainGOV:
		if task.Kind == db.SessionKindLecture {
			return evalGovLecture(ctx, q, ev, task, sessions, cutoff)
		}
		return evalGovPractice(ctx, q, ev, task, sessions, cutoff)
	case db.DomainDSA:
		return evalDSA(ctx, q, ev, task, sessions, cutoff)
	case db.DomainSWE:
		return evalSWE(ctx, q, ev, task, sessions, cutoff)
	default:
		ev.ConfigError = fmt.Sprintf("unknown domain %q", string(task.Domain))
		return ev, nil
	}
}

// isActiveAsOf reports whether a session counts as active at the cutoff:
// currently ACTIVE, or (historically) still running at the cutoff bound.
func isActiveAsOf(s db.Session, cutoff int64) bool {
	if cutoff == 0 {
		return s.Status == db.SessionStatusActive
	}
	if s.StartedAt.Unix() >= cutoff {
		return false
	}
	if s.Status == db.SessionStatusActive {
		return true
	}
	return s.StoppedAt != nil && s.StoppedAt.Unix() >= cutoff
}

// evidenceSessions returns stopped sessions whose evidence may count:
// all stopped sessions currently, or (historically) those stopped strictly
// before the cutoff bound.
func evidenceSessions(sessions []db.Session, cutoff int64) []db.Session {
	var out []db.Session
	for _, s := range sessions {
		if s.Status != db.SessionStatusStopped || s.StoppedAt == nil {
			continue
		}
		if cutoff != 0 && s.StoppedAt.Unix() >= cutoff {
			continue
		}
		out = append(out, s)
	}
	return out
}

// beforeCutoff reports whether a recorded Unix timestamp counts under the
// cutoff (exclusive bound; 0 disables filtering).
func beforeCutoff(ts int64, cutoff int64) bool {
	return cutoff == 0 || ts < cutoff
}

func sessionsForTask(ctx context.Context, q db.DBTX, taskID db.ID) ([]db.Session, error) {
	all, err := runtime.ListSessions(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("engine: list sessions: %w", err)
	}
	var out []db.Session
	for _, s := range all {
		if s.TodayTaskID == taskID {
			out = append(out, s)
		}
	}
	return out, nil
}

func evalGovLecture(ctx context.Context, q db.DBTX, ev TaskEvaluation, task db.TodayTask, sessions []db.Session, cutoff int64) (TaskEvaluation, error) {
	if task.TargetJSON == nil || *task.TargetJSON == "" {
		ev.ConfigError = "lecture task has no target"
		return ev, nil
	}
	lt, err := db.UnmarshalGovLectureTarget(*task.TargetJSON)
	if err != nil {
		ev.ConfigError = "invalid lecture target: " + err.Error()
		return ev, nil
	}
	if err := ValidateGovLectureTarget(ctx, q, lt); err != nil {
		ev.ConfigError = err.Error()
		return ev, nil
	}
	ev.Total = 1
	// Lecture completion is timestamped: historically only completions
	// strictly before the cutoff count. DB failures propagate; a genuine
	// no-row result means "not done".
	comp, err := runtime.GetLectureCompletion(ctx, q, lt.LectureID)
	if err != nil {
		if isNotFoundErr(err) {
			ev.Missing = []string{string(lt.LectureID)}
			if ev.Active == nil {
				ev.State = StateTodo
			}
			return ev, nil
		}
		return TaskEvaluation{}, fmt.Errorf("engine: lecture completion lookup: %w", err)
	}
	if !beforeCutoff(comp.CompletedAt.Unix(), cutoff) {
		ev.Missing = []string{string(lt.LectureID)}
		if ev.Active == nil {
			ev.State = StateTodo
		}
		return ev, nil
	}
	ev.Done = 1
	ev.Completed = []string{string(lt.LectureID)}
	if ev.Active == nil {
		ev.State = StateCompleted
	}
	return ev, nil
}

func evalGovPractice(ctx context.Context, q db.DBTX, ev TaskEvaluation, task db.TodayTask, sessions []db.Session, cutoff int64) (TaskEvaluation, error) {
	if task.TargetJSON == nil || *task.TargetJSON == "" {
		ev.ConfigError = "practice task has no target"
		return ev, nil
	}
	pt, err := db.UnmarshalGovPracticeTarget(*task.TargetJSON)
	if err != nil {
		ev.ConfigError = "invalid practice target: " + err.Error()
		return ev, nil
	}
	if err := ValidateGovPracticeTarget(ctx, q, pt); err != nil {
		ev.ConfigError = err.Error()
		return ev, nil
	}
	ev.Total = len(pt.TopicIDs)
	satisfied := map[string]bool{}
	for _, s := range evidenceSessions(sessions, cutoff) {
		recs, err := runtime.GetGovPracticeRecordsBySession(ctx, q, s.ID)
		if err != nil {
			return TaskEvaluation{}, fmt.Errorf("engine: practice records: %w", err)
		}
		for _, r := range recs {
			if !beforeCutoff(r.RecordedAt.Unix(), cutoff) {
				continue
			}
			if !validPracticeEvidence(r) {
				continue
			}
			satisfied[string(r.TopicID)] = true
		}
	}
	for _, tid := range pt.TopicIDs {
		if satisfied[string(tid)] {
			ev.Done++
			ev.Completed = append(ev.Completed, string(tid))
		} else {
			ev.Missing = append(ev.Missing, string(tid))
		}
	}
	if ev.Active != nil {
		ev.State = StateActive
	} else if ev.Done == ev.Total && ev.Total > 0 {
		ev.State = StateCompleted
	} else {
		ev.State = StateTodo
	}
	return ev, nil
}

// validPracticeEvidence mirrors the completion rule: time>0, correct<=attempted,
// avg speed >0 when present. Rows violating these are not completion evidence
// (legacy rows with time 0 or NULL speed still count if time>0; NULL speed
// is tolerated for backward compatibility but new saves require speed).
func validPracticeEvidence(r db.GovPracticeRecord) bool {
	if r.TimeSpentSeconds <= 0 {
		return false
	}
	if r.Correct > r.Attempted || r.Attempted < 0 || r.Correct < 0 {
		return false
	}
	if r.AvgQuestionSpeedSeconds != nil && *r.AvgQuestionSpeedSeconds <= 0 {
		return false
	}
	return true
}

func evalDSA(ctx context.Context, q db.DBTX, ev TaskEvaluation, task db.TodayTask, sessions []db.Session, cutoff int64) (TaskEvaluation, error) {
	if task.TargetJSON == nil || *task.TargetJSON == "" {
		ev.ConfigError = "DSA task has no target"
		return ev, nil
	}
	dt, err := db.UnmarshalDSATarget(*task.TargetJSON)
	if err != nil {
		ev.ConfigError = "invalid DSA target: " + err.Error()
		return ev, nil
	}
	if err := ValidateDSATarget(ctx, q, dt); err != nil {
		ev.ConfigError = err.Error()
		return ev, nil
	}
	items := dt.Items()
	ev.Total = len(items)
	satisfied := map[string]bool{}
	for _, s := range evidenceSessions(sessions, cutoff) {
		atts, err := runtime.GetDSAAttemptsBySession(ctx, q, s.ID)
		if err != nil {
			return TaskEvaluation{}, fmt.Errorf("engine: dsa attempts: %w", err)
		}
		for _, a := range atts {
			if !beforeCutoff(a.AttemptedAt.Unix(), cutoff) {
				continue
			}
			if a.SolveTimeSeconds == nil || *a.SolveTimeSeconds <= 0 {
				continue
			}
			satisfied[string(a.ProblemID)] = true
		}
	}
	for _, it := range items {
		if satisfied[string(it.ProblemID)] {
			ev.Done++
			ev.Completed = append(ev.Completed, string(it.ProblemID))
		} else {
			ev.Missing = append(ev.Missing, string(it.ProblemID))
		}
	}
	if ev.Active != nil {
		ev.State = StateActive
	} else if ev.Done == ev.Total && ev.Total > 0 {
		ev.State = StateCompleted
	} else {
		ev.State = StateTodo
	}
	return ev, nil
}

func evalSWE(ctx context.Context, q db.DBTX, ev TaskEvaluation, task db.TodayTask, sessions []db.Session, cutoff int64) (TaskEvaluation, error) {
	if task.TargetJSON == nil || *task.TargetJSON == "" {
		ev.ConfigError = "SWE task has no target"
		return ev, nil
	}
	st, err := db.UnmarshalSWETarget(*task.TargetJSON)
	if err != nil {
		ev.ConfigError = "invalid SWE target: " + err.Error()
		return ev, nil
	}
	if err := ValidateSWETarget(ctx, q, st); err != nil {
		ev.ConfigError = err.Error()
		return ev, nil
	}
	items := st.Items()
	ev.Total = len(items)
	satisfied := map[string]bool{}
	for _, s := range evidenceSessions(sessions, cutoff) {
		execs, err := runtime.GetSWEExecutionsBySession(ctx, q, s.ID)
		if err != nil {
			return TaskEvaluation{}, fmt.Errorf("engine: swe executions: %w", err)
		}
		for _, e := range execs {
			if !beforeCutoff(e.RecordedAt.Unix(), cutoff) {
				continue
			}
			if !validSWEStatus(e.Status) {
				continue
			}
			satisfied[string(e.TaskID)] = true
		}
	}
	for _, it := range items {
		if it.TaskID == nil {
			// Legacy area-only target: satisfied when any execution exists
			// for a task in that area from this task's sessions.
			ok, err := areaSatisfied(ctx, q, it.AreaID, sessions, cutoff)
			if err != nil {
				return TaskEvaluation{}, err
			}
			if ok {
				ev.Done++
				ev.Completed = append(ev.Completed, "area:"+string(it.AreaID))
			} else {
				ev.Missing = append(ev.Missing, "area:"+string(it.AreaID))
			}
			continue
		}
		if satisfied[string(*it.TaskID)] {
			ev.Done++
			ev.Completed = append(ev.Completed, string(*it.TaskID))
		} else {
			ev.Missing = append(ev.Missing, string(*it.TaskID))
		}
	}
	if ev.Active != nil {
		ev.State = StateActive
	} else if ev.Done == ev.Total && ev.Total > 0 {
		ev.State = StateCompleted
	} else {
		ev.State = StateTodo
	}
	return ev, nil
}

func validSWEStatus(s db.SWEExecutionStatus) bool {
	return s == db.SWEStatusCompleted || s == db.SWEStatusPartial || s == db.SWEStatusBlocked
}

func areaSatisfied(ctx context.Context, q db.DBTX, areaID db.ID, sessions []db.Session, cutoff int64) (bool, error) {
	tasks, err := catalog.GetSWETasks(ctx, q, areaID)
	if err != nil {
		return false, fmt.Errorf("engine: list swe tasks: %w", err)
	}
	ids := map[string]bool{}
	for _, t := range tasks {
		ids[string(t.ID)] = true
	}
	for _, s := range evidenceSessions(sessions, cutoff) {
		execs, err := runtime.GetSWEExecutionsBySession(ctx, q, s.ID)
		if err != nil {
			return false, fmt.Errorf("engine: swe executions: %w", err)
		}
		for _, e := range execs {
			if !beforeCutoff(e.RecordedAt.Unix(), cutoff) {
				continue
			}
			if ids[string(e.TaskID)] && validSWEStatus(e.Status) {
				return true, nil
			}
		}
	}
	return false, nil
}
