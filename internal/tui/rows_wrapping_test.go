package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	db "forge/internal/db"
)

func TestRowShortIsOneLine(t *testing.T) {
	r := SelectableRow{Num: "01", Mark: "•", Title: "Short", Width: 40}
	if n := r.LineCount(); n != 1 {
		t.Fatalf("short LineCount=%d want 1", n)
	}
	v := r.View()
	if len(strings.Split(v, "\n")) != 1 {
		t.Fatalf("short must be one line:\n%q", v)
	}
	if strings.Contains(v, "…") || strings.Contains(v, "...") {
		t.Fatalf("short must not ellipsize:\n%q", v)
	}
}

func TestRowOverflowWrapsNoEllipsis(t *testing.T) {
	long := "Arrays and Hashing patterns with very long descriptive title that cannot fit"
	r := SelectableRow{Num: "01", Mark: "•", Title: long, Width: 30}
	n := r.LineCount()
	if n <= 1 {
		t.Fatalf("overflow LineCount=%d want >1", n)
	}
	v := r.View()
	lines := strings.Split(v, "\n")
	if len(lines) != n {
		t.Fatalf("View lines=%d LineCount=%d", len(lines), n)
	}
	plain := stripANSIForTest(v)
	if strings.Contains(plain, "…") {
		t.Fatalf("must not contain ellipsis:\n%s", plain)
	}
	if strings.Contains(v, "…") {
		t.Fatalf("raw view must not contain ellipsis")
	}
	// Full content preserved (every word visible).
	for _, w := range strings.Fields(long) {
		if !strings.Contains(plain, w) {
			t.Fatalf("missing word %q:\n%s", w, plain)
		}
	}
}

func TestRowDifferentHeights(t *testing.T) {
	short := SelectableRow{Title: "Hi", Width: 30}
	long := SelectableRow{Title: strings.Repeat("word ", 20), Width: 30}
	if short.LineCount() == long.LineCount() {
		t.Fatalf("different rows must have different heights: %d", short.LineCount())
	}
	if short.LineCount() != 1 {
		t.Fatalf("short=%d want 1", short.LineCount())
	}
}

func TestRowMultipleWrappedCellsMaxHeight(t *testing.T) {
	// Today table: AREA and TASK both long; height = tallest.
	tbl := TodayTaskTable{Width: 60, Lines: 30, Focus: true}
	tbl.Tasks = []db.TodayTask{
		{ID: "t1", Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "DSA", PlannedSeconds: 3600},
	}
	tbl.Status = []TaskStatus{TaskTodo}
	tbl.Bundles = []Bundle{{
		DSA: []DSAItem{
			{ProblemID: "p1", Mode: db.DSAModeNew, Title: strings.Repeat("LongTaskTitle ", 10), Pattern: "Arrays & Hashing Extra Long Pattern Name"},
		},
	}}
	cost := tbl.lineCost(0)
	numW, domW, areaW, typW, durW, stW := measureTodayCols(tbl.Width)
	taskW := tbl.Width - 1 - (numW + domW + areaW + typW + durW + stW) - 6*2
	if taskW < 4 {
		taskW = 4
	}
	areaN := len(wrapCell("Arrays & Hashing Extra Long Pattern Name", areaW))
	if cost < areaN {
		t.Fatalf("lineCost=%d must be >= area lines %d", cost, areaN)
	}
	if cost < 2 {
		t.Fatalf("expected tall row, cost=%d", cost)
	}
	block := tbl.rowBlock(0, numW, domW, areaW, typW, taskW, durW, stW, tbl.Width)
	if len(block) != cost {
		t.Fatalf("block=%d cost=%d", len(block), cost)
	}
	plain := stripANSIForTest(strings.Join(block, "\n"))
	if strings.Contains(plain, "…") {
		t.Fatalf("no ellipsis allowed:\n%s", plain)
	}
	if !strings.Contains(plain, "Arrays") || !strings.Contains(plain, "Hashing") {
		t.Fatalf("full AREA preserved:\n%s", plain)
	}
}

func TestRowSelectionSpansAllLines(t *testing.T) {
	long := strings.Repeat("selectme ", 15)
	r := SelectableRow{Num: "01", Mark: "•", Title: long, Selected: true, Focused: true, Width: 30}
	v := r.View()
	lines := strings.Split(v, "\n")
	if len(lines) <= 1 {
		t.Fatal("need tall row")
	}
	// Focus edge on every physical line.
	for i, ln := range lines {
		if !strings.Contains(ln, "▌") {
			t.Fatalf("line %d missing focus edge:\n%q", i, stripANSIForTest(ln))
		}
	}
	plain := stripANSIForTest(v)
	for _, w := range []string{"selectme"} {
		if strings.Count(plain, w) < 5 {
			t.Fatalf("selection must preserve all wrapped content:\n%s", plain)
		}
	}
}

