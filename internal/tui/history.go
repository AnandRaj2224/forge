package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// HistoryScreen is the operational current-week view.
// ONLY the current week (Mon..Sun) is displayed; older DB records are never
// listed. Selecting a day updates the right daily-metrics panel. Panes
// scroll independently: the day list owns its viewport; the daily body
// scrolls via the shared ScrollViewport when focused (H/L).
type HistoryScreen struct {
	app   *App
	dates []string // YYYY-MM-DD Mon..Sun
	list  Navigator
	sums  map[string]runtime.DailySummary
	focusR bool // right daily pane owns focus (H/L); j/k scrolls it
	bodyScroll ScrollViewport
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
	sum := s.sums[d]
	var sb strings.Builder
	// TITLE + TIME CONTEXT on one line: WED · 16 SEP 2026 + DAILY REPORT.
	// Explicit day context without extra height.
	title := FormatDateISO(d)
	if t, err := time.Parse("2006-01-02", d); err == nil {
		title = strings.ToUpper(t.Format("Mon")) + " · " + strings.ToUpper(t.Format("02 Jan 2006"))
	}
	sb.WriteString(thScreenTitle.Render(title) + "  " + thMeta.Render("DAILY REPORT") + "\n")
	sb.WriteString(SectionHeader("ACTIVITY") + "\n")
	// HIGH-VALUE METRICS as a compact bento: task-count completion, never hours.
	variance := formatSignedLong(sum.Actual - sum.Planned)
	completion := "—"
	tasksVal := "—"
	if sum.TasksTotal > 0 {
		completion = fmt.Sprintf("%.0f%%", float64(sum.TasksCompleted)/float64(sum.TasksTotal)*100)
		tasksVal = fmt.Sprintf("%d of %d", sum.TasksCompleted, sum.TasksTotal)
	}
	cards := []MetricCard{
		{Label: "PLANNED", Value: FormatDurationLong(sum.Planned)},
		{Label: "ACTUAL", Value: FormatDurationLong(sum.Actual)},
		{Label: "VARIANCE", Value: variance},
		{Label: "TASKS", Value: tasksVal},
		{Label: "SESSIONS", Value: fmt.Sprint(sum.Sessions)},
		{Label: "COMPLETION", Value: completion},
	}
	sb.WriteString(MetricGrid(cards, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("TIME BY DOMAIN") + "\n")
	total := sum.GovSeconds + sum.DSASeconds + sum.SWESeconds
	sb.WriteString(MetricGrid([]MetricCard{
		{Label: "GOV", Value: FormatDurationLong(sum.GovSeconds) + " · " + pctOf(sum.GovSeconds, total) + " of day", Accent: cGov},
		{Label: "DSA", Value: FormatDurationLong(sum.DSASeconds) + " · " + pctOf(sum.DSASeconds, total) + " of day", Accent: cDSA},
		{Label: "SWE", Value: FormatDurationLong(sum.SWESeconds) + " · " + pctOf(sum.SWESeconds, total) + " of day", Accent: cSWE},
	}, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	// DOMAIN DETAIL as structured sections (not compressed prose). Every
	// number states what it measures: lectures completed, questions
	// attempted/correct/incorrect, accuracy, problems attempted/passed/
	// not passed, SWE outcomes.
	govBlock := DomainDetailBlock("GOV", "GOV ACTIVITY", []StatPair{
		{"Lectures completed", fmt.Sprint(sum.GovLectures)},
		{"Questions attempted", fmt.Sprint(sum.GovAttempted)},
		{"Questions correct", fmt.Sprint(sum.GovCorrect)},
		{"Questions incorrect", fmt.Sprint(sum.GovAttempted - sum.GovCorrect)},
		{"Accuracy", fmtAcc(sum.GovAccuracy, sum.GovAttempted)},
	})
	dsaNotPassed := sum.DSAAttempts - sum.DSAPassed
	if dsaNotPassed < 0 {
		dsaNotPassed = 0
	}
	dsaBlock := DomainDetailBlock("DSA", "DSA ACTIVITY", []StatPair{
		{"Problems attempted", fmt.Sprint(sum.DSAAttempts)},
		{"Passed", fmt.Sprint(sum.DSAPassed)},
		{"Not passed", fmt.Sprint(dsaNotPassed)},
	})
	sweBlock := DomainDetailBlock("SWE", "SWE ACTIVITY", []StatPair{
		{"Completed", fmt.Sprint(sum.SWECompleted)},
		{"Partial", fmt.Sprint(sum.SWEPartial)},
		{"Blocked", fmt.Sprint(sum.SWEBlocked)},
	})
	sb.WriteString(JoinDetailBlocks([]string{govBlock, dsaBlock, sweBlock}, inner))
	// UNFINISHED: planned tasks with no persisted evidence (factual only).
	sb.WriteString("\n" + RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("UNFINISHED") + "\n")
	sb.WriteString(s.unfinishedView(inner, d))
	return sb.String()
}

// unfinishedItem is one planned-but-incomplete task for a day.
type unfinishedItem struct {
	num     int
	domain  string
	area    string
	typ     string
	label   string
	planned int
}

// unfinishedForDate returns snapshot tasks for dateStr with no persisted
// evidence, in position order. It uses actual DB data only.
func (s *HistoryScreen) unfinishedForDate(dateStr string) []unfinishedItem {
	ctx := context.Background()
	var snapID string
	row := s.app.DB.QueryRowContext(ctx, `SELECT id FROM today_snapshots WHERE local_date = ? ORDER BY revision DESC LIMIT 1`, dateStr)
	if err := row.Scan(&snapID); err != nil {
		return nil
	}
	tasks, err := runtime.GetTodayTasks(ctx, s.app.DB, db.ID(snapID))
	if err != nil {
		return nil
	}
	var out []unfinishedItem
	n := 0
	for _, t := range tasks {
		if ComputeTaskStatus(ctx, s.app.DB, t.ID) == TaskCompleted {
			continue
		}
		n++
		b := ResolveBundle(ctx, s.app.DB, t)
		area, typ, label := TaskColumns(t, b)
		out = append(out, unfinishedItem{num: n, domain: string(t.Domain), area: area, typ: typ, label: label, planned: t.PlannedSeconds})
	}
	return out
}

func (s *HistoryScreen) unfinishedView(inner int, dateStr string) string {
	items := s.unfinishedForDate(dateStr)
	if len(items) == 0 {
		return thMeta.Render("— all complete —")
	}
	var sb strings.Builder
	for i, it := range items {
		head := fmt.Sprintf("%02d · %s", it.num, it.domain)
		if it.area != "" && it.area != "—" {
			head += " · " + TruncateToWidth(it.area, 18)
		}
		if it.label != "" {
			head += " · " + TruncateToWidth(it.label, inner-len(head)-4)
		}
		sb.WriteString(thPrimary.Render(head) + "\n")
		sb.WriteString(thMeta.Render(fmt.Sprintf("Planned %s · Not completed", FormatDurationLong(it.planned))))
		if i < len(items)-1 {
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

func fmtAcc(acc float64, n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", acc*100)
}


// Ensure lipgloss import is used in small viewports.
var _ = lipgloss.Top
