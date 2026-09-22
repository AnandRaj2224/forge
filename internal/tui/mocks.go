package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// MocksScreen: LEFT mock list, RIGHT dense mock detail with per-section
// weak topics (gov_topic_id references, max 3 per section, ordered).
type MocksScreen struct {
	app      *App
	mocks    []db.Mock
	exams    []db.GovExam
	examName map[string]string
	list     Navigator
	focusR   bool
	sections []db.MockSection
	weak     map[string][]db.MockWeakTopic
	topNames map[string]string
	secCur   int
	mode     string // browse, create, weak, section
	err      string

// create form
	cExamCur int
	cSubjects []string // cached subjects of the selected exam
	cInputs  []textinput.Model
	cLabels  []string
	cFocus   int
	cErr     string

// create-chain weak selection across fresh sections
	wChain     bool
	wChainSecs []db.MockSection
	wChainIdx  int

	// weak editor
	wMockID   db.ID
	wSection  db.MockSection
	wExamID   db.ID
	wChoices  []db.GovTopic
	wSubjName map[string]string
	wFilter   textinput.Model
	wList     Navigator
	wStaged   []db.ID
	wFocus    int // 0 filter, 1 list, 2 save
	wErr      string

	// Shared scroll viewports: detail and create panes scroll
	// independently whenever they overflow.
	detailScroll ScrollViewport
	formScroll   ScrollViewport
}

func NewMocksScreen(a *App) *MocksScreen {
	return &MocksScreen{app: a, list: NewNavigator(), weak: map[string][]db.MockWeakTopic{}, topNames: map[string]string{}, mode: "browse"}
}

func (s *MocksScreen) ownsKeys() bool {
	return s.mode == "create" || s.mode == "weak"
}

