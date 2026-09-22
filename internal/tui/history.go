package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"forge/internal/engine"
)

// HistoryScreen is the operational current-week view (Forge calendar).
// ONLY the current week (Mon..Sun) is displayed; older DB records are never
// listed. Selecting a day updates the right daily-activity panel. Panes
// scroll independently: the day list owns its viewport; the daily body
// scrolls via the shared ScrollViewport when focused (H/L).
type HistoryScreen struct {
	app        *App
	dates      []string // YYYY-MM-DD Mon..Sun (Forge calendar)
	list       Navigator
	reports    map[string]engine.DailyReport
	reportErr  map[string]string
	focusR     bool // right daily pane owns focus (H/L); j/k scrolls it
	bodyScroll ScrollViewport
	nowFn      func() time.Time
	err        string
}

func NewHistoryScreen(a *App) *HistoryScreen {
	return &HistoryScreen{app: a, list: NewNavigator(), reports: map[string]engine.DailyReport{}, reportErr: map[string]string{}, nowFn: time.Now}
}

func (s *HistoryScreen) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

func (s *HistoryScreen) Reload() {
	ctx := context.Background()
	s.err = ""
	cal := engine.DefaultCalendar()
	s.dates = cal.CurrentWeek(s.now())
	todayStr := cal.LocalDate(s.now())
	items := make([]RowItem, 0, len(s.dates))
	for _, d := range s.dates {
		t, _ := time.Parse("2006-01-02", d)
		label := t.Format("Mon") + " · " + FormatDate(t)
		mark := "•"
		if d == todayStr {
			mark = "●"
		}
		rep, err := engine.BuildDailyReport(ctx, s.app.DB, d)
		if err != nil {
			s.reportErr[d] = err.Error()
		} else {
			s.reports[d] = rep
			delete(s.reportErr, d)
		}
		meta, sub := "", ""
		if v, ok := s.reports[d]; ok && (v.Sessions > 0 || v.Planned > 0) {
			meta = engine.FormatDurationLong(v.Actual)
			sub = "PLAN " + engine.FormatDurationLong(v.Planned)
		}
		items = append(items, RowItem{ID: d, Title: label, Meta: meta, MetaSub: sub, Mark: mark})
	}
	s.list.SetItems(items)
	// Preselect today when possible.
	for i, d := range s.dates {
		if d == todayStr {
			s.list.Select(i)
		}
	}
}

func (s *HistoryScreen) selectedDate() string { return s.list.SelectedID() }

func (s *HistoryScreen) onBack() bool {
	if s.focusR {
		s.focusR = false
		return true
	}
	return false
}

func (s *HistoryScreen) bodyViewportH() int {
	_, h := s.app.contentViewport()
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	return innerH
}

func (s *HistoryScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "j", "down", "k", "up":
			if s.focusR {
				// Scroll the daily body independently.
				inner := s.bodyViewportH()
				total := len(strings.Split(s.dayBody(40), "\n"))
				if k.String() == "j" || k.String() == "down" {
					s.bodyScroll.ScrollDown(total, inner)
				} else {
					s.bodyScroll.ScrollUp(total, inner)
				}
				return nil
			}
			prev := s.list.SelectedID()
			if k.String() == "j" || k.String() == "down" {
				s.list.CursorDown()
			} else {
				s.list.CursorUp()
			}
			if s.list.SelectedID() != prev {
				s.bodyScroll.Reset()
			}
			return nil
		case "h", "left":
			s.focusR = false
			return nil
		case "l", "right":
			if s.selectedDate() != "" {
				s.focusR = true
			}
			return nil
		}
	}
	return nil
}

func (s *HistoryScreen) View() string {
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
	s.list.SetWidth(lw - 4)
	s.list.FitHeight(innerH - 1)
	s.list.SetFocused(!s.focusR)
	left := PanelFixed("DAYS", SectionHeader("CURRENT WEEK")+"\n"+s.list.View(), lw, bodyH, !s.focusR)
	right := PanelFixed("DAILY SUMMARY", ViewportView(s.dayBody(rw-6), &s.bodyScroll, innerH), rw, bodyH, s.focusR)
	return lipglossJoin(left, right)
}

