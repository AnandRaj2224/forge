package engine

import (
	"context"
	"fmt"
	"sort"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// GenerateToday creates a new Today snapshot revision for localDate with
// exact target_json per the locked scheduling rules. It preserves old
// snapshots (new revision) and never mutates sessions.
//
// Rules:
//   - GOV lecture: 3 x 1h slots, scan global subject order, first
//     incomplete subject not already picked today; activate if inactive.
//   - GOV practice: 3 x 1h, slot1 Quant, slot2 Reasoning, slot3 round-robin
//     among other active subjects with practice-eligible topics.
//   - DSA: one 2h30 block, 2 repeat + 1 new, due/review state, catalogue order.
//   - SWE: one 2h block, daily combination from routine areas, current
//     unfinished task per area.
func GenerateToday(ctx context.Context, database *db.DB, localDate string, now time.Time) (db.TodaySnapshot, []db.TodayTask, error) {
	cal := DefaultCalendar()
	if _, err := cal.StartOfDay(localDate); err != nil {
		return db.TodaySnapshot{}, nil, wrapErr("generate", ErrValidation, "bad local date")
	}
	rev, err := nextRevision(ctx, database, localDate)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	snap := db.TodaySnapshot{
		ID: db.ID(fmt.Sprintf("today-%s-r%d", localDate, rev)),
		LocalDate: localDate, Revision: rev, GeneratedAt: now,
	}
	tasks, err := planDay(ctx, database, localDate, now)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	err = database.WithTx(ctx, func(q db.DBTX) error {
		if err := runtime.CreateTodaySnapshot(ctx, q, snap); err != nil {
			return err
		}
		for i, t := range tasks {
			t.SnapshotID = snap.ID
			t.Position = i + 1
			t.ID = db.ID(fmt.Sprintf("%s-task-%02d", string(snap.ID), i+1))
			if err := runtime.InsertTodayTask(ctx, q, t); err != nil {
				return err
			}
			tasks[i] = t
		}
		return nil
	})
	if err != nil {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: generate today: %w", err)
	}
	return snap, tasks, nil
}

// EnsureToday returns the latest snapshot for localDate, generating only
// when missing. It never creates a new revision on every reload.
func EnsureToday(ctx context.Context, database *db.DB, localDate string, now time.Time) (db.TodaySnapshot, []db.TodayTask, error) {
	snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if err == nil {
		tasks, err := runtime.GetTodayTasks(ctx, database, snap.ID)
		if err != nil {
			return db.TodaySnapshot{}, nil, fmt.Errorf("engine: load today tasks: %w", err)
		}
		return snap, tasks, nil
	}
	return GenerateToday(ctx, database, localDate, now)
}

func nextRevision(ctx context.Context, database *db.DB, localDate string) (int, error) {
	// Latest revision + 1, or 1 when none.
	snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if err != nil {
		return 1, nil
	}
	return snap.Revision + 1, nil
}

func planDay(ctx context.Context, database *db.DB, localDate string, now time.Time) ([]db.TodayTask, error) {
	var out []db.TodayTask
	lec, err := planLectureSlots(ctx, database, now)
	if err != nil {
		return nil, err
	}
	out = append(out, lec...)
	prac, err := planPracticeSlots(ctx, database, localDate, now)
	if err != nil {
		return nil, err
	}
	out = append(out, prac...)
	dsa, err := planDSABlock(ctx, database, localDate)
	if err != nil {
		return nil, err
	}
	out = append(out, dsa...)
	swe, err := planSWEBlock(ctx, database, localDate)
	if err != nil {
		return nil, err
	}
	out = append(out, swe...)
	return out, nil
}

// ---- lecture ----

func planLectureSlots(ctx context.Context, database *db.DB, now time.Time) ([]db.TodayTask, error) {
	subjects, err := catalog.GetGovSubjects(ctx, database)
	if err != nil {
		return nil, fmt.Errorf("engine: gov subjects: %w", err)
	}
	routine, err := catalog.GetGovLectureRoutine(ctx, database)
	if err != nil || len(routine) == 0 {
		// Default 3 x 1h when routine missing.
		routine = []db.GovLectureRoutineSlot{{SlotOrder: 1, DurationSeconds: 3600}, {SlotOrder: 2, DurationSeconds: 3600}, {SlotOrder: 3, DurationSeconds: 3600}}
	}
	var out []db.TodayTask
	picked := map[db.ID]bool{}
	for _, slot := range routine {
		subj, topic, lec, err := firstIncompleteLecture(ctx, database, subjects, picked)
		if err != nil {
			return nil, err
		}
		if subj == nil {
			break // everything complete
		}
		// Activate immediately when selected while inactive.
		active, err := runtime.IsGovSubjectActive(ctx, database, subj.ID)
		if err != nil {
			return nil, fmt.Errorf("engine: subject active check: %w", err)
		}
		if !active {
			if err := runtime.ActivateGovSubject(ctx, database, subj.ID, now); err != nil {
				return nil, fmt.Errorf("engine: activate subject: %w", err)
			}
		}
		picked[subj.ID] = true
		target, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: subj.ID, TopicID: topic.ID, LectureID: lec.ID})
		tj := target
		out = append(out, db.TodayTask{
			Domain: db.DomainGOV, Kind: db.SessionKindLecture,
			Title: fmt.Sprintf("%s • %s • %s", subj.Name, topic.Name, lec.Title),
			PlannedSeconds: slot.DurationSeconds, TargetJSON: &tj,
		})
	}
	return out, nil
}