func (s *MocksScreen) Reload() {
	ctx := context.Background()
	s.err = ""
	exams, err := catalog.GetGovExams(ctx, s.app.DB)
	if err == nil {
		s.exams = exams
		s.examName = map[string]string{}
		for _, e := range exams {
			s.examName[string(e.ID)] = e.Name
		}
	}
	mocks, err := runtime.ListMocks(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.mocks = mocks
	items := make([]RowItem, 0, len(mocks))
	for _, m := range mocks {
		ex := s.examName[string(m.ExamID)]
		items = append(items, RowItem{ID: string(m.ID), Title: m.Name, Meta: FormatDateISO(m.MockDate) + "  ·  " + ex, Mark: "•"})
	}
	// Numbered rows: use global row system with numbers.
	s.list.SetItems(items)
	s.syncDetail()
}

func (s *MocksScreen) selMock() *db.Mock {
	id := s.list.SelectedID()
	if id == "" {
		return nil
	}
	for i, m := range s.mocks {
		if string(m.ID) == id {
			return &s.mocks[i]
		}
	}
	return nil
}

func (s *MocksScreen) syncDetail() {
	ctx := context.Background()
	m := s.selMock()
	s.sections = nil
	s.weak = map[string][]db.MockWeakTopic{}
	s.secCur = 0
	s.detailScroll.Reset()
	if m == nil {
		return
	}
	secs, _ := runtime.ListMockSections(ctx, s.app.DB, m.ID)
	s.sections = secs
	for _, sec := range secs {
		ws, _ := runtime.ListWeakTopics(ctx, s.app.DB, sec.ID)
		s.weak[string(sec.ID)] = ws
		for _, w := range ws {
			if _, ok := s.topNames[string(w.TopicID)]; !ok {
				if tp, err := catalog.GetGovTopic(ctx, s.app.DB, w.TopicID); err == nil {
					s.topNames[string(w.TopicID)] = tp.Name
				} else {
					s.topNames[string(w.TopicID)] = string(w.TopicID)
				}
			}
		}
	}
}

// detailViewportH mirrors browseView geometry for scrolling.
func (s *MocksScreen) detailViewportH() int {
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

// followSection keeps the selected weak-topic section visible: top on the
// first section, bottom on the last, proportional in between.
func (s *MocksScreen) followSection() {
	n := len(s.sections)
	if n <= 1 {
		return
	}
	total := len(strings.Split(s.mockBody(40), "\n"))
	height := s.detailViewportH()
	maxOff := total - height
	if maxOff <= 0 {
		s.detailScroll.Reset()
		return
	}
	s.detailScroll.Offset = s.secCur * maxOff / (n - 1)
	s.detailScroll.Clamp(total, height)
}
// qBacksOut reports whether q should leave the form instead of typing.
// Inside text inputs q always types; on selectors and buttons it backs out.
// Kept for compat; form-mode routing in App guarantees q never quits Forge.
func (s *MocksScreen) qBacksOut() bool {
	switch s.mode {
	case "create":
		return s.cFocus == 0
	case "weak":
		return s.wFocus != 0
	}
	return true
}

// textEditing reports whether a text input owns focus in a mock form.
// While true, letters (including q/s/x) type instead of acting as shortcuts.
func (s *MocksScreen) textEditing() bool {
	switch s.mode {
	case "create":
		return s.cFocus >= 1 && s.cFocus <= len(s.cInputs)
	case "weak":
		return s.wFocus == 0
	}
	return false
}

func (s *MocksScreen) onBack() bool {
	switch s.mode {
	case "create", "weak":
		s.mode = "browse"
		s.wChain = false
		s.wChainSecs = nil
		s.cErr, s.wErr = "", ""
		s.Reload()
		return true
	}
	if s.focusR {
		s.focusR = false
		return true
	}
	return false
}

func (s *MocksScreen) Update(msg tea.Msg) tea.Cmd {
	// Form modes own keys.
	if s.mode == "create" {
		return s.updateCreate(msg)
	}
	if s.mode == "weak" {
		return s.updateWeak(msg)
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "j", "down":
			if s.focusR {
				if s.secCur < len(s.sections)-1 {
					s.secCur++
					s.followSection()
				} else {
					// At the end: scroll the detail tail independently.
					s.detailScroll.ScrollDown(len(strings.Split(s.mockBody(40), "\n")), s.detailViewportH())
				}
			} else {
				prev := s.list.SelectedID()
				s.list.CursorDown()
				if s.list.SelectedID() != prev {
					s.syncDetail()
				}
			}
			return nil
		case "k", "up":
			if s.focusR {
				if s.secCur > 0 {
					s.secCur--
					s.followSection()
				} else {
					// At the top: scroll the detail head independently.
					s.detailScroll.ScrollUp(len(strings.Split(s.mockBody(40), "\n")), s.detailViewportH())
				}
			} else {
				prev := s.list.SelectedID()
				s.list.CursorUp()
				if s.list.SelectedID() != prev {
					s.syncDetail()
				}
			}
			return nil
		case "h", "left":
			s.focusR = false
			return nil
		case "l", "right":
			if s.selMock() != nil {
				s.focusR = true
			}
			return nil
		case "enter":
			if s.focusR {
				s.openWeakEditor()
				return nil
			}
			s.focusR = true
			return nil
		case "n":
			s.startCreate()
			return nil
		case "D":
			s.deleteSelected()
			return nil
		}
	}
	return nil
}

func (s *MocksScreen) deleteSelected() {
	m := s.selMock()
	if m == nil {
		return
	}
	ctx := context.Background()
	_ = runtime.DeleteMock(ctx, s.app.DB, m.ID)
	s.focusR = false
	s.Reload()
}

// ---- create ----

func (s *MocksScreen) startCreate() {
	s.mode = "create"
	s.cExamCur = 0
// Raw facts only: Incorrect, Unattempted and Accuracy are derived at save.
	s.cLabels = []string{"Name", "Date", "Platform", "Score", "Total Marks", "Total Questions", "Attempted", "Correct", "Rank", "Percentile", "Average Speed"}
	s.cInputs = make([]textinput.Model, len(s.cLabels))
	for i, lb := range s.cLabels {
		ti := textinput.New()
		ti.Prompt = "│ "
		ti.CharLimit = 32
		ti.Placeholder = lb
		if lb == "Date" {
			ti.Placeholder = "21 Sep 2026"
		}
		ti.SetWidth(20)
		s.cInputs[i] = ti
	}
	s.cFocus = 0 // 0 = exam selector, 1..N inputs, N+1 = SAVE
	s.cErr = ""
	s.refreshCreateSubjects()
	s.updateCreateFocus()
}

// refreshCreateSubjects caches the selected exam's subject names: the
// sections the new mock will derive automatically.
func (s *MocksScreen) refreshCreateSubjects() {
	s.cSubjects = nil
	if len(s.exams) == 0 {
		return
	}
	exam := s.exams[s.cExamCur%len(s.exams)]
	cov, err := ComputeExamCoverage(context.Background(), s.app.DB, exam.ID)
	if err != nil {
		return
	}
	for _, sc := range cov.Subjects {
		s.cSubjects = append(s.cSubjects, sc.Subject)
	}
}

func (s *MocksScreen) nCreateFocus() int { return 1 + len(s.cInputs) + 1 } // exam + inputs + save

func (s *MocksScreen) updateCreateFocus() {
	for i := range s.cInputs {
		s.cInputs[i].Blur()
	}
	if s.cFocus >= 1 && s.cFocus <= len(s.cInputs) {
		s.cInputs[s.cFocus-1].Focus()
	}
}

// followCreateFocus keeps the focused create field reachable: exam on top,
// SAVE on bottom, proportional in between.
func (s *MocksScreen) followCreateFocus() {
	n := s.nCreateFocus()
	if n <= 1 {
		return
	}
	_, h := s.app.contentViewport()
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	// Approximate full create-form height: header + inputs + divider + save.
	total := 2 + len(s.cInputs) + 3
	if s.cFocus <= 0 {
		s.formScroll.Reset()
		return
	}
	if s.cFocus >= n-1 {
		s.formScroll.Offset = 1 << 30
		s.formScroll.Clamp(total, innerH)
		return
	}
	maxOff := total - innerH
	if maxOff <= 0 {
		s.formScroll.Reset()
		return
	}
	s.formScroll.Offset = s.cFocus * maxOff / (n - 1)
	s.formScroll.Clamp(total, innerH)
}

func (s *MocksScreen) updateCreate(msg tea.Msg) tea.Cmd {
	k, isKey := msg.(tea.KeyPressMsg)
	if !isKey {
		// Forward to focused input.
		if s.cFocus >= 1 && s.cFocus <= len(s.cInputs) {
			var cmd tea.Cmd
			s.cInputs[s.cFocus-1], cmd = s.cInputs[s.cFocus-1].Update(msg)
			return cmd
		}
		return nil
	}
	switch k.String() {
	case "down":
		s.cFocus = (s.cFocus + 1) % s.nCreateFocus()
		s.updateCreateFocus()
		s.followCreateFocus()
		return nil
	case "up":
		s.cFocus = (s.cFocus - 1 + s.nCreateFocus()) % s.nCreateFocus()
		s.updateCreateFocus()
		s.followCreateFocus()
		return nil
	case "left", "h":
		if s.cFocus == 0 && len(s.exams) > 0 {
			s.cExamCur = (s.cExamCur - 1 + len(s.exams)) % len(s.exams)
			s.refreshCreateSubjects()
			return nil
		}
	case "right", "l":
		if s.cFocus == 0 && len(s.exams) > 0 {
			s.cExamCur = (s.cExamCur + 1) % len(s.exams)
			s.refreshCreateSubjects()
			return nil
		}
	case "enter":
		if s.cFocus == s.nCreateFocus()-1 {
			// saveCreate owns the next mode: weak-chain for fresh
			// sections, browse when there is nothing to pick.
			if s.saveCreate() && !s.wChain {
				s.mode = "browse"
				s.Reload()
			}
			return nil
		}
		if s.cFocus == 0 {
			s.cFocus = 1
			s.updateCreateFocus()
			return nil
		}
		// Enter in input moves next.
		s.cFocus = (s.cFocus + 1) % s.nCreateFocus()
		s.updateCreateFocus()
		return nil
	}
	if s.cFocus >= 1 && s.cFocus <= len(s.cInputs) {
		var cmd tea.Cmd
		s.cInputs[s.cFocus-1], cmd = s.cInputs[s.cFocus-1].Update(msg)
		return cmd
	}
	return nil
}

func parseOptInt(v string) (*int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 0 {
		return nil, fmt.Errorf("must be int >= 0")
	}
	return &n, nil
}

func parseOptFloat(v string) (*float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%f", &f); err != nil {
		return nil, fmt.Errorf("must be number")
	}
	return &f, nil
}

