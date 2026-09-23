package engine_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

func openEngineDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "eng.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	return database
}

func seedGov(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2},
		{ID: "subj-english", Name: "English", SortOrder: 3},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	topics := []db.GovTopic{
		{ID: "topic-perc", SubjectID: "subj-quant", Name: "Percentage", SortOrder: 1},
		{ID: "topic-ratio", SubjectID: "subj-quant", Name: "Ratio", SortOrder: 2},
		{ID: "topic-ns", SubjectID: "subj-quant", Name: "Number System", SortOrder: 3},
		{ID: "topic-series", SubjectID: "subj-reasoning", Name: "Series", SortOrder: 1},
	}
	for _, tp := range topics {
		if err := catalog.InsertGovTopic(ctx, d, tp); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lec-1", TopicID: "topic-perc", Title: "Core", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lec-series-1", TopicID: "topic-series", Title: "Core", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: "topic-eng1", SubjectID: "subj-english", Name: "Grammar", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: "lec-eng-1", TopicID: "topic-eng1", Title: "Core", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctx, d, db.GovExam{ID: "exam-1", Name: "E1", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for i, tid := range []db.ID{"topic-perc", "topic-ratio", "topic-ns", "topic-series", "topic-eng1"} {
		if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: "exam-1", TopicID: tid, SortOrder: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, ms := range []db.GovExamSubject{
		{ExamID: "exam-1", SubjectID: "subj-quant", SortOrder: 1},
		{ExamID: "exam-1", SubjectID: "subj-reasoning", SortOrder: 2},
		{ExamID: "exam-1", SubjectID: "subj-english", SortOrder: 3},
	} {
		if err := catalog.InsertGovExamSubject(ctx, d, ms); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertGovLectureRoutineSlot(ctx, d, db.GovLectureRoutineSlot{SlotOrder: 1, DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLectureRoutineSlot(ctx, d, db.GovLectureRoutineSlot{SlotOrder: 2, DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovLectureRoutineSlot(ctx, d, db.GovLectureRoutineSlot{SlotOrder: 3, DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	q := db.ID("subj-quant")
	rr := db.ID("subj-reasoning")
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 1, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &q}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 2, DurationSeconds: 3600, SelectionMode: db.GovSelectionFixed, FixedSubjectID: &rr}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertGovPracticeRoutineSlot(ctx, d, db.GovPracticeRoutineSlot{SlotOrder: 3, DurationSeconds: 3600, SelectionMode: db.GovSelectionRoundRobin}); err != nil {
		t.Fatal(err)
	}
}

func seedDSA(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for i, p := range []db.DSAProblem{
		{ID: "dsa-1", Title: "Two Sum", Pattern: "Hash", Difficulty: "EASY", SortOrder: 1},
		{ID: "dsa-2", Title: "Anagram", Pattern: "Hash", Difficulty: "EASY", SortOrder: 2},
		{ID: "dsa-3", Title: "Dup", Pattern: "Set", Difficulty: "EASY", SortOrder: 3},
		{ID: "dsa-4", Title: "Group", Pattern: "Hash", Difficulty: "MED", SortOrder: 4},
	} {
		_ = i
		if err := catalog.InsertDSAProblem(ctx, d, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.UpsertDSARoutine(ctx, d, db.DSARoutine{ID: 1, PlannedSeconds: 9000}); err != nil {
		t.Fatal(err)
	}
}

func seedSWE(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	if err := catalog.InsertSWEArea(ctx, d, db.SWEArea{ID: "area-go", Name: "Go", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWEArea(ctx, d, db.SWEArea{ID: "area-sql", Name: "SQL", SortOrder: 2}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-1", Weekday: 1, Position: 0, AreaID: "area-go", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "mon-2", Weekday: 1, Position: 1, AreaID: "area-sql", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
}

func mkTask(t *testing.T, ctx context.Context, d *db.DB, taskID, date string, domain db.Domain, kind db.SessionKind, target string) {
	t.Helper()
	now := time.Now()
	snapID := db.ID("snap-" + taskID)
	_ = runtime.CreateTodaySnapshot(ctx, d, db.TodaySnapshot{ID: snapID, LocalDate: date, Revision: 1, GeneratedAt: now})
	tj := target
	if err := runtime.InsertTodayTask(ctx, d, db.TodayTask{ID: db.ID(taskID), SnapshotID: snapID, Position: 1, Domain: domain, Kind: kind, Title: "T", PlannedSeconds: 3600, TargetJSON: &tj}); err != nil {
		t.Fatal(err)
	}
}

// ---- calendar ----

func TestCalendarKolkataMidnight(t *testing.T) {
	cal := engine.DefaultCalendar()
	// 2026-09-21 23:59 IST = Sep 21; 2026-09-22 00:00 IST = Sep 22.
	before := time.Date(2026, 9, 21, 23, 59, 0, 0, cal.Location())
	after := time.Date(2026, 9, 22, 0, 0, 0, 0, cal.Location())
	if got := cal.LocalDate(before); got != "2026-09-21" {
		t.Fatalf("before=%s", got)
	}
	if got := cal.LocalDate(after); got != "2026-09-22" {
		t.Fatalf("after=%s", got)
	}
	if cal.Location().String() != "Asia/Kolkata" {
		t.Fatalf("tz=%s", cal.Location())
	}
}

func TestCalendarCrossMidnightSplit(t *testing.T) {
	cal := engine.DefaultCalendar()
	start := time.Date(2026, 9, 21, 23, 50, 0, 0, cal.Location()).Unix()
	end := time.Date(2026, 9, 22, 0, 20, 0, 0, cal.Location()).Unix()
	parts := cal.SplitIntervalAtDayBoundaries(start, end)
	if parts["2026-09-21"] != 600 {
		t.Fatalf("sep21=%d", parts["2026-09-21"])
	}
	if parts["2026-09-22"] != 1200 {
		t.Fatalf("sep22=%d", parts["2026-09-22"])
	}
}

func TestCalendarWeekBoundaries(t *testing.T) {
	cal := engine.DefaultCalendar()
	// Monday 2026-09-21.
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, cal.Location())
	w := cal.CurrentWeek(now)
	if len(w) != 7 || w[0] != "2026-09-21" || w[6] != "2026-09-27" {
		t.Fatalf("week=%v", w)
	}
}

// ---- task state ----

func TestTaskTodoActiveCompleted(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "task1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	ev, err := engine.EvaluateTodayTask(ctx, d, "task1")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("want TODO got %s", ev.State)
	}
	now := time.Now()
	sess, err := engine.StartTodayTask(ctx, d, "task1", now)
	if err != nil {
		t.Fatal(err)
	}
	ev, err = engine.EvaluateTodayTask(ctx, d, "task1")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateActive {
		t.Fatalf("want ACTIVE got %s", ev.State)
	}
	if ev.Active == nil || ev.Active.ID != sess.ID {
		t.Fatal("active session missing")
	}
	closed, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != db.SessionStatusStopped {
		t.Fatal("not stopped")
	}
	// Stopping without evidence stays TODO.
	ev, err = engine.EvaluateTodayTask(ctx, d, "task1")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("stop without evidence must stay TODO, got %s", ev.State)
	}
}

func TestLectureYesNo(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "task1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "task1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Minute))
	// NO: no insert, stays TODO.
	ev, err := engine.SaveGovLectureEvidence(ctx, d, "task1", sess.ID, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("NO must stay TODO, got %s", ev.State)
	}
	// YES: completes.
	sess2, err := engine.StartTodayTask(ctx, d, "task1", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = engine.StopActiveSession(ctx, d, now.Add(3*time.Hour))
	ev, err = engine.SaveGovLectureEvidence(ctx, d, "task1", sess2.ID, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("YES must complete, got %s", ev.State)
	}
	// Duplicate blocked cleanly.
	sess3, _ := engine.StartTodayTask(ctx, d, "task1", now.Add(4*time.Hour))
	_ = sess3
	// Task is completed so start must fail; direct duplicate save also fails.
	_, err = engine.SaveGovLectureEvidence(ctx, d, "task1", sess2.ID, true, now)
	if err == nil {
		t.Fatal("duplicate lecture must fail")
	}
}

func TestPracticeExactBundle(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-perc", "topic-ratio", "topic-ns"}})
	mkTask(t, ctx, d, "task1", "2026-09-20", db.DomainGOV, db.SessionKindPractice, prac)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "task1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	avg := 20.0
	// Partial: 2 of 3 -> TODO.
	ev, err := engine.SaveGovPracticeEvidence(ctx, d, "task1", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-perc", TimeSecs: 1200, Attempted: 20, Correct: 16, AvgSpeed: &avg},
		{TopicID: "topic-ratio", TimeSecs: 900, Attempted: 15, Correct: 10, AvgSpeed: &avg},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("partial must be TODO, got %s", ev.State)
	}
	if len(ev.Missing) != 1 || ev.Missing[0] != "topic-ns" {
		t.Fatalf("missing=%v", ev.Missing)
	}
	// Second session saves the remainder (no duplication of satisfied).
	sess2, err := engine.StartTodayTask(ctx, d, "task1", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = engine.StopActiveSession(ctx, d, now.Add(3*time.Hour))
	ev, err = engine.SaveGovPracticeEvidence(ctx, d, "task1", sess2.ID, []engine.GovPracticeInput{
		{TopicID: "topic-ns", TimeSecs: 600, Attempted: 10, Correct: 5, AvgSpeed: &avg},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("exact must complete, got %s", ev.State)
	}
	// Duplicate satisfied rejected.
	sess3snap := sess2
	_ = sess3snap
	_, err = engine.SaveGovPracticeEvidence(ctx, d, "task1", sess2.ID, []engine.GovPracticeInput{
		{TopicID: "topic-perc", TimeSecs: 100, Attempted: 1, Correct: 1, AvgSpeed: &avg},
	}, now)
	if err == nil {
		t.Fatal("duplicate satisfied must fail")
	}
	// Invalid rolls back everything (bad correct>attempted writes nothing).
	mkTask(t, ctx, d, "task2", "2026-09-21", db.DomainGOV, db.SessionKindPractice, prac)
	s2, _ := engine.StartTodayTask(ctx, d, "task2", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	_, err = engine.SaveGovPracticeEvidence(ctx, d, "task2", s2.ID, []engine.GovPracticeInput{
		{TopicID: "topic-perc", TimeSecs: 100, Attempted: 1, Correct: 1, AvgSpeed: &avg},
		{TopicID: "topic-ratio", TimeSecs: 100, Attempted: 1, Correct: 5, AvgSpeed: &avg},
	}, now)
	if err == nil {
		t.Fatal("invalid must fail")
	}
	var n int
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_records WHERE session_id = ?`, string(s2.ID)).Scan(&n)
	if n != 0 {
		t.Fatalf("rollback failed, n=%d", n)
	}
}

func TestStartRejects(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: "subj-quant", TopicID: "topic-perc", LectureID: "lec-1"})
	mkTask(t, ctx, d, "t1", "2026-09-20", db.DomainGOV, db.SessionKindLecture, lec)
	mkTask(t, ctx, d, "t2", "2026-09-21", db.DomainGOV, db.SessionKindLecture, lec)
	now := time.Now()
	if _, err := engine.StartTodayTask(ctx, d, "t1", now); err != nil {
		t.Fatal(err)
	}
	// Same task cannot have 2 active.
	if _, err := engine.StartTodayTask(ctx, d, "t1", now); err == nil {
		t.Fatal("active task must reject start")
	}
	// Another task blocked while global active exists.
	if _, err := engine.StartTodayTask(ctx, d, "t2", now); err == nil {
		t.Fatal("global busy must reject")
	}
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
}

func TestPassRule(t *testing.T) {
	cases := []struct {
		a    db.DSAAttempt
		pass bool
	}{
		{db.DSAAttempt{Independent: true, Optimal: true, HintUsed: false}, true},
		{db.DSAAttempt{Independent: true, Optimal: false, HintUsed: false}, false},
		{db.DSAAttempt{Independent: false, Optimal: true, HintUsed: false}, false},
		{db.DSAAttempt{Independent: true, Optimal: true, HintUsed: true}, false},
	}
	for i, c := range cases {
		if got := engine.IsDSAPass(c.a); got != c.pass {
			t.Fatalf("case %d got %v", i, got)
		}
	}
}

func TestDSABundleAndRepeat(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedDSA(t, ctx, d)
	raw, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{
		{ProblemID: "dsa-1", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-2", Mode: db.DSAModeRepeat},
		{ProblemID: "dsa-3", Mode: db.DSAModeNew},
	}})
	mkTask(t, ctx, d, "tdsa", "2026-09-20", db.DomainDSA, db.SessionKindDSA, raw)
	now := time.Now()
	sess, _ := engine.StartTodayTask(ctx, d, "tdsa", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	// Partial: 1 of 3 -> TODO.
	_, err := engine.SaveDSAEvidence(ctx, d, "tdsa", sess.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-1", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 200},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	ev, _ := engine.EvaluateTodayTask(ctx, d, "tdsa")
	if ev.State != engine.StateTodo {
		t.Fatalf("partial DSA must be TODO, got %s", ev.State)
	}
	// Save remainder: bundle completes regardless of pass (fail still counts).
	sess2, _ := engine.StartTodayTask(ctx, d, "tdsa", now.Add(2*time.Hour))
	_, _ = engine.StopActiveSession(ctx, d, now.Add(3*time.Hour))
	ev, err = engine.SaveDSAEvidence(ctx, d, "tdsa", sess2.ID, []engine.DSAAttemptInput{
		{ProblemID: "dsa-2", Independent: false, Optimal: false, HintUsed: true, SolveSecs: 300},
		{ProblemID: "dsa-3", Independent: true, Optimal: true, HintUsed: false, SolveSecs: 400},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateCompleted {
		t.Fatalf("full DSA must complete, got %s", ev.State)
	}
	// Repeat state updated: dsa-1 passed repeat -> stage advanced; dsa-2 failed -> reset.
	rs1, err := runtime.GetDSARepeatState(ctx, d, "dsa-1")
	if err != nil {
		t.Fatalf("repeat state missing: %v", err)
	}
	if rs1.RepeatStage <= 0 {
		t.Fatalf("pass must advance, stage=%d", rs1.RepeatStage)
	}
	rs2, _ := runtime.GetDSARepeatState(ctx, d, "dsa-2")
	if rs2.RepeatStage != 0 {
		t.Fatalf("fail must reset, stage=%d", rs2.RepeatStage)
	}
	// New attempt entered scheduling.
	if _, err := runtime.GetDSARepeatState(ctx, d, "dsa-3"); err != nil {
		t.Fatalf("new must init repeat state: %v", err)
	}
}

func TestSWEAreaTargetValidation(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedSWE(t, ctx, d)
	raw, _ := db.MarshalTarget(db.SWETarget{AreaID: "area-go"})
	mkTask(t, ctx, d, "tswe", "2026-09-20", db.DomainSWE, db.SessionKindSWE, raw)
	now := time.Now()
	sess, err := engine.StartTodayTask(ctx, d, "tswe", now)
	if err != nil {
		t.Fatalf("SWE area block must start: %v", err)
	}
	closed, err := engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if closed.ElapsedSeconds() != 3600 {
		t.Fatalf("elapsed=%d want 3600", closed.ElapsedSeconds())
	}
	// SWE blocks never complete via evidence: stopping is sufficient and
	// the task stays TODO (time allocation, not task progression).
	ev, err := engine.EvaluateTodayTask(ctx, d, "tswe")
	if err != nil {
		t.Fatal(err)
	}
	if ev.State != engine.StateTodo {
		t.Fatalf("SWE block must stay TODO after stop, got %s", ev.State)
	}
	// Restarting the same block creates a normal new session.
	sess2, err := engine.StartTodayTask(ctx, d, "tswe", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("SWE restart must succeed: %v", err)
	}
	if sess2.ID == sess.ID {
		t.Fatal("restart must create a new session")
	}
	_, _ = engine.StopActiveSession(ctx, d, now.Add(3*time.Hour))
}

func TestTodayGeneration(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	now := time.Now()
	snap, tasks, err := engine.GenerateToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Revision != 1 {
		t.Fatalf("rev=%d", snap.Revision)
	}
	// Count lecture slots (3), practice (Quant only: Reasoning has no
	// eligible topic, slot-3 English has no eligible topic), dsa (1),
	// swe (2: one block per routine slot).
	var lec, prac, dsa, swe int
	for _, tk := range tasks {
		switch {
		case tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture:
			lec++
		case tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice:
			prac++
		case tk.Domain == db.DomainDSA:
			dsa++
		case tk.Domain == db.DomainSWE:
			swe++
		}
	}
	if lec != 3 {
		t.Fatalf("lectures=%d", lec)
	}
	if prac != 1 {
		t.Fatalf("practice=%d want 1 (only Quant has eligible topics)", prac)
	}
	if dsa != 1 || swe != 2 {
		t.Fatalf("dsa=%d swe=%d want 1/2", dsa, swe)
	}
	// Practice bundle holds only eligible topics: incomplete-lecture
	// Percentage excluded, zero-lecture Ratio + Number System included.
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindPractice {
			pt, _ := db.UnmarshalGovPracticeTarget(*tk.TargetJSON)
			if len(pt.TopicIDs) != 2 {
				t.Fatalf("practice topics=%v", pt.TopicIDs)
			}
			for _, tid := range pt.TopicIDs {
				if tid == "topic-perc" {
					t.Fatal("incomplete-lecture topic must be excluded")
				}
			}
		}
	} // DSA bundle: 3 distinct problems.
	for _, tk := range tasks {
		if tk.Domain == db.DomainDSA {
			dt, _ := db.UnmarshalDSATarget(*tk.TargetJSON)
			if len(dt.Items()) != 3 {
				t.Fatalf("dsa items=%d", len(dt.Items()))
			}
			seen := map[db.ID]bool{}
			for _, it := range dt.Items() {
				if seen[it.ProblemID] {
					t.Fatal("dup dsa problem")
				}
				seen[it.ProblemID] = true
			}
		}
	}
	// EnsureToday idempotent.
	snap2, _, err := engine.EnsureToday(ctx, d, "2026-09-21", now)
	if err != nil {
		t.Fatal(err)
	}
	if snap2.ID != snap.ID {
		t.Fatal("EnsureToday must not create duplicate revision")
	}
	// Lecture subjects distinct that day.
	seenSubj := map[db.ID]bool{}
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture {
			lt, _ := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
			if seenSubj[lt.SubjectID] {
				t.Fatal("dup lecture subject same day")
			}
			seenSubj[lt.SubjectID] = true
		}
	}
}

func TestMocksAtomic(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	now := time.Now()
	m := db.Mock{ID: "mock-1", ExamID: "exam-1", Name: "M1", MockDate: "2026-09-18", CreatedAt: now}
	_, err := engine.CreateMockAtomic(ctx, d, m, []engine.MockSectionInput{
		{Name: "Quant Sec", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-perc", "topic-ratio"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	// Invalid subject mapping rejected, nothing created.
	m2 := db.Mock{ID: "mock-2", ExamID: "exam-1", Name: "M2", MockDate: "2026-09-18", CreatedAt: now}
	_, err = engine.CreateMockAtomic(ctx, d, m2, []engine.MockSectionInput{
		{Name: "Bad", GovSubjectID: "subj-quant", WeakTopics: []db.ID{"topic-series"}},
	}, now)
	if err == nil {
		t.Fatal("off-subject weak topic must fail")
	}
	if _, err := runtime.GetMock(ctx, d, "mock-2"); err == nil {
		t.Fatal("failed mock must roll back")
	}
	// Metric validation.
	bad := 200
	tot := 100
	m3 := db.Mock{ID: "mock-3", ExamID: "exam-1", Name: "M3", MockDate: "2026-09-18", Score: &bad, TotalScore: &tot, CreatedAt: now}
	if _, err := engine.CreateMockAtomic(ctx, d, m3, nil, now); err == nil {
		t.Fatal("score>total must fail")
	}
}

func TestReports(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	prac, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-perc"}})
	mkTask(t, ctx, d, "rep1", "2026-09-22", db.DomainGOV, db.SessionKindPractice, prac)
	cal := engine.DefaultCalendar()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, cal.Location())
	sess, _ := engine.StartTodayTask(ctx, d, "rep1", now)
	_, _ = engine.StopActiveSession(ctx, d, now.Add(time.Hour))
	avg := 20.0
	if _, err := engine.SaveGovPracticeEvidence(ctx, d, "rep1", sess.ID, []engine.GovPracticeInput{
		{TopicID: "topic-perc", TimeSecs: 1200, Attempted: 20, Correct: 16, AvgSpeed: &avg},
	}, now); err != nil {
		t.Fatal(err)
	}
	rep, err := engine.BuildDailyReport(ctx, d, "2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if rep.TasksTotal != 1 || rep.TasksCompleted != 1 {
		t.Fatalf("tasks=%d/%d", rep.TasksCompleted, rep.TasksTotal)
	}
	if len(rep.Activities) != 1 {
		t.Fatalf("activities=%d", len(rep.Activities))
	}
	if rep.Gov.QuestionsAttempted != 20 || rep.Gov.QuestionsCorrect != 16 {
		t.Fatalf("gov=%+v", rep.Gov)
	}
	if rep.Gov.Accuracy == nil {
		t.Fatal("accuracy nil")
	}
	// Weekly uses latest snapshot per day.
	w, err := engine.BuildWeeklyDraft(ctx, d, "2026-W38", "2026-09-21", "2026-09-27")
	if err != nil {
		t.Fatal(err)
	}
	if w.Planned <= 0 {
		t.Fatal("weekly planned missing")
	}
	// Stored snapshot stable.
	js, _ := engine.MarshalWeeklySnapshot(w)
	back, err := engine.UnmarshalWeeklySnapshot(js)
	if err != nil || back.Planned != w.Planned {
		t.Fatal("snapshot round trip failed")
	}
}

func TestWeightedSpeed(t *testing.T) {
	a1 := 10.0
	a2 := 20.0
	got := engine.WeightedAvgSpeed([]struct {
		Attempted int
		AvgSpeed  *float64
	}{{10, &a1}, {100, &a2}})
	// (10*10 + 100*20)/110 = 2100/110 ≈ 19.09, not 15.
	if got == nil || *got < 19.0 || *got > 19.2 {
		t.Fatalf("weighted=%v", got)
	}
}

func TestTargetValidation(t *testing.T) {
	d := openEngineDB(t)
	ctx := context.Background()
	seedGov(t, ctx, d)
	seedDSA(t, ctx, d)
	seedSWE(t, ctx, d)
	if err := engine.ValidateGovPracticeTarget(ctx, d, db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: nil}); err == nil {
		t.Fatal("empty bundle must fail")
	}
	if err := engine.ValidateGovPracticeTarget(ctx, d, db.GovPracticeTarget{SubjectID: "subj-quant", TopicIDs: []db.ID{"topic-perc", "topic-perc"}}); err == nil {
		t.Fatal("dup must fail")
	}
	if err := engine.ValidateDSATarget(ctx, d, db.DSATarget{Problems: []db.DSAProblemTarget{{ProblemID: "nope", Mode: db.DSAModeNew}}}); err == nil {
		t.Fatal("unknown problem must fail")
	}
	if err := engine.ValidateSWETarget(ctx, d, db.SWETarget{AreaID: "nope"}); err == nil {
		t.Fatal("unknown SWE area must fail")
	}
	if err := engine.ValidateSWETarget(ctx, d, db.SWETarget{}); err == nil {
		t.Fatal("empty SWE area must fail")
	}
	if err := engine.ValidateSWETarget(ctx, d, db.SWETarget{AreaID: "area-go"}); err != nil {
		t.Fatalf("valid SWE area must pass: %v", err)
	}
}
