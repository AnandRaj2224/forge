package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	db "forge/internal/db"
)

// SWECardSlot is one routine slot inside a day card (display-only view of
// db.SWERoutineSlot; storage values unchanged).
type SWECardSlot struct {
	ID              string
	Position        int
	DurationSeconds int
	AreaID          string
	Area            string // display name (SWEDisplayName), empty when unknown
}

// SWERoutineCard is a compact READ-ONLY GOV-style bento card for one
// weekday's routine slots. Visual language follows ExamCard: bordered card,
// same border treatment, same padding, same title treatment, same divider
// style. No selection state, no focus marker, no cursor: the card is static
// information (one day = one card, each entry is "<area>  <duration>").
type SWERoutineCard struct {
	Day   string // full weekday name, e.g. "Monday"
	Slots []SWECardSlot
	Width int
}

// View renders the card with wrapping (never truncating) slot content.
// Compact natural height: title + divider + one line per slot entry
// (wrapping long areas to more physical lines). No minimum padding.
func (c SWERoutineCard) View() string {
	return c.ViewWithHeight(0)
}

// ViewWithHeight renders the card padded to at least targetTotal total
// lines (including borders) so cards on the same grid row can share a
// uniform row height. targetTotal <= 0 means natural height.
func (c SWERoutineCard) ViewWithHeight(targetTotal int) string {
	w := c.Width
	if w < 28 {
		w = 28
	}
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	title := c.Day
	if title == "" {
		title = "Day"
	}
	content := []string{
		thScreenTitle.Render(TruncateToWidth(title, inner)),
		RowDivider(inner),
	}
	for _, sl := range c.Slots {
		content = append(content, c.slotLines(sl, inner)...)
	}
	// Uniform row height: pad shorter cards in the same grid row to the
	// row's max total height (borders add 2 lines). No minimum density:
	// one-slot cards stay short, two-slot cards slightly taller.
	if targetTotal > 0 {
		want := targetTotal - 2
		if want < len(content) {
			want = len(content)
		}
		for len(content) < want {
			content = append(content, "")
		}
	}
	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cBorder).Padding(0, 1).Width(w)
	return style.Render(strings.Join(content, "\n"))
}

// slotLines renders one routine entry as read-only text lines:
//
//	<display area>     <duration>
//
// Duration is right-aligned on the first physical line; long area names
// wrap via wrapCell (never truncated). No slot numbering, no section
// headers, no cursor, no selection.
func (c SWERoutineCard) slotLines(sl SWECardSlot, inner int) []string {
	area := sl.Area
	if area == "" && sl.AreaID != "" {
		area = SWEDisplayName(sl.AreaID)
	}
	if strings.TrimSpace(area) == "" {
		area = "—"
	}
	dur := FormatDurationLong(sl.DurationSeconds)
	durW := lipgloss.Width(dur)
	gap := 2
	titleW := inner - durW - gap
	if titleW < 1 {
		titleW = 1
	}
	wrapped := wrapCell(area, titleW)
	out := make([]string, 0, len(wrapped))
	for i, ln := range wrapped {
		if i == 0 {
			out = append(out, thPrimary.Render(ln)+strings.Repeat(" ", gap)+thMeta.Render(dur))
		} else {
			out = append(out, thPrimary.Render(ln)+strings.Repeat(" ", gap+durW))
		}
	}
	return out
}

// CardHeight returns the rendered total height (including borders).
func (c SWERoutineCard) CardHeight() int {
	return len(strings.Split(c.View(), "\n"))
}

// sweDayGroup groups routine slots by weekday preserving deterministic
// weekday/position order (slots are already ORDER BY weekday, position).
type sweDayGroup struct {
	Weekday int
	DayName string
	Slots   []SWECardSlot
	SlotIDs []string
}

// groupSWESlotsByDay groups slots by weekday in first-appearance order
// (which is weekday ascending given ORDER BY weekday, position). Never
// hardcodes a weekday; multiple slots on the same weekday stay inside that
// day's group; multiple weekdays produce multiple groups.
func groupSWESlotsByDay(slots []db.SWERoutineSlot) []sweDayGroup {
	var out []sweDayGroup
	idx := map[int]int{}
	for _, sl := range slots {
		cardSlot := SWECardSlot{ID: string(sl.ID), Position: sl.Position, DurationSeconds: sl.DurationSeconds, AreaID: string(sl.AreaID)}
		if i, ok := idx[sl.Weekday]; ok {
			out[i].Slots = append(out[i].Slots, cardSlot)
			out[i].SlotIDs = append(out[i].SlotIDs, string(sl.ID))
			continue
		}
		idx[sl.Weekday] = len(out)
		out = append(out, sweDayGroup{
			Weekday: sl.Weekday,
			DayName: weekdayFullName(sl.Weekday),
			Slots: []SWECardSlot{
				cardSlot,
			},
			SlotIDs: []string{string(sl.ID)},
		})
	}
	return out
}

// weekdayFullName returns the full weekday display name (Monday..Sunday).
// Storage weekdays are 1..7; unknown values render as Day N (never panic).
func weekdayFullName(wd int) string {
	switch wd {
	case 1:
		return "Monday"
	case 2:
		return "Tuesday"
	case 3:
		return "Wednesday"
	case 4:
		return "Thursday"
	case 5:
		return "Friday"
	case 6:
		return "Saturday"
	case 7:
		return "Sunday"
	default:
		return fmt.Sprintf("Day %d", wd)
	}
}
