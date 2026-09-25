package tui

import (
	"reflect"
	"strings"
	"testing"

	db "forge/internal/db"
)

func TestSWEGridColsResponsive(t *testing.T) {
	// SWE read-only overview: 4 wide / 3 medium / 2 narrower / 1 very narrow.
	if got := sweGridCols(140); got != 4 {
		t.Fatalf("wide 140 => %d want 4", got)
	}
	if got := sweGridCols(160); got != 4 {
		t.Fatalf("wide 160 => %d want 4", got)
	}
	if got := sweGridCols(100); got != 3 {
		t.Fatalf("medium 100 => %d want 3", got)
	}
	if got := sweGridCols(90); got != 3 {
		t.Fatalf("medium 90 => %d want 3", got)
	}
	if got := sweGridCols(70); got != 2 {
		t.Fatalf("narrow 70 => %d want 2", got)
	}
	if got := sweGridCols(60); got != 2 {
		t.Fatalf("narrow 60 => %d want 2", got)
	}
	if got := sweGridCols(50); got != 1 {
		t.Fatalf("very narrow 50 => %d want 1", got)
	}
	if got := sweGridCols(40); got != 1 {
		t.Fatalf("very narrow 40 => %d want 1", got)
	}
	// Wide must differ from GOV (GOV uses 3 at 140): SWE prefers 4.
	if sweGridCols(140) == BentoCols(140) {
		t.Fatalf("SWE wide must prefer 4 cols, GOV uses %d", BentoCols(140))
	}
}

func seedSWESlots(s *SWEScreen, slots []db.SWERoutineSlot) {
	s.slots = slots
	s.gridCols = 0
	s.err = ""
}

func TestSWEGridMultipleCardsVisible(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 36
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 5400},
		{ID: "s2", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 1800},
		{ID: "s3", Weekday: 3, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s4", Weekday: 4, Position: 0, AreaID: "a", DurationSeconds: 2700},
	})
	// Full content width: wide terminal => 4 cards per row.
	v := stripANSIForTest(s.routineCardsView(140, 30))
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday"} {
		if !strings.Contains(v, day) {
			t.Fatalf("grid must show %s:\n%s", day, v)
		}
	}
	if s.gridCols != 4 {
		t.Fatalf("wide must use 4 cols, got %d", s.gridCols)
	}
	if lines := strings.Split(v, "\n"); len(lines) == 0 {
		t.Fatal("empty grid view")
	}
}

func TestSWEGridSingleDaySingleCard(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 5400},
	})
	v := stripANSIForTest(s.routineCardsView(120, 30))
	if !strings.Contains(v, "Monday") {
		t.Fatalf("single day card missing Monday:\n%s", v)
	}
	if !strings.Contains(v, "1h 30m") {
		t.Fatalf("single day card must show duration 1h 30m:\n%s", v)
	}
	// Read-only: no slot numbering.
	for _, bad := range []string{"Slot 1", "Slot 2", "SLOT"} {
		if strings.Contains(v, bad) {
			t.Fatalf("read-only card must not contain %q:\n%s", bad, v)
		}
	}
}

func TestSWEGridMultiSlotSameDayOneCard(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 5400},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "a", DurationSeconds: 1800},
		{ID: "s3", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	v := stripANSIForTest(s.routineCardsView(120, 30))
	// Monday appears once as a day card containing both slots.
	if strings.Count(v, "Monday") != 1 {
		t.Fatalf("Monday day card must appear once (both slots inside):\n%s", v)
	}
	// Both durations rendered inside the same card.
	if !strings.Contains(v, "1h 30m") || !strings.Contains(v, "30m") {
		t.Fatalf("day card must contain both durations:\n%s", v)
	}
	for _, bad := range []string{"Slot 1", "Slot 2", "SLOT", "SELECTED"} {
		if strings.Contains(v, bad) {
			t.Fatalf("read-only card must not contain %q:\n%s", bad, v)
		}
	}
}

