package tui

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	db "forge/internal/db"
)

func seedSWESlots(s *SWEScreen, slots []db.SWERoutineSlot) {
	s.slots = slots
	s.err = ""
}

// blankCardLine reports whether a stripped card line carries no content
// (only the card borders and padding).
func blankCardLine(ln string) bool {
	return strings.Trim(ln, " │") == ""
}

// leadingSpaces counts the spaces before the first non-space rune.
func leadingSpaces(ln string) int {
	n := 0
	for _, r := range ln {
		if r != ' ' {
			return n
		}
		n++
	}
	return n
}

// trailingSpaces counts the spaces after the last non-space rune.
func trailingSpaces(ln string) int {
	n := 0
	runes := []rune(ln)
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] != ' ' {
			return n
		}
		n++
	}
	return n
}

func TestSWEGridSevenDaysTwoRows(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 40
	app.goTab(TabSWE)
	s := app.swe
	var slots []db.SWERoutineSlot
	for wd := 1; wd <= 7; wd++ {
		slots = append(slots, db.SWERoutineSlot{ID: db.ID(string(rune('a' + wd))), Weekday: wd, Position: 0, AreaID: "a", DurationSeconds: 3600})
	}
	seedSWESlots(s, slots)
	v := stripANSIForTest(s.routineCardsView(140, 60))
	// 7 day cards, each once, in Monday..Sunday order.
	days := []string{"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"}
	prev := -1
	for _, day := range days {
		if strings.Count(v, day) != 1 {
			t.Fatalf("day %q must appear exactly once (one card per day):\n%s", day, v)
		}
		idx := strings.Index(v, day)
		if idx <= prev {
			t.Fatalf("days must stay in Monday..Sunday order:\n%s", v)
		}
		prev = idx
	}
	// Row 1 holds Monday–Thursday (4 cards), row 2 the rest (3 cards).
	lines := strings.Split(v, "\n")
	if got := strings.Count(lines[0], "┌"); got != 4 {
		t.Fatalf("first grid row must hold 4 day cards, got %d:\n%s", got, v)
	}
	if got := strings.Count(v, "┌"); got != 7 {
		t.Fatalf("grid must hold 7 day cards, got %d:\n%s", got, v)
	}
}

func TestSWECardDayTotalTopRight(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.goTab(TabSWE)
	s := app.swe
	s.areas = []db.SWEArea{{ID: "area-go", Name: "golang"}, {ID: "area-docker", Name: "docker"}}
	seedSWESlots(s, []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "area-go", DurationSeconds: 5400},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "area-docker", DurationSeconds: 1800},
	})
	v := stripANSIForTest(s.routineCardsView(140, 40))
	for _, want := range []string{"MONDAY", "Go", "Docker", "1h 30m", "30m"} {
		if !strings.Contains(v, want) {
			t.Fatalf("grid missing %q:\n%s", want, v)
		}
	}
	// Day total (5400+1800 = 2h) sits top-right of the day header line.
	found := false
	for _, ln := range strings.Split(v, "\n") {
		if strings.Contains(ln, "MONDAY") {
			found = true
			if !strings.HasSuffix(ln, "2h │") {
				t.Fatalf("day header must carry the day total top-right:\n%s", ln)
			}
		}
	}
	if !found {
		t.Fatalf("MONDAY header missing:\n%s", v)
	}
}

func TestSWECardTasksCenteredAndAligned(t *testing.T) {
	card := SWERoutineCard{
		Day: "MONDAY", Total: "2h",
		Tasks:  []SWECardTask{{Name: "Go", Dur: "1h 30m"}, {Name: "Docker", Dur: "30m"}},
		Width:  40,
		Height: 15,
		NameW:  6,
		DurW:   6,
	}.View()
	v := stripANSIForTest(card)
	lines := strings.Split(v, "\n")
	// Uniform card geometry: every line exactly the card width.
	for _, ln := range lines {
		if lipgloss.Width(ln) != 40 {
			t.Fatalf("every card line must span the card width (want 40, got %d):\n%s", lipgloss.Width(ln), v)
		}
	}
	// Fixed total height incl borders.
	if len(lines) != 15 {
		t.Fatalf("card height=%d want 15:\n%s", len(lines), v)
	}
	// Durations end at the same column (aligned).
	var ends []int
	for _, ln := range lines {
		for _, dur := range []string{"1h 30m", "30m"} {
			if i := strings.LastIndex(ln, dur); i >= 0 {
				ends = append(ends, i+len(dur))
			}
		}
	}
	if len(ends) == 0 {
		t.Fatalf("no durations found:\n%s", v)
	}
	for _, e := range ends[1:] {
		if e != ends[0] {
			t.Fatalf("durations must align (end cols %v):\n%s", ends, v)
		}
	}
	// Task block centered, not crowded into the upper-left: the Go row
	// carries symmetric breathing room inside the card (measured inside
	// the card borders, beyond the 1-cell card padding).
	var goLine string
	for _, ln := range lines {
		if strings.Contains(ln, "Go") {
			goLine = ln
		}
	}
	if goLine == "" {
		t.Fatalf("Go row missing:\n%s", v)
	}
	innerGo := goLine[strings.Index(goLine, "│")+len("│") : strings.LastIndex(goLine, "│")]
	lead, trail := leadingSpaces(innerGo)-1, trailingSpaces(innerGo)-1
	if lead < 1 || trail < 1 {
		t.Fatalf("task block must sit centered with breathing room (lead %d, trail %d):\n%s", lead, trail, v)
	}
	if d := lead - trail; d < -1 || d > 1 {
		t.Fatalf("task block must be centered (lead %d, trail %d):\n%s", lead, trail, v)
	}
	// Vertically centered: blanks above and below the 2-row task block.
	goIdx := -1
	for i, ln := range lines {
		if strings.Contains(ln, "Go") {
			goIdx = i
		}
	}
	if goIdx < 0 {
		t.Fatalf("Go row missing:\n%s", v)
	}
	above, below := 0, 0
	for i := goIdx - 1; i >= 0 && blankCardLine(lines[i]); i-- {
		above++
	}
	for i := goIdx + 2; i < len(lines) && blankCardLine(lines[i]); i++ {
		below++
	}
	if above < 1 || below < 1 {
		t.Fatalf("task block must be vertically centered (above %d, below %d):\n%s", above, below, v)
	}
}

