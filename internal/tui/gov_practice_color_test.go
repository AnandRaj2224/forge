package tui

import (
	"strings"
	"testing"

	db "forge/internal/db"
)

func TestGovPracticeColor(t *testing.T) {
	// 1. DomainFgForWork checks
	govLecFg := DomainFgForWork("GOV", "LECTURE")
	govPracFg := DomainFgForWork("GOV", "PRACTICE")
	dsaFg := DomainFgForWork("DSA", "PRACTICE")

	if govLecFg != cGov {
		t.Fatalf("expected GOV LECTURE fg to be cGov (%v), got %v", cGov, govLecFg)
	}
	if govPracFg != cGovPractice {
		t.Fatalf("expected GOV PRACTICE fg to be cGovPractice (%v), got %v", cGovPractice, govPracFg)
	}
	if dsaFg != cDSA {
		t.Fatalf("expected DSA fg to be cDSA (%v), got %v", cDSA, dsaFg)
	}

	// 2. StatusPillForWork checks
	pillLec := StatusPillForWork(TaskCompleted, "GOV", "LECTURE")
	pillPrac := StatusPillForWork(TaskCompleted, "GOV", "PRACTICE")

	if pillLec == pillPrac {
		t.Fatalf("expected GOV LECTURE and GOV PRACTICE completion pills to have distinct styles")
	}

	// 3. Table row rendering check
	tbl := NewTodayTaskTable()
	tasks := []db.TodayTask{
		{
			ID:             "t1",
			Domain:         "GOV",
			Kind:           db.SessionKindLecture,
			Title:          "Lecture Task",
			PlannedSeconds: 3600,
		},
		{
			ID:             "t2",
			Domain:         "GOV",
			Kind:           db.SessionKindPractice,
			Title:          "Practice Task",
			PlannedSeconds: 3600,
		},
	}
	bundles := []Bundle{
		{
			Lecture: &LectureItem{
				Subject: "QUANT",
				Topic:   "Arithmetic",
			},
		},
		{
			Practice: []PracticeItem{
				{
					Subject: "QUANT",
					Topic:   "Number System",
				},
			},
		},
	}
	tbl.SetData(tasks, []TaskStatus{TaskTodo, TaskTodo}, bundles)

	lines1 := tbl.rowBlock(0, 3, 5, 10, 10, 30, 8, 8, 80)
	lines2 := tbl.rowBlock(1, 3, 5, 10, 10, 30, 8, 8, 80)

	if len(lines1) == 0 || len(lines2) == 0 {
		t.Fatalf("expected rendered lines")
	}

	raw1 := strings.Join(lines1, "\n")
	raw2 := strings.Join(lines2, "\n")
	if raw1 == raw2 {
		t.Fatalf("rows should not be identical")
	}
}
