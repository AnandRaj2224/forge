package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// todayGenMu serializes Today ensure/generation in-process.
var todayGenMu sync.Mutex

// GenerateToday creates a new Today snapshot revision for localDate using
// the explicit focus exam. Two-phase: pure plan, then one transaction.
// Rotation cursors and activations persist atomically; failures roll back.
func GenerateToday(ctx context.Context, database *db.DB, localDate string, now time.Time) (db.TodaySnapshot, []db.TodayTask, error) {
	cal, err := VerifyCalendarSettings(ctx, database)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	if _, err := cal.StartOfDay(localDate); err != nil {
		return db.TodaySnapshot{}, nil, wrapErr("generate", ErrValidation, "bad local date")
	}
	// Ensure a focus exists (default persisted on first run).
	focus, err := EnsureFocusExam(ctx, database, now)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	plan, err := planDay(ctx, database, localDate, now, focus)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	rev, err := nextRevision(ctx, database, localDate)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	snap := db.TodaySnapshot{
		ID:        db.ID(fmt.Sprintf("today-%s-r%d", localDate, rev)),
		LocalDate: localDate, Revision: rev, GeneratedAt: now,
	}
	tasks := make([]db.TodayTask, len(plan.tasks))
	copy(tasks, plan.tasks)
	for i := range tasks {
		tasks[i].SnapshotID = snap.ID
		tasks[i].Position = i + 1
		tasks[i].ID = db.ID(fmt.Sprintf("%s-task-%02d", string(snap.ID), i+1))
	}
	err = database.WithTx(ctx, func(q db.DBTX) error {
		if err := runtime.CreateTodaySnapshot(ctx, q, snap); err != nil {
			return err
		}
		for _, t := range tasks {
			if err := runtime.InsertTodayTask(ctx, q, t); err != nil {
				return err
			}
			if err := runtime.InsertSnapshotTask(ctx, q, snap.ID, t.ID, t.Position); err != nil {
				return err
			}
		}
		for _, sid := range plan.activate {
			if err := runtime.ActivateGovSubject(ctx, q, sid, now); err != nil {
				return fmt.Errorf("engine: activate subject: %w", err)
			}
		}
		for _, r := range plan.rotations {
			if err := catalog.SetGovPracticeRotation(ctx, q, r); err != nil {
				return fmt.Errorf("engine: rotation state: %w", err)
			}
		}
		return nil
	})
	if err == nil {
		return snap, tasks, nil
	}
	if !isUniqueConflict(err) {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: generate today: %w", err)
	}
	existing, rerr := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if rerr != nil {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: generate today (revision conflict): %w", err)
	}
	existingTasks, rerr := runtime.GetTodayTasks(ctx, database, existing.ID)
	if rerr != nil {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: load today tasks: %w", rerr)
	}
	return existing, existingTasks, nil
}

// EnsureToday returns the latest snapshot for localDate, generating only
// when no snapshot row exists. Idempotent: reopening never advances
// rotation or creates revisions.
//
// A latest snapshot that holds no assignments is treated as premature
// (planned before the catalogue was configured): the next open replans,
// committing a new revision only when the fresh plan holds real work.
// An still-empty fresh plan returns the existing snapshot without churn,
// rotation writes, or new revisions.
func EnsureToday(ctx context.Context, database *db.DB, localDate string, now time.Time) (db.TodaySnapshot, []db.TodayTask, error) {
	todayGenMu.Lock()
	defer todayGenMu.Unlock()
	snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if err == nil {
		tasks, err := runtime.GetTodayTasks(ctx, database, snap.ID)
		if err != nil {
			return db.TodaySnapshot{}, nil, fmt.Errorf("engine: load today tasks: %w", err)
		}
		if len(tasks) > 0 {
			return snap, tasks, nil
		}
		return healEmptyToday(ctx, database, localDate, now, snap)
	}
	if !isNotFoundErr(err) {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: latest today snapshot: %w", err)
	}
	return GenerateToday(ctx, database, localDate, now)
}

// healEmptyToday replans a date whose latest snapshot carries no tasks.
// Healing commits a new revision only for non-empty fresh work; otherwise
// the existing empty snapshot is returned unchanged.
func healEmptyToday(ctx context.Context, database *db.DB, localDate string, now time.Time, snap db.TodaySnapshot) (db.TodaySnapshot, []db.TodayTask, error) {
	focus, err := EnsureFocusExam(ctx, database, now)
	if err != nil {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: focus: %w", err)
	}
	plan, err := planDay(ctx, database, localDate, now, focus)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	if len(plan.tasks) == 0 {
		return snap, []db.TodayTask{}, nil
	}
	return GenerateToday(ctx, database, localDate, now)
}