func (s *MocksScreen) saveCreate() bool {
	ctx := context.Background()
	if len(s.exams) == 0 {
		s.cErr = "no exams"
		return false
	}
	name := strings.TrimSpace(s.cInputs[0].Value())
	date := strings.TrimSpace(s.cInputs[1].Value())
	platform := strings.TrimSpace(s.cInputs[2].Value())
	if name == "" {
		s.cErr = "name required"
		return false
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		parsed, err = time.Parse("02 Jan 2006", date)
	}
	if err != nil {
		s.cErr = "date as YYYY-MM-DD or 21 Sep 2026"
		return false
	}
	date = parsed.Format("2006-01-02")
	get := func(i int) string {
		if i < len(s.cInputs) {
			return s.cInputs[i].Value()
		}
		return ""
	}
	score, err := parseOptInt(get(3))
	if err != nil {
		s.cErr = "score " + err.Error()
		return false
	}
	total, err := parseOptInt(get(4))
	if err != nil {
		s.cErr = "total marks " + err.Error()
		return false
	}
	totalQ, err := parseOptInt(get(5))
	if err != nil {
		s.cErr = "total questions " + err.Error()
		return false
	}
	attempted, err := parseOptInt(get(6))
	if err != nil {
		s.cErr = "attempted " + err.Error()
		return false
	}
	correct, err := parseOptInt(get(7))
	if err != nil {
		s.cErr = "correct " + err.Error()
		return false
	}
	rankV, err := parseOptInt(get(8))
	if err != nil {
		s.cErr = "rank " + err.Error()
		return false
	}
	perc, err := parseOptFloat(get(9))
	if err != nil {
		s.cErr = "percentile " + err.Error()
		return false
	}
	avg, err := parseOptFloat(get(10))
	if err != nil {
		s.cErr = "average speed " + err.Error()
		return false
	}
	// Raw facts must compose: correct within attempted within total.
	att, corr, tot := 0, 0, 0
	if attempted != nil {
		att = *attempted
	}
	if correct != nil {
		corr = *correct
	}
	if totalQ != nil {
		tot = *totalQ
	}
	if corr > att || att > tot {
		s.cErr = "need 0 <= correct <= attempted <= total"
		return false
	}
	// Derived metrics (never entered): incorrect, unattempted, accuracy.
	incorrect := att - corr
	unatt := tot - att
	var acc *float64
	if att > 0 {
		a := float64(corr) / float64(att) * 100
		acc = &a
	}
	exam := s.exams[s.cExamCur%len(s.exams)]
	id := db.ID(fmt.Sprintf("mock-%d", time.Now().UTC().UnixNano()))
	m := db.Mock{ID: id, ExamID: exam.ID, Name: name, MockDate: date, Platform: platform,
		Score: score, TotalScore: total, Attempted: attempted, TotalQuestions: totalQ,
		Correct: correct, Incorrect: &incorrect, Unattempted: &unatt, RankValue: rankV,
		Percentile: perc, Accuracy: acc, AvgSpeedSeconds: avg, CreatedAt: time.Now().UTC()}
	if err := runtime.CreateMock(ctx, s.app.DB, m); err != nil {
		s.cErr = err.Error()
		return false
	}
	// Sections derive automatically from the exam's subjects.
	secs := s.examSubjectSections(ctx, exam.ID, id)
	if len(secs) == 0 {
		s.mode = "browse"
		s.Reload()
		return true
	}
	// One continuous flow: pick weak topics per fresh section in order.
	s.wChain = true
	s.wChainSecs = secs
	s.wChainIdx = 0
	s.openWeakFor(m, secs[0])
	return true
}

