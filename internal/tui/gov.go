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

// Gov focus zones: section switcher, left navigator, subjects, topics.
const (
	gzSection = iota
	gzLeft
	gzSubjects
	gzTopics
)

type GovScreen struct {
	app         *App
	section     string // "exams" or "subjects"
	zone        int
	secExams    Button
	secSubjects Button
	examList    list.Model
	subjList    list.Model
	subjMenu    list.Model // subjects within the selected exam
	topicList   list.Model // exam-filtered or global topics
	coverage    *ExamCoverage
	examSubject string
	detail      *GovTopicView
	showing     string // "exam" | "subject-topics" | "topic"
	err         string
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
	// List titles stay empty: panel titles + section headers already label
	// every list, so list chrome never duplicates them.
	mk := func() list.Model {
		l := list.New(nil, list.NewDefaultDelegate(), 28, 12)
		l.SetShowHelp(false)
		l.SetShowTitle(false)
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(false)
		return l
	}
	return &GovScreen{app: a, section: "exams", secExams: Button{Label: "EXAMS", Focused: true},
		secSubjects: Button{Label: "SUBJECTS"}, examList: mk(), subjList: mk(),
		subjMenu: mk(), topicList: mk(), showing: "exam"}
}

func (s *GovScreen) Reload() {
	ctx := context.Background()
	s.err = ""
	if s.section == "exams" {
		exams, err := catalog.GetGovExams(ctx, s.app.DB)
		if err != nil {
			s.err = err.Error()
			return
		}
		items := make([]list.Item, 0, len(exams))
		for _, e := range exams {
			items = append(items, listItem{id: string(e.ID), title: e.Name})
		}
		s.examList.SetItems(items)
		s.showing = "exam"
		s.reloadExam()
	} else {
		subjs, err := catalog.GetGovSubjects(ctx, s.app.DB)
		if err != nil {
			s.err = err.Error()
			return
		}
		items := make([]list.Item, 0, len(subjs))
		for _, g := range subjs {
			items = append(items, listItem{id: string(g.ID), title: g.Name})
		}
		s.subjList.SetItems(items)
		s.showing = "subject-topics"
		s.reloadSubjectTopics()
	}
	s.syncButtons()
}

func (s *GovScreen) syncButtons() {
	s.secExams.Focused = s.zone == gzSection && s.section == "exams"
	s.secSubjects.Focused = s.zone == gzSection && s.section == "subjects"
}

func (s *GovScreen) selectedExamID() db.ID {
	if it, ok := s.examList.SelectedItem().(listItem); ok {
		return db.ID(it.id)
	}
	return ""
}

func (s *GovScreen) selectedGlobalSubjectID() db.ID {
	if it, ok := s.subjList.SelectedItem().(listItem); ok {
		return db.ID(it.id)
	}
	return ""
}

func (s *GovScreen) reloadExam() {
	ctx := context.Background()
	eid := s.selectedExamID()
	if eid == "" {
		return
	}
	cov, err := ComputeExamCoverage(ctx, s.app.DB, eid)
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
	s.subjMenu.Title = "SUBJECTS"
	// Keep menu cursor on the selected subject.
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

func (s *GovScreen) reloadSubjectTopics() {
	ctx := context.Background()
	sid := s.selectedGlobalSubjectID()
	if sid == "" {
		return
	}
	topics, err := catalog.GetGovTopics(ctx, s.app.DB, sid)
	if err != nil {
		s.err = err.Error()
		return
	}
	items := make([]list.Item, 0, len(topics))
	for _, t := range topics {
		done, total, _, _ := TopicLectureProgress(ctx, s.app.DB, t.ID)
		items = append(items, listItem{id: string(t.ID), title: fmt.Sprintf("%s  %d/%d", t.Name, done, total)})
	}
	s.topicList.SetItems(items)
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
	s.showing = "topic"
}

// onBack pops right-panel detail first, then focus. True = consumed.
func (s *GovScreen) onBack() bool {
	if s.showing == "topic" {
		if s.section == "exams" {
			s.showing = "exam"
		} else {
			s.showing = "subject-topics"
		}
		return true
	}
	if s.zone != gzLeft {
		s.zone = gzLeft
		s.syncButtons()
		return true
	}
	return false
}

func (s *GovScreen) setSection(sec string) {
	if s.section != sec {
		s.section = sec
		s.zone = gzLeft
		s.examSubject = ""
		s.Reload()
	}
}

func (s *GovScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "tab":
			s.zone = (s.zone + 1) % 4
			// Skip the subjects zone when it has no items.
			if s.zone == gzSubjects && len(s.subjMenu.Items()) == 0 && s.section == "subjects" {
				s.zone = gzTopics
			}
			if s.section == "subjects" && s.zone == gzSubjects {
				s.zone = gzTopics
			}
			s.syncButtons()
			return nil
		case "m":
			if s.section == "exams" {
				s.setSection("subjects")
			} else {
				s.setSection("exams")
			}
			return nil
		case "enter":
			return s.pressEnter()
		case "j", "down", "k", "up":
			return s.move(k.String())
		}
	}
	// Delegate typing/filtering to the focused list.
	var cmd tea.Cmd
	switch s.zone {
	case gzLeft:
		if s.section == "exams" {
			prev := s.selectedExamID()
			s.examList, cmd = s.examList.Update(msg)
			if s.selectedExamID() != prev {
				s.examSubject = ""
				s.reloadExam()
			}
		} else {
			prev := s.selectedGlobalSubjectID()
			s.subjList, cmd = s.subjList.Update(msg)
			if s.selectedGlobalSubjectID() != prev {
				s.reloadSubjectTopics()
			}
		}
	case gzSubjects:
		s.subjMenu, cmd = s.subjMenu.Update(msg)
	case gzTopics:
		s.topicList, cmd = s.topicList.Update(msg)
	}
	return cmd
}