func TestSWECardLongNamesKeepAlignment(t *testing.T) {
	card := SWERoutineCard{
		Day:   "MONDAY",
		Total: "1h",
		Tasks: []SWECardTask{
			{Name: "Linux (Arch) + My Own Setup Extra Long Descriptive Suffix Words", Dur: "1h"},
		},
		Width: 36,
	}.View()
	v := stripANSIForTest(card)
	if strings.Contains(v, "…") || strings.Contains(v, "...") {
		t.Fatalf("long task must wrap, not truncate:\n%s", v)
	}
	for _, w := range []string{"Linux", "Arch", "Setup"} {
		if !strings.Contains(v, w) {
			t.Fatalf("wrapped task must preserve word %q:\n%s", w, v)
		}
	}
	// Layout holds: uniform widths, duration on the first physical line.
	for _, ln := range strings.Split(v, "\n") {
		if lipgloss.Width(ln) != 36 {
			t.Fatalf("wrapped card must keep uniform widths (want 36, got %d):\n%s", lipgloss.Width(ln), v)
		}
	}
	found := false
	for _, ln := range strings.Split(v, "\n") {
		if strings.Contains(ln, "Linux") && strings.Contains(ln, "1h") {
			found = true
		}
	}
	if !found {
		t.Fatalf("duration must stay on the first physical line:\n%s", v)
	}
}

func TestSWEGridFillsVerticalSpace(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 40
	app.goTab(TabSWE)
	s := app.swe
	var slots []db.SWERoutineSlot
	for wd := 1; wd <= 7; wd++ {
		slots = append(slots,
			db.SWERoutineSlot{ID: db.ID(string(rune('a'+wd)) + "1"), Weekday: wd, Position: 0, AreaID: "a", DurationSeconds: 3600},
			db.SWERoutineSlot{ID: db.ID(string(rune('a'+wd)) + "2"), Weekday: wd, Position: 1, AreaID: "a", DurationSeconds: 3600},
		)
	}
	seedSWESlots(s, slots)
	v := stripANSIForTest(s.routineCardsView(140, 40))
	// Two uniform rows of 19 plus the inter-row gap: the grid fills the
	// area instead of floating at the top with empty space below.
	if got := len(strings.Split(v, "\n")); got != 39 {
		t.Fatalf("grid must fill vertical space (want 39 lines, got %d):\n%s", got, v)
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
	// Render-level: no list selection chrome inside the cards.
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

func TestSWEGridPreservesAllSlots(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 140, 40
	app.goTab(TabSWE)
	s := app.swe
	var slots []db.SWERoutineSlot
	for wd := 1; wd <= 7; wd++ {
		slots = append(slots,
			db.SWERoutineSlot{ID: db.ID(string(rune('a'+wd)) + "1"), Weekday: wd, Position: 0, AreaID: "a", DurationSeconds: 3600},
			db.SWERoutineSlot{ID: db.ID(string(rune('a'+wd)) + "2"), Weekday: wd, Position: 1, AreaID: "a", DurationSeconds: 3600},
		)
	}
	seedSWESlots(s, slots)
	v := stripANSIForTest(s.routineCardsView(140, 60))
	// 14 slots ("1h" each) + 7 day totals ("2h" each): nothing may be lost.
	if got := strings.Count(v, "1h"); got != 14 {
		t.Fatalf("grid must preserve all 14 slots (want 14x 1h, got %d):\n%s", got, v)
	}
	if got := strings.Count(v, "2h"); got != 7 {
		t.Fatalf("grid must show all 7 day totals (want 7x 2h, got %d):\n%s", got, v)
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
