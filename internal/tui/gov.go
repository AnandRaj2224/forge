package tui

import (
	"context"
	"fmt"
	"strings"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listItem is a real selectable list row.
type listItem struct {
	id    string
	title string
	desc  string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.desc }
func (i listItem) FilterValue() string { return i.title }

// Gov levels: dashboard -> exam workspace | subjects browser -> topic.
// No tab system, no hidden cycle keys. Topic detail always keeps the left
// navigator visible (right-panel replacement only).
const (
	govDashboard = "dashboard"
	govExam      = "exam"
	govSubjects  = "subjects"
	govTopic     = "topic"
)

// Gov focus zones inside workspaces.
const (
	gzLeft = iota
	gzRight
)

type GovScreen struct {
	app       *App
	level     string
	exams     []db.GovExam
	coverages map[db.ID]ExamCoverage
	cursor    int // dashboard grid cursor over exams + subjects card
	gridCols  int

	// Selected exam workspace.
	examID      db.ID
	coverage    *ExamCoverage
	examSubject string
	subjMenu    Navigator
	topicList   Navigator
	zone        int

	// Global subjects browser.
	subjNav      Navigator
	subTopicList Navigator
	subZone      int

	detail   *GovTopicView
	returnTo string
	err      string
}

type GovTopicView struct {
	Topic    db.GovTopic
	Subject  string
	Done     int
	Total    int
	Flags    []bool
	Lectures []db.GovLecture
	Stats    TopicPracticeStats
	UsedBy   []string
}

func NewGovScreen(a *App) *GovScreen {
	return &GovScreen{app: a, level: govDashboard, coverages: map[db.ID]ExamCoverage{},
		subjMenu: NewNavigator(), topicList: NewNavigator(),
		subjNav: NewNavigator(), subTopicList: NewNavigator()}
}

func (s *GovScreen) Reload() {
	ctx := context.Background()
	s.err = ""
	exams, err := catalog.GetGovExams(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.exams = exams
	for _, e := range exams {
		if cov, err := ComputeExamCoverage(ctx, s.app.DB, e.ID); err == nil {
			s.coverages[e.ID] = cov
		}
	}
	if s.level == govExam && s.examID != "" {
		s.reloadExam()
	} else if s.level == govSubjects {
		s.reloadSubjectsBrowser()
	}
}

func (s *GovScreen) dashboardItems() int { return len(s.exams) + 1 } // + subjects card

func (s *GovScreen) openSelected() {
	if s.cursor < len(s.exams) {
		s.examID = s.exams[s.cursor].ID
		s.examSubject = ""
		s.level = govExam
		s.zone = gzLeft
		s.reloadExam()
	} else {
		s.level = govSubjects
		s.subZone = gzLeft
		s.reloadSubjectsBrowser()
	}
}

// ---- selected exam workspace (exam-only subjects/topics) ----

func (s *GovScreen) reloadExam() {
	ctx := context.Background()
	cov, err := ComputeExamCoverage(ctx, s.app.DB, s.examID)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.coverage = &cov
	if s.examSubject == "" {
		s.examSubject = "Quant"
		found := false
		for _, sc := range cov.Subjects {
			if sc.Subject == "Quant" {
				found = true
			}
		}
		if !found && len(cov.Subjects) > 0 {
			s.examSubject = cov.Subjects[0].Subject
		}
	}
	menu := make([]list.Item, 0, len(cov.Subjects))
	for _, sc := range cov.Subjects {
		r := 0.0
		if sc.Total > 0 {
			r = float64(sc.Done) / float64(sc.Total)
		}
		menu = append(menu, listItem{id: string(sc.SubjectID), title: fmt.Sprintf("%s  %d/%d  %3.0f%%", sc.Subject, sc.Done, sc.Total, r*100)})
	}
	s.subjMenu.SetItems(menu)
	for i, sc := range cov.Subjects {
		if sc.Subject == s.examSubject {
			s.subjMenu.List.Select(i)
		}
	}
	s.reloadExamTopics()
}

func (s *GovScreen) reloadExamTopics() {
	ctx := context.Background()
	if s.coverage == nil {
		return
	}
	syll, _ := catalog.GetGovExamSyllabus(ctx, s.app.DB, s.coverage.ExamID)
	inExam := map[db.ID]bool{}
	for _, mp := range syll {
		inExam[mp.TopicID] = true
	}
	var subjID db.ID
	for _, sc := range s.coverage.Subjects {
		if sc.Subject == s.examSubject {
			subjID = sc.SubjectID
		}
	}
	items := []list.Item{}
	if subjID != "" {
		topics, _ := catalog.GetGovTopics(ctx, s.app.DB, subjID)
		for _, t := range topics {
			if !inExam[t.ID] {
				continue
			}
			mark := "○"
			if topicLectureComplete(ctx, s.app.DB, t.ID) {
				mark = "✓"
			}
			items = append(items, listItem{id: string(t.ID), title: mark + "  " + t.Name})
		}
	}
	s.topicList.SetItems(items)
}

// ---- global subjects browser ----

func (s *GovScreen) reloadSubjectsBrowser() {
	ctx := context.Background()
	subjs, err := catalog.GetGovSubjects(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	items := make([]list.Item, 0, len(subjs))
	for _, g := range subjs {
		items = append(items, listItem{id: string(g.ID), title: g.Name})
	}
	s.subjNav.SetItems(items)
	s.reloadBrowserTopics()
}

func (s *GovScreen) reloadBrowserTopics() {
	ctx := context.Background()
	sid := s.subjNav.SelectedID()
	if sid == "" {
		s.subTopicList.SetItems(nil)
		return
	}
	topics, err := catalog.GetGovTopics(ctx, s.app.DB, db.ID(sid))
	if err != nil {
		s.err = err.Error()
		return
	}
	items := make([]list.Item, 0, len(topics))
	for _, t := range topics {
		done, total, _, _ := TopicLectureProgress(ctx, s.app.DB, t.ID)
		items = append(items, listItem{id: string(t.ID), title: fmt.Sprintf("%s  %d/%d", t.Name, done, total)})
	}
	s.subTopicList.SetItems(items)
}

func (s *GovScreen) openTopic(topicID string) {
	ctx := context.Background()
	tp, err := catalog.GetGovTopic(ctx, s.app.DB, db.ID(topicID))
	if err != nil {
		s.err = err.Error()
		return
	}
	subj := string(tp.SubjectID)
	if g, err := catalog.GetGovSubject(ctx, s.app.DB, tp.SubjectID); err == nil {
		subj = g.Name
	}
	done, total, flags, lecs := TopicLectureProgress(ctx, s.app.DB, tp.ID)
	s.detail = &GovTopicView{Topic: tp, Subject: subj, Done: done, Total: total, Flags: flags, Lectures: lecs,
		Stats: ComputeTopicPracticeStats(ctx, s.app.DB, tp.ID), UsedBy: ExamsForTopic(ctx, s.app.DB, tp.ID)}
	s.returnTo = s.level
	s.level = govTopic
}

// onBack pops right-panel detail first, then workspace to dashboard.
func (s *GovScreen) onBack() bool {
	switch s.level {
	case govTopic:
		s.level = s.returnTo
		if s.level == "" {
			s.level = govExam
		}
		return true
	case govExam, govSubjects:
		s.level = govDashboard
		return true
	}
	return false
}

func (s *GovScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "tab":
			switch s.level {
			case govExam:
				if s.zone == gzLeft {
					s.zone = gzRight
				} else {
					s.zone = gzLeft
				}
			case govSubjects:
				if s.subZone == gzLeft {
					s.subZone = gzRight
				} else {
					s.subZone = gzLeft
				}
			}
			return nil
		case "enter":
			return s.pressEnter()
		case "j", "down", "k", "up", "h", "left", "l", "right":
			return s.move(k.String())
		}
	}
	return s.delegate(msg)
}

func (s *GovScreen) delegate(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch s.level {
	case govExam:
		if s.zone == gzLeft {
			s.subjMenu.List, cmd = s.subjMenu.List.Update(msg)
		} else {
			s.topicList.List, cmd = s.topicList.List.Update(msg)
		}
	case govSubjects:
		if s.subZone == gzLeft {
			s.subjNav.List, cmd = s.subjNav.List.Update(msg)
		} else {
			s.subTopicList.List, cmd = s.subTopicList.List.Update(msg)
		}
	}
	return cmd
}

func (s *GovScreen) move(key string) tea.Cmd {
	down := key == "j" || key == "down"
	up := key == "k" || key == "up"
	left := key == "h" || key == "left"
	right := key == "l" || key == "right"
	switch s.level {
	case govDashboard:
		n := s.dashboardItems()
		cols := s.gridCols
		if cols < 1 {
			cols = 1
		}
		switch {
		case down:
			s.cursor = (s.cursor + cols) % n
		case up:
			s.cursor = (s.cursor - cols + n) % n
		case right:
			s.cursor = (s.cursor + 1) % n
		case left:
			s.cursor = (s.cursor - 1 + n) % n
		}
		return nil
	case govExam:
		if s.zone == gzLeft {
			prev := s.subjMenu.SelectedID()
			if down {
				s.subjMenu.List.CursorDown()
			} else if up {
				s.subjMenu.List.CursorUp()
			} else {
				return nil
			}
			if id := s.subjMenu.SelectedID(); id != prev && s.coverage != nil {
				for _, sc := range s.coverage.Subjects {
					if string(sc.SubjectID) == id {
						s.examSubject = sc.Subject
					}
				}
				s.reloadExamTopics()
			}
		} else {
			if down {
				s.topicList.List.CursorDown()
			} else if up {
				s.topicList.List.CursorUp()
			}
		}
	case govSubjects:
		if s.subZone == gzLeft {
			prev := s.subjNav.SelectedID()
			if down {
				s.subjNav.List.CursorDown()
			} else if up {
				s.subjNav.List.CursorUp()
			} else {
				return nil
			}
			if s.subjNav.SelectedID() != prev {
				s.reloadBrowserTopics()
			}
		} else {
			if down {
				s.subTopicList.List.CursorDown()
			} else if up {
				s.subTopicList.List.CursorUp()
			}
		}
	}
	return nil
}

func (s *GovScreen) pressEnter() tea.Cmd {
	switch s.level {
	case govDashboard:
		s.openSelected()
	case govExam:
		if s.zone == gzLeft {
			s.zone = gzRight
		} else if id := s.topicList.SelectedID(); id != "" {
			s.openTopic(id)
		}
	case govSubjects:
		if s.subZone == gzLeft {
			s.subZone = gzRight
		} else if id := s.subTopicList.SelectedID(); id != "" {
			s.openTopic(id)
		}
	}
	return nil
}

func (s *GovScreen) View() string {
	w, h := s.app.contentViewport()
	switch s.level {
	case govExam:
		return s.examView(w, h)
	case govSubjects:
		return s.subjectsView(w, h)
	case govTopic:
		if s.returnTo == govSubjects {
			return s.topicInWorkspace(w, h, true)
		}
		return s.topicInWorkspace(w, h, false)
	default:
		return s.dashboardView(w, h)
	}
}

// ---- dashboard: responsive bento grid of real selectable exam cards ----

func (s *GovScreen) dashboardView(w, h int) string {
	cols := 1
	if w >= 130 {
		cols = 3
	} else if w >= 80 {
		cols = 2
	}
	s.gridCols = cols
	cardW := (w - (cols-1)*3) / cols
	if cardW < 26 {
		cardW = 26
	}
	// Uniform cells: one per exam plus the subjects-browser entry.
	cells := make([]string, 0, len(s.exams)+1)
	for i, e := range s.exams {
		cov := s.coverages[e.ID]
		cells = append(cells, ExamCard{Name: e.Name, Done: cov.Done, Total: cov.Total, Focused: s.cursor == i, Width: cardW - 4}.View())
	}
	cells = append(cells, s.subjectsCell(cardW-4, s.cursor == len(s.exams)))
	// Scroll the grid so the cursor row stays visible at any height.
	gridRows := [][]string{}
	for i := 0; i < len(cells); i += cols {
		gridRows = append(gridRows, cells[i:minInt(i+cols, len(cells))])
	}
	headLines := 3 // title + subtitle + gap
	maxRows := (h - headLines) / (examCardHeight + 1)
	if maxRows < 1 {
		maxRows = 1
	}
	curRow := s.cursor / cols
	start := 0
	if len(gridRows) > maxRows {
		start = curRow - maxRows + 1
		if start < 0 {
			start = 0
		}
		if start > len(gridRows)-maxRows {
			start = len(gridRows) - maxRows
		}
	}
	gap := "   "
	var rendered []string
	for _, r := range gridRows[start:minInt(start+maxRows, len(gridRows))] {
		parts := make([]string, 0, len(r)*2-1)
		for i, c := range r {
			if i > 0 {
				parts = append(parts, gap)
			}
			parts = append(parts, c)
		}
		rendered = append(rendered, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	}
	body := thScreenTitle.Render("GOVERNMENT EXAMS") + "\n"
	body += thMeta.Render("Select an exam to enter its workspace.") + "\n\n"
	body += strings.Join(rendered, "\n\n")
	if start > 0 {
		body += "\n" + thHelp.Render("\u2191 more above")
	}
	if start+maxRows < len(gridRows) {
		body += "\n" + thHelp.Render("\u2193 more below")
	}
	if s.err != "" {
		body += "\n" + thError.Render(s.err)
	}
	return body
}

func (s *GovScreen) subjectsCell(w int, focused bool) string {
	if w < 28 {
		w = 28
	}
	lines := []string{
		thScreenTitle.Render("ALL SUBJECTS"),
		"",
		thMeta.Render("Global topic browser."),
		thMeta.Render("Every subject, every topic."),
		"",
		thHelp.Render("Enter to open"),
		"",
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDimLine).Padding(0, 1).Width(w)
	if focused {
		style = style.BorderForeground(cPrimary)
		lines[0] = thPanelTitle.Render("ALL SUBJECTS  \u25cf")
	}
	return style.Render(strings.Join(lines, "\n"))
}

// ---- exam workspace + subjects browser: persistent two-panel ----

func (s *GovScreen) examView(w, h int) string {
	lw, rw := splitW(w, 30, 22, 40)
	listH := h - 6
	if listH < 8 {
		listH = 8
	}
	s.subjMenu.List.SetWidth(lw - 4)
	s.topicList.List.SetWidth(rw - 4)
	s.subjMenu.FitHeight(listH)
	s.topicList.FitHeight(listH - 12)
	name := ""
	if s.coverage != nil {
		name = s.coverage.Exam
	}
	leftBody := thScreenTitle.Render(name) + "\n\n" + SectionHeader("SUBJECTS") + "\n" + s.subjMenu.List.View()
	left := Panel("EXAM", leftBody, lw, s.zone == gzLeft)
	var right string
	if s.coverage != nil {
		c := s.coverage
		var sb strings.Builder
		sb.WriteString(SectionHeader("SYLLABUS COVERAGE") + "\n")
		sb.WriteString(ProgressBar{Ratio: c.Percent(), Width: 26, ShowPct: true}.View() + "\n")
		sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d topics", c.Done, c.Total)) + "\n\n")
		sb.WriteString(SectionHeader("PRACTICE READINESS") + "\n")
		sb.WriteString(ProgressBar{Ratio: 0, Width: 26}.View() + "  " + thHelp.Render("not yet calculated") + "\n\n")
		sb.WriteString(SectionHeader("TOPICS — "+s.examSubject) + "\n")
		sb.WriteString(s.topicList.List.View())
		right = Panel(s.examSubject, sb.String(), rw, s.zone == gzRight)
	} else {
		right = Panel("EXAM", EmptyState("Select a subject."), rw, s.zone == gzRight)
	}
	return lipglossJoin(left, right)
}

func (s *GovScreen) subjectsView(w, h int) string {
	lw, rw := splitW(w, 30, 22, 40)
	listH := h - 6
	if listH < 8 {
		listH = 8
	}
	s.subjNav.List.SetWidth(lw - 4)
	s.subTopicList.List.SetWidth(rw - 4)
	s.subjNav.FitHeight(listH)
	s.subTopicList.FitHeight(listH - 10)
	left := Panel("SUBJECTS", s.subjNav.List.View(), lw, s.subZone == gzLeft)
	right := Panel("TOPICS", s.browserRight(), rw, s.subZone == gzRight)
	return lipglossJoin(left, right)
}

func (s *GovScreen) browserRight() string {
	sid := s.subjNav.SelectedID()
	if sid == "" {
		return EmptyState("Select a subject.")
	}
	ctx := context.Background()
	name := sid
	if g, err := catalog.GetGovSubject(ctx, s.app.DB, db.ID(sid)); err == nil {
		name = g.Name
	}
	topics, _ := catalog.GetGovTopics(ctx, s.app.DB, db.ID(sid))
	done, total := 0, len(topics)
	for _, t := range topics {
		if topicLectureComplete(ctx, s.app.DB, t.ID) {
			done++
		}
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(name) + "\n\n")
	sb.WriteString(SectionHeader("LECTURES") + "\n")
	sb.WriteString(ProgressBar{Ratio: ratio(done, total), Width: 26, ShowPct: true}.View() + "\n")
	sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d topics lecture-complete", done, total)) + "\n\n")
	sb.WriteString(SectionHeader("TOPICS") + "\n")
	sb.WriteString(s.subTopicList.List.View())
	return sb.String()
}

func (s *GovScreen) topicInWorkspace(w, h int, fromSubjects bool) string {
	lw, rw := splitW(w, 30, 22, 40)
	var left string
	if fromSubjects {
		s.subjNav.List.SetWidth(lw - 4)
		s.subjNav.FitHeight(h - 6)
		left = Panel("SUBJECTS", s.subjNav.List.View(), lw, false)
	} else {
		s.subjMenu.List.SetWidth(lw - 4)
		s.subjMenu.FitHeight(h - 6)
		name := ""
		if s.coverage != nil {
			name = s.coverage.Exam
		}
		left = Panel(name, SectionHeader("SUBJECTS")+"\n"+s.subjMenu.List.View(), lw, false)
	}
	return lipglossJoin(left, Panel("TOPIC", s.topicBody(), rw, true))
}

func (s *GovScreen) topicBody() string {
	v := s.detail
	if v == nil {
		return EmptyState("Topic not found.")
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(v.Topic.Name) + "  " + thMeta.Render("·  "+v.Subject) + "\n\n")
	sb.WriteString(SectionHeader("LECTURES") + "\n")
	sb.WriteString(ProgressBar{Ratio: ratio(v.Done, v.Total), Width: 26, ShowPct: true}.View() + "\n")
	sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d", v.Done, v.Total)) + "\n\n")
	sb.WriteString(SectionHeader("PRACTICE") + "\n")
	st := v.Stats
	sb.WriteString(MetaRow("Questions", fmt.Sprint(st.Attempted), 10) + "\n")
	sb.WriteString(MetaRow("Correct", fmt.Sprint(st.Correct), 10) + "\n")
	sb.WriteString(MetaRow("Incorrect", fmt.Sprint(st.Incorrect), 10) + "\n")
	sb.WriteString(MetaRow("Accuracy", fmt.Sprintf("%.1f%%", st.Accuracy*100), 10) + "\n")
	sb.WriteString(MetaRow("Avg speed", avgStr(st), 10) + "\n\n")
	sb.WriteString(SectionHeader("PROGRESSION") + "  " + thHelp.Render("not yet calculated") + "\n\n")
	sb.WriteString(SectionHeader("LECTURES") + "\n")
	for i, l := range v.Lectures {
		mark := thTodo.Render("○")
		if i < len(v.Flags) && v.Flags[i] {
			mark = thSuccess.Render("✓")
		}
		sb.WriteString(fmt.Sprintf("  %s  %s\n", mark, thPrimary.Render(l.Title)))
	}
	if len(v.Lectures) == 0 {
		sb.WriteString("  " + thHelp.Render("no lectures — practice unlocked") + "\n")
	}
	sb.WriteString("\n" + SectionHeader("USED BY") + "\n  " + thMeta.Render(strings.Join(v.UsedBy, "  ·  ")))
	return sb.String()
}

func (s *GovScreen) Hints() string {
	switch s.level {
	case govDashboard:
		return "h/j/k/l Move   Enter Open"
	default:
		return "Tab Focus   j/k Move   Enter Open   Esc Back"
	}
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}

func avgStr(st TopicPracticeStats) string {
	if !st.HasSpeed {
		return "—"
	}
	return fmt.Sprintf("%.1fs", st.AvgSpeed)
}