// examSubjectSections creates one mock section per subject of the exam,
// ordered as the exam maps them. Names come from catalog subjects.
func (s *MocksScreen) examSubjectSections(ctx context.Context, examID, mockID db.ID) []db.MockSection {
	cov, err := ComputeExamCoverage(ctx, s.app.DB, examID)
	if err != nil {
		s.cErr = err.Error()
		return nil
	}
	secs := make([]db.MockSection, 0, len(cov.Subjects))
	for i, sc := range cov.Subjects {
		sec := db.MockSection{
			ID:        db.ID(fmt.Sprintf("msec-%d-%d", time.Now().UTC().UnixNano(), i)),
			MockID:    mockID,
			Name:      sc.Subject,
			SortOrder: i + 1,
		}
		if err := runtime.CreateMockSection(ctx, s.app.DB, sec); err != nil {
			s.cErr = err.Error()
			return nil
		}
		secs = append(secs, sec)
	}
	return secs
}

// ---- weak editor ----

func (s *MocksScreen) openWeakEditor() {
	m := s.selMock()
	if m == nil || len(s.sections) == 0 {
		return
	}
	if s.secCur < 0 || s.secCur >= len(s.sections) {
		s.secCur = 0
	}
	s.wChain = false
	s.openWeakFor(*m, s.sections[s.secCur])
}

