package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// ---------- REVIEWS: persistent two-panel weekly workspace ----------

type weekEntry struct {
	ID     string
	Start  string
	End    string
	stored bool
}

type ReviewsScreen struct {
	app      *App
	weeks    Navigator
	reports  []db.WeeklyReview
	entries  []weekEntry
	selected *db.WeeklyReview
	focusR   bool // right report pane owns focus
	editCur  int  // reflection cursor in view mode (0 worked, 1 change)
	editing  bool // reflection editing mode
	editFoc  int  // editing focus (0 worked, 1 change, 2 save)
	editIn   [2]textinput.Model
	nowFn    func() time.Time
	err      string
}

func NewReviewsScreen(a *App) *ReviewsScreen {
	return &ReviewsScreen{app: a, weeks: NewNavigator(), nowFn: time.Now}
}

// lastWeeks returns the last n Mon..Sun weeks (newest first) as id/start/end.
func lastWeeks(now time.Time, n int) []weekEntry {
	days := runtime.CurrentWeekDays(now)
	if len(days) == 0 {
		return nil
	}
	monday := days[0]
	out := make([]weekEntry, 0, n)
	for i := 0; i < n; i++ {
		mon := monday.AddDate(0, 0, -7*i)
		sun := mon.AddDate(0, 0, 6)
		isoYear, isoWeek := mon.ISOWeek()
		out = append(out, weekEntry{
			ID:    fmt.Sprintf("%d-W%02d", isoYear, isoWeek),
			Start: mon.Format("2006-01-02"),
			End:   sun.Format("2006-01-02"),
		})
	}
	return out
}