func nextRevision(ctx context.Context, database *db.DB, localDate string) (int, error) {
	snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if err != nil {
		if isNotFoundErr(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("engine: latest today snapshot: %w", err)
	}
	return snap.Revision + 1, nil
}

// dayPlan is the pure Phase-1 output.
type dayPlan struct {
	tasks     []db.TodayTask
	activate  []db.ID
	rotations []db.GovPracticeRotation
}

func planDay(ctx context.Context, database *db.DB, localDate string, now time.Time, focus *db.GovExam) (*dayPlan, error) {
	plan := &dayPlan{}
	lec, activate, err := planLectureSlots(ctx, database, focus)
	if err != nil {
		return nil, err
	}
	plan.tasks = append(plan.tasks, lec...)
	plan.activate = append(plan.activate, activate...)
	extraActive := map[db.ID]bool{}
	for _, sid := range plan.activate {
		extraActive[sid] = true
	}
	prac, rotations, err := planPracticeSlots(ctx, database, focus, extraActive, now)
	if err != nil {
		return nil, err
	}
	plan.tasks = append(plan.tasks, prac...)
	plan.rotations = rotations
	dsa, err := planDSABlock(ctx, database, localDate)
	if err != nil {
		return nil, err
	}
	plan.tasks = append(plan.tasks, dsa...)
	swe, err := planSWEBlock(ctx, database, localDate)
	if err != nil {
		return nil, err
	}
	plan.tasks = append(plan.tasks, swe...)
	return plan, nil
}

// ---- lecture ----

// planLectureSlots plans up to 4 GOV lectures from the focus exam only.
// No filler from other exams. Skips completed lectures; shared completed
// material is reused (never repeated). Zero-lecture topics are complete
// (intentional) and yield no lecture task. Empty/unconfigured focus yields
// no lecture work rather than invented work.
func planLectureSlots(ctx context.Context, database *db.DB, focus *db.GovExam) ([]db.TodayTask, []db.ID, error) {
	routine, err := catalog.GetGovLectureRoutine(ctx, database)
	if err != nil {
		return nil, nil, fmt.Errorf("engine: gov lecture routine: %w", err)
	}
	if len(routine) == 0 {
		routine = []db.GovLectureRoutineSlot{{SlotOrder: 1, DurationSeconds: 3600}, {SlotOrder: 2, DurationSeconds: 3600}, {SlotOrder: 3, DurationSeconds: 3600}, {SlotOrder: 4, DurationSeconds: 3600}}
	}
	sort.Slice(routine, func(i, j int) bool { return routine[i].SlotOrder < routine[j].SlotOrder })
	capacity := len(routine)
	if capacity > 4 {
		capacity = 4
		routine = routine[:4]
	}
	if focus == nil {
		return nil, nil, nil
	}
	configured, err := IsGovExamConfigured(ctx, database, focus.ID)
	if err != nil {
		return nil, nil, err
	}
	if !configured {
		return nil, nil, nil
	}
	type lectureCandidate struct {
		subject db.GovSubject
		topic   db.GovTopic
		lecture db.GovLecture
	}
	var cands []lectureCandidate
	seen := map[db.ID]bool{}
	progress, err := ExamSubjectProgress(ctx, database, focus.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range progress {
		if len(cands) >= capacity {
			break
		}
		if p.Complete {
			continue
		}
		if seen[p.Subject.ID] {
			continue
		}
		topic, lec, err := firstIncompleteExamLecture(ctx, database, focus.ID, p.Subject.ID)
		if err != nil {
			return nil, nil, err
		}
		if topic == nil || lec == nil {
			continue
		}
		seen[p.Subject.ID] = true
		cands = append(cands, lectureCandidate{subject: p.Subject, topic: *topic, lecture: *lec})
	}
	if len(cands) == 0 {
		return nil, nil, nil
	}
	var out []db.TodayTask
	var activate []db.ID
	for i, c := range cands {
		slot := routine[i]
		active, err := runtime.IsGovSubjectActive(ctx, database, c.subject.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: subject active check: %w", err)
		}
		if !active {
			activate = append(activate, c.subject.ID)
		}
		target, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: c.subject.ID, TopicID: c.topic.ID, LectureID: c.lecture.ID})
		tj := target
		out = append(out, db.TodayTask{
			Domain: db.DomainGOV, Kind: db.SessionKindLecture,
			Title:          fmt.Sprintf("%s • %s • %s", c.subject.Name, c.topic.Name, c.lecture.Title),
			PlannedSeconds: slot.DurationSeconds, TargetJSON: &tj,
		})
	}
	return out, activate, nil
}

