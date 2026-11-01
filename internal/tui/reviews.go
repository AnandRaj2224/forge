package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// ---------- REVIEWS: persistent two-panel weekly workspace ----------

type weekEntry struct {
	ID     string
	Start  string
	End    string
	stored bool
}

type ReviewsScreen struct {
	app        *App
	weeks      Navigator
	reports    []db.WeeklyReview
	entries    []weekEntry
	selected   *db.WeeklyReview
	focusR     bool // right report pane owns focus
	editCur    int  // reflection cursor in view mode (0 worked, 1 change)
	editing    bool // reflection editing mode
	editFoc    int  // editing focus (0 worked, 1 change, 2 save)
	editIn     [2]textinput.Model
	bodyScroll ScrollViewport // report pane scrolls independently when overflowing
	nowFn      func() time.Time
	err        string
	// liveReport caches the live weekly draft for the selected unstored week
	// (populated in syncSelected). View renders from this cache and never
	// queries the database.
	liveReport *engine.WeeklyReport
	liveErr    string
}

func NewReviewsScreen(a *App) *ReviewsScreen {
	return &ReviewsScreen{app: a, weeks: NewNavigator(), nowFn: time.Now}
}

// lastWeeks returns the last n Mon..Sun weeks (newest first) as id/start/end
// on the Forge calendar (Asia/Kolkata, Monday start). Never UTC date math:
// week membership must match History and the stored YYYY-Www review IDs.
func lastWeeks(now time.Time, n int) []weekEntry {
	cal := engine.DefaultCalendar()
	week := cal.CurrentWeek(now)
	if len(week) == 0 {
		return nil
	}
	monday, err := time.ParseInLocation("2006-01-02", week[0], cal.Location())
	if err != nil {
		return nil
	}
	out := make([]weekEntry, 0, n)
	for i := 0; i < n; i++ {
		mon := monday.AddDate(0, 0, -7*i)
		sun := mon.AddDate(0, 0, 6)
		start := mon.Format("2006-01-02")
		out = append(out, weekEntry{
			ID:    cal.ISOWeekID(start),
			Start: start,
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
	stored := map[string]db.WeeklyReview{}
	for _, r := range reps {
		stored[r.ID] = r
	}
	// Display weeks: the current week (auto-created live when required via
	// the Forge calendar, Monday 00:00 Asia/Kolkata) plus previous saved
	// weeks. No manual creation, no draft labels. The current-week entry
	// automatically exists the first time Forge ensures data for that week.
	cur := lastWeeks(s.nowFn(), 1)
	var curEntry weekEntry
	if len(cur) > 0 {
		curEntry = cur[0]
		if r, ok := stored[curEntry.ID]; ok {
			curEntry.stored = true
			_ = r
		}
	}
	entries := make([]weekEntry, 0, 1+len(reps))
	if len(cur) > 0 {
		entries = append(entries, curEntry)
	}
	for _, r := range reps {
		if r.ID == curEntry.ID {
			continue
		}
		entries = append(entries, weekEntry{ID: r.ID, Start: r.WeekStart, End: r.WeekEnd, stored: true})
	}
	sortWeekEntries(entries)
	// Preserve the cursor across reloads; default to the current week.
	keep := s.weeks.SelectedID()
	s.entries = entries
	items := make([]RowItem, 0, len(entries))
	for _, e := range entries {
		mark := "○"
		if e.stored {
			mark = "•"
		}
		items = append(items, RowItem{ID: e.ID, Title: FormatDateRange(e.Start, e.End), Meta: e.ID, Mark: mark})
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
		// Default to the current week (last in sorted order when current
		// is newest, otherwise find it explicitly).
		found := false
		for i, e := range entries {
			if len(cur) > 0 && e.ID == cur[0].ID {
				s.weeks.Select(i)
				found = true
				break
			}
		}
		if !found {
			s.weeks.Select(len(entries) - 1)
		}
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
	s.bodyScroll.Reset()
	s.liveReport = nil
	s.liveErr = ""
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
	// No stored review: live current-week calculations from the engine.
	// Metrics stay automatically updated as the week progresses; the
	// structured snapshot is frozen only on explicit SAVE. Internal
	// live-vs-saved distinction never surfaces as draft language.
	// The draft is loaded once here (Reload/selection change); View renders
	// purely from the s.selected snapshot + s.liveReport cache.
	for _, e := range s.entries {
		if e.ID != id {
			continue
		}
		now := s.nowFn()
		live, err := engine.BuildWeeklyDraft(s.app.ctx(), s.app.DB, e.ID, e.Start, e.End)
		if err != nil {
			s.err = "data error: " + err.Error()
			s.liveErr = err.Error()
			return
		}
		cached := live
		s.liveReport = &cached
		js, _ := engine.MarshalWeeklySnapshot(live)
		if js == "" {
			js = "{}"
		}
		s.selected = &db.WeeklyReview{
			ID:             e.ID,
			WeekStart:      e.Start,
			WeekEnd:        e.End,
			GeneratedAt:    now,
			UpdatedAt:      now,
			PlannedSeconds: live.Planned,
			ElapsedSeconds: live.Actual,
			OverrunSeconds: live.Actual - live.Planned,
			ReportJSON:     js,
		}
		return
	}
	s.selected = nil
}

// isStored reports whether a week ID has a saved snapshot.
func (s *ReviewsScreen) isStored(id string) bool {
	for _, r := range s.reports {
		if r.ID == id {
			return true
		}
	}
	return false
}

// isStoredCurrent reports whether the selected week is already saved
// (read-only, frozen). The live current week (not yet saved) is editable
// once via its two reflection inputs + SAVE.
func (s *ReviewsScreen) isStoredCurrent() bool {
	if s.selected == nil {
		return false
	}
	return s.isStored(s.selected.ID)
}

// textFocus reports whether a reflection text input owns focus (letters
// type there, including q; Esc cancels editing via the app, never quits).
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

// startEditing opens the reflection editor with the two user-entered
// fields. Saved reviews are read-only and cannot be edited again.
func (s *ReviewsScreen) startEditing() {
	if s.selected == nil {
		return
	}
	if s.isStored(s.selected.ID) {
		return // saved review is frozen: metrics + reflections read-only
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
	s.editFoc = 0
	s.editing = true
	s.bodyScroll.Offset = 1 << 30 // clamped on render: inputs stay visible
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

// saveReflections persists the two reflection answers once and freezes the
// review: metrics + reflections become read-only, no further editing,
// refresh or regenerate. For the live current week it validates and freezes
// the snapshot together with the reflections; stored reviews never reach
// here (startEditing blocks them).
func (s *ReviewsScreen) saveReflections() {
	if s.selected == nil {
		return
	}
	r := *s.selected
	worked := s.editIn[0].Value()
	change := s.editIn[1].Value()
	if _, err := engine.SaveReviewReflections(s.app.ctx(), s.app.DB, r, worked, change, s.nowFn()); err != nil {
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
		// FORM rule: ↑/↓ always moves field focus (0 worked, 1 change,
		// 2 SAVE) and the viewport follows; j/k types inside text inputs
		// and moves like ↑/↓ on the SAVE button. Enter on SAVE saves once.
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
		case "j":
			if s.editFoc >= 2 {
				return nil // SAVE is bottom; stays visible
			}
			// In text inputs j types (forward below).
		case "k":
			if s.editFoc >= 2 {
				s.editFoc--
				s.updateEditFocus()
				return nil
			}
			// In text inputs k types (forward below).
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
	case IsMoveDown(ks) || IsMoveUp(ks):
		down := IsMoveDown(ks)
		if s.focusR {
			// NON-SELECTABLE detail/report content: ↑/↓ and j/k scroll
			// the pane directly. No fake cursor.
			inner, total := s.reportViewportH(), len(strings.Split(s.reportView(s.reportInnerW()), "\n"))
			if down {
				s.bodyScroll.ScrollDown(total, inner)
			} else {
				s.bodyScroll.ScrollUp(total, inner)
			}
			return nil
		}
		prev := s.weeks.SelectedID()
		if down {
			s.weeks.CursorDown()
		} else {
			s.weeks.CursorUp()
		}
		if s.weeks.SelectedID() != prev {
			s.syncSelected()
			s.bodyScroll.Reset()
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
		PanelFixed("WEEKLY REPORT", ViewportView(s.reportView(rw-6), &s.bodyScroll, innerH), rw, bodyH, s.focusR),
	)
}

// reportViewportH mirrors View geometry for scrolling.
func (s *ReviewsScreen) reportViewportH() int {
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

// reportInnerW returns the actual report inner width (same geometry as View).
func (s *ReviewsScreen) reportInnerW() int {
	w, _ := s.app.contentViewport()
	_, rw := TwoPaneLayout(w, 32, 24, 40)
	inner := rw - 6
	if inner < 24 {
		inner = 24
	}
	return inner
}

func (s *ReviewsScreen) reportView(inner int) string {
	if s.selected == nil {
		return thMeta.Render("Select a week.")
	}
	if inner < 24 {
		inner = 24
	}
	r := s.selected
	// Stored reviews render the persisted snapshot; the live current week
	// renders the cached draft loaded in syncSelected. Never recompute a
	// stored review silently, and never query the database during render.
	var rep engine.WeeklyReport
	stored := false
	for _, sr := range s.reports {
		if sr.ID == r.ID {
			stored = true
			break
		}
	}
	if stored && r.ReportJSON != "" && r.ReportJSON != "{}" {
		if parsed, err := engine.UnmarshalWeeklySnapshot(r.ReportJSON); err == nil && parsed.WeekID == r.ID {
			rep = parsed
		} else {
			rep = snapshotFromRow(r)
		}
	} else if stored {
		rep = snapshotFromRow(r)
	} else {
		// Live week: render purely from the syncSelected cache. The cache
		// holds the authoritative draft; View performs no I/O.
		if s.liveErr != "" {
			return thError.Render("report unavailable · data error") + "\n" + thMeta.Render(s.liveErr)
		}
		if s.liveReport != nil {
			rep = *s.liveReport
		} else if r.ReportJSON != "" && r.ReportJSON != "{}" {
			if parsed, err := engine.UnmarshalWeeklySnapshot(r.ReportJSON); err == nil {
				rep = parsed
			} else {
				rep = snapshotFromRow(r)
			}
		} else {
			rep = snapshotFromRow(r)
		}
	}
	completion := "—"
	tasksValue := "—"
	if rep.TasksTotal > 0 {
		if rep.CompletionPct != nil {
			completion = fmt.Sprintf("%.0f%%", *rep.CompletionPct)
		} else {
			completion = fmt.Sprintf("%.0f%%", float64(rep.TasksCompleted)/float64(rep.TasksTotal)*100)
		}
		tasksValue = fmt.Sprintf("%d of %d", rep.TasksCompleted, rep.TasksTotal)
	}
	contextTag := "SELECTED WEEK"
	if cur := lastWeeks(s.nowFn(), 1); len(cur) > 0 && cur[0].ID == r.ID {
		contextTag = "THIS WEEK"
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render("WEEKLY REVIEW") + "\n")
	sb.WriteString(thPrimary.Render(FormatDateRange(r.WeekStart, r.WeekEnd)) + "  " + thMeta.Render(r.ID+" · "+contextTag) + "\n")
	cards := []MetricCard{
		{Label: "PLANNED", Value: engine.FormatDurationLong(rep.Planned)},
		{Label: "ACTUAL", Value: engine.FormatDurationLong(rep.Actual)},
		{Label: "COMPLETION", Value: completion},
		{Label: "SESSIONS", Value: fmt.Sprint(rep.Sessions)},
		{Label: "TASKS", Value: tasksValue},
		{Label: "OVER/UNDER", Value: engine.FormatSigned(rep.Actual - rep.Planned)},
	}
	sb.WriteString(MetricGrid(cards, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("TIME BY DOMAIN · % OF TRACKED TIME") + "\n")
	total := rep.GovSeconds + rep.DSASeconds
	sb.WriteString(MetricGrid([]MetricCard{
		{Label: "GOV", Value: engine.FormatDurationLong(rep.GovSeconds) + " · " + pctOf(rep.GovSeconds, total) + " of tracked time", Accent: cGov},
		{Label: "DSA", Value: engine.FormatDurationLong(rep.DSASeconds) + " · " + pctOf(rep.DSASeconds, total) + " of tracked time", Accent: cDSA},
	}, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	summaryHeader := strings.ToUpper(r.ID) + " · DOMAIN SUMMARY"
	if contextTag == "THIS WEEK" || strings.HasPrefix(contextTag, "THIS WEEK") {
		summaryHeader = "THIS WEEK · DOMAIN SUMMARY"
	}
	sb.WriteString(SectionHeader(summaryHeader) + "\n")
	govBlock := weeklyGovBlock(rep)
	dsaBlock := DomainDetailBlock("DSA", "DSA PERFORMANCE", []StatPair{
		{"Problems attempted", fmt.Sprint(rep.DSAAttempted)},
		{"Problems passed", fmt.Sprint(rep.DSAPassed)},
		{"Problems not passed", fmt.Sprint(rep.DSANotPassed)},
	})
	sb.WriteString(JoinDetailBlocks([]string{govBlock, dsaBlock}, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("REFLECTIONS") + "\n")
	sb.WriteString(s.reflectionsBlock(inner) + "\n")
	return sb.String()
}

func snapshotFromRow(r *db.WeeklyReview) engine.WeeklyReport {
	rep := engine.WeeklyReport{
		WeekID: r.ID, WeekStart: r.WeekStart, WeekEnd: r.WeekEnd,
		Planned: r.PlannedSeconds, Actual: r.ElapsedSeconds,
		Variance: r.ElapsedSeconds - r.PlannedSeconds,
	}
	return rep
}

// weeklyGovBlock renders structured GOV performance per subject with explicit
// units, or aggregate rows when no subject evidence exists.
func weeklyGovBlock(rep engine.WeeklyReport) string {
	if len(rep.GovSubjects) == 0 {
		acc := "—"
		if rep.GovAccuracy != nil {
			acc = fmt.Sprintf("%.1f%%", *rep.GovAccuracy*100)
		}
		return DomainDetailBlock("GOV", "GOV PERFORMANCE", []StatPair{
			{"Lectures completed", fmt.Sprint(rep.GovLectures)},
			{"Questions attempted", fmt.Sprint(rep.GovAttempted)},
			{"Questions correct", fmt.Sprint(rep.GovCorrect)},
			{"Questions incorrect", fmt.Sprint(rep.GovAttempted - rep.GovCorrect)},
			{"Accuracy", acc},
		})
	}
	var sb strings.Builder
	sb.WriteString(DomainStyle("GOV").Render("GOV PERFORMANCE"))
	for _, g := range rep.GovSubjects {
		sb.WriteString("\n" + thPrimary.Render(TruncateToWidth(g.Subject, 18)))
		lecLine := fmt.Sprintf("%d lectures completed", g.Lectures)
		if g.Lectures == 1 {
			lecLine = "1 lecture completed"
		}
		qLine := fmt.Sprintf("%d questions attempted", g.Attempted)
		if g.Attempted == 1 {
			qLine = "1 question attempted"
		}
		cLine := fmt.Sprintf("%d correct · %d incorrect", g.Correct, g.Incorrect)
		accLine := "— accuracy"
		if g.Accuracy != nil {
			accLine = fmt.Sprintf("%.1f%% accuracy", *g.Accuracy*100)
		}
		sb.WriteString("\n" + MetaRow("  ", lecLine, 2))
		sb.WriteString("\n" + MetaRow("  ", qLine, 2))
		sb.WriteString("\n" + MetaRow("  ", cLine, 2))
		sb.WriteString("\n" + MetaRow("  ", accLine, 2))
	}
	return sb.String()
}

// reflectionsBlock renders the two user reflections (the only manual
// fields; everything else is automated): static rows in view mode, live
// text inputs plus SAVE in editing mode. Saved reviews are read-only.
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
		sb.WriteString(SelectableRow{Num: "01", Mark: "•", Title: "What worked well this week?", Meta: worked, Width: inner}.View() + "\n")
		sb.WriteString(SelectableRow{Num: "02", Title: "What should change next week?", Meta: change, Mark: "•", Width: inner}.View() + "\n")
		if s.isStored(r.ID) {
			sb.WriteString(thHelp.Render("Saved · read-only"))
		} else if s.focusR {
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

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