// openWeakFor opens the weak-topic editor for one concrete section.
// Choices are real gov topics filtered to that section's subject.
func (s *MocksScreen) openWeakFor(m db.Mock, sec db.MockSection) {
	ctx := context.Background()
	choices, err := s.sectionTopicChoices(ctx, m.ExamID, sec.Name)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.wMockID = m.ID
	s.wSection = sec
	s.wExamID = m.ExamID
	s.wChoices = choices
	s.wSubjName = map[string]string{}
	subjs, _ := catalog.GetGovSubjects(ctx, s.app.DB)
	for _, g := range subjs {
		s.wSubjName[string(g.ID)] = g.Name
	}
	exist, _ := runtime.ListWeakTopics(ctx, s.app.DB, sec.ID)
	s.wStaged = make([]db.ID, 0, len(exist))
	for _, w := range exist {
		s.wStaged = append(s.wStaged, w.TopicID)
	}
	ti := textinput.New()
	ti.Prompt = "│ "
	ti.Placeholder = "filter topics…"
	ti.CharLimit = 32
	ti.SetWidth(24)
	ti.Focus()
	s.wFilter = ti
	s.wList = NewNavigator()
	s.wFocus = 0
	s.wErr = ""
	s.mode = "weak"
	s.refreshWeakList()
}

// saveWeakTopics persists the staged ordered weak topics. Inside the
// creation chain it advances to the next fresh section; otherwise it
// returns to browsing.
func (s *MocksScreen) saveWeakTopics() {
	ctx := context.Background()
	if err := runtime.SetWeakTopics(ctx, s.app.DB, s.wMockID, s.wSection.ID, s.wStaged); err != nil {
		s.wErr = err.Error()
		return
	}
	if s.wChain && s.wChainIdx+1 < len(s.wChainSecs) {
		mock, err := runtime.GetMock(ctx, s.app.DB, s.wMockID)
		if err != nil {
			s.wErr = err.Error()
			s.wChain = false
			s.mode = "browse"
			s.Reload()
			return
		}
		s.wChainIdx++
		s.openWeakFor(mock, s.wChainSecs[s.wChainIdx])
		return
	}
	s.wChain = false
	s.wChainSecs = nil
	s.mode = "browse"
	s.Reload()
}

// sectionTopicChoices returns real gov topics for one section: the exam
// topics belonging to the section's subject. It falls back to every exam
// topic when the section does not map to an exam subject (legacy customs).
func (s *MocksScreen) sectionTopicChoices(ctx context.Context, examID db.ID, section string) ([]db.GovTopic, error) {
	all, err := runtime.ExamTopicChoices(ctx, s.app.DB, examID)
	if err != nil {
		return nil, err
	}
	cov, cerr := ComputeExamCoverage(ctx, s.app.DB, examID)
	if cerr != nil {
		return all, nil
	}
	var subjectID db.ID
	for _, sc := range cov.Subjects {
		if strings.EqualFold(sc.Subject, section) {
			subjectID = sc.SubjectID
			break
		}
	}
	if subjectID == "" {
		return all, nil
	}
	out := make([]db.GovTopic, 0)
	for _, tp := range all {
		if tp.SubjectID == subjectID {
			out = append(out, tp)
		}
	}
	if len(out) == 0 {
		return all, nil
	}
	return out, nil
}