func (s *HistoryScreen) dayBody(inner int) string {
	if inner < 24 {
		inner = 24
	}
	d := s.selectedDate()
	if d == "" {
		return EmptyState("Select a day.")
	}
	if msg, bad := s.reportErr[d]; bad {
		return thError.Render("report unavailable · data error") + "\n" + thMeta.Render(msg)
	}
	rep, ok := s.reports[d]
	if !ok {
		return EmptyState("No data for this day.")
	}
	var sb strings.Builder
	title := FormatDateISO(d)
	if t, err := time.Parse("2006-01-02", d); err == nil {
		title = strings.ToUpper(t.Format("Mon")) + " · " + strings.ToUpper(t.Format("02 Jan 2006"))
	}
	sb.WriteString(thScreenTitle.Render(title) + "  " + thMeta.Render("DAILY ACTIVITY") + "\n")
	sb.WriteString(SectionHeader("ACTIVITY") + "\n")
	variance := engine.FormatSigned(rep.Actual - rep.Planned)
	completion := "—"
	tasksVal := "—"
	if rep.TasksTotal > 0 {
		completion = fmt.Sprintf("%.0f%%", float64(rep.TasksCompleted)/float64(rep.TasksTotal)*100)
		tasksVal = fmt.Sprintf("%d of %d", rep.TasksCompleted, rep.TasksTotal)
	}
	cards := []MetricCard{
		{Label: "PLANNED", Value: engine.FormatDurationLong(rep.Planned)},
		{Label: "ACTUAL", Value: engine.FormatDurationLong(rep.Actual)},
		{Label: "VARIANCE", Value: variance},
		{Label: "TASKS", Value: tasksVal},
		{Label: "SESSIONS", Value: fmt.Sprint(rep.Sessions)},
		{Label: "COMPLETION", Value: completion},
	}
	sb.WriteString(MetricGrid(cards, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("TIME BY DOMAIN · % OF TRACKED TIME") + "\n")
	total := rep.GovSeconds + rep.DSASeconds + rep.SWESeconds
	sb.WriteString(MetricGrid([]MetricCard{
		{Label: "GOV", Value: engine.FormatDurationLong(rep.GovSeconds) + " · " + pctOf(rep.GovSeconds, total) + " of tracked time", Accent: cGov},
		{Label: "DSA", Value: engine.FormatDurationLong(rep.DSASeconds) + " · " + pctOf(rep.DSASeconds, total) + " of tracked time", Accent: cDSA},
		{Label: "SWE", Value: engine.FormatDurationLong(rep.SWESeconds) + " · " + pctOf(rep.SWESeconds, total) + " of tracked time", Accent: cSWE},
	}, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("DAILY WORK") + "\n")
	sb.WriteString(s.dailyWorkView(inner, rep))
	sb.WriteString("\n" + RowDivider(inner) + "\n")
	govAcc := "—"
	if rep.Gov.Accuracy != nil {
		govAcc = fmt.Sprintf("%.1f%%", *rep.Gov.Accuracy*100)
	}
	govPairs := []StatPair{
		{"Lectures completed", fmt.Sprint(rep.Gov.LecturesCompleted)},
		{"Questions attempted", fmt.Sprint(rep.Gov.QuestionsAttempted)},
		{"Questions correct", fmt.Sprint(rep.Gov.QuestionsCorrect)},
		{"Questions incorrect", fmt.Sprint(rep.Gov.QuestionsIncorrect)},
		{"Accuracy", govAcc},
	}
	if rep.Gov.HasSpeed && rep.Gov.AvgSpeed != nil {
		govPairs = append(govPairs, StatPair{"Avg question speed", fmt.Sprintf("%.1fs", *rep.Gov.AvgSpeed)})
	}
	govBlock := DomainDetailBlock("GOV", "GOV ACTIVITY", govPairs)
	dsaBlock := DomainDetailBlock("DSA", "DSA ACTIVITY", []StatPair{
		{"Problems attempted", fmt.Sprint(rep.DSA.Attempted)},
		{"Problems passed", fmt.Sprint(rep.DSA.Passed)},
		{"Problems not passed", fmt.Sprint(rep.DSA.NotPassed)},
	})
	sweBlock := DomainDetailBlock("SWE", "SWE ACTIVITY", []StatPair{
		{"Tasks completed", fmt.Sprint(rep.SWE.Completed)},
		{"Tasks partial", fmt.Sprint(rep.SWE.Partial)},
		{"Tasks blocked", fmt.Sprint(rep.SWE.Blocked)},
	})
	sb.WriteString(JoinDetailBlocks([]string{govBlock, dsaBlock, sweBlock}, inner))
	sb.WriteString("\n" + RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("UNFINISHED") + "\n")
	sb.WriteString(s.unfinishedView(inner, rep))
	return sb.String()
}

// dailyWorkView lists the actual Today tasks from the selected day with
// planned/actual/status so the screen answers what happened.
func (s *HistoryScreen) dailyWorkView(inner int, rep engine.DailyReport) string {
	if len(rep.Activities) == 0 {
		return thMeta.Render("No planned tasks.")
	}
	var sb strings.Builder
	for i, a := range rep.Activities {
		st := "TODO"
		switch a.State {
		case engine.StateCompleted:
			st = "DONE"
		case engine.StateActive:
			st = "ACTIVE"
		}
		head := fmt.Sprintf("%02d  %s · %s · %s", a.Position, a.Domain, a.Kind, TruncateToWidth(a.Title, inner-20))
		sb.WriteString(thPrimary.Render(head) + "\n")
		sub := fmt.Sprintf("planned %s · actual %s · %s", engine.FormatDurationLong(a.Planned), engine.FormatDurationLong(a.Actual), st)
		if a.Detail != "" && a.Detail != a.Title {
			sub = TruncateToWidth(a.Detail, inner-4) + "\n" + thMeta.Render(sub)
			sb.WriteString(thMeta.Render(TruncateToWidth(a.Detail, inner-4)) + "\n")
			sb.WriteString(thMeta.Render(fmt.Sprintf("planned %s · actual %s · %s", engine.FormatDurationLong(a.Planned), engine.FormatDurationLong(a.Actual), st)))
		} else {
			sb.WriteString(thMeta.Render(sub))
		}
		if i < len(rep.Activities)-1 {
			sb.WriteString("\n" + RowDivider(inner) + "\n")
		}
	}
	return sb.String()
}

func (s *HistoryScreen) unfinishedView(inner int, rep engine.DailyReport) string {
	if len(rep.Unfinished) == 0 {
		return thMeta.Render("— all complete —")
	}
	var sb strings.Builder
	for i, a := range rep.Unfinished {
		head := fmt.Sprintf("%02d · %s · %s", a.Position, a.Domain, TruncateToWidth(a.Title, inner-12))
		sb.WriteString(thPrimary.Render(head) + "\n")
		sb.WriteString(thMeta.Render(fmt.Sprintf("Planned %s · Not completed", engine.FormatDurationLong(a.Planned))))
		if i < len(rep.Unfinished)-1 {
			sb.WriteString("\n" + RowDivider(inner) + "\n")
		}
	}
	return sb.String()
}

func pctOf(v, total int) string {
	if total <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", float64(v)/float64(total)*100)
}

// Ensure lipgloss import is used in small viewports.
var _ = lipgloss.Top