func TestSWEGridSevenDaysSevenCards(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 40
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s2", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s3", Weekday: 3, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s4", Weekday: 4, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s5", Weekday: 5, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s6", Weekday: 6, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s7", Weekday: 7, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	v := stripANSIForTest(s.routineCardsView(140, 60))
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if strings.Count(v, day) != 1 {
			t.Fatalf("day %q must appear exactly once (one card per day):\n%s", day, v)
		}
	}
}

func TestSWEGridReadOnlyNoSlotLabels(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 40
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 5400},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "a", DurationSeconds: 1800},
		{ID: "s3", Weekday: 3, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	v := stripANSIForTest(s.routineCardsView(140, 40))
	for _, bad := range []string{"SLOT", "Slot 1", "Slot 2", "SELECTED", "▶", "●", "▌"} {
		if strings.Contains(v, bad) {
			t.Fatalf("read-only grid must not contain %q:\n%s", bad, v)
		}
	}
	// No per-slot cursor: durations present as plain info.
	if !strings.Contains(v, "1h 30m") {
		t.Fatalf("grid must still show durations read-only:\n%s", v)
	}
}

func TestSWEGridReadOnlyNoNavigation(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 130, 36
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "a", DurationSeconds: 1800},
		{ID: "s3", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	before := stripANSIForTest(s.routineCardsView(120, 30))
	for _, k := range []string{"j", "k", "h", "l", "up", "down", "left", "right", "enter"} {
		s.Update(testKey(k))
	}
	after := stripANSIForTest(s.routineCardsView(120, 30))
	if before != after {
		t.Fatalf("read-only grid must not move on navigation keys:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if strings.Contains(after, "●") || strings.Contains(after, "▶") || strings.Contains(after, "SELECTED") {
		t.Fatalf("read-only grid must have no cursor/selection:\n%s", after)
	}
}

func TestSWEGridNoNestedSelectableList(t *testing.T) {
	// Struct-level: no Navigator/RowList/cursor state on the screen.
	st := reflect.TypeOf(SWEScreen{})
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		ft := f.Type.String()
		if strings.Contains(ft, "Navigator") || strings.Contains(ft, "RowList") {
			t.Fatalf("SWEScreen must not embed selectable list (field %s %s)", f.Name, ft)
		}
		for _, bad := range []string{"slotList", "cardOffset", "focusR", "cursor", "Cursor"} {
			if f.Name == bad {
				t.Fatalf("SWEScreen must not keep %q state (read-only)", bad)
			}
		}
	}
	ct := reflect.TypeOf(SWERoutineCard{})
	for i := 0; i < ct.NumField(); i++ {
		f := ct.Field(i)
		for _, bad := range []string{"Focused", "Selected", "CursorSlotID", "Cursor", "Focus"} {
			if f.Name == bad {
				t.Fatalf("SWERoutineCard must be read-only (no %q field)", bad)
			}
		}
	}
	// Render-level: no list selection chrome inside cards.
	app := newFlowApp(t)
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "a", DurationSeconds: 1800},
	})
	v := stripANSIForTest(s.routineCardsView(120, 30))
	for _, bad := range []string{"▌", "▶", "●", "SELECTED"} {
		if strings.Contains(v, bad) {
			t.Fatalf("card contents must not use selectable-list chrome %q:\n%s", bad, v)
		}
	}
}

func TestSWEGridWideFourColumnsTwoRows(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 40
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s2", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s3", Weekday: 3, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s4", Weekday: 4, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s5", Weekday: 5, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s6", Weekday: 6, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s7", Weekday: 7, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	v := stripANSIForTest(s.routineCardsView(140, 40))
	if s.gridCols != 4 {
		t.Fatalf("wide 140 must use 4 cols, got %d", s.gridCols)
	}
	// 7 days in 4 cols => exactly 2 grid rows.
	wantRows := 2
	if got := (7 + s.gridCols - 1) / s.gridCols; got != wantRows {
		t.Fatalf("7 days in %d cols => %d rows, want %d", s.gridCols, got, wantRows)
	}
	for _, day := range []string{"Monday", "Sunday"} {
		if !strings.Contains(v, day) {
			t.Fatalf("wide grid must show %s without scrolling:\n%s", day, v)
		}
	}
	if strings.Contains(v, "more below") || strings.Contains(v, "more above") {
		t.Fatalf("wide grid with 2 rows must fit without scroll hints:\n%s", v)
	}
}