func firstIncompleteLecture(ctx context.Context, q db.DBTX, subjects []db.GovSubject, exclude map[db.ID]bool) (*db.GovSubject, *db.GovTopic, *db.GovLecture, error) {
	for _, s := range subjects {
		if exclude[s.ID] {
			continue
		}
		topics, err := catalog.GetGovTopics(ctx, q, s.ID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("engine: gov topics: %w", err)
		}
		allDone := true
		for _, tp := range topics {
			lecs, err := catalog.GetGovLectures(ctx, q, tp.ID)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("engine: gov lectures: %w", err)
			}
			if len(lecs) == 0 {
				continue
			}
			for _, l := range lecs {
				done, err := runtime.IsLectureComplete(ctx, q, l.ID)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("engine: lecture complete: %w", err)
				}
				if !done {
					sc := s
					tc := tp
					lc := l
					return &sc, &tc, &lc, nil
				}
			}
		}
		_ = allDone
		// Subject with zero topics or all lectures done: complete, skip.
		continue
	}
	return nil, nil, nil, nil
}

// ---- practice ----

func planPracticeSlots(ctx context.Context, database *db.DB, localDate string, now time.Time) ([]db.TodayTask, error) {
	routine, err := catalog.GetGovPracticeRoutine(ctx, database)
	if err != nil || len(routine) == 0 {
		return nil, nil
	}
	sort.Slice(routine, func(i, j int) bool { return routine[i].SlotOrder < routine[j].SlotOrder })
	var out []db.TodayTask
	for _, slot := range routine {
		var subjectID db.ID
		switch slot.SelectionMode {
		case db.GovSelectionFixed:
			if slot.FixedSubjectID == nil {
				continue
			}
			subjectID = *slot.FixedSubjectID
		case db.GovSelectionRoundRobin:
			sid, err := pickRoundRobinSubject(ctx, database, now)
			if err != nil {
				return nil, err
			}
			if sid == "" {
				continue
			}
			subjectID = sid
		default:
			continue
		}
		topics := practiceBundleTopics(ctx, database, subjectID)
		if len(topics) == 0 {
			continue
		}
		subj, err := catalog.GetGovSubject(ctx, database, subjectID)
		if err != nil {
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
			Title: title, PlannedSeconds: slot.DurationSeconds, TargetJSON: &tj,
		})
		// Persist rotation after slot-3 picks.
		if slot.SelectionMode == db.GovSelectionRoundRobin {
			_ = runtime.SetGovPracticeRotationState(ctx, database, db.GovPracticeRotationState{
				ID: 1, LastSubjectID: &subjectID, UpdatedAt: now,
			})
		}
	}
	return out, nil
}

