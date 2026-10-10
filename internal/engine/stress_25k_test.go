package engine_test

// Large-scale stress helper (opt-in via STRESS25K=1).
// Generates deterministic synthetic history on disposable DBs only.
// Never touches the real user database.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	dbRuntime "forge/internal/db/runtime"
	"forge/internal/engine"
)

// buildMinimalCatalogue creates the small isolated catalogue required by the
// stress benchmarks: 3 exams sharing 4 subjects, a few topics/lectures per
// subject, a small DSA/SWE catalogue. It replaces the former dependency on
// the full production seed file.
func buildMinimalCatalogue(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	for _, s := range []db.GovSubject{
		{ID: "subj-quant", Name: "Quant", SortOrder: 1},
		{ID: "subj-reasoning", Name: "Reasoning", SortOrder: 2},
		{ID: "subj-english", Name: "English", SortOrder: 3},
		{ID: "subj-banking-fa", Name: "Banking Awareness", SortOrder: 4},
	} {
		if err := catalog.InsertGovSubject(ctx, d, s); err != nil {
			t.Fatal(err)
		}
	}
	topicCounts := map[db.ID]int{
		"subj-quant": 6, "subj-reasoning": 3, "subj-english": 3, "subj-banking-fa": 3,
	}
	for _, sid := range []db.ID{"subj-quant", "subj-reasoning", "subj-english", "subj-banking-fa"} {
		for tp := 0; tp < topicCounts[sid]; tp++ {
			tid := db.ID(fmt.Sprintf("st-topic-%s-%d", string(sid)[5:], tp))
			if err := catalog.InsertGovTopic(ctx, d, db.GovTopic{ID: tid, SubjectID: sid, Name: fmt.Sprintf("Topic %s %d", sid, tp), SortOrder: tp + 1}); err != nil {
				t.Fatal(err)
			}
			for l := 0; l < 2; l++ {
				if err := catalog.InsertGovLecture(ctx, d, db.GovLecture{ID: db.ID(fmt.Sprintf("st-lec-%s-%d-%d", string(sid)[5:], tp, l)), TopicID: tid, Title: fmt.Sprintf("Lec %d", l), SortOrder: l + 1}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, e := range []db.GovExam{
		{ID: "exam-sbi-po", Name: "SBI PO", SortOrder: 1},
		{ID: "exam-ibps-so-it", Name: "IBPS SO IT", SortOrder: 2},
		{ID: "exam-ibps-clerk", Name: "IBPS Clerk", SortOrder: 3},
	} {
		if err := catalog.InsertGovExam(ctx, d, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, eid := range []db.ID{"exam-sbi-po", "exam-ibps-so-it", "exam-ibps-clerk"} {
		for i, sid := range []db.ID{"subj-quant", "subj-reasoning", "subj-english", "subj-banking-fa"} {
			if err := catalog.InsertGovExamSubject(ctx, d, db.GovExamSubject{ExamID: eid, SubjectID: sid, SortOrder: i + 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, eid := range []db.ID{"exam-sbi-po", "exam-ibps-so-it", "exam-ibps-clerk"} {
		sortOrder := 1
		for _, sid := range []db.ID{"subj-quant", "subj-reasoning", "subj-english", "subj-banking-fa"} {
			topics, err := catalog.GetGovTopics(ctx, d, sid)
			if err != nil {
				t.Fatal(err)
			}
			for _, tp := range topics {
				if err := catalog.InsertGovExamSyllabus(ctx, d, db.GovExamSyllabus{ExamID: eid, TopicID: tp.ID, SortOrder: sortOrder}); err != nil {
					t.Fatal(err)
				}
				sortOrder++
			}
		}
	}
	for i := 0; i < 10; i++ {
		if err := catalog.InsertDSAProblem(ctx, d, db.DSAProblem{ID: db.ID(fmt.Sprintf("st-dsa-%02d", i)), Title: fmt.Sprintf("Problem %d", i), Pattern: "Hash", Difficulty: "EASY", SortOrder: 500 + i}); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.UpsertDSARoutine(ctx, d, db.DSARoutine{ID: 1, PlannedSeconds: 9000}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []db.SWEArea{{ID: "st-go", Name: "Go", SortOrder: 1}, {ID: "st-sql", Name: "SQL", SortOrder: 2}} {
		if err := catalog.InsertSWEArea(ctx, d, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "st-mon-1", Weekday: 1, Position: 0, AreaID: "st-go", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InsertSWERoutineSlot(ctx, d, db.SWERoutineSlot{ID: "st-mon-2", Weekday: 1, Position: 1, AreaID: "st-sql", DurationSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
}

func openStressDB(t *testing.T, path string) *db.DB {
	t.Helper()
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	return database
}

type stressCounts struct {
	sessions, tasks, snapshots, pracRecs, dsaAtts, completions, mocks int
	sizeBytes, walBytes                                               int64
}

func stressCountsOf(t *testing.T, ctx context.Context, d *db.DB) stressCounts {
	t.Helper()
	var c stressCounts
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&c.sessions)
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_tasks`).Scan(&c.tasks)
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_snapshots`).Scan(&c.snapshots)
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_practice_records`).Scan(&c.pracRecs)
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM dsa_attempts`).Scan(&c.dsaAtts)
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM gov_lecture_completions`).Scan(&c.completions)
	_ = d.QueryRowContext(ctx, `SELECT COUNT(*) FROM mocks`).Scan(&c.mocks)
	if st, err := os.Stat(d.Path()); err == nil {
		c.sizeBytes = st.Size()
	}
	if st, err := os.Stat(d.Path() + "-wal"); err == nil {
		c.walBytes = st.Size()
	}
	return c
}

// buildStressHistory generates nDates days × 10 sessions/day of valid history
// plus a current-day plan at curDate with focus exam-sbi-po.
func buildStressHistory(t *testing.T, ctx context.Context, d *db.DB, nDates int, base time.Time, curDate string) (completedLecture db.ID, completedTask db.ID) {
	t.Helper()
	// Cache valid lecture targets: first lecture of first topic per subject for exam-sbi-po.
	type lt struct {
		subj, topic, lec db.ID
		name             string
	}
	subjects := []db.ID{"subj-quant", "subj-reasoning", "subj-english", "subj-banking-fa"}
	var cands []lt
	for _, sid := range subjects {
		syll, err := catalog.GetGovExamSyllabusForSubject(ctx, d, "exam-sbi-po", sid)
		if err != nil || len(syll) == 0 {
			t.Fatal(err)
		}
		tid := syll[0].TopicID
		lecs, err := catalog.GetGovLectures(ctx, d, tid)
		if err != nil || len(lecs) == 0 {
			t.Fatalf("no lectures for %s", tid)
		}
		subj, _ := catalog.GetGovSubject(ctx, d, sid)
		tp, _ := catalog.GetGovTopic(ctx, d, tid)
		cands = append(cands, lt{subj: sid, topic: tid, lec: lecs[0].ID, name: fmt.Sprintf("%s • %s • %s", subj.Name, tp.Name, lecs[0].Title)})
	}
	// All lecture IDs for unique completions.
	var allLecs []db.GovLecture
	{
		topics, err := catalog.GetGovTopics(ctx, d, "subj-quant")
		if err != nil {
			t.Fatal(err)
		}
		for _, tp := range topics {
			lecs, _ := catalog.GetGovLectures(ctx, d, tp.ID)
			allLecs = append(allLecs, lecs...)
		}
	}
	probs, err := catalog.GetDSAProblems(ctx, d)
	if err != nil || len(probs) < 4 {
		t.Fatal("dsa problems")
	}
	areas, err := catalog.GetSWEAreas(ctx, d)
	if err != nil || len(areas) == 0 {
		t.Fatal("swe areas")
	}
	// Practice target: quant first topic (FK-valid regardless of eligibility).
	pracTopic := cands[0].topic
	pracSubj := cands[0].subj

	batch := 50
	for start := 0; start < nDates; start += batch {
		end := start + batch
		if end > nDates {
			end = nDates
		}
		if err := d.WithTx(ctx, func(q db.DBTX) error {
			for di := start; di < end; di++ {
				date := base.AddDate(0, 0, -(di + 30)).Format("2006-01-02")
				snapID := db.ID(fmt.Sprintf("st-snap-%05d", di))
				if err := dbRuntime.CreateTodaySnapshot(ctx, q, db.TodaySnapshot{ID: snapID, LocalDate: date, Revision: 1, GeneratedAt: base}); err != nil {
					return err
				}
				// 4 tasks: 2 lectures (distinct subjects), 1 practice, 1 DSA/SWE alternating.
				s1 := cands[di%len(cands)]
				s2 := cands[(di+1)%len(cands)]
				if s2.subj == s1.subj {
					s2 = cands[(di+2)%len(cands)]
				}
				t1t, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: s1.subj, TopicID: s1.topic, LectureID: s1.lec})
				t2t, _ := db.MarshalTarget(db.GovPracticeTarget{SubjectID: pracSubj, TopicIDs: []db.ID{pracTopic}})
				// DSA bundle: 2 REPEAT problems (no NEW-uniqueness risk).
				p1, p2 := probs[(di*2)%len(probs)], probs[(di*2+1)%len(probs)]
				dt, _ := db.MarshalTarget(db.DSATarget{Problems: []db.DSAProblemTarget{{ProblemID: p1.ID, Mode: db.DSAModeRepeat}, {ProblemID: p2.ID, Mode: db.DSAModeRepeat}}})
				tids := []db.ID{
					db.ID(fmt.Sprintf("st-task-%05d-0", di)),
					db.ID(fmt.Sprintf("st-task-%05d-1", di)),
					db.ID(fmt.Sprintf("st-task-%05d-2", di)),
					db.ID(fmt.Sprintf("st-task-%05d-3", di)),
				}
				tdefs := []db.TodayTask{
					{ID: tids[0], SnapshotID: snapID, Position: 1, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: s1.name, PlannedSeconds: 3600, TargetJSON: &t1t},
					{ID: tids[1], SnapshotID: snapID, Position: 2, Domain: db.DomainGOV, Kind: db.SessionKindPractice, Title: "Practice", PlannedSeconds: 3600, TargetJSON: &t2t},
					{ID: tids[2], SnapshotID: snapID, Position: 3, Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "DSA", PlannedSeconds: 9000, TargetJSON: &dt},
				}
				// 4th alternates SWE / second lecture to keep allocations valid (≤1/subject).
				if di%2 == 0 {
					area := areas[di%len(areas)]
					swt, _ := db.MarshalTarget(db.SWETarget{AreaID: area.ID})
					tdefs = append(tdefs, db.TodayTask{ID: tids[3], SnapshotID: snapID, Position: 4, Domain: db.DomainSWE, Kind: db.SessionKindSWE, Title: "SWE " + area.Name, PlannedSeconds: 3600, TargetJSON: &swt})
				} else {
					t2lec, _ := db.MarshalTarget(db.GovLectureTarget{SubjectID: s2.subj, TopicID: s2.topic, LectureID: s2.lec})
					tdefs = append(tdefs, db.TodayTask{ID: tids[3], SnapshotID: snapID, Position: 4, Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: s2.name, PlannedSeconds: 3600, TargetJSON: &t2lec})
				}
				for _, tk := range tdefs {
					if err := dbRuntime.InsertTodayTask(ctx, q, tk); err != nil {
						return err
					}
					if err := dbRuntime.InsertSnapshotTask(ctx, q, snapID, tk.ID, tk.Position); err != nil {
						return err
					}
				}
				// 10 sessions/day: 3+3+2+2 across tasks.
				dist := []int{3, 3, 2, 2}
				si := 0
				for ti, tk := range tdefs {
					for k := 0; k < dist[ti]; k++ {
						sessID := db.ID(fmt.Sprintf("st-sess-%05d-%d-%d", di, ti, k))
						st := base.AddDate(0, 0, -(di + 30)).Add(time.Duration(8*60+si*45) * time.Minute)
						sp := st.Add(30 * time.Minute)
						if err := dbRuntime.CreateSession(ctx, q, db.Session{ID: sessID, TodayTaskID: tk.ID, Domain: tk.Domain, Kind: tk.Kind, TitleSnapshot: tk.Title, PlannedSeconds: 3600, StartedAt: st, StoppedAt: &sp, Status: db.SessionStatusStopped}); err != nil {
							return err
						}
						switch tk.Kind {
						case db.SessionKindPractice:
							avg := 20.0
							if err := dbRuntime.InsertGovPracticeRecord(ctx, q, db.GovPracticeRecord{ID: db.ID(fmt.Sprintf("st-pr-%05d-%d-%d", di, ti, k)), SessionID: sessID, TopicID: pracTopic, TimeSpentSeconds: 600, Attempted: 10, Correct: 6, AvgQuestionSpeedSeconds: &avg, RecordedAt: sp}); err != nil {
								return err
							}
						case db.SessionKindDSA:
							prob := p1.ID
							if k%2 == 1 {
								prob = p2.ID
							}
							solve := 300
							if err := dbRuntime.InsertDSAAttempt(ctx, q, db.DSAAttempt{ID: db.ID(fmt.Sprintf("st-da-%05d-%d-%d", di, ti, k)), SessionID: sessID, ProblemID: prob, Mode: db.DSAModeRepeat, AttemptedAt: sp, Independent: true, Optimal: true, SolveTimeSeconds: &solve}); err != nil {
								return err
							}
						}
						si++
					}
				}
				// Unique lecture completions for first 150 days (distinct lectures).
				if di < 150 && di < len(allLecs) {
					lid := allLecs[di].ID
					// Completion session: reuse first session of task0.
					if err := dbRuntime.RecordLectureCompletion(ctx, q, db.GovLectureCompletion{LectureID: lid, SessionID: db.ID(fmt.Sprintf("st-sess-%05d-0-0", di)), CompletedAt: base}); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Mocks: 20 deterministic mocks for history loading.
	for m := 0; m < 20; m++ {
		mid := db.ID(fmt.Sprintf("st-mock-%03d", m))
		_, err := engine.CreateMockAtomic(ctx, d, db.Mock{ID: mid, ExamID: "exam-sbi-po", Name: fmt.Sprintf("Mock %d", m), MockDate: base.AddDate(0, 0, -(m + 40)).Format("2006-01-02"), CreatedAt: base}, nil, base)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Current-day plan with focus + one completed shared lecture.
	if _, err := engine.SetFocusExam(ctx, d, "exam-sbi-po", base); err != nil {
		t.Fatal(err)
	}
	snap, tasks, err := engine.GenerateToday(ctx, d, curDate, base)
	if err != nil {
		t.Fatal(err)
	}
	_ = snap
	// Complete first GOV lecture task found.
	for _, tk := range tasks {
		if tk.Domain == db.DomainGOV && tk.Kind == db.SessionKindLecture && tk.TargetJSON != nil {
			lt, _ := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
			sess, err := engine.StartTodayTask(ctx, d, tk.ID, base)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.StopActiveSession(ctx, d, base.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.SaveGovLectureEvidence(ctx, d, tk.ID, sess.ID, true, base.Add(2*time.Minute)); err != nil {
				t.Fatal(err)
			}
			return lt.LectureID, tk.ID
		}
	}
	t.Fatal("no lecture task on current date")
	return "", ""
}

func stressTimed(t *testing.T, label string, iters int, fn func()) (med, p95 time.Duration) {
	t.Helper()
	durs := make([]time.Duration, 0, iters)
	for i := 0; i < iters; i++ {
		s := time.Now()
		fn()
		durs = append(durs, time.Since(s))
	}
	sort.Slice(durs, func(a, b int) bool { return durs[a] < durs[b] })
	med = durs[len(durs)/2]
	idx := int(float64(len(durs)) * 0.95)
	if idx >= len(durs) {
		idx = len(durs) - 1
	}
	p95 = durs[idx]
	t.Logf("%s iters=%d median=%s p95=%s min=%s max=%s", label, iters, med, p95, durs[0], durs[len(durs)-1])
	return med, p95
}

func stressExplain(t *testing.T, ctx context.Context, d *db.DB, sql string) string {
	t.Helper()
	rows, err := d.QueryContext(ctx, "EXPLAIN QUERY PLAN "+sql)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var a, b, c, e string
		if err := rows.Scan(&a, &b, &c, &e); err != nil {
			t.Fatal(err)
		}
		plan += e + "|"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan
}

func stressIntegrity(t *testing.T, ctx context.Context, d *db.DB) {
	t.Helper()
	var v string
	rows, err := d.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		_ = rows.Scan(&v)
	}
	rows.Close()
	if v != "ok" {
		t.Fatalf("integrity=%q", v)
	}
	n := 0
	rows2, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	for rows2.Next() {
		n++
	}
	rows2.Close()
	if n != 0 {
		t.Fatalf("fk violations=%d", n)
	}
}

// TestStressScale runs one dataset size end-to-end. Guarded by STRESS25K=1 so
// the default suite stays fast. Writes explicit results for the final table.
func TestStressScale(t *testing.T) {
	if os.Getenv("STRESS25K") == "" {
		t.Skip("set STRESS25K=1")
	}
	for _, tc := range []struct {
		name   string
		nDates int
	}{
		{"small-80", 8},       // 8×10 = 80 sessions
		{"medium-3000", 300},  // 300×10 = 3000 sessions
		{"large-25000", 2500}, // 2500×10 = 25000 sessions
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 9, 21, 9, 0, 0, 0, engine.DefaultCalendar().Location())
			curDate := "2026-09-21"
			path := fmt.Sprintf("/tmp/opencode/stress/stress-%s.db", tc.name)
			genStart := time.Now()
			d2 := openStressDB(t, path)
			buildMinimalCatalogue(t, ctx, d2)
			completedLec, completedTask := buildStressHistory(t, ctx, d2, tc.nDates, base, curDate)
			genDur := time.Since(genStart)
			c := stressCountsOf(t, ctx, d2)
			t.Logf("DATASET %s dates=%d sessions=%d tasks=%d snaps=%d prac=%d dsa=%d compl=%d mocks=%d size=%d wal=%d gen=%s completedLec=%s completedTask=%s",
				tc.name, tc.nDates, c.sessions, c.tasks, c.snapshots, c.pracRecs, c.dsaAtts, c.completions, c.mocks, c.sizeBytes, c.walBytes, genDur, completedLec, completedTask)
			stressIntegrity(t, ctx, d2)
			// Warmup.
			_, _, _ = engine.EnsureToday(ctx, d2, curDate, base)
			// Benchmarks (read-only first).
			stressTimed(t, tc.name+"/EnsureToday-reopen", 20, func() {
				if _, _, err := engine.EnsureToday(ctx, d2, curDate, base); err != nil {
					t.Fatal(err)
				}
			})
			stressTimed(t, tc.name+"/BuildDailyReport", 10, func() {
				if _, err := engine.BuildDailyReport(ctx, d2, curDate); err != nil {
					t.Fatal(err)
				}
			})
			stressTimed(t, tc.name+"/BuildWeeklyDraft", 5, func() {
				if _, err := engine.BuildWeeklyDraft(ctx, d2, "2026-W38", "2026-09-15", curDate); err != nil {
					t.Fatal(err)
				}
			})
			stressTimed(t, tc.name+"/GovDashboardCoverage", 10, func() {
				exams, err := catalog.GetGovExams(ctx, d2)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range exams {
					if _, err := engine.ExamSubjectProgress(ctx, d2, e.ID); err != nil {
						t.Fatal(err)
					}
				}
			})
			// Mutating benchmarks on isolated dates (do not disturb curDate locks).
			for i := 0; i < 3; i++ {
				nd := fmt.Sprintf("2026-10-%02d", i+1)
				s := time.Now()
				if _, _, err := engine.GenerateToday(ctx, d2, nd, base.Add(time.Duration(i)*time.Hour)); err != nil {
					t.Fatal(err)
				}
				t.Logf("%s/GenerateToday-fresh %s took %s", tc.name, nd, time.Since(s))
			}
			// Focus switches on current date (locked path: completed lecture present).
			stressTimed(t, tc.name+"/SwitchFocus-locked", 4, func() {
				if _, _, err := engine.SwitchFocusExam(ctx, d2, "exam-ibps-so-it", curDate, time.Now()); err != nil {
					t.Fatal(err)
				}
				if _, _, err := engine.SwitchFocusExam(ctx, d2, "exam-sbi-po", curDate, time.Now()); err != nil {
					t.Fatal(err)
				}
			})
			// Subject-allocation invariant survives switches at scale.
			_, wtasks, err := engine.SwitchFocusExam(ctx, d2, "exam-ibps-so-it", curDate, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			seen := map[db.ID]int{}
			lecTotal := 0
			for _, tk := range wtasks {
				if tk.Domain != db.DomainGOV || tk.Kind != db.SessionKindLecture || tk.TargetJSON == nil {
					continue
				}
				lecTotal++
				lt, _ := db.UnmarshalGovLectureTarget(*tk.TargetJSON)
				seen[lt.SubjectID]++
			}
			for sid, n := range seen {
				if n != 1 {
					t.Fatalf("subject %q x%d at scale %s", sid, n, tc.name)
				}
			}
			if lecTotal > 4 {
				t.Fatalf("lectures=%d >4 at %s", lecTotal, tc.name)
			}
			found := false
			for _, tk := range wtasks {
				if tk.ID == completedTask {
					found = true
				}
			}
			if !found {
				t.Fatalf("completed task %q lost at %s", completedTask, tc.name)
			}
			if ev, _ := engine.EvaluateTodayTask(ctx, d2, completedTask); ev.State != engine.StateCompleted {
				t.Fatalf("completed task not COMPLETED at %s: %s", tc.name, ev.State)
			}
			// Failure injection: abort task inserts, verify all-or-nothing.
			rotBefore := ""
			{
				rows, _ := d2.QueryContext(ctx, `SELECT exam_id, subject_id, next_index FROM gov_practice_rotation ORDER BY exam_id, subject_id`)
				if rows != nil {
					for rows.Next() {
						var a, b string
						var n int
						_ = rows.Scan(&a, &b, &n)
						rotBefore += a + b
					}
					rows.Close()
				}
			}
			var tasksBefore, membersBefore int
			_ = d2.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_tasks`).Scan(&tasksBefore)
			_ = d2.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_snapshot_tasks`).Scan(&membersBefore)
			focusBefore, _ := engine.GetFocusExam(ctx, d2)
			if _, err := d2.ExecContext(ctx, `CREATE TRIGGER st_sab BEFORE INSERT ON today_tasks BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
				t.Fatal(err)
			}
			if _, _, err := engine.SwitchFocusExam(ctx, d2, "exam-ibps-clerk", curDate, time.Now()); err == nil {
				t.Fatal("sabotaged switch must fail")
			}
			focusAfter, _ := engine.GetFocusExam(ctx, d2)
			if focusBefore == nil || focusAfter == nil || focusBefore.ID != focusAfter.ID {
				t.Fatalf("focus changed on failure: %v -> %v", focusBefore, focusAfter)
			}
			var tasksAfter, membersAfter int
			_ = d2.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_tasks`).Scan(&tasksAfter)
			_ = d2.QueryRowContext(ctx, `SELECT COUNT(*) FROM today_snapshot_tasks`).Scan(&membersAfter)
			if tasksAfter != tasksBefore || membersAfter != membersBefore {
				t.Fatalf("tasks/members changed: %d/%d -> %d/%d", tasksBefore, membersBefore, tasksAfter, membersAfter)
			}
			if _, err := d2.ExecContext(ctx, `DROP TRIGGER st_sab`); err != nil {
				t.Fatal(err)
			}
			_ = rotBefore
			stressIntegrity(t, ctx, d2)
			// Query plans at scale.
			for _, q := range []string{
				`SELECT * FROM sessions WHERE today_task_id = 'x'`,
				`SELECT * FROM sessions`,
			} {
				t.Logf("PLAN %q => %s", q, stressExplain(t, ctx, d2, q))
			}
		})
	}
}

func BenchmarkEnsureTodaySmall(b *testing.B) {
	ctx := context.Background()
	base := time.Date(2026, 9, 21, 9, 0, 0, 0, engine.DefaultCalendar().Location())
	d := openEngineDBForBench(b)
	if _, err := engine.SetFocusExam(ctx, d, "e1", base); err != nil {
		b.Fatal(err)
	}
	if _, _, err := engine.GenerateToday(ctx, d, "2026-09-21", base); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := engine.EnsureToday(ctx, d, "2026-09-21", base); err != nil {
			b.Fatal(err)
		}
	}
}

func openEngineDBForBench(b *testing.B) *db.DB {
	b.Helper()
	dir, err := os.MkdirTemp("", "bench-eng")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(dir) })
	database, err := db.Open(filepath.Join(dir, "eng.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		b.Fatal(err)
	}
	// Minimal catalogue for benchmark.
	if err := catalog.InsertGovSubject(ctxBench, database, db.GovSubject{ID: "s1", Name: "S1", SortOrder: 1}); err != nil {
		b.Fatal(err)
	}
	if err := catalog.InsertGovTopic(ctxBench, database, db.GovTopic{ID: "t1", SubjectID: "s1", Name: "T1", SortOrder: 1}); err != nil {
		b.Fatal(err)
	}
	if err := catalog.InsertGovLecture(ctxBench, database, db.GovLecture{ID: "l1", TopicID: "t1", Title: "L1", SortOrder: 1}); err != nil {
		b.Fatal(err)
	}
	if err := catalog.InsertGovExam(ctxBench, database, db.GovExam{ID: "e1", Name: "E1", SortOrder: 1}); err != nil {
		b.Fatal(err)
	}
	if err := catalog.InsertGovExamSubject(ctxBench, database, db.GovExamSubject{ExamID: "e1", SubjectID: "s1", SortOrder: 1}); err != nil {
		b.Fatal(err)
	}
	if err := catalog.InsertGovExamSyllabus(ctxBench, database, db.GovExamSyllabus{ExamID: "e1", TopicID: "t1", SortOrder: 1}); err != nil {
		b.Fatal(err)
	}
	return database
}

var ctxBench = context.Background()