func (s *MocksScreen) refreshWeakList() {
	q := strings.ToLower(strings.TrimSpace(s.wFilter.Value()))
	staged := map[string]int{}
	for i, id := range s.wStaged {
		staged[string(id)] = i + 1
	}
	items := []RowItem{}
	for _, t := range s.wChoices {
		subj := s.wSubjName[string(t.SubjectID)]
		if q != "" && !strings.Contains(strings.ToLower(t.Name+" "+subj+" "+string(t.ID)), q) {
			continue
		}
		mark := "○"
		suffix := ""
		if ord, ok := staged[string(t.ID)]; ok {
			mark = "◉"
			suffix = fmt.Sprintf("  [%d]", ord)
		}
		title := t.Name + suffix
		meta := subj
		items = append(items, RowItem{ID: string(t.ID), Title: title, Meta: meta, Mark: mark})
	}
	prev := s.wList.SelectedID()
	s.wList.SetItems(items)
	// Keep cursor on same id when filtering.
	if prev != "" {
		for i, it := range items {
			if it.ID == prev {
				s.wList.Select(i)
			}
		}
	}
	s.wList.SetFocused(s.wFocus == 1)
}

func (s *MocksScreen) toggleWeak(id string) {
	if id == "" {
		return
	}
	for i, v := range s.wStaged {
		if string(v) == id {
			// Remove.
			s.wStaged = append(s.wStaged[:i], s.wStaged[i+1:]...)
			s.wErr = ""
			s.refreshWeakList()
			return
		}
	}
	if len(s.wStaged) >= 3 {
		s.wErr = "maximum 3 weak topics per section"
		return
	}
	s.wStaged = append(s.wStaged, db.ID(id))
	s.wErr = ""
	s.refreshWeakList()
}

func (s *MocksScreen) updateWeak(msg tea.Msg) tea.Cmd {
	k, isKey := msg.(tea.KeyPressMsg)
	if !isKey {
		if s.wFocus == 0 {
			var cmd tea.Cmd
			s.wFilter, cmd = s.wFilter.Update(msg)
			s.refreshWeakList()
			return cmd
		}
		return nil
	}
	switch k.String() {
	case "down":
		if s.wFocus == 0 {
			s.wFocus = 1
			s.wFilter.Blur()
			s.refreshWeakList()
		} else if s.wFocus == 1 {
			s.wList.CursorDown()
		}
		return nil
	case "up":
		if s.wFocus == 2 {
			s.wFocus = 1
			s.refreshWeakList()
		} else if s.wFocus == 1 {
			s.wList.CursorUp()
		}
		return nil
	case "l", "right":
		if s.wFocus == 1 {
			s.wFocus = 2
			s.refreshWeakList()
		}
		return nil
	case "h", "left":
		if s.wFocus == 2 {
			s.wFocus = 1
			s.refreshWeakList()
			return nil
		}
		if s.wFocus == 1 {
			s.wFocus = 0
			s.wFilter.Focus()
			s.refreshWeakList()
		}
		return nil
	case "j":
		if s.wFocus == 1 {
			s.wList.CursorDown()
		}
		return nil
	case "k":
		if s.wFocus == 1 {
			s.wList.CursorUp()
		}
		return nil
	case "enter", " ", "space":
		if s.wFocus == 1 {
			s.toggleWeak(s.wList.SelectedID())
			return nil
		}
		if s.wFocus == 2 {
			s.saveWeakTopics()
			return nil
		}
		if s.wFocus == 0 {
			s.wFocus = 1
			s.wFilter.Blur()
			s.refreshWeakList()
			return nil
		}
		return nil
	case "s":
		if s.wFocus != 0 {
			s.saveWeakTopics()
			return nil
		}
	}
	if s.wFocus == 0 {
		var cmd tea.Cmd
		s.wFilter, cmd = s.wFilter.Update(msg)
		s.refreshWeakList()
		return cmd
	}
	return nil
}

// ---- view ----

func (s *MocksScreen) View() string {
	switch s.mode {
	case "create":
		return s.createView()
	case "weak":
		return s.weakView()
	default:
		return s.browseView()
	}
}

func (s *MocksScreen) browseView() string {
	w, h := s.app.contentViewport()
	lw, rw := TwoPaneLayout(w, 36, 26, 40)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	s.list.SetWidth(lw - 4)
	s.list.FitHeight(innerH - 3)
	s.list.SetFocused(!s.focusR)
	leftBody := SectionHeader("MOCK LIST") + "\n" + s.list.View() + "\n" +
		RowDivider(lw-6) + "\n" +
		KeyBadge("N") + thMeta.Render(" New mock   ") + KeyBadge("D") + thMeta.Render(" Delete")
	left := PanelFixed("MOCKS", leftBody, lw, bodyH, !s.focusR)
	right := PanelFixed("MOCK DETAIL", ViewportView(s.mockBody(rw-6), &s.detailScroll, innerH), rw, bodyH, s.focusR)
	out := lipglossJoin(left, right)
	if s.err != "" {
		out += "\n" + thError.Render(s.err)
	}
	return out
}

