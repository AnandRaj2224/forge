package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"forge/internal/db/runtime"
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
		label := t.Format("Mon") + " · " + FormatDate(t)
		mark := "•"
		if d == todayStr {
			mark = "●"
		}
		sum, err := runtime.ComputeDailySummary(ctx, s.app.DB, d)
		if err == nil {
			s.sums[d] = sum
		}
		meta, sub := "", ""
		if v, ok := s.sums[d]; ok && (v.Sessions > 0 || v.Planned > 0) {
			meta = FormatDurationLong(v.Actual)
			sub = "PLAN " + FormatDurationLong(v.Planned)
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

func (s *HistoryScreen) onBack() bool { return false }

func (s *HistoryScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
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
	sb.WriteString(thScreenTitle.Render(FormatDateISO(d)) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	// Top metric cards: planned, actual, variance with task/session context.
	// Completion is task-count based (DONE over planned), never hours.
	variance := formatSignedLong(sum.Actual - sum.Planned)
	completion := "—"
	if sum.TasksTotal > 0 {
		completion = fmt.Sprintf("%.0f%%", float64(sum.TasksCompleted)/float64(sum.TasksTotal)*100)
	}
	cards := []MetricCard{
		{Label: "PLANNED", Value: FormatDurationLong(sum.Planned), Sub: fmt.Sprintf("%d tasks", sum.TasksTotal)},
		{Label: "ACTUAL", Value: FormatDurationLong(sum.Actual), Sub: completion},
		{Label: "VARIANCE", Value: variance, Sub: fmt.Sprintf("%d Sessions", sum.Sessions)},
	}
	sb.WriteString(MetricGrid(cards, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("COMPLETION") + "\n")
	sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d tasks  ·  %d sessions", sum.TasksCompleted, sum.TasksTotal, sum.Sessions)) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("TIME BY DOMAIN") + "\n")
	total := sum.GovSeconds + sum.DSASeconds + sum.SWESeconds
	sb.WriteString(MetricGrid([]MetricCard{
		{Label: "GOV", Value: FormatDurationLong(sum.GovSeconds) + " · " + pctOf(sum.GovSeconds, total), Accent: cGov},
		{Label: "DSA", Value: FormatDurationLong(sum.DSASeconds) + " · " + pctOf(sum.DSASeconds, total), Accent: cDSA},
		{Label: "SWE", Value: FormatDurationLong(sum.SWESeconds) + " · " + pctOf(sum.SWESeconds, total), Accent: cSWE},
	}, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("DOMAIN EVIDENCE") + "\n")
	sb.WriteString(EvidenceRow("GOV", []StatPair{
		{"Lectures", fmt.Sprint(sum.GovLectures)},
		{"Questions", fmt.Sprint(sum.GovAttempted)},
		{"Accuracy", fmtAcc(sum.GovAccuracy, sum.GovAttempted)},
	}) + "\n")
	sb.WriteString(EvidenceRow("DSA", []StatPair{
		{"Problems", fmt.Sprint(sum.DSAAttempts)},
		{"Passed", fmt.Sprint(sum.DSAPassed)},
	}) + "\n")
	sb.WriteString(EvidenceRow("SWE", []StatPair{
		{"Completed", fmt.Sprint(sum.SWECompleted)},
		{"Partial", fmt.Sprint(sum.SWEPartial)},
		{"Blocked", fmt.Sprint(sum.SWEBlocked)},
	}))
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


// Ensure lipgloss import is used in small viewports.
var _ = lipgloss.Top