func (s *GovScreen) move(key string) tea.Cmd {
	down := key == "j" || key == "down"
	var cmd tea.Cmd
	switch s.zone {
	case gzSection:
		// Left/right moves between section buttons; j/k also toggles.
		if key == "j" || key == "k" || key == "down" || key == "up" || key == "left" || key == "right" || key == "h" || key == "l" {
			if s.section == "exams" {
				s.setSection("subjects")
			} else {
				s.setSection("exams")
			}
		}
		return nil
	case gzLeft:
		if s.section == "exams" {
			prev := s.selectedExamID()
			if down {
				s.examList.CursorDown()
			} else {
				s.examList.CursorUp()
			}
			if s.selectedExamID() != prev {
				s.examSubject = ""
				s.reloadExam()
			}
		} else {
			prev := s.selectedGlobalSubjectID()
			if down {
				s.subjList.CursorDown()
			} else {
				s.subjList.CursorUp()
			}
			if s.selectedGlobalSubjectID() != prev {
				s.reloadSubjectTopics()
			}
		}
	case gzSubjects:
		if s.section != "exams" {
			return nil
		}
		if down {
			s.subjMenu.CursorDown()
		} else {
			s.subjMenu.CursorUp()
		}
		if it, ok := s.subjMenu.SelectedItem().(listItem); ok {
			for _, sc := range s.coverage.Subjects {
				if string(sc.SubjectID) == it.id {
					s.examSubject = sc.Subject
				}
			}
			s.reloadExamTopics()
		}
	case gzTopics:
		if down {
			s.topicList.CursorDown()
		} else {
			s.topicList.CursorUp()
		}
	}
	return cmd
}

func (s *GovScreen) pressEnter() tea.Cmd {
	switch s.zone {
	case gzSection:
		// Enter confirms the highlighted section button.
		return nil
	case gzLeft:
		s.zone = s.nextZone()
		s.syncButtons()
	case gzSubjects:
		s.zone = gzTopics
		s.syncButtons()
	case gzTopics:
		if it, ok := s.topicList.SelectedItem().(listItem); ok && it.id != "" {
			s.openTopic(it.id)
		}
	}
	return nil
}

func (s *GovScreen) nextZone() int {
	if s.section == "exams" {
		return gzSubjects
	}
	return gzTopics
}

func (s *GovScreen) View() string {
	w, h := s.app.contentViewport()
	leftW, rightW := splitW(w, 30, 22, 40)
	listH := h - 12
	if listH < 6 {
		listH = 6
	}
	for _, l := range []*list.Model{&s.examList, &s.subjList, &s.subjMenu, &s.topicList} {
		l.SetWidth(leftW - 6)
		if l == &s.topicList {
			l.SetWidth(rightW - 6)
		}
	}
	secRow := lipgloss.JoinHorizontal(lipgloss.Top, s.secExams.View(), "   ", s.secSubjects.View())
	var nav list.Model
	if s.section == "exams" {
		s.examList.SetHeight(fitListHeight(len(s.examList.Items()), listH))
		nav = s.examList
	} else {
		s.subjList.SetHeight(fitListHeight(len(s.subjList.Items()), listH))
		nav = s.subjList
	}
	left := Panel("NAVIGATE", secRow+"\n\n"+nav.View(), leftW, s.zone == gzLeft || s.zone == gzSection)
	right := Panel(s.rightTitle(), s.rightView(rightW, h), rightW, s.zone == gzSubjects || s.zone == gzTopics)
	if s.err != "" {
		right += "\n" + thError.Render(s.err)
	}
	return lipglossJoin(left, right)
}

func (s *GovScreen) rightTitle() string {
	if s.showing == "topic" {
		return "TOPIC"
	}
	if s.section == "exams" {
		return "EXAM"
	}
	return "SUBJECT"
}

