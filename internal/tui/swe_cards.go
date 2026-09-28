package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	db "forge/internal/db"
)

// SWECardSlot is one routine slot inside a day card (display-only view
// of db.SWERoutineSlot; storage values unchanged).
type SWECardSlot struct {
	ID              string
	Position        int
	DurationSeconds int
	AreaID          string
	Area            string // display name (SWEDisplayName), empty when unknown
}

// SWECardTask is one resolved task row inside a day card: display name
// plus duration text.
type SWECardTask struct {
	Name string
	Dur  string
}

// SWERoutineCard is a compact READ-ONLY day card for one weekday's routine
// slots: day name top-left with the day total top-right, a thin divider,
// then the task block centered horizontally and vertically in the
// remaining body. Task durations share one fixed column (NameW/DurW,
// measured across the whole grid) so they align inside every card; long
// names wrap via wrapCell without moving the duration column. No selection
// state, no focus marker, no cursor: the card is static information.
type SWERoutineCard struct {
	Day    string // full weekday display name, e.g. "MONDAY"
	Total  string // day total, e.g. "2h"
	Tasks  []SWECardTask
	Width  int // total incl borders
	Height int // total incl borders; <=0 = natural height
	NameW  int // fixed name column (grid-wide); <=0 = widest task
	DurW   int // fixed duration column (grid-wide); <=0 = widest duration
}

// View renders the card: header + divider + centered task block. Fixed
// total height (see Height) so grid rows stay uniform and fill the
// available vertical space.
func (c SWERoutineCard) View() string {
	w := c.Width
	if w < 28 {
		w = 28
	}
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	nameW, durW := c.NameW, c.DurW
	if nameW <= 0 || durW <= 0 {
		for _, t := range c.Tasks {
			if w := lipgloss.Width(t.Name); w > nameW && c.NameW <= 0 {
				nameW = w
			}
			if w := lipgloss.Width(t.Dur); w > durW && c.DurW <= 0 {
				durW = w
			}
		}
	}
	// The name column flexes down to fit the card; durations never move.
	const gap = 2
	if nameW > inner-durW-gap {
		nameW = inner - durW - gap
	}
	if nameW < 1 {
		nameW = 1
	}
	if durW < 1 {
		durW = 1
	}
	// One row per task (name + duration); rows form a fixed-width block.
	var rows []string
	for _, t := range c.Tasks {
		for i, ln := range wrapCell(t.Name, nameW) {
			if i == 0 {
				rows = append(rows, thPrimary.Render(ln)+strings.Repeat(" ", gap)+thMeta.Render(padLeftToWidth(t.Dur, durW)))
			} else {
				rows = append(rows, thPrimary.Render(ln)+strings.Repeat(" ", gap+durW))
			}
		}
	}
	// Center the block horizontally; an overlong block stays left.
	blockW := nameW + gap + durW
	left := 0
	if blockW < inner {
		left = (inner - blockW) / 2
	}
	right := inner - left - blockW
	if right < 0 {
		right = 0
	}
	for i, ln := range rows {
		rows[i] = strings.Repeat(" ", left) + ln + strings.Repeat(" ", right)
	}
	content := []string{
		scheduleEdgeRow(thScreenTitle.Render(c.Day), thMeta.Render(c.Total), inner),
		RowDivider(inner),
	}
	if c.Height > 0 {
		bodyH := c.Height - 2 - len(content)
		if bodyH < 0 {
			bodyH = 0
		}
		top := 0
		if len(rows) < bodyH {
			top = (bodyH - len(rows)) / 2
		}
		for i := 0; i < top; i++ {
			content = append(content, "")
		}
		content = append(content, rows...)
		for len(content) < c.Height-2 {
			content = append(content, "")
		}
		if len(content) > c.Height-2 {
			content = content[:c.Height-2]
		}
	} else {
		content = append(content, rows...)
	}
	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cBorder).Padding(0, 1).Width(w)
	return style.Render(strings.Join(content, "\n"))
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
