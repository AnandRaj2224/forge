package engine

import (
	"context"
	"fmt"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// SwitchFocusExam atomically updates the focus exam and replans today's
// untouched tasks. Locked tasks (completed, active, stopped sessions, or
// partial evidence) keep original IDs/targets/sessions/evidence. Only
// never-started, evidence-free tasks are replaced with new focus candidates.
// Same-focus requests are no-ops (no new revision, no rotation advance).
// Failures leave focus, plan, and rotation unchanged.
func SwitchFocusExam(ctx context.Context, database *db.DB, newExamID db.ID, localDate string, now time.Time) (db.TodaySnapshot, []db.TodayTask, error) {
	todayGenMu.Lock()
	defer todayGenMu.Unlock()

	if _, err := DefaultCalendar().StartOfDay(localDate); err != nil {
		return db.TodaySnapshot{}, nil, wrapErr("focus switch", ErrValidation, "bad local date")
	}
	if _, err := catalog.GetGovExam(ctx, database, newExamID); err != nil {
		if isNotFoundErr(err) {
			return db.TodaySnapshot{}, nil, wrapErr("focus switch", ErrInvalidTarget, fmt.Sprintf("unknown exam %q", string(newExamID)))
		}
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: exam lookup: %w", err)
	}
	cur, err := GetFocusExam(ctx, database)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	if cur != nil && cur.ID == newExamID {
		snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
		if err != nil {
			if isNotFoundErr(err) {
				return GenerateToday(ctx, database, localDate, now)
			}
			return db.TodaySnapshot{}, nil, err
		}
		tasks, err := runtime.GetTodayTasks(ctx, database, snap.ID)
		if err != nil {
			return db.TodaySnapshot{}, nil, fmt.Errorf("engine: load today tasks: %w", err)
		}
		return snap, tasks, nil
	}

	snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if err != nil {
		if !isNotFoundErr(err) {
			return db.TodaySnapshot{}, nil, fmt.Errorf("engine: latest snapshot: %w", err)
		}
		// No plan today: plan with the new focus and commit focus state,
		// snapshot/tasks, membership, activations, and rotations in ONE
		// SQLite transaction (crash-atomic; no compensating restore).
		// Reuses planDay/nextRevision and the same snapshot/task ID scheme
		// as GenerateToday.
		if _, err := VerifyCalendarSettings(ctx, database); err != nil {
			return db.TodaySnapshot{}, nil, err
		}
		newFocus := &db.GovExam{ID: newExamID}
		if e, ferr := catalog.GetGovExam(ctx, database, newExamID); ferr == nil {
			newFocus = &e
		}
		freshPlan, perr := planDay(ctx, database, localDate, now, newFocus)
		if perr != nil {
			return db.TodaySnapshot{}, nil, perr
		}
		rev, rerr := nextRevision(ctx, database, localDate)
		if rerr != nil {
			return db.TodaySnapshot{}, nil, rerr
		}
		newSnap := db.TodaySnapshot{
			ID:        db.ID(fmt.Sprintf("today-%s-r%d", localDate, rev)),
			LocalDate: localDate, Revision: rev, GeneratedAt: now,
		}
		newTasks := make([]db.TodayTask, len(freshPlan.tasks))
		copy(newTasks, freshPlan.tasks)
		for i := range newTasks {
			newTasks[i].SnapshotID = newSnap.ID
			newTasks[i].Position = i + 1
			newTasks[i].ID = db.ID(fmt.Sprintf("%s-task-%02d", string(newSnap.ID), i+1))
		}
		txerr := database.WithTx(ctx, func(q db.DBTX) error {
			if err := catalog.SetGovFocusState(ctx, q, db.GovFocusState{ID: 1, FocusedExamID: &newExamID, UpdatedAt: now}); err != nil {
				return err
			}
			if err := runtime.CreateTodaySnapshot(ctx, q, newSnap); err != nil {
				return err
			}
			for _, t := range newTasks {
				if err := runtime.InsertTodayTask(ctx, q, t); err != nil {
					return err
				}
				if err := runtime.InsertSnapshotTask(ctx, q, newSnap.ID, t.ID, t.Position); err != nil {
					return err
				}
			}
			for _, sid := range freshPlan.activate {
				if err := runtime.ActivateGovSubject(ctx, q, sid, now); err != nil {
					return err
				}
			}
			for _, r := range freshPlan.rotations {
				if err := catalog.SetGovPracticeRotation(ctx, q, r); err != nil {
					return err
				}
			}
			return nil
		})
		if txerr != nil {
			if isUniqueConflict(txerr) {
				latest, rerr := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
				if rerr != nil {
					return db.TodaySnapshot{}, nil, fmt.Errorf("engine: focus switch conflict: %w", txerr)
				}
				tasks, rerr := runtime.GetTodayTasks(ctx, database, latest.ID)
				if rerr != nil {
					return db.TodaySnapshot{}, nil, rerr
				}
				return latest, tasks, nil
			}
			return db.TodaySnapshot{}, nil, fmt.Errorf("engine: focus switch: %w", txerr)
		}
		return newSnap, newTasks, nil
	}
	prevTasks, err := runtime.GetTodayTasks(ctx, database, snap.ID)
	if err != nil {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: load today tasks: %w", err)
	}

	// Classify locked vs replaceable.
	var locked []db.TodayTask
	var replaceableIdx []int
	for i, t := range prevTasks {
		isLocked, err := isTaskLocked(ctx, database, t.ID)
		if err != nil {
			return db.TodaySnapshot{}, nil, err
		}
		if isLocked {
			locked = append(locked, t)
		} else {
			replaceableIdx = append(replaceableIdx, i)
		}
		_ = i
	}

	// Plan fresh candidates under the new focus (pure, no writes yet).
	newFocus := &db.GovExam{ID: newExamID}
	if e, err := catalog.GetGovExam(ctx, database, newExamID); err == nil {
		newFocus = &e
	}
	freshPlan, err := planDay(ctx, database, localDate, now, newFocus)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	// Split fresh plan by domain/kind to replace like-for-like positions:
	// lectures replace lectures, practice replace practice, etc.
	// Final-revision capacity (spec daily limits):
	// at most 4 lectures, 3 practice, 1 DSA across locked + fresh work.
	// Locked tasks are never dropped to satisfy a cap; fresh candidates
	// beyond a kind's remaining budget are left out (never duplicated to
	// fill). SWE capacity is the routine's own slot count for the date.
	freshByKind := map[string][]db.TodayTask{}
	for _, t := range freshPlan.tasks {
		k := string(t.Domain) + "/" + string(t.Kind)
		freshByKind[k] = append(freshByKind[k], t)
	}
	const (
		maxLectureTasks  = 4
		maxPracticeTasks = 3
		maxDSATasks      = 1
	)
	kindOf := func(t db.TodayTask) string { return string(t.Domain) + "/" + string(t.Kind) }
	lectureKey := string(db.DomainGOV) + "/" + string(db.SessionKindLecture)
	practiceKey := string(db.DomainGOV) + "/" + string(db.SessionKindPractice)
	dsaKey := string(db.DomainDSA) + "/" + string(db.SessionKindDSA)
	sweKey := string(db.DomainSWE) + "/" + string(db.SessionKindSWE)

	// Identities already carried by locked tasks. Fresh candidates that
	// would reassign the same one-time work are skipped: a globally
	// completed or in-progress lecture, an already-bundled DSA problem, or
	// an already-covered SWE area must never appear twice in one revision.
	// (Practice topics legitimately recur, so they are never deduped.)
	//
	// Daily GOV subject-allocation rule (V2): the unit of daily
	// deduplication is the subject allocation, not the individual lecture.
	// Each GOV subject has a shared daily allocation. Once that allocation
	// is locked for a date under any exam (completed, active, stopped
	// sessions, or partial evidence), no second task from that same subject
	// may be scheduled for that date, regardless of exam focus. Fresh
	// lecture candidates whose subject is already locked for this date are
	// therefore skipped even when they reference a different lecture ID.
	// Consumption is derived from this date's persisted plan + task state
	// (locked subjects), never from global lecture completion alone, so the
	// limit resets on the next local date.
	lockedLecture := map[db.ID]bool{}
	lockedLectureSubjects := map[db.ID]bool{}
	lockedDSAProblems := map[db.ID]bool{}
	lockedSWEAreas := map[db.ID]bool{}
	lockedCount := map[string]int{}
	for _, t := range locked {
		lockedCount[kindOf(t)]++
		if t.TargetJSON == nil {
			continue
		}
		raw := *t.TargetJSON
		switch {
		case t.Domain == db.DomainGOV && t.Kind == db.SessionKindLecture:
			if lt, err := db.UnmarshalGovLectureTarget(raw); err == nil {
				lockedLecture[lt.LectureID] = true
				if lt.SubjectID != "" {
					lockedLectureSubjects[lt.SubjectID] = true
				}
			}
		case t.Domain == db.DomainDSA:
			if dt, err := db.UnmarshalDSATarget(raw); err == nil {
				for _, it := range dt.Items() {
					lockedDSAProblems[it.ProblemID] = true
				}
			}
		case t.Domain == db.DomainSWE:
			if st, err := db.UnmarshalSWETarget(raw); err == nil {
				lockedSWEAreas[st.AreaID] = true
			}
		}
	}
	sweAreaOf := func(t db.TodayTask) (db.ID, bool) {
		if t.TargetJSON == nil {
			return "", false
		}
		st, err := db.UnmarshalSWETarget(*t.TargetJSON)
		if err != nil || st.AreaID == "" {
			return "", false
		}
		return st.AreaID, true
	}
	lectureIDOf := func(t db.TodayTask) (db.ID, bool) {
		if t.TargetJSON == nil {
			return "", false
		}
		lt, err := db.UnmarshalGovLectureTarget(*t.TargetJSON)
		if err != nil || lt.LectureID == "" {
			return "", false
		}
		return lt.LectureID, true
	}
	lectureSubjectOf := func(t db.TodayTask) (db.ID, bool) {
		if t.TargetJSON == nil {
			return "", false
		}
		lt, err := db.UnmarshalGovLectureTarget(*t.TargetJSON)
		if err != nil || lt.SubjectID == "" {
			return "", false
		}
		return lt.SubjectID, true
	}
	// Fresh lecture pool minus lectures already carried as locked, minus any
	// lecture whose subject already has a locked allocation for this date
	// (different lecture ID, same subject still consumes the daily slot).
	var lectPool []db.TodayTask
	for _, t := range freshByKind[lectureKey] {
		if id, ok := lectureIDOf(t); ok && lockedLecture[id] {
			continue
		}
		if sid, ok := lectureSubjectOf(t); ok && lockedLectureSubjects[sid] {
			continue
		}
		lectPool = append(lectPool, t)
	}
	pracPool := freshByKind[practiceKey]
	// Fresh DSA bundle minus locked problems; an emptied bundle is dropped.
	var dsaTask *db.TodayTask
	if pool := freshByKind[dsaKey]; len(pool) > 0 && pool[0].TargetJSON != nil {
		if dt, err := db.UnmarshalDSATarget(*pool[0].TargetJSON); err == nil {
			var kept []db.DSAProblemTarget
			for _, it := range dt.Items() {
				if !lockedDSAProblems[it.ProblemID] {
					kept = append(kept, it)
				}
			}
			if len(kept) > 0 {
				nt := pool[0]
				if len(kept) != len(dt.Items()) {
					ndt := db.DSATarget{Problems: kept}
					target, _ := db.MarshalTarget(ndt)
					tj := target
					nt.TargetJSON = &tj
					nt.Title = "DSA Practice • " + joinPlus(problemTitles(ctx, database, kept))
				}
				dsaTask = &nt
			}
		}
	}
	swePool := freshByKind[sweKey]
	lectBudget := max(0, maxLectureTasks-lockedCount[lectureKey])
	pracBudget := max(0, maxPracticeTasks-lockedCount[practiceKey])
	dsaBudget := max(0, maxDSATasks-lockedCount[dsaKey])
	sweBudget := max(0, len(swePool)-lockedCount[sweKey])
	li, pi, si := 0, 0, 0
	lectUsed, pracUsed, dsaUsed, sweUsed := 0, 0, false, 0
	takenSWEAreas := map[db.ID]bool{}
	for id := range lockedSWEAreas {
		takenSWEAreas[id] = true
	}
	// usedLectureSubjects starts with every locked subject so fresh picks
	// can never create a second allocation for the same subject, and tracks
	// fresh picks so surplus cannot duplicate a replacement subject either.
	usedLectureSubjects := map[db.ID]bool{}
	for sid := range lockedLectureSubjects {
		usedLectureSubjects[sid] = true
	}
	// takeFresh pops the next acceptable fresh item of the slot's kind, or
	// reports absence so the slot stays empty (never a duplicate fill).
	takeFresh := func(kind string) (db.TodayTask, bool) {
		switch kind {
		case lectureKey:
			for lectUsed < lectBudget && li < len(lectPool) {
				t := lectPool[li]
				li++
				if sid, ok := lectureSubjectOf(t); ok {
					if usedLectureSubjects[sid] {
						continue
					}
					usedLectureSubjects[sid] = true
				}
				lectUsed++
				return t, true
			}
		case practiceKey:
			if pracUsed < pracBudget && pi < len(pracPool) {
				t := pracPool[pi]
				pi++
				pracUsed++
				return t, true
			}
		case dsaKey:
			if !dsaUsed && dsaTask != nil && dsaBudget > 0 {
				dsaUsed = true
				return *dsaTask, true
			}
		case sweKey:
			for si < len(swePool) && sweUsed < sweBudget {
				t := swePool[si]
				si++
				area, ok := sweAreaOf(t)
				if ok && takenSWEAreas[area] {
					continue
				}
				if ok {
					takenSWEAreas[area] = true
				}
				sweUsed++
				return t, true
			}
		default:
			// Unknown kinds have no fresh pool: slots stay empty.
		}
		return db.TodayTask{}, false
	}
	nextTasks := make([]db.TodayTask, 0, len(prevTasks))
	// Preserve locked tasks in their original positions; fill replaceable
	// positions with fresh same-kind candidates when available.
	replaceablePos := map[int]bool{}
	for _, i := range replaceableIdx {
		replaceablePos[i] = true
	}
	for i, t := range prevTasks {
		if !replaceablePos[i] {
			nextTasks = append(nextTasks, t)
			continue
		}
		if nt, ok := takeFresh(kindOf(t)); ok {
			nextTasks = append(nextTasks, nt)
		}
		// Else: no fresh candidate of this kind within budget — leave the
		// slot empty (do not duplicate locked work to fill).
	}
	// Surplus fresh work of kinds the previous revision never had still
	// respects the same budgets (e.g. a newly configured practice subject).
	for _, kind := range []string{lectureKey, practiceKey, dsaKey, sweKey} {
		for {
			nt, ok := takeFresh(kind)
			if !ok {
				break
			}
			nextTasks = append(nextTasks, nt)
		}
	}

	rev := snap.Revision + 1
	newSnap := db.TodaySnapshot{
		ID:        db.ID(fmt.Sprintf("today-%s-r%d", localDate, rev)),
		LocalDate: localDate, Revision: rev, GeneratedAt: now,
	}
	// Assign new IDs only to replacement tasks; locked tasks keep IDs.
	lockedIDs := map[db.ID]bool{}
	for _, t := range locked {
		lockedIDs[t.ID] = true
	}
	finalTasks := make([]db.TodayTask, 0, len(nextTasks))
	for i, t := range nextTasks {
		if lockedIDs[t.ID] {
			t.Position = i + 1
			finalTasks = append(finalTasks, t)
			continue
		}
		t.SnapshotID = newSnap.ID
		t.Position = i + 1
		t.ID = db.ID(fmt.Sprintf("%s-task-%02d", string(newSnap.ID), i+1))
		finalTasks = append(finalTasks, t)
	}

	// Rotation cursors advance only for practice subjects actually present
	// in the committed revision: cursors for dropped surplus slots must not
	// move, or reopening and future plans would skip topics.
	finalPracticeSubjects := map[db.ID]bool{}
	for _, t := range finalTasks {
		if t.Domain != db.DomainGOV || t.Kind != db.SessionKindPractice || t.TargetJSON == nil {
			continue
		}
		if pt, err := db.UnmarshalGovPracticeTarget(*t.TargetJSON); err == nil {
			finalPracticeSubjects[pt.SubjectID] = true
		}
	}
	var keptRotations []db.GovPracticeRotation
	for _, r := range freshPlan.rotations {
		if finalPracticeSubjects[r.SubjectID] {
			keptRotations = append(keptRotations, r)
		}
	}

	err = database.WithTx(ctx, func(q db.DBTX) error {
		if err := catalog.SetGovFocusState(ctx, q, db.GovFocusState{ID: 1, FocusedExamID: &newExamID, UpdatedAt: now}); err != nil {
			return err
		}
		if err := runtime.CreateTodaySnapshot(ctx, q, newSnap); err != nil {
			return err
		}
		for _, t := range finalTasks {
			if lockedIDs[t.ID] {
				// Reused locked task: only link into the new revision.
				if err := runtime.InsertSnapshotTask(ctx, q, newSnap.ID, t.ID, t.Position); err != nil {
					return err
				}
				continue
			}
			if err := runtime.InsertTodayTask(ctx, q, t); err != nil {
				return err
			}
			if err := runtime.InsertSnapshotTask(ctx, q, newSnap.ID, t.ID, t.Position); err != nil {
				return err
			}
		}
		for _, sid := range freshPlan.activate {
			if err := runtime.ActivateGovSubject(ctx, q, sid, now); err != nil {
				return err
			}
		}
		for _, r := range keptRotations {
			if err := catalog.SetGovPracticeRotation(ctx, q, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if isUniqueConflict(err) {
			latest, rerr := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
			if rerr != nil {
				return db.TodaySnapshot{}, nil, fmt.Errorf("engine: focus switch conflict: %w", err)
			}
			tasks, rerr := runtime.GetTodayTasks(ctx, database, latest.ID)
			if rerr != nil {
				return db.TodaySnapshot{}, nil, rerr
			}
			return latest, tasks, nil
		}
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: focus switch: %w", err)
	}
	return newSnap, finalTasks, nil
}

// problemTitles resolves DSA problem titles for a rebuilt bundle, falling
// back to problem IDs when catalogue rows are missing (never invent titles).
func problemTitles(ctx context.Context, q db.DBTX, items []db.DSAProblemTarget) []string {
	byID := map[db.ID]string{}
	if probs, err := catalog.GetDSAProblems(ctx, q); err == nil {
		for _, p := range probs {
			byID[p.ID] = p.Title
		}
	}
	titles := make([]string, 0, len(items))
	for _, it := range items {
		if title, ok := byID[it.ProblemID]; ok {
			titles = append(titles, title)
		} else {
			titles = append(titles, string(it.ProblemID))
		}
	}
	return titles
}

// isTaskLocked reports whether a task has any execution history:
// completed state, active session, stopped sessions, or partial evidence.
func isTaskLocked(ctx context.Context, q db.DBTX, taskID db.ID) (bool, error) {
	ev, err := EvaluateTodayTask(ctx, q, taskID)
	if err != nil {
		return false, err
	}
	if ev.State == StateCompleted || ev.State == StateActive {
		return true, nil
	}
	if ev.Done > 0 {
		return true, nil
	}
	sessions, err := runtime.ListSessionsForTask(ctx, q, taskID)
	if err != nil {
		return false, err
	}
	return len(sessions) > 0, nil
}