func (s *GovScreen) rightView(w, h int) string {
	if s.showing == "topic" {
		return s.topicView()
	}
	if s.section == "exams" {
		return s.examView(w, h)
	}
	return s.subjectView()
}

func (s *GovScreen) examView(w, h int) string {
	if s.coverage == nil {
		return thMeta.Render("Select an exam.")
	}
	c := s.coverage
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", thScreenTitle.Render(c.Exam))
	sb.WriteString(SectionHeader("SYLLABUS COVERAGE") + "\n")
	sb.WriteString(ProgressBar{Ratio: c.Percent(), Width: 26, ShowPct: true}.View() + "\n")
	fmt.Fprintf(&sb, "%s\n\n", thMeta.Render(fmt.Sprintf("%d / %d topics", c.Done, c.Total)))
	sb.WriteString(SectionHeader("PRACTICE READINESS") + "\n")
	sb.WriteString(ProgressBar{Ratio: 0, Width: 26}.View() + "  " + thHelp.Render("not yet calculated") + "\n\n")
	_ = w
	_ = h
	s.subjMenu.SetHeight(fitListHeight(len(s.subjMenu.Items()), 10))
	sb.WriteString(SectionHeader("SUBJECTS") + "\n" + s.subjMenu.View() + "\n\n")
	sb.WriteString(SectionHeader("TOPICS — "+s.examSubject) + "\n")
	s.topicList.SetHeight(fitListHeight(len(s.topicList.Items()), 10))
	sb.WriteString(s.topicList.View())
	return sb.String()
}

func (s *GovScreen) subjectView() string {
	sid := s.selectedGlobalSubjectID()
	if sid == "" {
		return thMeta.Render("Select a subject.")
	}
	ctx := context.Background()
	name := string(sid)
	if g, err := catalog.GetGovSubject(ctx, s.app.DB, sid); err == nil {
		name = g.Name
	}
	topics, _ := catalog.GetGovTopics(ctx, s.app.DB, sid)
	done, total := 0, len(topics)
	var pAtt, pCor int
	for _, t := range topics {
		if topicLectureComplete(ctx, s.app.DB, t.ID) {
			done++
		}
		st := ComputeTopicPracticeStats(ctx, s.app.DB, t.ID)
		pAtt += st.Attempted
		pCor += st.Correct
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n\n", thScreenTitle.Render(name))
	sb.WriteString(SectionHeader("LECTURES") + "\n")
	sb.WriteString(ProgressBar{Ratio: ratio(done, total), Width: 26, ShowPct: true}.View() + "\n")
	fmt.Fprintf(&sb, "%s\n\n", thMeta.Render(fmt.Sprintf("%d / %d topics lecture-complete", done, total)))
	sb.WriteString(SectionHeader("PRACTICE") + "\n")
	acc := 0.0
	if pAtt > 0 {
		acc = float64(pCor) / float64(pAtt)
	}
	fmt.Fprintf(&sb, "%s\n\n", thMeta.Render(fmt.Sprintf("%d questions · %.0f%% accuracy", pAtt, acc*100)))
	s.topicList.SetHeight(fitListHeight(len(s.topicList.Items()), 10))
	sb.WriteString(SectionHeader("TOPICS") + "\n" + s.topicList.View())
	return sb.String()
}

func (s *GovScreen) topicView() string {
	v := s.detail
	if v == nil {
		return thMeta.Render("Topic not found.")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s  ·  %s\n\n", thScreenTitle.Render(v.Topic.Name), thMeta.Render(v.Subject))
	sb.WriteString(SectionHeader("LECTURES") + "\n")
	sb.WriteString(ProgressBar{Ratio: ratio(v.Done, v.Total), Width: 26, ShowPct: true}.View() + "\n")
	fmt.Fprintf(&sb, "%s\n\n", thMeta.Render(fmt.Sprintf("%d / %d", v.Done, v.Total)))
	sb.WriteString(SectionHeader("PRACTICE") + "\n")
	st := v.Stats
	fmt.Fprintf(&sb, "%s\n", MetaRow("Questions", fmt.Sprint(st.Attempted), 10))
	fmt.Fprintf(&sb, "%s\n", MetaRow("Correct", fmt.Sprint(st.Correct), 10))
	fmt.Fprintf(&sb, "%s\n", MetaRow("Accuracy", fmt.Sprintf("%.1f%%", st.Accuracy*100), 10))
	fmt.Fprintf(&sb, "%s\n\n", MetaRow("Avg speed", avgStr(st), 10))
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

func (s *GovScreen) Hints() string { return "Tab Focus   m Section   Enter Open" }

// fitListHeight sizes a list to its content so panels never float in
// padded empty viewports.
func fitListHeight(items, maxH int) int {
	h := items*2 + 1
	if h < 4 {
		h = 4
	}
	if h > maxH {
		h = maxH
	}
	return h
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
