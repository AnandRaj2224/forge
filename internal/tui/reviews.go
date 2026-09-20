package tui

import (
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------- REVIEWS ----------

type ReviewsScreen struct {
	app      *App
	weeks    Navigator
	reports  []db.WeeklyReview
	selected *db.WeeklyReview
	err      string
}

func NewReviewsScreen(a *App) *ReviewsScreen {
	return &ReviewsScreen{app: a, weeks: NewNavigator()}
}

func (s *ReviewsScreen) Reload() {
	reps, err := runtime.ListWeeklyReviews(s.app.ctx(), s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	SortReviews(reps)
	s.reports = reps
	items := make([]listItem, 0, len(reps))
	for _, r := range reps {
		items = append(items, listItem{id: r.ID, title: r.ID, desc: fmt.Sprintf("%s — %s", r.WeekStart, r.WeekEnd)})
	}
	s.weeks.SetItems(wrapItems(items))
	s.syncSelected()
}

func (s *ReviewsScreen) syncSelected() {
	id := s.weeks.SelectedID()
	if id == "" {
		s.selected = nil
		return
	}
	for i, r := range s.reports {
		if r.ID == id {
			s.selected = &s.reports[i]
			return
		}
	}
	s.selected = nil
}

func (s *ReviewsScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "j" || k.String() == "k" || k.String() == "down" || k.String() == "up") {
		down := k.String() == "j" || k.String() == "down"
		prev := s.weeks.SelectedID()
		if down {
			s.weeks.List.CursorDown()
		} else {
			s.weeks.List.CursorUp()
		}
		if s.weeks.SelectedID() != prev {
			s.syncSelected()
		}
		return nil
	}
	prev := s.weeks.SelectedID()
	var c tea.Cmd
	s.weeks.List, c = s.weeks.List.Update(msg)
	if s.weeks.SelectedID() != prev {
		s.syncSelected()
	}
	return c
}

func (s *ReviewsScreen) View() string {
	w, h := s.app.contentViewport()
	lw, rw := splitW(w, 30, 22, 40)
	listH := h - 6
	if listH < 8 {
		listH = 8
	}
	s.weeks.List.SetWidth(lw - 4)
	s.weeks.FitHeight(listH)
	return lipglossJoin(Panel("REVIEWS", s.weeks.List.View(), lw, true), Panel("WEEKLY REPORT", s.reportView(), rw, false))
}

func (s *ReviewsScreen) reportView() string {
	if s.selected == nil {
		return thMeta.Render("Select a week.")
	}
	r := s.selected
	var sb string
	sb += thScreenTitle.Render(r.WeekStart+"  —  "+r.WeekEnd) + "\n"
	sb += thMeta.Render(r.ID) + "\n\n"
	sb += SectionHeader("TIME") + "\n"
	sb += MetaRow("Planned", FormatDuration(r.PlannedSeconds), 9) + "\n"
	sb += MetaRow("Actual", FormatDuration(r.ElapsedSeconds), 9) + "\n"
	sb += MetaRow("Overrun", formatSigned(r.OverrunSeconds), 9) + "\n\n"
	sb += SectionHeader("WHAT WORKED WELL THIS WEEK?") + "\n"
	sb += thPrimary.Render(orDash(r.ReflectionWorked)) + "\n\n"
	sb += SectionHeader("WHAT SHOULD CHANGE NEXT WEEK?") + "\n"
	sb += thPrimary.Render(orDash(r.ReflectionChange)) + "\n"
	return sb
}

func formatSigned(secs int) string {
	if secs < 0 {
		return "-" + FormatDuration(-secs)
	}
	return FormatDuration(secs)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (s *ReviewsScreen) Hints() string { return "" }