func practiceBundleTopics(ctx context.Context, q db.DBTX, subjectID db.ID) []db.GovTopic {
	topics, err := catalog.GetGovTopics(ctx, q, subjectID)
	if err != nil {
		return nil
	}
	// Practice-eligible: topics with at least one lecture (or zero-lecture
	// topics count as unlocked). Take first up to 3 in catalogue order.
	if len(topics) > 3 {
		topics = topics[:3]
	}
	return topics
}

func topicIDs(ts []db.GovTopic) []db.ID {
	out := make([]db.ID, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

// pickRoundRobinSubject selects the next active subject (excluding Quant and
// Reasoning, excluding inactive Computer Awareness unless activated) with at
// least one practice-eligible topic, cycling after last_subject_id.
func pickRoundRobinSubject(ctx context.Context, q db.DBTX, now time.Time) (db.ID, error) {
	_ = now
	subjects, err := catalog.GetGovSubjects(ctx, q)
	if err != nil {
		return "", fmt.Errorf("engine: gov subjects: %w", err)
	}
	quantID, reasID := db.ID(""), db.ID("")
	for _, s := range subjects {
		if s.Name == "Quant" {
			quantID = s.ID
		}
		if s.Name == "Reasoning" {
			reasID = s.ID
		}
	}
	var eligible []db.GovSubject
	for _, s := range subjects {
		if s.ID == quantID || s.ID == reasID {
			continue
		}
		active, err := runtime.IsGovSubjectActive(ctx, q, s.ID)
		if err != nil {
			return "", fmt.Errorf("engine: subject active: %w", err)
		}
		if !active {
			continue // Computer Awareness (or any inactive) cannot appear before activation
		}
		topics, err := catalog.GetGovTopics(ctx, q, s.ID)
		if err != nil || len(topics) == 0 {
			continue
		}
		eligible = append(eligible, s)
	}
	if len(eligible) == 0 {
		return "", nil
	}
	rot, err := runtime.GetGovPracticeRotationState(ctx, q)
	last := db.ID("")
	if err == nil && rot.LastSubjectID != nil {
		last = *rot.LastSubjectID
	}
	// Next after last in eligible order (eligible preserves global order).
	for i, s := range eligible {
		if s.ID == last && i+1 < len(eligible) {
			return eligible[i+1].ID, nil
		}
	}
	// Wrap or first when last not found.
	for _, s := range eligible {
		if s.ID != last {
			// If last was in the list, we already returned next; here last
			// is absent so start from the front.
			return eligible[0].ID, nil
		}
	}
	return eligible[0].ID, nil
}

// ---- DSA ----

func planDSABlock(ctx context.Context, q db.DBTX, localDate string) ([]db.TodayTask, error) {
	probs, err := catalog.GetDSAProblems(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("engine: dsa problems: %w", err)
	}
	if len(probs) == 0 {
		return nil, nil
	}
	planned := 9000
	if r, err := catalog.GetDSARoutine(ctx, q); err == nil && r.PlannedSeconds > 0 {
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
	// Fill repeats from any repeat state when due < 2.
	if len(bundle) < 2 {
		all, err := allRepeatStates(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, rs := range all {
			if len(bundle) >= 2 {
				break
			}
			if used[rs.ProblemID] {
				continue
			}
			if _, ok := byID[rs.ProblemID]; !ok {
				continue
			}
			bundle = append(bundle, db.DSAProblemTarget{ProblemID: rs.ProblemID, Mode: db.DSAModeRepeat})
			used[rs.ProblemID] = true
		}
	}
	// One new: first catalogue problem with no NEW attempt, not in bundle.
	for _, p := range probs {
		if len(bundle) >= 3 {
			break
		}
		if used[p.ID] {
			continue
		}
		atts, err := runtime.GetDSAAttempts(ctx, q, p.ID)
		if err != nil {
			return nil, fmt.Errorf("engine: dsa attempts: %w", err)
		}
		hasNew := false
		for _, a := range atts {
			if a.Mode == db.DSAModeNew {
				hasNew = true
				break
			}
		}
		if hasNew {
			continue
		}
		// Also skip problems with any repeat state (already introduced).
		if _, err := runtime.GetDSARepeatState(ctx, q, p.ID); err == nil {
			continue
		}
		bundle = append(bundle, db.DSAProblemTarget{ProblemID: p.ID, Mode: db.DSAModeNew})
		used[p.ID] = true
		break
	}
	// Fresh DB fallback: fill remaining with new problems in catalogue order.
	for _, p := range probs {
		if len(bundle) >= 3 {
			break
		}
		if used[p.ID] {
			continue
		}
		bundle = append(bundle, db.DSAProblemTarget{ProblemID: p.ID, Mode: db.DSAModeNew})
		used[p.ID] = true
	}
	if len(bundle) == 0 {
		return nil, nil
	}
	target, _ := db.MarshalTarget(db.DSATarget{Problems: bundle})
	tj := target
	titles := make([]string, 0, len(bundle))
	for _, b := range bundle {
		if p, ok := byID[b.ProblemID]; ok {
			titles = append(titles, p.Title)
		}
	}
	return []db.TodayTask{{
		Domain: db.DomainDSA, Kind: db.SessionKindDSA,
		Title: "DSA Practice • " + joinPlus(titles),
		PlannedSeconds: planned, TargetJSON: &tj,
	}}, nil
}

func dueRepeats(ctx context.Context, q db.DBTX, localDate string) ([]db.DSARepeatState, error) {
	all, err := runtime.ListDSARepeatDue(ctx, q, localDate)
	if err != nil {
		return nil, fmt.Errorf("engine: repeat due: %w", err)
	}
	return all, nil
}

func allRepeatStates(ctx context.Context, q db.DBTX, ) ([]db.DSARepeatState, error) {
	// Far-future date returns all rows ordered by next_review_date.
	return runtime.ListDSARepeatDue(ctx, q, "9999-12-31")
}

// ---- SWE ----

func planSWEBlock(ctx context.Context, q db.DBTX, localDate string) ([]db.TodayTask, error) {
	cal := DefaultCalendar()
	t, err := cal.StartOfDay(localDate)
	if err != nil {
		return nil, err
	}
	// Go Weekday Sunday=0 -> Forge Mon=1..Sun=7.
	wd := int(t.Weekday())
	forgeWD := wd
	if wd == 0 {
		forgeWD = 7
	}
	slots, err := catalog.GetSWERoutine(ctx, q, forgeWD)
	if err != nil || len(slots) == 0 {
		return nil, nil
	}
	var bundle []db.SWETaskTarget
	planned := 0
	var names []string
	for _, sl := range slots {
		task := currentUnfinishedSWETask(ctx, q, sl.AreaID)
		planned += sl.DurationSeconds
		if task == nil {
			continue
		}
		tid := task.ID
		bundle = append(bundle, db.SWETaskTarget{AreaID: sl.AreaID, TaskID: &tid})
		names = append(names, task.Title)
	}
	if len(bundle) == 0 {
		return nil, nil
	}
	if planned <= 0 {
		planned = 7200
	}
	target, _ := db.MarshalTarget(db.SWETarget{Tasks: bundle})
	tj := target
	return []db.TodayTask{{
		Domain: db.DomainSWE, Kind: db.SessionKindSWE,
		Title: "SWE Practice • " + joinPlus(names),
		PlannedSeconds: planned, TargetJSON: &tj,
	}}, nil
}

// currentUnfinishedSWETask selects the first task in area order without a
// COMPLETED execution (same task may repeat on different days when nothing
// is unfinished? then return the first task).
func currentUnfinishedSWETask(ctx context.Context, q db.DBTX, areaID db.ID) *db.SWETask {
	tasks, err := catalog.GetSWETasks(ctx, q, areaID)
	if err != nil || len(tasks) == 0 {
		return nil
	}
	for _, t := range tasks {
		done, err := runtime.IsSWETaskComplete(ctx, q, t.ID)
		if err != nil || !done {
			c := t
			return &c
		}
	}
	c := tasks[0]
	return &c
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
