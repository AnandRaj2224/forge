package tui

import (
	"context"
	"fmt"
	"strings"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	menu := make([]RowItem, 0, len(cov.Subjects))
	for _, sc := range cov.Subjects {
		r := 0.0
		if sc.Total > 0 {
			r = float64(sc.Done) / float64(sc.Total)
		}
		menu = append(menu, RowItem{
			ID:    string(sc.SubjectID),
			Title: sc.Subject,
			Meta:  fmt.Sprintf("%d/%d %3.0f%%", sc.Done, sc.Total, r*100),
			Mark:  "•",
		})
	}
	s.subjMenu.SetItems(menu)
	for i, sc := range cov.Subjects {
		if sc.Subject == s.examSubject {
			s.subjMenu.Select(i)
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
	items := []RowItem{}
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
			items = append(items, RowItem{ID: string(t.ID), Title: t.Name, Mark: mark})
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
	items := make([]RowItem, 0, len(subjs))
	for _, g := range subjs {
		topics, _ := catalog.GetGovTopics(ctx, s.app.DB, g.ID)
		items = append(items, RowItem{ID: string(g.ID), Title: g.Name, Meta: fmt.Sprintf("%d topics", len(topics)), Mark: "•"})
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
	items := make([]RowItem, 0, len(topics))
	for _, t := range topics {
		done, total, _, _ := TopicLectureProgress(ctx, s.app.DB, t.ID)
		mark := "○"
		if done == total {
			mark = "✓"
		}
		items = append(items, RowItem{ID: string(t.ID), Title: t.Name, Meta: fmt.Sprintf("%d/%d", done, total), Mark: mark})
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
	return nil
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
				s.subjMenu.CursorDown()
			} else if up {
				s.subjMenu.CursorUp()
			} else if right {
				s.zone = gzRight
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
				s.topicList.CursorDown()
			} else if up {
				s.topicList.CursorUp()
			} else if left {
				s.zone = gzLeft
			}
		}
	case govSubjects:
		if s.subZone == gzLeft {
			prev := s.subjNav.SelectedID()
			if down {
				s.subjNav.CursorDown()
			} else if up {
				s.subjNav.CursorUp()
			} else if right {
				s.subZone = gzRight
			} else {
				return nil
			}
			if s.subjNav.SelectedID() != prev {
				s.reloadBrowserTopics()
			}
		} else {
			if down {
				s.subTopicList.CursorDown()
			} else if up {
				s.subTopicList.CursorUp()
			} else if left {
				s.subZone = gzLeft
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
	cols := BentoCols(w)
	s.gridCols = cols
	cardW := BentoGridCardW(w, cols, 3)
	if cardW < 26 {
		cardW = 26
	}
	// Uniform cells: one per exam plus the subjects-browser entry.
	cells := make([]string, 0, len(s.exams)+1)
	for i, e := range s.exams {
		cov := s.coverages[e.ID]
		cells = append(cells, ExamCard{Name: e.Name, Done: cov.Done, Total: cov.Total, Focused: s.cursor == i, Width: cardW - 2}.View())
	}
	cells = append(cells, s.subjectsCell(cardW-2, s.cursor == len(s.exams)))
	// Scroll the grid so the cursor row stays visible at any height.
	gridRows := [][]string{}
	for i := 0; i < len(cells); i += cols {
		gridRows = append(gridRows, cells[i:minInt(i+cols, len(cells))])
	}
	headLines := 2
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
	body := thScreenTitle.Render("GOVERNMENT EXAMS") + "  " + thMeta.Render("exam-card dashboard · Enter opens workspace") + "\n"
	body += strings.Join(rendered, "\n\n")
	if start > 0 {
		body += "\n" + thHelp.Render("↑ more above")
	}
	if start+maxRows < len(gridRows) {
		body += "\n" + thHelp.Render("↓ more below")
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
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	lines := []string{
		thScreenTitle.Render(TruncateToWidth("ALL SUBJECTS", inner)),
		RowDivider(inner),
		SectionHeader("GLOBAL BROWSER"),
		thMeta.Render("Every subject, every topic."),
		thMeta.Render("Lecture + practice progress."),
		"",
		thHelp.Render("Enter to open →"),
		"",
	}
	for len(lines) < 8 {
		lines = append(lines, "")
	}
	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cDimLine).Padding(0, 1).Width(w)
	if focused {
		style = style.BorderForeground(cPrimary)
		lines[0] = thPanelTitle.Render("ALL SUBJECTS  ●")
	}
	return style.Render(strings.Join(lines, "\n"))
}

// ---- exam workspace + subjects browser: persistent two-panel ----

func (s *GovScreen) examView(w, h int) string {
	lw, rw := TwoPaneLayout(w, 32, 24, 40)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4 // panel chrome
	if innerH < 4 {
		innerH = 4
	}
	s.subjMenu.SetWidth(lw - 4)
	s.topicList.SetWidth(rw - 4)
	s.subjMenu.FitHeight(innerH - 2)
	s.topicList.FitHeight(innerH - 7)
	s.subjMenu.SetFocused(s.zone == gzLeft)
	s.topicList.SetFocused(s.zone == gzRight)
	name := ""
	if s.coverage != nil {
		name = s.coverage.Exam
	}
	leftBody := thScreenTitle.Render(TruncateToWidth(name, lw-6)) + "\n" + RowDivider(lw-6) + "\n" + SectionHeader("SUBJECTS") + "\n" + s.subjMenu.View()
	left := PanelFixed("EXAM", leftBody, lw, bodyH, s.zone == gzLeft)
	var right string
	if s.coverage != nil {
		c := s.coverage
		barW := rw - 12
		if barW < 14 {
			barW = 14
		}
		if barW > 28 {
			barW = 28
		}
		var sb strings.Builder
		sb.WriteString(SectionHeader("SYLLABUS COVERAGE") + "\n")
		sb.WriteString(ProgressBar{Ratio: c.Percent(), Width: barW, ShowPct: true}.View() + "\n")
		sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d topics", c.Done, c.Total)) + "\n")
		sb.WriteString(RowDivider(rw - 6) + "\n")
		sb.WriteString(SectionHeader("PRACTICE READINESS") + "  " + thHelp.Render("not yet calculated") + "\n")
		sb.WriteString(RowDivider(rw-6) + "\n")
		sb.WriteString(SectionHeader("TOPICS — "+strings.ToUpper(TruncateToWidth(s.examSubject, 24))) + "\n")
		sb.WriteString(s.topicList.View())
		right = PanelFixed(strings.ToUpper(TruncateToWidth(s.examSubject, 20)), sb.String(), rw, bodyH, s.zone == gzRight)
	} else {
		right = PanelFixed("EXAM", EmptyState("Select a subject."), rw, bodyH, s.zone == gzRight)
	}
	return lipglossJoin(left, right)
}

func (s *GovScreen) subjectsView(w, h int) string {
	lw, rw := TwoPaneLayout(w, 32, 24, 40)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	s.subjNav.SetWidth(lw - 4)
	s.subTopicList.SetWidth(rw - 4)
	s.subjNav.FitHeight(innerH - 1)
	s.subTopicList.FitHeight(innerH - 7)
	s.subjNav.SetFocused(s.subZone == gzLeft)
	s.subTopicList.SetFocused(s.subZone == gzRight)
	left := PanelFixed("SUBJECTS", SectionHeader("GLOBAL SUBJECTS")+"\n"+s.subjNav.View(), lw, bodyH, s.subZone == gzLeft)
	right := PanelFixed("TOPICS", s.browserRight(), rw, bodyH, s.subZone == gzRight)
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
	barW := 24
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(TruncateToWidth(name, 30)) + "\n")
	sb.WriteString(RowDivider(30) + "\n")
	sb.WriteString(SectionHeader("LECTURE COVERAGE") + "\n")
	sb.WriteString(ProgressBar{Ratio: ratio(done, total), Width: barW, ShowPct: true}.View() + "\n")
	sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d topics lecture-complete", done, total)) + "\n")
	sb.WriteString(RowDivider(30) + "\n")
	sb.WriteString(SectionHeader("TOPICS") + "\n")
	sb.WriteString(s.subTopicList.View())
	return sb.String()
}

func (s *GovScreen) topicInWorkspace(w, h int, fromSubjects bool) string {
	lw, rw := TwoPaneLayout(w, 32, 24, 40)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	var left string
	if fromSubjects {
		s.subjNav.SetWidth(lw - 4)
		s.subjNav.FitHeight(innerH)
		s.subjNav.SetFocused(false)
		left = PanelFixed("SUBJECTS", SectionHeader("GLOBAL SUBJECTS")+"\n"+s.subjNav.View(), lw, bodyH, false)
	} else {
		s.subjMenu.SetWidth(lw - 4)
		s.subjMenu.FitHeight(innerH - 1)
		s.subjMenu.SetFocused(false)
		name := ""
		if s.coverage != nil {
			name = s.coverage.Exam
		}
		left = PanelFixed(strings.ToUpper(TruncateToWidth(name, 20)), SectionHeader("SUBJECTS")+"\n"+s.subjMenu.View(), lw, bodyH, false)
	}
	return lipglossJoin(left, PanelFixed("TOPIC", s.topicBody(rw-6), rw, bodyH, true))
}

func (s *GovScreen) topicBody(inner int) string {
	v := s.detail
	if v == nil {
		return EmptyState("Topic not found.")
	}
	if inner < 24 {
		inner = 24
	}
	barW := inner - 8
	if barW < 14 {
		barW = 14
	}
	if barW > 28 {
		barW = 28
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(TruncateToWidth(strings.ToUpper(v.Topic.Name), inner)) + "  " + thMeta.Render("·  "+v.Subject) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	// Zero-lecture topics: track satisfied, practice unlocked.
	if v.Total == 0 {
		sb.WriteString(SectionHeader("LECTURES") + "  " + thSuccess.Render("complete · no lecture track") + "\n")
	} else {
		sb.WriteString(SectionHeader("LECTURES") + "\n")
		sb.WriteString(ProgressBar{Ratio: ratio(v.Done, v.Total), Width: barW, ShowPct: true}.View() + "\n")
		sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d", v.Done, v.Total)) + "\n")
	}
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("PRACTICE") + "\n")
	st := v.Stats
	sb.WriteString(ProgressBar{Ratio: accuracyRatio(st), Width: barW}.View() + "  " + thMeta.Render(fmt.Sprintf("%.1f%% acc", st.Accuracy*100)) + "\n")
	sb.WriteString(MetaRow("Questions", fmt.Sprint(st.Attempted), 10) + "\n")
	sb.WriteString(MetaRow("Correct", fmt.Sprint(st.Correct), 10) + "\n")
	sb.WriteString(MetaRow("Incorrect", fmt.Sprint(st.Incorrect), 10) + "\n")
	sb.WriteString(MetaRow("Avg speed", avgStr(st), 10) + "\n")
	sb.WriteString(MetaRow("Progression", "not yet calculated", 10) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("LECTURES") + "\n")
	for i, l := range v.Lectures {
		mark := "○"
		if i < len(v.Flags) && v.Flags[i] {
			mark = "✓"
		}
		sb.WriteString(SelectableRow{Mark: mark, Title: l.Title, Width: inner}.View() + "\n")
		if i < len(v.Lectures)-1 {
			sb.WriteString(RowDivider(inner) + "\n")
		}
	}
	if len(v.Lectures) == 0 {
		sb.WriteString(thHelp.Render("no lectures — practice unlocked") + "\n")
	}
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("USED BY") + "\n" + thMeta.Render(strings.Join(v.UsedBy, "  ·  ")))
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

func accuracyRatio(st TopicPracticeStats) float64 {
	if st.Attempted == 0 {
		return 0
	}
	return st.Accuracy
}

func avgStr(st TopicPracticeStats) string {
	if !st.HasSpeed {
		return "—"
	}
	return fmt.Sprintf("%.1fs", st.AvgSpeed)
}