// ---- practice ----

// PracticeEligibleTopics returns practice-eligible topics of a subject in
// catalogue order: lecture-complete (including the intentional zero-lecture
// rule). PracticeEligibleTopicsForExam filters this set to the focused
// exam's syllabus. DB failures propagate (never an empty list).
func PracticeEligibleTopics(ctx context.Context, q db.DBTX, subjectID db.ID) ([]db.GovTopic, error) {
	topics, err := catalog.GetGovTopics(ctx, q, subjectID)
	if err != nil {
		return nil, fmt.Errorf("engine: gov topics: %w", err)
	}
	var out []db.GovTopic
	for _, tp := range topics {
		lecs, err := catalog.GetGovLectures(ctx, q, tp.ID)
		if err != nil {
			return nil, fmt.Errorf("engine: gov lectures: %w", err)
		}
		if len(lecs) == 0 {
			out = append(out, tp)
			continue
		}
		complete := true
		for _, l := range lecs {
			done, err := IsLectureComplete(ctx, q, l.ID)
			if err != nil {
				return nil, fmt.Errorf("engine: lecture complete: %w", err)
			}
			if !done {
				complete = false
				break
			}
		}
		if complete {
			out = append(out, tp)
		}
	}
	return out, nil
}

// PracticeEligibleTopicsForExam returns eligible topics for (exam, subject)
// in stable syllabus order: the shared lecture-complete rule from
// PracticeEligibleTopics (including the intentional zero-lecture rule),
// filtered to the focused exam's syllabus for that subject.
func PracticeEligibleTopicsForExam(ctx context.Context, q db.DBTX, examID, subjectID db.ID) ([]db.GovTopic, error) {
	eligible, err := PracticeEligibleTopics(ctx, q, subjectID)
	if err != nil {
		return nil, err
	}
	if len(eligible) == 0 {
		return nil, nil
	}
	syll, err := catalog.GetGovExamSyllabusForSubject(ctx, q, examID, subjectID)
	if err != nil {
		return nil, err
	}
	eligibleSet := make(map[db.ID]db.GovTopic, len(eligible))
	for _, tp := range eligible {
		eligibleSet[tp.ID] = tp
	}
	var out []db.GovTopic
	for _, s := range syll {
		if tp, ok := eligibleSet[s.TopicID]; ok {
			out = append(out, tp)
		}
	}
	return out, nil
}

func subjectIsActive(ctx context.Context, q db.DBTX, subjectID db.ID, extra map[db.ID]bool) (bool, error) {
	if extra[subjectID] {
		return true, nil
	}
	return runtime.IsGovSubjectActive(ctx, q, subjectID)
}

