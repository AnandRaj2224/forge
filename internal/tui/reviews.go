package tui

import (
	"strings"

	db "forge/internal/db"
	"forge/internal/db/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------- REVIEWS: persistent two-panel weekly workspace ----------

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
	items := make([]RowItem, 0, len(reps))
	for _, r := range reps {
		items = append(items, RowItem{ID: r.ID, Title: r.WeekStart + " — " + r.WeekEnd, Meta: r.ID, Mark: "•"})
	}
	s.weeks.SetItems(items)
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
			s.weeks.CursorDown()
		} else {
			s.weeks.CursorUp()
		}
		if s.weeks.SelectedID() != prev {
			s.syncSelected()
		}
		return nil
	}
	return nil
}

func (s *ReviewsScreen) View() string {
	w, h := s.app.contentViewport()
	lw, rw := TwoPaneLayout(w, 32, 24, 40)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	s.weeks.SetWidth(lw - 4)
	s.weeks.FitHeight(innerH - 1)
	s.weeks.SetFocused(true)
	return lipglossJoin(
		PanelFixed("REVIEWS", SectionHeader("WEEKLY REVIEWS")+"\n"+s.weeks.View(), lw, bodyH, true),
		PanelFixed("WEEKLY REPORT", s.reportView(rw-6), rw, bodyH, false),
	)
}

func (s *ReviewsScreen) reportView(inner int) string {
	if s.selected == nil {
		return thMeta.Render("Select a week.")
	}
	if inner < 24 {
		inner = 24
	}
	r := s.selected
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(r.WeekStart+"  —  "+r.WeekEnd) + "\n")
	sb.WriteString(thMeta.Render(r.ID) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("TIME") + "\n")
	sb.WriteString(MetaRow("Planned", FormatDuration(r.PlannedSeconds), 9) + "\n")
	sb.WriteString(MetaRow("Actual", FormatDuration(r.ElapsedSeconds), 9) + "\n")
	sb.WriteString(MetaRow("Overrun", formatSigned(r.OverrunSeconds), 9) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("WHAT WORKED WELL THIS WEEK?") + "\n")
	sb.WriteString(thPrimary.Render(orDash(r.ReflectionWorked)) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("WHAT SHOULD CHANGE NEXT WEEK?") + "\n")
	sb.WriteString(thPrimary.Render(orDash(r.ReflectionChange)) + "\n")
	return sb.String()
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

func (s *ReviewsScreen) Hints() string { return "j/k Move   Esc Back" }