func TestSWEGridFitsWithoutScrollingWide(t *testing.T) {
	// Realistic weekly load: 7 days x 2 slots, compact cards, wide terminal.
	app := newFlowApp(t)
	app.width, app.height = 160, 36
	app.goTab(TabSWE)
	s := app.swe
	var slots []db.SWERoutineSlot
	for wd := 1; wd <= 7; wd++ {
		slots = append(slots,
			db.SWERoutineSlot{ID: db.ID(strings.Repeat("x", 0) + string(rune('a'+wd)) + "1"), Weekday: wd, Position: 0, AreaID: "a", DurationSeconds: 5400},
			db.SWERoutineSlot{ID: db.ID(string(rune('a'+wd)) + "2"), Weekday: wd, Position: 1, AreaID: "a", DurationSeconds: 1800},
		)
	}
	seedSWESlots(s, slots)
	// Typical content height for a 36-row terminal: 36 - 8 header - 3 footer = 25, minus 2 header lines = 23.
	v := stripANSIForTest(s.routineCardsView(160, 23))
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if !strings.Contains(v, day) {
			t.Fatalf("all 7 days must fit without scrolling, missing %s:\n%s", day, v)
		}
	}
	if strings.Contains(v, "more below") {
		t.Fatalf("compact 2-row grid must not need scrolling on wide:\n%s", v)
	}
}

func TestSWEGridNarrowReducesColumns(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 80, 30
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s2", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	v := stripANSIForTest(s.routineCardsView(70, 30))
	if !strings.Contains(v, "Monday") || !strings.Contains(v, "Tuesday") {
		t.Fatalf("narrow grid must still show both days:\n%s", v)
	}
	if got := sweGridCols(70); got != 2 {
		t.Fatalf("70 width must be 2 cols, got %d", got)
	}
	if s.gridCols != 2 {
		t.Fatalf("narrow render must use 2 cols, got %d", s.gridCols)
	}
	// Very narrow collapses to 1.
	_ = stripANSIForTest(s.routineCardsView(50, 30))
	if s.gridCols != 1 {
		t.Fatalf("very narrow 50 must use 1 col, got %d", s.gridCols)
	}
	if got := sweGridCols(50); got != 1 {
		t.Fatalf("sweGridCols(50)=%d want 1", got)
	}
}

func TestSWEGridMediumThreeColumns(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 100, 36
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s2", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s3", Weekday: 3, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	_ = stripANSIForTest(s.routineCardsView(100, 30))
	if s.gridCols != 3 {
		t.Fatalf("medium 100 must use 3 cols, got %d", s.gridCols)
	}
}

func TestSWEGridNoTwoPanelLayout(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 36
	app.goTab(TabSWE)
	v := stripANSIForTest(app.swe.View())
	// Full grid screen: compact header, no permanent split panels.
	if !strings.Contains(v, "SWE ROUTINE") {
		t.Fatalf("SWE view must show compact SWE ROUTINE header:\n%s", v)
	}
	for _, bad := range []string{"SWE Areas", "Weekly Routine", "Routine Slots"} {
		if strings.Contains(v, bad) {
			t.Fatalf("SWE view must not contain two-panel %q:\n%s", bad, v)
		}
	}
}