// planPracticeSlots plans the V2 fixed routine: Quant, English, Reasoning
// (by stable ID, never display names), up to 3 distinct eligible topics each
// via independent per-exam+subject round-robin cursors. Durations come from
// gov_practice_routine slots in order (or 3600 defaults when unconfigured).
func planPracticeSlots(ctx context.Context, database *db.DB, focus *db.GovExam, extraActive map[db.ID]bool, now time.Time) ([]db.TodayTask, []db.GovPracticeRotation, error) {
	if focus == nil {
		return nil, nil, nil
	}
	configured, err := IsGovExamConfigured(ctx, database, focus.ID)
	if err != nil {
		return nil, nil, err
	}
	if !configured {
		return nil, nil, nil
	}
	routine, err := catalog.GetGovPracticeRoutine(ctx, database)
	if err != nil {
		return nil, nil, fmt.Errorf("engine: gov practice routine: %w", err)
	}
	sort.Slice(routine, func(i, j int) bool { return routine[i].SlotOrder < routine[j].SlotOrder })
	durations := []int{3600, 3600, 3600}
	if len(routine) >= 3 {
		for i := 0; i < 3; i++ {
			if routine[i].DurationSeconds > 0 {
				durations[i] = routine[i].DurationSeconds
			}
		}
	} else if len(routine) > 0 {
		for i := range routine {
			if i < 3 && routine[i].DurationSeconds > 0 {
				durations[i] = routine[i].DurationSeconds
			}
		}
	}
	var out []db.TodayTask
	var rotations []db.GovPracticeRotation
	for idx, subjectID := range db.V2FixedPracticeSubjects {
		subj, err := catalog.GetGovSubject(ctx, database, subjectID)
		if err != nil {
			// Fixed subject missing from catalogue: leave slot empty.
			continue
		}
		// Must belong to the focused exam.
		members, err := catalog.GetGovExamSubjects(ctx, database, focus.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: exam subjects: %w", err)
		}
		inExam := false
		for _, m := range members {
			if m.SubjectID == subjectID {
				inExam = true
				break
			}
		}
		if !inExam {
			continue
		}
		active, err := subjectIsActive(ctx, database, subjectID, extraActive)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: subject active: %w", err)
		}
		if !active {
			continue
		}
		eligible, err := PracticeEligibleTopicsForExam(ctx, database, focus.ID, subjectID)
		if err != nil {
			return nil, nil, err
		}
		if len(eligible) == 0 {
			continue
		}
		topics, next, err := nextPracticeTopicsForExam(ctx, database, focus.ID, subjectID, eligible)
		if err != nil {
			return nil, nil, err
		}
		if len(topics) == 0 {
			continue
		}
		names := make([]string, 0, len(topics))
		for _, t := range topics {
			names = append(names, t.Name)
		}
		target, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: subjectID, TopicIDs: topicIDs(topics)})
		tj := target
		title := fmt.Sprintf("%s practice • %s", subj.Name, joinPlus(names))
		out = append(out, db.TodayTask{
			Domain: db.DomainGOV, Kind: db.SessionKindPractice,
			Title: title, PlannedSeconds: durations[idx], TargetJSON: &tj,
		})
		rotations = append(rotations, db.GovPracticeRotation{
			ExamID: focus.ID, SubjectID: subjectID, NextIndex: next, UpdatedAt: now,
		})
	}
	return out, rotations, nil
}

// nextPracticeTopicsForExam selects up to 3 distinct topics using the
// persisted per-exam+subject cursor. Stable syllabus order; cursor advances
// by the number taken (wrapping). Missing cursor starts at 0. Cursor is
// reconciled with current eligible length (modulo) so syllabus changes
// cannot strand or duplicate.
func nextPracticeTopicsForExam(ctx context.Context, q db.DBTX, examID, subjectID db.ID, eligible []db.GovTopic) ([]db.GovTopic, int, error) {
	if len(eligible) == 0 {
		return nil, 0, nil
	}
	if len(eligible) <= 3 {
		out := make([]db.GovTopic, len(eligible))
		copy(out, eligible)
		return out, 0, nil
	}
	start := 0
	if rot, err := catalog.GetGovPracticeRotation(ctx, q, examID, subjectID); err == nil {
		start = rot.NextIndex % len(eligible)
		if start < 0 {
			start = 0
		}
	}
	out := make([]db.GovTopic, 0, 3)
	for i := 0; i < 3; i++ {
		out = append(out, eligible[(start+i)%len(eligible)])
	}
	next := (start + 3) % len(eligible)
	return out, next, nil
}