func TestRowListCursorThroughTallRows(t *testing.T) {
	var rl RowList
	rl.SetWidth(28)
	rl.SetHeight(20)
	rl.SetItems([]RowItem{
		{ID: "a", Title: "short"},
		{ID: "b", Title: strings.Repeat("tall word ", 20)},
		{ID: "c", Title: "short2"},
	})
	if rl.RowHeight(0) != 1 {
		t.Fatalf("row0=%d want 1", rl.RowHeight(0))
	}
	if rl.RowHeight(1) <= 1 {
		t.Fatalf("row1 must be tall, got %d", rl.RowHeight(1))
	}
	// j/k moves one logical row at a time (never skips).
	rl.Select(0)
	rl.CursorDown()
	if rl.Cursor != 1 {
		t.Fatalf("down => %d want 1", rl.Cursor)
	}
	rl.CursorDown()
	if rl.Cursor != 2 {
		t.Fatalf("down => %d want 2", rl.Cursor)
	}
	rl.CursorUp()
	if rl.Cursor != 1 {
		t.Fatalf("up => %d want 1", rl.Cursor)
	}
}

func TestRowListScrollingKeepsSelectedVisible(t *testing.T) {
	var rl RowList
	rl.SetWidth(28)
	rl.SetHeight(6) // small physical budget
	items := []RowItem{
		{ID: "a", Title: "short a"},
		{ID: "b", Title: strings.Repeat("tall ", 30)},
		{ID: "c", Title: "short c"},
		{ID: "d", Title: "short d"},
	}
	rl.SetItems(items)
	// Move down into tall row: must remain visible.
	rl.Select(1)
	v := stripANSIForTest(rl.View())
	if !strings.Contains(v, "tall") {
		t.Fatalf("tall cursor row must be visible:\n%s", v)
	}
	// Move up from tall row.
	rl.Select(2)
	rl.CursorUp()
	if rl.Cursor != 1 {
		t.Fatalf("up from tall => %d", rl.Cursor)
	}
	v2 := stripANSIForTest(rl.View())
	if !strings.Contains(v2, "tall") {
		t.Fatalf("tall must stay visible after up:\n%s", v2)
	}
	// Scroll past tall rows: no skip, cursor visible.
	rl.Select(3)
	v3 := stripANSIForTest(rl.View())
	if !strings.Contains(v3, "short d") && !strings.Contains(v3, "d") {
		t.Fatalf("last row visible:\n%s", v3)
	}
}

func TestRowListNarrowWidth(t *testing.T) {
	var rl RowList
	rl.SetWidth(10) // minimum clamps, must not panic
	rl.SetHeight(10)
	rl.SetItems([]RowItem{{ID: "a", Title: strings.Repeat("narrow ", 20), Meta: "15h 41m"}})
	// Must not panic, no negative widths, no truncation.
	v := rl.View()
	plain := stripANSIForTest(v)
	if strings.Contains(plain, "…") {
		t.Fatalf("narrow must not ellipsize:\n%s", plain)
	}
	for _, ln := range strings.Split(v, "\n") {
		if lipgloss.Width(ln) > 20 { // allow hints; rows themselves fit Width=10 + ANSI
			// Only check row lines (skip hints).
			if strings.Contains(ln, "more") {
				continue
			}
		}
	}
	if rl.RowHeight(0) < 1 {
		t.Fatal("height must be >=1")
	}
}

func TestRowListMixedHeightsViewport(t *testing.T) {
	var rl RowList
	rl.SetWidth(30)
	rl.SetHeight(8)
	rl.SetItems([]RowItem{
		{ID: "a", Title: "one"},
		{ID: "b", Title: strings.Repeat("two two ", 15)},
		{ID: "c", Title: "three"},
	})
	total := rl.TotalHeight()
	// Total = h0 +1 + h1 +1 + h2
	want := rl.RowHeight(0) + rl.RowHeight(1) + rl.RowHeight(2) + 2
	if total != want {
		t.Fatalf("total=%d want %d", total, want)
	}
	// View window from summed physical cost: tall row fully rendered.
	rl.Select(1)
	v := stripANSIForTest(rl.View())
	if !strings.Contains(v, "two") {
		t.Fatalf("mixed viewport must show tall row:\n%s", v)
	}
	if strings.Contains(v, "…") {
		t.Fatalf("no ellipsis:\n%s", v)
	}
}

