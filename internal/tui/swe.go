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

// ---------- SWE: read-only weekly day-card grid ----------
//
// Primary objects are DAYS: one compact card per weekday in a weekly grid
// (row 1: Monday–Thursday, row 2: Friday–Sunday). There is no slot
// selection, no cursor, no per-slot navigation: the user simply looks at
// the weekly routine.
type SWEScreen struct {
	app   *App
	areas []db.SWEArea
	slots []db.SWERoutineSlot
	err   string
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
	// Weekly grid: all slots grouped by weekday. The view always shows
	// the full week so all seven day cards are visible with real DB data
	// (each weekday carries its slots).
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

// Update is a no-op: the routine overview is read-only. There is no slot
// cursor and no per-card navigation; j/k/h/l/arrows never move anything
// here. Global navigation (tabs, back, quit) is owned by the app shell.
func (s *SWEScreen) Update(msg tea.Msg) tea.Cmd {
	return nil
}

// View renders the SWE screen as a header/summary followed directly by the
// weekly day-card grid across the whole content area. No left Areas panel,
// no right Routine panel.
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
	cards := s.routineCardsView(w, availLines)
	if cards == "" {
		return header
	}
	return header + "\n" + cards
}

// scheduleEdgeRow places left/right text on one line sharing a single
// right edge at width: left stays left, right ends exactly at width.
// Padding is calculated from ANSI-aware visible widths (never manual
// per-day coordinates). Day names and totals are far shorter than any
// real width, so the guard below only fires on degenerate widths.
func scheduleEdgeRow(left, right string, width int) string {
	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// padLeftToWidth right-aligns s inside w visible cells so every duration
// shares one fixed column. s is never truncated; widths are ANSI-aware.
func padLeftToWidth(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// routineCardsView renders the weekly routine as a grid of compact
// read-only day cards: row 1 holds Monday–Thursday, row 2 the remaining
// days. Cards are evenly sized: one shared width from the available width
// and one shared height that fills the available lines, so the grid uses
// the content area instead of floating at the top. Durations share one
// fixed column across every card (measured once from all data), so task
// durations align grid-wide. There is no cursor, no selection: card
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
	// Resolve display names and day totals once; measure the shared task
	// columns from every slot on screen.
	type schedSlot struct {
		name string
		dur  string
	}
	type schedDay struct {
		name  string
		total string
		slots []schedSlot
	}
	areaDisp := s.areaDisplayNames()
	days := make([]schedDay, 0, len(groups))
	nameW, durW := 0, 0
	for _, g := range groups {
		d := schedDay{name: strings.ToUpper(g.DayName)}
		daySecs := 0
		for _, sl := range g.Slots {
			name := sl.Area
			if disp, ok := areaDisp[sl.AreaID]; ok && disp != "" {
				name = disp
			} else if name == "" && sl.AreaID != "" {
				name = SWEDisplayName(sl.AreaID)
			}
			if strings.TrimSpace(name) == "" {
				name = "—"
			}
			if w := lipgloss.Width(name); w > nameW {
				nameW = w
			}
			dur := FormatDurationLong(sl.DurationSeconds)
			if w := lipgloss.Width(dur); w > durW {
				durW = w
			}
			d.slots = append(d.slots, schedSlot{name: name, dur: dur})
			daySecs += sl.DurationSeconds
		}
		d.total = FormatDurationLong(daySecs)
		days = append(days, d)
	}
	// Even cards: one width for the whole grid (4 across), one height that
	// fills the available lines over two rows plus the inter-row gap.
	cardW := BentoGridCardW(availW, 4, 3)
	if cardW < 28 {
		cardW = 28
	}
	cardH := (availLines - 1) / 2
	if cardH < 6 {
		cardH = 6
	}
	rendered := make([]string, 0, len(days))
	for _, d := range days {
		tasks := make([]SWECardTask, 0, len(d.slots))
		for _, sl := range d.slots {
			tasks = append(tasks, SWECardTask{Name: sl.name, Dur: sl.dur})
		}
		rendered = append(rendered, SWERoutineCard{
			Day: d.name, Total: d.total, Tasks: tasks,
			Width: cardW, Height: cardH, NameW: nameW, DurW: durW,
		}.View())
	}
	// Weekly rows of 4 cards (Monday–Thursday, then Friday–Sunday).
	var rows []string
	for i := 0; i < len(rendered); i += 4 {
		end := i + 4
		if end > len(rendered) {
			end = len(rendered)
		}
		parts := make([]string, 0, (end-i)*2-1)
		for j := i; j < end; j++ {
			if j > i {
				parts = append(parts, "   ")
			}
			parts = append(parts, rendered[j])
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	}
	return strings.Join(rows, "\n\n")
}
