package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"forge/internal/db/runtime"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HistoryScreen is the operational current-week view.
// ONLY the current week (Mon..Sun) is displayed; older DB records are never
// listed. Selecting a day updates the right daily-metrics panel.
type HistoryScreen struct {
	app   *App
	dates []string // YYYY-MM-DD Mon..Sun
	list  Navigator
	sums  map[string]runtime.DailySummary
	nowFn func() time.Time
	err   string
}

func NewHistoryScreen(a *App) *HistoryScreen {
	return &HistoryScreen{app: a, list: NewNavigator(), sums: map[string]runtime.DailySummary{}, nowFn: time.Now}
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
	s.dates = runtime.CurrentWeekDates(s.now())
	todayStr := s.now().UTC().Format("2006-01-02")
	items := make([]RowItem, 0, len(s.dates))
	for _, d := range s.dates {
		t, _ := time.Parse("2006-01-02", d)
		label := t.Format("Mon · 02 Jan")
		mark := "•"
		if d == todayStr {
			mark = "●"
		}
		sum, err := runtime.ComputeDailySummary(ctx, s.app.DB, d)
		if err == nil {
			s.sums[d] = sum
		}
		meta := ""
		if v, ok := s.sums[d]; ok && (v.Sessions > 0 || v.Planned > 0) {
			meta = fmt.Sprintf("%s/%s", FormatDuration(v.Actual), FormatDuration(v.Planned))
		}
		items = append(items, RowItem{ID: d, Title: label, Meta: meta, Mark: mark})
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

func (s *HistoryScreen) onBack() bool { return false }

func (s *HistoryScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "j", "down":
			s.list.CursorDown()
			return nil
		case "k", "up":
			s.list.CursorUp()
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
	s.list.SetFocused(true)
	left := PanelFixed("DAYS", SectionHeader("CURRENT WEEK")+"\n"+s.list.View(), lw, bodyH, true)
	right := PanelFixed("DAILY SUMMARY", s.dayBody(rw-6), rw, bodyH, false)
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
	sum := s.sums[d]
	var sb strings.Builder
	// Top metric cards: PLANNED ACTUAL VARIANCE TASKS SESSIONS.
	variance := formatSigned(sum.Actual - sum.Planned)
	completion := "—"
	if sum.Planned > 0 {
		completion = fmt.Sprintf("%.0f%%", float64(sum.Actual)/float64(sum.Planned)*100)
	}
	cards := []MetricCard{
		{Label: "PLANNED", Value: FormatDuration(sum.Planned), Sub: fmt.Sprintf("%d tasks", sum.TasksTotal)},
		{Label: "ACTUAL", Value: FormatDuration(sum.Actual), Sub: completion},
		{Label: "VARIANCE", Value: variance, Sub: fmt.Sprintf("%d sess", sum.Sessions)},
	}
	sb.WriteString(MetricGrid(cards, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("COMPLETION") + "\n")
	sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d tasks  ·  %d sessions", sum.TasksCompleted, sum.TasksTotal, sum.Sessions)) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("TIME BY DOMAIN") + "\n")
	total := sum.GovSeconds + sum.DSASeconds + sum.SWESeconds
	sb.WriteString(MetaRow("GOV", fmt.Sprintf("%s  %s", FormatDuration(sum.GovSeconds), pctOf(sum.GovSeconds, total)), 5) + "\n")
	sb.WriteString(MetaRow("DSA", fmt.Sprintf("%s  %s", FormatDuration(sum.DSASeconds), pctOf(sum.DSASeconds, total)), 5) + "\n")
	sb.WriteString(MetaRow("SWE", fmt.Sprintf("%s  %s", FormatDuration(sum.SWESeconds), pctOf(sum.SWESeconds, total)), 5) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("DOMAIN EVIDENCE") + "\n")
	sb.WriteString(thMeta.Render("GOV") + "  " + thPrimary.Render(fmt.Sprintf("lect %d · q %d · acc %s", sum.GovLectures, sum.GovAttempted, fmtAcc(sum.GovAccuracy, sum.GovAttempted))) + "\n")
	sb.WriteString(thMeta.Render("DSA") + "  " + thPrimary.Render(fmt.Sprintf("%d probs · %d passed", sum.DSAAttempts, sum.DSAPassed)) + "\n")
	sb.WriteString(thMeta.Render("SWE") + "  " + thPrimary.Render(fmt.Sprintf("%d done · %d partial · %d blocked", sum.SWECompleted, sum.SWEPartial, sum.SWEBlocked)))
	return sb.String()
}

func pctOf(v, total int) string {
	if total <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", float64(v)/float64(total)*100)
}

func fmtAcc(acc float64, n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", acc*100)
}

func (s *HistoryScreen) Hints() string { return "j/k Day" }

// Ensure lipgloss import is used in small viewports.
var _ = lipgloss.Top
