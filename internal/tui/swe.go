package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/engine"
)

// ---------- SWE: read-only weekly overview day-card grid ----------
//
// Primary objects are DAYS: one day = one static read-only card across the
// full content area (no left Areas panel, no right Routine panel). There is
// no slot selection, no cursor, no per-slot navigation: the user simply
// looks at the weekly schedule.
type SWEScreen struct {
	app   *App
	areas []db.SWEArea
	slots []db.SWERoutineSlot
	// gridCols is the responsive grid width used by the last
	// routineCardsView render.
	gridCols int
	err      string
}

func NewSWEScreen(a *App) *SWEScreen {
	return &SWEScreen{app: a}
}

func (s *SWEScreen) Reload() {
	ctx := context.Background()
	areas, err := catalog.GetSWEAreas(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.areas = areas
	s.reloadSlots()
}

func (s *SWEScreen) reloadSlots() {
	ctx := context.Background()
	// Weekly routine grid: all slots grouped by weekday. The grid always
	// shows the full week so multiple day cards are visible with real DB
	// data (each weekday carries its slots).
	slots, err := catalog.GetSWERoutineAll(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		s.slots = nil
		return
	}
	s.slots = slots
}

// areaDisplayNames returns AreaID -> display name for enriching day cards.
func (s *SWEScreen) areaDisplayNames() map[string]string {
	m := map[string]string{}
	for _, a := range s.areas {
		m[string(a.ID)] = SWEDisplayName(a.Name)
	}
	return m
}

// onBack: read-only grid has no sub-state to pop. Always returns false so
// q/Esc bubbles to the shell.
func (s *SWEScreen) onBack() bool {
	return false
}

func weekdayName(wd int) string {
	switch wd {
	case 1:
		return "Mon"
	case 2:
		return "Tue"
	case 3:
		return "Wed"
	case 4:
		return "Thu"
	case 5:
		return "Fri"
	case 6:
		return "Sat"
	case 7:
		return "Sun"
	default:
		return fmt.Sprintf("Day %d", wd)
	}
}

// Update is a no-op: the routine overview is read-only. There is no slot
// cursor and no per-card navigation; j/k/h/l/arrows never move anything
// here. Global navigation (tabs, back, quit) is owned by the app shell.
func (s *SWEScreen) Update(msg tea.Msg) tea.Cmd {
	return nil
}

// View renders the SWE screen as a full read-only card grid: a compact
// header/summary followed directly by day cards across the whole content
// area. No left Areas panel, no right Routine panel.
func (s *SWEScreen) View() string {
	w, h := s.app.contentViewport()
	total := 0
	for _, sl := range s.slots {
		total += sl.DurationSeconds
	}
	header := thScreenTitle.Render("SWE ROUTINE") + "\n" +
		thMeta.Render(fmt.Sprintf("%s / week · %d slots", engine.FormatDurationLong(total), len(s.slots)))
	if s.err != "" {
		header += "\n" + thError.Render(s.err)
	}
	headLines := 2
	if s.err != "" {
		headLines = 3
	}
	availLines := h - headLines
	if availLines < 4 {
		availLines = 4
	}
	grid := s.routineCardsView(w, availLines)
	if grid == "" {
		return header
	}
	return header + "\n" + grid
}

// sweGridCols picks responsive grid columns from the full content width.
//
// Wide terminals prefer 4 columns so 7 days fit in 2 rows (4 + 3).
// Medium uses 3, narrower uses 2, very narrow uses 1. Thresholds derive
// from the minimum compact card width (28) plus the 3-cell grid gap, the
// same geometry BentoGridCardW uses:
//
//	4 cols need 4*28 + 3*3 = 121  ->  >= 130
//	3 cols need 3*28 + 2*3 = 90   ->  >= 90
//	2 cols need 2*28 + 1*3 = 59   ->  >= 60
func sweGridCols(availW int) int {
	if availW >= 130 {
		return 4
	}
	if availW >= 90 {
		return 3
	}
	if availW >= 60 {
		return 2
	}
	return 1
}

// routineCardsView renders day-wise compact read-only cards in a responsive
// grid (same visual family as the GOV exam dashboard: borders, padding,
// title + divider treatment, spacing). Each weekday gets exactly one static
// card; multiple slots on the same weekday stay inside that day's card.
// There is no cursor, no selection, no nested selectable list: card
// contents are plain read-only text lines.
func (s *SWEScreen) routineCardsView(availW, availLines int) string {
	if len(s.slots) == 0 {
		return EmptyState("No routine slots.")
	}
	if availW < 28 {
		availW = 28
	}
	if availLines < 4 {
		availLines = 4
	}
	groups := groupSWESlotsByDay(s.slots)
	if len(groups) == 0 {
		return EmptyState("No routine slots.")
	}
	// Enrich day-card slots with area display names (storage unchanged).
	areaDisp := s.areaDisplayNames()
	for gi := range groups {
		for si := range groups[gi].Slots {
			if disp, ok := areaDisp[groups[gi].Slots[si].AreaID]; ok && disp != "" {
				groups[gi].Slots[si].Area = disp
			} else if groups[gi].Slots[si].AreaID != "" {
				groups[gi].Slots[si].Area = SWEDisplayName(groups[gi].Slots[si].AreaID)
			}
		}
	}
	cols := sweGridCols(availW)
	if cols < 1 {
		cols = 1
	}
	s.gridCols = cols
	cardW := BentoGridCardW(availW, cols, 3)
	if cardW < 28 {
		cardW = 28
	}
	// Render all cards once to measure physical heights (read-only: no
	// focused or selected card, no cursor).
	rendered := make([]string, 0, len(groups))
	heights := make([]int, 0, len(groups))
	for _, g := range groups {
		card := SWERoutineCard{
			Day:   g.DayName,
			Slots: g.Slots,
			Width: cardW,
		}
		v := card.View()
		rendered = append(rendered, v)
		h := len(strings.Split(v, "\n"))
		if h < 1 {
			h = 1
		}
		heights = append(heights, h)
	}
	// Chunk groups into grid rows of cols cards (GOV dashboard pattern).
	gridRows := [][]int{}
	for i := 0; i < len(groups); i += cols {
		end := i + cols
		if end > len(groups) {
			end = len(groups)
		}
		row := make([]int, 0, end-i)
		for gi := i; gi < end; gi++ {
			row = append(row, gi)
		}
		gridRows = append(gridRows, row)
	}
	rowHeights := make([]int, len(gridRows))
	for ri, row := range gridRows {
		mx := 0
		for _, gi := range row {
			if heights[gi] > mx {
				mx = heights[gi]
			}
		}
		if mx < 1 {
			mx = 1
		}
		rowHeights[ri] = mx
	}
	// Uniform row height: pad shorter cards in the same visual row to the
	// row max so bottom borders align (no truncation, only blank padding).
	for ri, row := range gridRows {
		for _, gi := range row {
			if heights[gi] < rowHeights[ri] {
				g := groups[gi]
				padded := SWERoutineCard{
					Day:   g.DayName,
					Slots: g.Slots,
					Width: cardW,
				}.ViewWithHeight(rowHeights[ri])
				rendered[gi] = padded
				heights[gi] = rowHeights[ri]
			}
		}
	}
	// Visible grid rows from summed physical cost (gap = 1 blank line).
	// Read-only: always start at the first row; show as many full rows as
	// fit in availLines. Compact cards + 4 cols let all 7 days fit in
	// 2 rows on wide terminals without scrolling.
	gap := "   "
	var renderedRows []string
	used := 0
	endRow := 0
	for ri := range gridRows {
		add := rowHeights[ri]
		if ri > 0 {
			add++ // gap
		}
		if used+add > availLines && ri > 0 {
			break
		}
		parts := make([]string, 0, len(gridRows[ri])*2-1)
		for i, gi := range gridRows[ri] {
			if i > 0 {
				parts = append(parts, gap)
			}
			parts = append(parts, rendered[gi])
		}
		renderedRows = append(renderedRows, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
		if ri > 0 {
			used++ // gap line
		}
		used += rowHeights[ri]
		endRow = ri + 1
	}
	var sb strings.Builder
	sb.WriteString(strings.Join(renderedRows, "\n\n"))
	if endRow < len(gridRows) {
		if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString(thHelp.Render("↓ more below") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