func (s *ReviewsScreen) Reload() {
	ctx := s.app.ctx()
	reps, err := runtime.ListWeeklyReviews(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.reports = reps
	stored := map[string]bool{}
	for _, r := range reps {
		stored[r.ID] = true
	}
	// Display weeks: the last 8 weeks plus any stored older reviews.
	gen := lastWeeks(s.nowFn(), 8)
	entries := make([]weekEntry, 0, len(gen)+len(reps))
	for _, g := range gen {
		g.stored = stored[g.ID]
		entries = append(entries, g)
	}
	for _, r := range reps {
		if !containsWeek(entries, r.ID) {
			entries = append(entries, weekEntry{ID: r.ID, Start: r.WeekStart, End: r.WeekEnd, stored: true})
		}
	}
	sortWeekEntries(entries)
	// Preserve the cursor across reloads; default to the current week.
	keep := s.weeks.SelectedID()
	s.entries = entries
	items := make([]RowItem, 0, len(entries))
	for _, e := range entries {
		sub, mark := "", "○"
		if e.stored {
			mark = "•"
		} else {
			sub = "DRAFT"
		}
		items = append(items, RowItem{ID: e.ID, Title: FormatDateRange(e.Start, e.End), Meta: e.ID, MetaSub: sub, Mark: mark})
	}
	s.weeks.SetItems(items)
	if keep != "" {
		for i, e := range entries {
			if e.ID == keep {
				s.weeks.Select(i)
				break
			}
		}
	} else if len(entries) > 0 {
		s.weeks.Select(len(entries) - 1)
	}
	s.syncSelected()
}

func containsWeek(entries []weekEntry, id string) bool {
	for _, e := range entries {
		if e.ID == id {
			return true
		}
	}
	return false
}

func sortWeekEntries(entries []weekEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Start < entries[j-1].Start; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func (s *ReviewsScreen) syncSelected() {
	s.editing = false
	s.editCur = 0
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
	// No stored review: synthesize a draft from generated metrics with
	// empty reflections. It persists only when the user saves.
	for _, e := range s.entries {
		if e.ID != id {
			continue
		}
		now := s.nowFn().UTC()
		ev := runtime.ComputeWeeklyEvidence(s.app.ctx(), s.app.DB, e.Start, e.End)
		s.selected = &db.WeeklyReview{
			ID:             e.ID,
			WeekStart:      e.Start,
			WeekEnd:        e.End,
			GeneratedAt:    now,
			UpdatedAt:      now,
			PlannedSeconds: ev.Planned,
			ElapsedSeconds: ev.Actual,
			OverrunSeconds: ev.Actual - ev.Planned,
			ReportJSON:     "{}",
		}
		return
	}
	s.selected = nil
}

// textFocus reports whether a reflection text input owns focus (letters
// type there; q backs out only from the SAVE button).
func (s *ReviewsScreen) textFocus() bool { return s.editing && s.editFoc < 2 }

func (s *ReviewsScreen) onBack() bool {
	if s.editing {
		s.editing = false
		return true
	}
	if s.focusR {
		s.focusR = false
		return true
	}
	return false
}

// startEditing opens the reflection editor on the cursor reflection.
func (s *ReviewsScreen) startEditing() {
	if s.selected == nil {
		return
	}
	for i := range s.editIn {
		ti := textinput.New()
		ti.Prompt = "│ "
		ti.CharLimit = 0
		ti.SetWidth(40)
		s.editIn[i] = ti
	}
	s.editIn[0].SetValue(s.selected.ReflectionWorked)
	s.editIn[1].SetValue(s.selected.ReflectionChange)
	s.editFoc = s.editCur
	if s.editFoc < 0 || s.editFoc > 1 {
		s.editFoc = 0
	}
	s.editing = true
	s.updateEditFocus()
}

func (s *ReviewsScreen) updateEditFocus() {
	for i := range s.editIn {
		s.editIn[i].Blur()
	}
	if s.editFoc >= 0 && s.editFoc < 2 {
		s.editIn[s.editFoc].Focus()
	}
}

// saveReflections persists the edited reflections to the stored review
// model (insert or update). Generated metrics are untouched.
func (s *ReviewsScreen) saveReflections() {
	if s.selected == nil {
		return
	}
	r := *s.selected
	r.ReflectionWorked = strings.TrimSpace(s.editIn[0].Value())
	r.ReflectionChange = strings.TrimSpace(s.editIn[1].Value())
	r.UpdatedAt = s.nowFn().UTC()
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = r.UpdatedAt
	}
	if err := runtime.UpsertWeeklyReview(s.app.ctx(), s.app.DB, r); err != nil {
		s.err = err.Error()
		return
	}
	s.editing = false
	s.Reload()
}

func (s *ReviewsScreen) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if s.editing {
			// Forward terminal events to the focused input.
			if s.editFoc >= 0 && s.editFoc < 2 {
				var cmd tea.Cmd
				s.editIn[s.editFoc], cmd = s.editIn[s.editFoc].Update(msg)
				return cmd
			}
		}
		return nil
	}
	ks := k.String()
	if s.editing {
		switch ks {
		case "down":
			if s.editFoc < 2 {
				s.editFoc++
				s.updateEditFocus()
			}
			return nil
		case "up":
			if s.editFoc > 0 {
				s.editFoc--
				s.updateEditFocus()
			}
			return nil
		case "enter":
			if s.editFoc >= 2 {
				s.saveReflections()
			} else {
				s.editFoc++
				s.updateEditFocus()
			}
			return nil
		}
		if s.editFoc >= 0 && s.editFoc < 2 {
			var cmd tea.Cmd
			s.editIn[s.editFoc], cmd = s.editIn[s.editFoc].Update(msg)
			return cmd
		}
		return nil
	}
	switch {
	case IsMoveDown(ks):
		if s.focusR {
			if s.editCur < 1 {
				s.editCur++
			}
		} else {
			prev := s.weeks.SelectedID()
			s.weeks.CursorDown()
			if s.weeks.SelectedID() != prev {
				s.syncSelected()
			}
		}
		return nil
	case IsMoveUp(ks):
		if s.focusR {
			if s.editCur > 0 {
				s.editCur--
			}
		} else {
			prev := s.weeks.SelectedID()
			s.weeks.CursorUp()
			if s.weeks.SelectedID() != prev {
				s.syncSelected()
			}
		}
		return nil
	case IsMoveLeft(ks):
		s.focusR = false
		return nil
	case IsMoveRight(ks):
		if s.selected != nil {
			s.focusR = true
		}
		return nil
	case IsConfirm(ks):
		if s.focusR {
			s.startEditing()
		} else if s.selected != nil {
			s.focusR = true
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
	s.weeks.SetFocused(!s.focusR)
	return lipglossJoin(
		PanelFixed("REVIEWS", SectionHeader("WEEKLY REVIEWS")+"\n"+s.weeks.View(), lw, bodyH, !s.focusR),
		PanelFixed("WEEKLY REPORT", s.reportView(rw-6), rw, bodyH, s.focusR),
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
	ctx := context.Background()
	ev := runtime.ComputeWeeklyEvidence(ctx, s.app.DB, r.WeekStart, r.WeekEnd)
	// Prefer stored review totals when present, else derived evidence.
	planned := r.PlannedSeconds
	actual := r.ElapsedSeconds
	if planned == 0 {
		planned = ev.Planned
	}
	if actual == 0 {
		actual = ev.Actual
	}
	// Completion is task-count based: DONE planned tasks over total planned
	// tasks. Hours never determine it; "—" when nothing was planned.
	completion := "—"
	tasksValue := "—"
	if ev.TasksTotal > 0 {
		completion = fmt.Sprintf("%.0f%%", float64(ev.Tasks)/float64(ev.TasksTotal)*100)
		tasksValue = fmt.Sprintf("%d / %d", ev.Tasks, ev.TasksTotal)
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(FormatDateRange(r.WeekStart, r.WeekEnd)) + "  " + thMeta.Render(r.ID) + "\n")
	// Compact metric cards: PLANNED ACTUAL COMPLETION SESSIONS TASKS OVER/UNDER.
	cards := []MetricCard{
		{Label: "PLANNED", Value: FormatDurationLong(planned)},
		{Label: "ACTUAL", Value: FormatDurationLong(actual)},
		{Label: "COMPLETION", Value: completion},
		{Label: "SESSIONS", Value: fmt.Sprint(ev.Sessions)},
		{Label: "TASKS", Value: tasksValue},
		{Label: "OVER/UNDER", Value: formatSignedLong(r.OverrunSeconds)},
	}
	sb.WriteString(MetricGrid(cards, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("TIME BY DOMAIN") + "\n")
	total := ev.GovSeconds + ev.DSASeconds + ev.SWESeconds
	sb.WriteString(MetricGrid([]MetricCard{
		{Label: "GOV", Value: FormatDurationLong(ev.GovSeconds) + " · " + pctOf(ev.GovSeconds, total), Accent: cGov},
		{Label: "DSA", Value: FormatDurationLong(ev.DSASeconds) + " · " + pctOf(ev.DSASeconds, total), Accent: cDSA},
		{Label: "SWE", Value: FormatDurationLong(ev.SWESeconds) + " · " + pctOf(ev.SWESeconds, total), Accent: cSWE},
	}, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("DOMAIN EVIDENCE") + "\n")
	sb.WriteString(s.govEvidenceBlock(ctx, r.WeekStart, r.WeekEnd, ev, inner))
	sb.WriteString(EvidenceRow("DSA", []StatPair{
		{"Problems", fmt.Sprint(ev.DSAAttempts)},
		{"Passed", fmt.Sprint(ev.DSAPassed)},
	}) + "\n")
	sb.WriteString(EvidenceRow("SWE", []StatPair{
		{"Completed", fmt.Sprint(ev.SWECompleted)},
		{"Partial", fmt.Sprint(ev.SWEPartial)},
		{"Blocked", fmt.Sprint(ev.SWEBlocked)},
	}) + "\n")
	sb.WriteString(SectionHeader("USER REFLECTIONS") + "\n")
	sb.WriteString(s.reflectionsBlock(inner) + "\n")
	return sb.String()
}

// govEvidenceBlock renders subject-aware GOV evidence: one Lectures /
// Practice Questions / Accuracy row per subject with evidence, plus the
// aggregate zero row when the week has none.
func (s *ReviewsScreen) govEvidenceBlock(ctx context.Context, start, end string, ev runtime.WeeklyEvidence, inner int) string {
	var sb strings.Builder
	subs := runtime.ComputeWeeklyGovSubjects(ctx, s.app.DB, start, end)
	if len(subs) == 0 {
		sb.WriteString(EvidenceRow("GOV", []StatPair{
			{"Lectures", fmt.Sprint(ev.GovLectures)},
			{"Questions", fmt.Sprint(ev.GovAttempted)},
			{"Accuracy", fmtAcc(ev.GovAccuracy, ev.GovAttempted)},
		}) + "\n")
		return sb.String()
	}
	sb.WriteString(DomainStyle("GOV").Render("GOV") + "\n")
	for _, g := range subs {
		sb.WriteString("  " + thPrimary.Render(TruncateToWidth(g.Subject, 12)) +
			"   " + thMeta.Render(padRight("Lectures", 9)) + " " + thPrimary.Render(fmt.Sprint(g.Lectures)) +
			"   " + thMeta.Render(padRight("Questions", 9)) + " " + thPrimary.Render(fmt.Sprint(g.Attempted)) +
			"   " + thMeta.Render(padRight("Accuracy", 9)) + " " + thPrimary.Render(fmtAcc(g.Accuracy, g.Attempted)) + "\n")
	}
	return TruncateToWidth(sb.String(), inner)
}

// reflectionsBlock renders the two user reflections: selectable rows in
// view mode, live text inputs plus SAVE in editing mode.
func (s *ReviewsScreen) reflectionsBlock(inner int) string {
	r := s.selected
	if r == nil {
		return EmptyState("Select a week.")
	}
	if inner < 24 {
		inner = 24
	}
	if !s.editing {
		var sb strings.Builder
		worked := orDash(r.ReflectionWorked)
		if worked == "—" {
			worked = "— empty —"
		}
		change := orDash(r.ReflectionChange)
		if change == "—" {
			change = "— empty —"
		}
		sb.WriteString(SelectableRow{Num: "01", Mark: "•", Title: "What worked well this week?", Meta: worked,
			Selected: s.focusR && s.editCur == 0, Focused: s.focusR && s.editCur == 0, Width: inner}.View() + "\n")
		sb.WriteString(SelectableRow{Num: "02", Title: "What should change next week?", Meta: change, Mark: "•",
			Selected: s.focusR && s.editCur == 1, Focused: s.focusR && s.editCur == 1, Width: inner}.View() + "\n")
		if s.focusR {
			sb.WriteString(thHelp.Render("Enter edit"))
		} else {
			sb.WriteString(thHelp.Render("H/L pane · Enter report"))
		}
		return sb.String()
	}
	// Compact labeled rows + single-line save: the full edit block must
	// fit the panel viewport together with the report above it.
	iw := inner - 16
	if iw < 12 {
		iw = 12
	}
	if iw > 44 {
		iw = 44
	}
	s.editIn[0].SetWidth(iw)
	s.editIn[1].SetWidth(iw)
	var sb strings.Builder
	f0 := FormField{Label: "Worked well", Input: s.editIn[0]}
	f1 := FormField{Label: "Should change", Input: s.editIn[1]}
	sb.WriteString(f0.View(s.editFoc == 0, 13) + "\n")
	sb.WriteString(f1.View(s.editFoc == 1, 13) + "\n")
	save := Button{Label: "SAVE", Focused: s.editFoc == 2}
	sb.WriteString(save.CompactView() + "  " + thHelp.Render("↑/↓ move · Enter save") + "\n")
	return sb.String()
}

// formatSignedLong renders signed metric durations: -600->"-10m".
func formatSignedLong(secs int) string {
	if secs < 0 {
		return "-" + FormatDurationLong(-secs)
	}
	return FormatDurationLong(secs)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