func TestSWEGridCompactHeights(t *testing.T) {
	// Compact: single-slot card is short, two-slot only slightly taller,
	// both well below the old GOV minimum (10 total lines).
	one := SWERoutineCard{Day: "Monday", Slots: []SWECardSlot{{ID: "s1", Position: 0, DurationSeconds: 3600}}, Width: 34}
	two := SWERoutineCard{Day: "Monday", Slots: []SWECardSlot{{ID: "s1", Position: 0, DurationSeconds: 3600}, {ID: "s2", Position: 1, DurationSeconds: 1800}}, Width: 34}
	h1, h2 := one.CardHeight(), two.CardHeight()
	if h1 >= 10 {
		t.Fatalf("single-slot card height=%d must be compact (<10, old GOV minimum)", h1)
	}
	if h2 >= 10 {
		t.Fatalf("two-slot card height=%d must be compact (<10)", h2)
	}
	if h2 <= h1 {
		t.Fatalf("two-slot (%d) must be taller than one-slot (%d)", h2, h1)
	}
	if h2-h1 > 3 {
		t.Fatalf("two-slot should be only slightly taller: one=%d two=%d", h1, h2)
	}
}

func TestSWEGridLongAreaWraps(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 36
	app.goTab(TabSWE)
	s := app.swe
	longArea := "Linux (Arch) + My Own Setup Extra Long Descriptive Suffix Words"
	s.areas = append(s.areas, db.SWEArea{ID: "ax", Name: longArea})
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "ax", DurationSeconds: 3600},
	})
	// Narrow card forces wrapping.
	v := stripANSIForTest(s.routineCardsView(70, 30))
	if strings.Contains(v, "…") || strings.Contains(v, "...") {
		t.Fatalf("long area must wrap, not truncate:\n%s", v)
	}
	for _, w := range []string{"Linux", "Arch", "Setup"} {
		if !strings.Contains(v, w) {
			t.Fatalf("wrapped card must preserve word %q:\n%s", w, v)
		}
	}
}

func TestSWEGridRowHeightsUniform(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 36
	app.goTab(TabSWE)
	s := app.swe
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "a", DurationSeconds: 3600},
		{ID: "s3", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
	})
	// Wide => 4 cols, first two cards share row 0.
	v := stripANSIForTest(s.routineCardsView(140, 40))
	lines := strings.Split(v, "\n")
	if !strings.Contains(lines[0], "Monday") && !strings.Contains(v, "Monday") {
		t.Fatalf("grid must show Monday:\n%s", v)
	}
	// Direct card-level check: same-row cards share one height.
	c1 := SWERoutineCard{Day: "Monday", Slots: []SWECardSlot{{ID: "s1", Position: 0, DurationSeconds: 3600}, {ID: "s2", Position: 1, DurationSeconds: 3600}}, Width: 40}
	c2 := SWERoutineCard{Day: "Tuesday", Slots: []SWECardSlot{{ID: "s3", Position: 0, DurationSeconds: 3600}}, Width: 40}
	h1, h2 := c1.CardHeight(), c2.CardHeight()
	if h1 == h2 {
		t.Fatalf("setup: heights should differ before equalization (%d)", h1)
	}
	rowMax := h1
	if h2 > rowMax {
		rowMax = h2
	}
	padded := SWERoutineCard{Day: "Tuesday", Slots: []SWECardSlot{{ID: "s3", Position: 0, DurationSeconds: 3600}}, Width: 40}.ViewWithHeight(rowMax)
	if got := len(strings.Split(padded, "\n")); got != rowMax {
		t.Fatalf("padded card height=%d want %d", got, rowMax)
	}
}

func TestSWEGridNoGovRegression(t *testing.T) {
	// GOV ExamCard still renders with its own conventions.
	gov := stripANSIForTest(ExamCard{Name: "SBI Clerk", Done: 1, Total: 2, Width: 34}.View())
	if !strings.Contains(gov, "SBI Clerk") || !strings.Contains(gov, "SYLLABUS COVERAGE") {
		t.Fatalf("GOV ExamCard regressed:\n%s", gov)
	}
	// Bento grid helpers still work for GOV.
	cards := []string{"a", "b", "c"}
	g := BentoGrid(cards, 2, "   ")
	if !strings.Contains(g, "a") || !strings.Contains(g, "c") {
		t.Fatalf("BentoGrid regressed: %q", g)
	}
	if BentoCols(140) != 3 || BentoCols(90) != 2 || BentoCols(60) != 1 {
		t.Fatal("BentoCols regressed")
	}
}