func intStr(v *int) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprint(*v)
}

func floatStr(v *float64, suffix string) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%s", *v, suffix)
}

func (s *MocksScreen) mockBody(inner int) string {
	m := s.selMock()
	if m == nil {
		return EmptyState("No mocks yet.")
	}
	if inner < 24 {
		inner = 24
	}
	exam := s.examName[string(m.ExamID)]
	if exam == "" {
		exam = string(m.ExamID)
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(TruncateToWidth(m.Name, inner)) + "\n")
	sb.WriteString(thMeta.Render(TruncateToWidth(FormatDateISO(m.MockDate)+"  ·  "+exam+"  ·  "+orDash(m.Platform), inner)) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	// Dense metric bento: responsive grid, never overflows.
	scoreV := "—"
	if m.Score != nil && m.TotalScore != nil {
		scoreV = fmt.Sprintf("%d / %d", *m.Score, *m.TotalScore)
	} else if m.Score != nil {
		scoreV = fmt.Sprint(*m.Score)
	}
	attV := "—"
	if m.Attempted != nil && m.TotalQuestions != nil {
		attV = fmt.Sprintf("%d / %d", *m.Attempted, *m.TotalQuestions)
	} else if m.Attempted != nil {
		attV = fmt.Sprint(*m.Attempted)
	}
	cards := []MetricCard{
		{Label: "SCORE", Value: scoreV},
		{Label: "RANK", Value: intStr(m.RankValue)},
		{Label: "PERCENTILE", Value: floatStr(m.Percentile, "")},
		{Label: "ACCURACY", Value: floatStr(m.Accuracy, "%")},
		{Label: "AVG SPEED", Value: floatStr(m.AvgSpeedSeconds, "s")},
		{Label: "ATTEMPTED", Value: attV},
		{Label: "CORRECT", Value: intStr(m.Correct)},
		{Label: "INCORRECT", Value: intStr(m.Incorrect)},
		{Label: "UNATTEMPTED", Value: intStr(m.Unattempted)},
	}
	sb.WriteString(MetricGrid(cards, inner) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("WEAK TOPICS") + "\n")
	if len(s.sections) == 0 {
		sb.WriteString(thHelp.Render("No sections yet.") + "\n")
	}
	for i, sec := range s.sections {
		head := SectionHeader(strings.ToUpper(TruncateToWidth(sec.Name, inner-4)))
		if s.focusR && i == s.secCur {
			head = thFocusMark.Render("▶ ") + thPanelTitle.Render(strings.ToUpper(sec.Name))
		}
		sb.WriteString(head + "\n")
		ws := s.weak[string(sec.ID)]
		if len(ws) == 0 {
			sb.WriteString("  " + thHelp.Render("—") + "\n")
		}
		for j, w := range ws {
			nm := s.topNames[string(w.TopicID)]
			if nm == "" {
				nm = string(w.TopicID)
			}
			sb.WriteString("  " + thPrimary.Render(fmt.Sprintf("%02d  %s", j+1, TruncateToWidth(nm, inner-8))) + "\n")
			if j < len(ws)-1 {
				sb.WriteString("  " + RowDivider(inner-2) + "\n")
			}
		}
		if i < len(s.sections)-1 {
			sb.WriteString(RowDivider(inner) + "\n")
		}
	}
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(thHelp.Render("Enter weak topics"))
	return sb.String()
}

// createSubjects returns the cached subject names of the selected exam.
func (s *MocksScreen) createSubjects() []string { return s.cSubjects }

func (s *MocksScreen) createView() string {
	w, h := s.app.contentViewport()
	lw, rw := TwoPaneLayout(w, 32, 24, 40)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	var sb strings.Builder
	sb.WriteString(SectionHeader("EXAM") + "\n")
	for i, e := range s.exams {
		mark := "○"
		if i == s.cExamCur {
			mark = "◉"
		}
		// Names start at the same column as the form inputs (label 15).
		arrow := "  "
		if i == s.cExamCur && s.cFocus == 0 {
			arrow = thFocusMark.Render("▶ ")
		}
		sb.WriteString(arrow + padRight(mark, 13) + " " + thPrimary.Render(TruncateToWidth(e.Name, lw-24)) + "\n")
	}
	// Sections derive automatically from the selected exam's subjects.
	sb.WriteString(RowDivider(lw-6) + "\n")
	sb.WriteString(SectionHeader("SECTIONS") + "\n")
	for _, sc := range s.createSubjects() {
		sb.WriteString(thPrimary.Render(TruncateToWidth(sc, lw-8)) + "\n")
	}
	left := PanelFixed("EXAM", sb.String(), lw, bodyH, s.cFocus == 0)
	var rb strings.Builder
	rb.WriteString(SectionHeader("NEW MOCK") + "\n")
	for i, lb := range s.cLabels {
		focused := s.cFocus == i+1
		// Fixed label column + fixed-width inputs: every input starts
		// at the same column with the same usable width.
		ff := FormField{Label: lb, Input: s.cInputs[i]}
		rb.WriteString(ff.View(focused, 15) + "\n")
	}
	rb.WriteString(RowDivider(rw-6) + "\n")
	save := Button{Label: "SAVE", Focused: s.cFocus == s.nCreateFocus()-1}
	rb.WriteString(save.View() + "\n")
	if s.cErr != "" {
		rb.WriteString(thError.Render(s.cErr) + "\n")
	}
	rb.WriteString(thHelp.Render("↑/↓ move · H/L exam · Enter SAVE · ESC back"))
	// The create form scrolls independently when it overflows.
	right := PanelFixed("DETAILS", ViewportView(rb.String(), &s.formScroll, bodyH-4), rw, bodyH, s.cFocus > 0)
	return lipglossJoin(left, right)
}

func (s *MocksScreen) weakView() string {
	w, h := s.app.contentViewport()
	lw, rw := TwoPaneLayout(w, 45, 30, 32)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	// Left: filter + exam topic choices (selector).
	var lb strings.Builder
	lb.WriteString(SectionHeader("FILTER") + "\n")
	lb.WriteString(s.wFilter.View() + "\n")
	lb.WriteString(RowDivider(lw-6) + "\n")
	s.wList.SetWidth(lw - 6)
	innerH := bodyH - 10
	if innerH < 4 {
		innerH = 4
	}
	s.wList.FitHeight(innerH)
	lb.WriteString(SectionHeader(fmt.Sprintf("TOPICS — %d / 3 SELECTED", len(s.wStaged))) + "\n")
	lb.WriteString(s.wList.View())
	left := PanelFixed("TOPIC SELECTOR", lb.String(), lw, bodyH, s.wFocus <= 1)
	// Right: staged ordered + save.
	var rb strings.Builder
	rb.WriteString(thPrimary.Render(TruncateToWidth(string(s.wSection.Name), rw-8)) + "\n")
	rb.WriteString(RowDivider(rw-6) + "\n")
	rb.WriteString(SectionHeader("SELECTED (ORDERED)") + "\n")
	if len(s.wStaged) == 0 {
		rb.WriteString(thHelp.Render("none yet — Enter toggles") + "\n")
	}
	ctx := context.Background()
	for i, tid := range s.wStaged {
		nm := ""
		if tp, err := catalog.GetGovTopic(ctx, s.app.DB, tid); err == nil {
			nm = tp.Name
		} else {
			nm = string(tid)
		}
		rb.WriteString(thPrimary.Render(fmt.Sprintf("%02d  %s", i+1, TruncateToWidth(nm, rw-12))) + "\n")
		rb.WriteString(thMeta.Render("    "+string(tid)) + "\n")
		if i < len(s.wStaged)-1 {
			rb.WriteString(RowDivider(rw-6) + "\n")
		}
	}
	rb.WriteString(RowDivider(rw-6) + "\n")
	save := Button{Label: "SAVE", Focused: s.wFocus == 2, Disabled: false}
	rb.WriteString(save.View() + "\n")
	if s.wErr != "" {
		rb.WriteString(thError.Render(s.wErr) + "\n")
	}
	rb.WriteString(thHelp.Render("↑/↓ move · Enter toggle · S save · ESC back"))
	right := PanelFixed("WEAK TOPICS", rb.String(), rw, bodyH, s.wFocus == 2)
	return lipglossJoin(left, right)
}