func topicIDs(ts []db.GovTopic) []db.ID {
	out := make([]db.ID, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

// ---- DSA ----

// DSA capacity: up to 4 problems per day (2 due + 2 new steady state, but
// smaller bundles allowed). Never pulls future-due repeats to fill.
func planDSABlock(ctx context.Context, q db.DBTX, localDate string) ([]db.TodayTask, error) {
	probs, err := catalog.GetDSAProblems(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("engine: dsa problems: %w", err)
	}
	if len(probs) == 0 {
		return nil, nil
	}
	planned := 9000
	if r, err := catalog.GetDSARoutine(ctx, q); err != nil {
		if !isNotFoundErr(err) {
			return nil, fmt.Errorf("engine: dsa routine: %w", err)
		}
	} else if r.PlannedSeconds > 0 {
		planned = r.PlannedSeconds
	}
	due, err := dueRepeats(ctx, q, localDate)
	if err != nil {
		return nil, err
	}
	byID := map[db.ID]db.DSAProblem{}
	for _, p := range probs {
		byID[p.ID] = p
	}
	var bundle []db.DSAProblemTarget
	used := map[db.ID]bool{}
	// Due/overdue repeats first, up to 2.
	for _, rs := range due {
		if len(bundle) >= 2 {
			break
		}
		p, ok := byID[rs.ProblemID]
		if !ok || used[p.ID] {
			continue
		}
		bundle = append(bundle, db.DSAProblemTarget{ProblemID: p.ID, Mode: db.DSAModeRepeat})
		used[p.ID] = true
	}
	// Genuinely new problems to fill up to 4 (no future-repeat fill).
	for _, p := range probs {
		if len(bundle) >= 4 {
			break
		}
		if used[p.ID] {
			continue
		}
		genuine, err := isGenuinelyNew(ctx, q, p.ID)
		if err != nil {
			return nil, err
		}
		if !genuine {
			continue
		}
		bundle = append(bundle, db.DSAProblemTarget{ProblemID: p.ID, Mode: db.DSAModeNew})
		used[p.ID] = true
	}
	if len(bundle) == 0 {
		// Explicit empty: nothing due and nothing new. Caller omits the
		// DSA block; history shows no DSA task rather than a fake bundle.
		return nil, nil
	}
	dt := db.DSATarget{Problems: bundle}
	if err := ValidateDSABundleV2(ctx, q, dt); err != nil {
		return nil, nil
	}
	target, _ := db.MarshalTarget(dt)
	tj := target
	titles := make([]string, 0, len(bundle))
	for _, b := range bundle {
		if p, ok := byID[b.ProblemID]; ok {
			titles = append(titles, p.Title)
		}
	}
	return []db.TodayTask{{
		Domain: db.DomainDSA, Kind: db.SessionKindDSA,
		Title:          "DSA Practice • " + joinPlus(titles),
		PlannedSeconds: planned, TargetJSON: &tj,
	}}, nil
}

// isGenuinelyNew reports whether a problem was never introduced.
func isGenuinelyNew(ctx context.Context, q db.DBTX, problemID db.ID) (bool, error) {
	atts, err := runtime.GetDSAAttempts(ctx, q, problemID)
	if err != nil {
		return false, fmt.Errorf("engine: dsa attempts: %w", err)
	}
	for _, a := range atts {
		if a.Mode == db.DSAModeNew {
			return false, nil
		}
	}
	if _, err := runtime.GetDSARepeatState(ctx, q, problemID); err == nil {
		return false, nil
	} else if !isNotFoundErr(err) {
		return false, fmt.Errorf("engine: repeat state: %w", err)
	}
	return true, nil
}

func dueRepeats(ctx context.Context, q db.DBTX, localDate string) ([]db.DSARepeatState, error) {
	all, err := runtime.ListDSARepeatDue(ctx, q, localDate)
	if err != nil {
		return nil, fmt.Errorf("engine: repeat due: %w", err)
	}
	return all, nil
}

// ---- SWE ----

func planSWEBlock(ctx context.Context, q db.DBTX, localDate string) ([]db.TodayTask, error) {
	cal := DefaultCalendar()
	t, err := cal.StartOfDay(localDate)
	if err != nil {
		return nil, err
	}
	wd := int(t.Weekday())
	forgeWD := wd
	if wd == 0 {
		forgeWD = 7
	}
	slots, err := catalog.GetSWERoutine(ctx, q, forgeWD)
	if err != nil {
		return nil, fmt.Errorf("engine: swe routine: %w", err)
	}
	if len(slots) == 0 {
		return nil, nil
	}
	areas, err := catalog.GetSWEAreas(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("engine: swe areas: %w", err)
	}
	areaName := map[db.ID]string{}
	for _, a := range areas {
		areaName[a.ID] = a.Name
	}
	var out []db.TodayTask
	for _, sl := range slots {
		name, ok := areaName[sl.AreaID]
		if !ok {
			return nil, wrapErr("generate", ErrInvalidTarget, fmt.Sprintf("unknown SWE area %q", string(sl.AreaID)))
		}
		if sl.DurationSeconds <= 0 {
			return nil, wrapErr("generate", ErrInvalidTarget, "SWE slot duration must be > 0")
		}
		target, _ := db.MarshalTarget(db.SWETarget{AreaID: sl.AreaID})
		tj := target
		out = append(out, db.TodayTask{
			Domain: db.DomainSWE, Kind: db.SessionKindSWE,
			Title:          fmt.Sprintf("SWE — %s — %s", name, FormatDurationLong(sl.DurationSeconds)),
			PlannedSeconds: sl.DurationSeconds, TargetJSON: &tj,
		})
	}
	return out, nil
}

func joinPlus(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += " + "
		}
		out += x
	}
	return out
}