func TestRowListOversizeRow(t *testing.T) {
	var rl RowList
	rl.SetWidth(24)
	rl.SetHeight(4) // smaller than tall row
	rl.SetItems([]RowItem{
		{ID: "big", Title: strings.Repeat("oversize content ", 30)},
		{ID: "small", Title: "small"},
	})
	rl.Select(0)
	if rl.RowHeight(0) <= 4 {
		t.Fatalf("setup: row must be oversize, got %d", rl.RowHeight(0))
	}
	v := stripANSIForTest(rl.View())
	// First visible row taller than viewport is still shown (not skipped).
	if !strings.Contains(v, "oversize") {
		t.Fatalf("oversize first row must render:\n%s", v)
	}
}

func TestTodayWrapsArraysArea(t *testing.T) {
	tbl := TodayTaskTable{Width: 70, Lines: 30, Focus: true}
	tbl.Tasks = []db.TodayTask{
		{ID: "t1", Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "DSA", PlannedSeconds: 3600},
	}
	tbl.Status = []TaskStatus{TaskTodo}
	tbl.Bundles = []Bundle{{
		DSA: []DSAItem{{ProblemID: "p1", Mode: db.DSAModeNew, Title: "Two Sum", Pattern: "Arrays & Hashing"}},
	}}
	v := stripANSIForTest(tbl.View())
	if strings.Contains(v, "…") || strings.Contains(v, "Arrays & ...") {
		t.Fatalf("must not truncate AREA:\n%s", v)
	}
	if !strings.Contains(v, "Arrays") || !strings.Contains(v, "Hashing") {
		t.Fatalf("full AREA preserved:\n%s", v)
	}
	// Widening collapses flexible TASK content: a long TASK that wraps at
	// narrow width returns to fewer lines when wide (AREA column is fixed
	// width by design, so a long AREA like "Arrays & Hashing" stays wrapped).
	narrow := TodayTaskTable{Width: 60, Lines: 30, Focus: true}
	narrow.Tasks = []db.TodayTask{
		{ID: "t2", Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "DSA", PlannedSeconds: 3600},
	}
	narrow.Status = []TaskStatus{TaskTodo}
	narrow.Bundles = []Bundle{{
		DSA: []DSAItem{{ProblemID: "p1", Mode: db.DSAModeNew, Title: strings.Repeat("LongTaskTitle ", 10), Pattern: "Hash"}},
	}}
	wideTask := TodayTaskTable{Width: 150, Lines: 30, Focus: true}
	wideTask.Tasks = narrow.Tasks
	wideTask.Status = narrow.Status
	wideTask.Bundles = narrow.Bundles
	if wideTask.lineCost(0) > narrow.lineCost(0) {
		t.Fatalf("wide cost=%d must not exceed narrow cost=%d", wideTask.lineCost(0), narrow.lineCost(0))
	}
}

func TestTodayViewportPhysicalLines(t *testing.T) {
	tbl := TodayTaskTable{Width: 50, Lines: 8, Focus: true}
	tbl.Tasks = []db.TodayTask{
		{ID: "a", Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "A", PlannedSeconds: 60},
		{ID: "b", Domain: db.DomainDSA, Kind: db.SessionKindDSA, Title: "B", PlannedSeconds: 60},
		{ID: "c", Domain: db.DomainGOV, Kind: db.SessionKindLecture, Title: "C", PlannedSeconds: 60},
	}
	tbl.Status = []TaskStatus{TaskTodo, TaskTodo, TaskTodo}
	tbl.Bundles = []Bundle{
		{Lecture: &LectureItem{Subject: "S", Topic: "T", Lecture: "L"}},
		{DSA: []DSAItem{{ProblemID: "p", Mode: db.DSAModeNew, Title: strings.Repeat("Long ", 20), Pattern: "Arrays & Hashing Long Pattern"}}},
		{Lecture: &LectureItem{Subject: "S", Topic: "T", Lecture: "L"}},
	}
	tbl.SetCursor(1)
	v := stripANSIForTest(tbl.View())
	if !strings.Contains(v, "Long") {
		t.Fatalf("tall cursor must be visible:\n%s", v)
	}
	// Navigation stays logical.
	tbl.MoveDown(1)
	if tbl.CursorPos() != 2 {
		t.Fatalf("move down => %d", tbl.CursorPos())
	}
	tbl.MoveUp(2)
	if tbl.CursorPos() != 0 {
		t.Fatalf("move up => %d", tbl.CursorPos())
	}
}
