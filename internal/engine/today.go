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

// todayGenMu serializes Today ensure/generation in-process so concurrent
// EnsureToday calls for one date converge on a single snapshot instead of
// racing check-then-insert into multiple revisions. The UNIQUE
// (local_date, revision) index remains the cross-process safety net; a
// collision there resolves to the existing authoritative snapshot, never
// a second revision.
var todayGenMu sync.Mutex

// GenerateToday creates a new Today snapshot revision for localDate with
// exact target_json per the locked scheduling rules. It preserves old
// snapshots (new revision) and never mutates sessions. On a uniqueness
// collision with a concurrent writer it returns the existing authoritative
// snapshot instead of a second revision (see EnsureToday).
//
// Generation is two-phase:
//
//	Phase 1 (PURE PLAN): read current state, determine selected lectures,
//	practice topics, DSA/SWE bundles, required activation changes and
//	rotation-state changes. No writes happen here.
//
//	Phase 2 (ONE TRANSACTION): create snapshot + Today tasks, persist
//	activation changes and rotation state, commit. Any failure rolls back
//	everything: no orphan activation, no consumed rotation, no partial
//	snapshot or tasks.
//
// Rules:
//   - GOV lecture: 3 x 1h slots, current exam (first incomplete in
//     sort_order), first incomplete subjects in exam subject order, first
//     incomplete exam-mapped topic per subject; activate if inactive.
//   - GOV practice: 3 x 1h, slot1 Quant, slot2 Reasoning, slot3 round-robin
//     among other active subjects with practice-eligible topics — all
//     restricted to the current exam's subject set.
//   - DSA: one 2h30 block, 2 repeat + 1 new, due/review state, catalogue order.
//   - SWE: one block per routine slot for the weekday (area + duration from
//     the slot). Each block identifies its SWE area, never a task.
func GenerateToday(ctx context.Context, database *db.DB, localDate string, now time.Time) (db.TodaySnapshot, []db.TodayTask, error) {
	cal, err := VerifyCalendarSettings(ctx, database)
	if err != nil {
		return db.TodaySnapshot{}, nil, err
	}
	if _, err := cal.StartOfDay(localDate); err != nil {
		return db.TodaySnapshot{}, nil, wrapErr("generate", ErrValidation, "bad local date")
	}
	plan, err := planDay(ctx, database, localDate, now)
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
		}
		for _, sid := range plan.activate {
			if err := runtime.ActivateGovSubject(ctx, q, sid, now); err != nil {
				return fmt.Errorf("engine: activate subject: %w", err)
			}
		}
		if plan.rotation != nil {
			if err := runtime.SetGovPracticeRotationState(ctx, q, *plan.rotation); err != nil {
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
	// Uniqueness collision: another writer won the date. Return the
	// existing authoritative snapshot (idempotent ensure) instead of
	// minting a second revision.
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
// when no snapshot row exists. A genuine no-row result means "generate";
// a real DB/query failure propagates and never triggers generation.
//
// Idempotent under repeated/concurrent calls: the check-and-generate
// sequence is serialized in-process, and a uniqueness collision against a
// concurrent writer resolves to the same authoritative snapshot instead of
// a second revision. Explicit regeneration still goes through
// GenerateToday.
func EnsureToday(ctx context.Context, database *db.DB, localDate string, now time.Time) (db.TodaySnapshot, []db.TodayTask, error) {
	todayGenMu.Lock()
	defer todayGenMu.Unlock()
	snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if err == nil {
		tasks, err := runtime.GetTodayTasks(ctx, database, snap.ID)
		if err != nil {
			return db.TodaySnapshot{}, nil, fmt.Errorf("engine: load today tasks: %w", err)
		}
		return snap, tasks, nil
	}
	if !isNotFoundErr(err) {
		return db.TodaySnapshot{}, nil, fmt.Errorf("engine: latest today snapshot: %w", err)
	}
	return GenerateToday(ctx, database, localDate, now)
}

func nextRevision(ctx context.Context, database *db.DB, localDate string) (int, error) {
	// Latest revision + 1, or 1 when no snapshot exists. DB failures
	// propagate; only a genuine no-row result means "first revision".
	snap, err := runtime.GetLatestTodaySnapshot(ctx, database, localDate)
	if err != nil {
		if isNotFoundErr(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("engine: latest today snapshot: %w", err)
	}
	return snap.Revision + 1, nil
}

// dayPlan is the pure Phase-1 output: tasks plus the state changes that
// Phase 2 must persist atomically alongside the snapshot.
type dayPlan struct {
	tasks    []db.TodayTask
	activate []db.ID // subjects to ensure active (idempotent)
	rotation *db.GovPracticeRotationState
}

func planDay(ctx context.Context, database *db.DB, localDate string, now time.Time) (*dayPlan, error) {
	plan := &dayPlan{}
	lec, activate, err := planLectureSlots(ctx, database)
	if err != nil {
		return nil, err
	}
	plan.tasks = append(plan.tasks, lec...)
	plan.activate = append(plan.activate, activate...)
	// Subjects activated by lecture selection count as active for the
	// practice phase of the same plan (they commit in the same tx).
	extraActive := map[db.ID]bool{}
	for _, sid := range plan.activate {
		extraActive[sid] = true
	}
	prac, rotation, err := planPracticeSlots(ctx, database, extraActive, now)
	if err != nil {
		return nil, err
	}
	plan.tasks = append(plan.tasks, prac...)
	plan.rotation = rotation
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

// planLectureSlots plans the GOV lecture block exam-wise: the current exam
// (first incomplete exam in sort_order) contributes its first up-to-three
// incomplete subjects in exam subject order, and each slot serves the first
// incomplete lecture of the first incomplete exam-mapped topic in that
// subject. Subjects already picked are never repeated the same day (each
// incomplete subject yields at most one slot). Picked subjects are
// collected for activation in the atomic commit. With no current exam (or
// no incomplete subjects) no lecture work is produced.
func planLectureSlots(ctx context.Context, database *db.DB) ([]db.TodayTask, []db.ID, error) {
	routine, err := catalog.GetGovLectureRoutine(ctx, database)
	if err != nil {
		return nil, nil, fmt.Errorf("engine: gov lecture routine: %w", err)
	}
	if len(routine) == 0 {
		// Default 3 x 1h when routine missing.
		routine = []db.GovLectureRoutineSlot{{SlotOrder: 1, DurationSeconds: 3600}, {SlotOrder: 2, DurationSeconds: 3600}, {SlotOrder: 3, DurationSeconds: 3600}}
	}
	cur, incompletes, err := CurrentGovSubjects(ctx, database)
	if err != nil {
		return nil, nil, err
	}
	if cur == nil || len(incompletes) == 0 {
		return nil, nil, nil
	}
	var out []db.TodayTask
	var activate []db.ID
	for i, slot := range routine {
		if i >= len(incompletes) {
			break
		}
		sp := incompletes[i]
		topic, lec, err := firstIncompleteExamLecture(ctx, database, cur.ID, sp.Subject.ID)
		if err != nil {
			return nil, nil, err
		}
		if topic == nil || lec == nil {
			continue // nothing servable in this subject; try the next one
		}
		// Collect the activation for the atomic commit instead of
		// writing it here: a later planning failure must not leave an
		// orphan activation behind.
		active, err := runtime.IsGovSubjectActive(ctx, database, sp.Subject.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: subject active check: %w", err)
		}
		if !active {
			activate = append(activate, sp.Subject.ID)
		}
		target, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: sp.Subject.ID, TopicID: topic.ID, LectureID: lec.ID})
		tj := target
		out = append(out, db.TodayTask{
			Domain: db.DomainGOV, Kind: db.SessionKindLecture,
			Title:          fmt.Sprintf("%s • %s • %s", sp.Subject.Name, topic.Name, lec.Title),
			PlannedSeconds: slot.DurationSeconds, TargetJSON: &tj,
		})
	}
	return out, activate, nil
}

// ---- practice ----

// PracticeEligibleTopics returns the practice-eligible topics of a subject
// in catalogue order. Locked rule: a topic is practice-eligible only when
// its lecture track is complete OR the topic has zero lectures. Every
// candidate topic is evaluated; anything else is excluded. DB failures
// propagate (never an empty list).
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
			// Zero lectures: track satisfied, practice unlocked.
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

// subjectIsActive reports activation including pending plan activations
// (subjects the same plan will activate in its atomic commit).
func subjectIsActive(ctx context.Context, q db.DBTX, subjectID db.ID, extra map[db.ID]bool) (bool, error) {
	if extra[subjectID] {
		return true, nil
	}
	return runtime.IsGovSubjectActive(ctx, q, subjectID)
}

func planPracticeSlots(ctx context.Context, database *db.DB, extraActive map[db.ID]bool, now time.Time) ([]db.TodayTask, *db.GovPracticeRotationState, error) {
	routine, err := catalog.GetGovPracticeRoutine(ctx, database)
	if err != nil {
		return nil, nil, fmt.Errorf("engine: gov practice routine: %w", err)
	}
	if len(routine) == 0 {
		return nil, nil, nil
	}
	sort.Slice(routine, func(i, j int) bool { return routine[i].SlotOrder < routine[j].SlotOrder })
	// Practice respects the current exam's subject set: only subjects
	// belonging to the current exam can be selected. With no current exam
	// no practice work is produced.
	cur, err := CurrentGovExam(ctx, database)
	if err != nil {
		return nil, nil, err
	}
	if cur == nil {
		return nil, nil, nil
	}
	memberSet, err := examMemberSet(ctx, database, cur.ID)
	if err != nil {
		return nil, nil, err
	}
	// Round-robin chaining within one plan: each pick advances the cursor
	// for the next round-robin slot; the final pick is the rotation write.
	rot, err := runtime.GetGovPracticeRotationState(ctx, database)
	last := db.ID("")
	haveRot := err == nil
	if err != nil && !isNotFoundErr(err) {
		return nil, nil, fmt.Errorf("engine: rotation state: %w", err)
	}
	if haveRot && rot.LastSubjectID != nil {
		last = *rot.LastSubjectID
	}
	var rotation *db.GovPracticeRotationState
	var out []db.TodayTask
	for _, slot := range routine {
		var subjectID db.ID
		switch slot.SelectionMode {
		case db.GovSelectionFixed:
			if slot.FixedSubjectID == nil {
				continue
			}
			if !memberSet[*slot.FixedSubjectID] {
				continue // fixed subject not in the current exam
			}
			active, err := subjectIsActive(ctx, database, *slot.FixedSubjectID, extraActive)
			if err != nil {
				return nil, nil, fmt.Errorf("engine: subject active: %w", err)
			}
			if !active {
				continue
			}
			subjectID = *slot.FixedSubjectID
		case db.GovSelectionRoundRobin:
			sid, err := pickRoundRobinSubject(ctx, database, extraActive, last, memberSet, cur.ID)
			if err != nil {
				return nil, nil, err
			}
			if sid == "" {
				continue
			}
			subjectID = sid
		default:
			continue
		}
		eligible, err := PracticeEligibleTopics(ctx, database, subjectID)
		if err != nil {
			return nil, nil, err
		}
		if len(eligible) == 0 {
			// No eligible topic: the subject is skipped for practice.
			continue
		}
		topics, err := NextPracticeTopics(ctx, database, eligible)
		if err != nil {
			return nil, nil, err
		}
		subj, err := catalog.GetGovSubject(ctx, database, subjectID)
		if err != nil {
			return nil, nil, fmt.Errorf("engine: gov subject: %w", err)
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
		// Collect the rotation write for the atomic commit instead of
		// persisting it here.
		if slot.SelectionMode == db.GovSelectionRoundRobin {
			last = subjectID
			cp := subjectID
			rotation = &db.GovPracticeRotationState{
				ID: 1, LastSubjectID: &cp, UpdatedAt: now,
			}
		}
	}
	return out, rotation, nil
}

func topicIDs(ts []db.GovTopic) []db.ID {
	out := make([]db.ID, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

// NextPracticeTopics selects the next up-to-3 topics for a practice block
// using SIMPLE PURE ROUND-ROBIN derived from EXISTING practice history.
//
// Eligible topics are already ordered by sort_order. The algorithm finds
// the last topic position reached by the most recent practice rotation and
// continues from the next topic, wrapping around:
//
//	A B C / D E F / G H I / A B C ...
//	A B C / D E F / G H I / J A B / C D E ... (10 topics)
//
// No new state table: the start offset is reconstructed from
// gov_practice_records (topic_id + recorded_at) for the eligible topics.
//   - no history -> start at 0 (first 3)
//   - history -> find max recorded_at among eligible topics' records,
//     take that timestamp's bundle, find its cyclic start (the member
//     whose predecessor is absent), advance by bundle size.
//   - fewer than 3 eligible -> all; exactly 3 -> all.
//   - new eligible topics slot into normal sort_order without resetting;
//     ineligible topics are skipped safely.
//
// Subject rotation is untouched; this only rotates topics inside the
// already-selected subject.
func NextPracticeTopics(ctx context.Context, q db.DBTX, eligible []db.GovTopic) ([]db.GovTopic, error) {
	if len(eligible) == 0 {
		return nil, nil
	}
	if len(eligible) <= 3 {
		out := make([]db.GovTopic, len(eligible))
		copy(out, eligible)
		return out, nil
	}
	ids := make([]db.ID, 0, len(eligible))
	indexOf := make(map[db.ID]int, len(eligible))
	for i, t := range eligible {
		ids = append(ids, t.ID)
		indexOf[t.ID] = i
	}
	recs, err := runtime.GetGovPracticeRecordsForTopics(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("engine: topic rotation history: %w", err)
	}
	if len(recs) == 0 {
		out := make([]db.GovTopic, 3)
		copy(out, eligible[:3])
		return out, nil
	}
	var maxTS int64 = -1
	for _, r := range recs {
		if ts := r.RecordedAt.Unix(); ts > maxTS {
			maxTS = ts
		}
	}
	inBundle := map[int]bool{}
	var bundleIdx []int
	for _, r := range recs {
		if r.RecordedAt.Unix() != maxTS {
			continue
		}
		if idx, ok := indexOf[r.TopicID]; ok {
			if !inBundle[idx] {
				inBundle[idx] = true
				bundleIdx = append(bundleIdx, idx)
			}
		}
	}
	if len(bundleIdx) == 0 {
		out := make([]db.GovTopic, 3)
		copy(out, eligible[:3])
		return out, nil
	}
	n := len(eligible)
	// Cyclic start: member whose predecessor (mod n) is absent from the bundle.
	start := -1
	for _, idx := range bundleIdx {
		prev := (idx - 1 + n) % n
		if !inBundle[prev] {
			start = idx
			break
		}
	}
	var next int
	if start >= 0 {
		next = (start + len(bundleIdx)) % n
	} else {
		// Non-contiguous (eligibility changed mid-history): fall back to
		// max index + 1 to skip safely without panic or stall.
		maxIdx := bundleIdx[0]
		for _, idx := range bundleIdx[1:] {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		next = (maxIdx + 1) % n
	}
	out := make([]db.GovTopic, 0, 3)
	for i := 0; i < 3; i++ {
		out = append(out, eligible[(next+i)%n])
	}
	return out, nil
}

// examMemberSet returns the subject IDs belonging to an exam.
func examMemberSet(ctx context.Context, q db.DBTX, examID db.ID) (map[db.ID]bool, error) {
	members, err := catalog.GetGovExamSubjects(ctx, q, examID)
	if err != nil {
		return nil, fmt.Errorf("engine: exam subjects: %w", err)
	}
	out := make(map[db.ID]bool, len(members))
	for _, m := range members {
		out[m.SubjectID] = true
	}
	return out, nil
}

// pickRoundRobinSubject selects the next active subject of the current exam
// (excluding Quant and Reasoning, excluding inactive subjects such as
// Computer Awareness before activation, excluding subjects outside the
// current exam) with at least one practice-eligible topic, cycling after
// lastSubjectID in exam subject order. Pure read: the caller persists the
// pick atomically.
func pickRoundRobinSubject(ctx context.Context, q db.DBTX, extraActive map[db.ID]bool, last db.ID, memberSet map[db.ID]bool, examID db.ID) (db.ID, error) {
	members, err := catalog.GetGovExamSubjects(ctx, q, examID)
	if err != nil {
		return "", fmt.Errorf("engine: exam subjects: %w", err)
	}
	quantID, reasID := db.ID(""), db.ID("")
	for _, m := range members {
		subj, err := catalog.GetGovSubject(ctx, q, m.SubjectID)
		if err != nil {
			if isNotFoundErr(err) {
				continue
			}
			return "", fmt.Errorf("engine: gov subject lookup: %w", err)
		}
		if subj.Name == "Quant" {
			quantID = subj.ID
		}
		if subj.Name == "Reasoning" {
			reasID = subj.ID
		}
	}
	var eligible []db.GovSubject
	for _, m := range members {
		if !memberSet[m.SubjectID] {
			continue
		}
		if m.SubjectID == quantID || m.SubjectID == reasID {
			continue
		}
		active, err := subjectIsActive(ctx, q, m.SubjectID, extraActive)
		if err != nil {
			return "", fmt.Errorf("engine: subject active: %w", err)
		}
		if !active {
			continue // inactive subjects cannot appear before activation
		}
		ets, err := PracticeEligibleTopics(ctx, q, m.SubjectID)
		if err != nil {
			return "", err
		}
		if len(ets) == 0 {
			continue // no eligible topic: subject skipped
		}
		subj, err := catalog.GetGovSubject(ctx, q, m.SubjectID)
		if err != nil {
			return "", fmt.Errorf("engine: gov subject lookup: %w", err)
		}
		eligible = append(eligible, subj)
	}
	if len(eligible) == 0 {
		return "", nil
	}
	// Next after last in eligible order (eligible preserves exam order).
	for i, s := range eligible {
		if s.ID == last && i+1 < len(eligible) {
			return eligible[i+1].ID, nil
		}
	}
	// Wrap or first when last not found.
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
	if r, err := catalog.GetDSARoutine(ctx, q); err != nil {
		if !isNotFoundErr(err) {
			return nil, fmt.Errorf("engine: dsa routine: %w", err)
		}
		// No routine row yet: keep the default. A genuine absence is
		// normal on fresh DBs; only real query failures propagate.
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
	// One new: first catalogue problem with no NEW attempt and no repeat
	// state (genuinely new), not in bundle.
	for _, p := range probs {
		if len(bundle) >= 3 {
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
		break
	}
	// Bootstrap/fill: remaining slots take genuinely-new problems labeled
	// NEW (truthful: never label an introduced problem NEW). If no valid
	// bundle can be formed, emit no DSA block rather than a fake one.
	for _, p := range probs {
		if len(bundle) >= 3 {
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
	if len(bundle) != 3 {
		return nil, nil
	}
	dt := db.DSATarget{Problems: bundle}
	if err := ValidateDSACurrentBundle(ctx, q, dt); err != nil {
		// Our own plan failed strict validation: emit nothing rather
		// than a mislabeled bundle.
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

// isGenuinelyNew reports whether a problem was never introduced: no NEW
// attempt and no repeat state. DB failures propagate (never default).
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

func allRepeatStates(ctx context.Context, q db.DBTX) ([]db.DSARepeatState, error) {
	// Far-future date returns all rows ordered by next_review_date.
	return runtime.ListDSARepeatDue(ctx, q, "9999-12-31")
}

// ---- SWE ----

// planSWEBlock generates one Today SWE block per routine slot for the day's
// weekday. Each slot resolves directly to its area: routine slot -> area ->
// Today block. Duration comes from the slot; the target identifies the area.
// No task lookup, no unfinished-task selection, no bundles, no completion
// state. The routine is authoritative for planned SWE work.
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
