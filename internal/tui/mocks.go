package tui

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// MocksScreen: LEFT mock list, RIGHT dense mock detail with per-subject
// weak topics (gov_topic_id references, max 3 per subject, ordered).
// Mocks are historical learning evidence: create, view, edit/correct —
// no one-key destructive deletion in the normal UI.
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
	// subjNames caches GOV subject display names for render-pure detail
	// rendering (populated in Reload, never queried in View).
	subjNames map[string]string
	secCur    int
	mode      string // browse, create, weak, section
	err       string

	// create form
	cExamCur int
	cInputs  []textinput.Model
	cLabels  []string
	cFocus   int
	cErr     string

	// Direct GOV-subject mapping for the new mock: the user selects which
	// real GOV subjects were represented in the mock (no arbitrary section
	// layer, no custom section names). Weak topics are staged per subject
	// via the weak editor before the atomic engine save. Section names in
	// storage derive from the subject name for DB compatibility.
	govSubjects []db.GovSubject
	secSubjCur  int
	stagedSecs  []stagedSubject

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
	// Staged mode: the editor picks weak topics for a not-yet-persisted
	// staged section (create flow) instead of a stored section.
	wStagedMode bool
	wStagedIdx  int
	wScroll     ScrollViewport // staged summary scrolls when overflowing

	// Shared scroll viewports: detail and create panes scroll
	// independently whenever they overflow.
	detailScroll ScrollViewport
	formScroll   ScrollViewport
}

// stagedSubject is one selected GOV subject pending the atomic save:
// the canonical mapping unit + ordered weak topics (0-3, rank = index+1).
// Storage section names derive from the subject name; the UI never asks
// for an arbitrary section name.
type stagedSubject struct {
	GovSubjectID db.ID
	Weak         []db.ID
}

// stagedSection is kept for compat (legacy name); new code uses stagedSubject.
type stagedSection = stagedSubject

func NewMocksScreen(a *App) *MocksScreen {
	return &MocksScreen{app: a, list: NewNavigator(), weak: map[string][]db.MockWeakTopic{}, topNames: map[string]string{}, subjNames: map[string]string{}, mode: "browse"}
}

func (s *MocksScreen) ownsKeys() bool {
	return s.mode == "create" || s.mode == "weak"
}

func (s *MocksScreen) Reload() {
	ctx := context.Background()
	s.err = ""
	exams, err := catalog.GetGovExams(ctx, s.app.DB)
	if err != nil {
		s.err = "data error: " + err.Error()
		return
	}
	s.exams = exams
	s.examName = map[string]string{}
	for _, e := range exams {
		s.examName[string(e.ID)] = e.Name
	}
	// Cache GOV subject names once for render-pure detail rendering.
	if s.subjNames == nil {
		s.subjNames = map[string]string{}
	}
	if subjs, err := catalog.GetGovSubjects(ctx, s.app.DB); err == nil {
		for _, g := range subjs {
			s.subjNames[string(g.ID)] = g.Name
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
	secs, err := runtime.ListMockSections(ctx, s.app.DB, m.ID)
	if err != nil {
		s.err = "data error: " + err.Error()
		return
	}
	s.sections = secs
	// Collect uncached topic IDs first, then resolve in batch with a single
	// follow-up pass (no nested query while rows are open; cache prevents
	// repeated lookups for static catalogue data).
	var missing []db.ID
	seenMissing := map[string]bool{}
	for _, sec := range secs {
		ws, err := runtime.ListWeakTopics(ctx, s.app.DB, sec.ID)
		if err != nil {
			s.err = "data error: " + err.Error()
			return
		}
		s.weak[string(sec.ID)] = ws
		for _, w := range ws {
			if _, ok := s.topNames[string(w.TopicID)]; !ok && !seenMissing[string(w.TopicID)] {
				seenMissing[string(w.TopicID)] = true
				missing = append(missing, w.TopicID)
			}
		}
	}
	for _, tid := range missing {
		if tp, err := catalog.GetGovTopic(ctx, s.app.DB, tid); err == nil {
			s.topNames[string(tid)] = tp.Name
		} else {
			s.topNames[string(tid)] = string(tid)
		}
	}
	// Keep the subject-name cache warm for any mapped sections.
	if s.subjNames == nil {
		s.subjNames = map[string]string{}
	}
	for _, sec := range secs {
		if sec.GovSubjectID != nil && *sec.GovSubjectID != "" {
			if _, ok := s.subjNames[string(*sec.GovSubjectID)]; !ok {
				if g, err := catalog.GetGovSubject(ctx, s.app.DB, *sec.GovSubjectID); err == nil {
					s.subjNames[string(*sec.GovSubjectID)] = g.Name
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

// mockInnerW returns the actual mock detail inner width (same geometry as View).
func (s *MocksScreen) mockInnerW() int {
	w, _ := s.app.contentViewport()
	_, rw := TwoPaneLayout(w, mockBrowseLeftPct, 26, 40)
	inner := rw - 6
	if inner < 24 {
		inner = 24
	}
	return inner
}

// createInnerW returns the actual create-form inner width.
func (s *MocksScreen) createInnerW() int {
	w, _ := s.app.contentViewport()
	_, rw := TwoPaneLayout(w, 32, 24, 40)
	inner := rw - 6
	if inner < 18 {
		inner = 18
	}
	return inner
}

// followSection keeps the selected weak-topic section visible: top on the
// first section, bottom on the last, proportional in between.
func (s *MocksScreen) followSection() {
	n := len(s.sections)
	if n <= 1 {
		return
	}
	total := len(strings.Split(s.mockBody(s.mockInnerW()), "\n"))
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
	case "weak":
		if s.wStagedMode {
			// Back to the create form; the staged section (and its
			// staged weak topics) is kept for the atomic save.
			s.mode = "create"
			s.wStagedMode = false
			s.wErr = ""
			return true
		}
		s.mode = "browse"
		s.cErr, s.wErr = "", ""
		s.Reload()
		return true
	case "create":
		s.mode = "browse"
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
		case "j", "down", "k", "up":
			// ONE shared scrolling model: selectable content moves focus
			// and the viewport follows (RowList + followSection). No
			// hybrid move-until-boundary-then-scroll step.
			down := k.String() == "j" || k.String() == "down"
			if s.focusR {
				if down {
					if s.secCur < len(s.sections)-1 {
						s.secCur++
						s.followSection()
					}
				} else {
					if s.secCur > 0 {
						s.secCur--
						s.followSection()
					}
				}
			} else {
				prev := s.list.SelectedID()
				if down {
					s.list.CursorDown()
				} else {
					s.list.CursorUp()
				}
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
		}
	}
	return nil
}

// deleteSelected is admin/maintenance only (tests, dev, compatibility).
// It is deliberately NOT bound to any key in the normal product UI:
// mocks are historical evidence, not disposable todos.
func (s *MocksScreen) deleteSelected() {
	m := s.selMock()
	if m == nil {
		return
	}
	ctx := context.Background()
	// Engine-owned mutation (existence-checked); runtime performs SQL only.
	if err := engine.DeleteMock(ctx, s.app.DB, m.ID); err != nil {
		s.err = err.Error()
		return
	}
	s.focusR = false
	s.Reload()
}

// ---- create ----

// createFocus layout: 0 exam selector, 1..N mock inputs, then the GOV
// subject block (subject selector, ADD button), then SAVE. No arbitrary
// section-name layer: the real GOV subject is the canonical mapping unit.
const createInputCount = 11

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
	s.secSubjCur = 0
	s.stagedSecs = nil
	subjs, err := catalog.GetGovSubjects(context.Background(), s.app.DB)
	if err != nil {
		s.cErr = "data error: " + err.Error()
	} else {
		s.govSubjects = subjs
		if s.subjNames == nil {
			s.subjNames = map[string]string{}
		}
		for _, g := range subjs {
			s.subjNames[string(g.ID)] = g.Name
		}
	}
	s.cFocus = 0 // 0 = exam selector, 1..11 inputs, 12 subject, 13 add, 14 save
	s.cErr = ""
	s.formScroll.Reset()
	s.updateCreateFocus()
}

func (s *MocksScreen) nCreateFocus() int { return 1 + createInputCount + 3 } // exam + inputs + subject + add + save

func (s *MocksScreen) secSubjName() string {
	if len(s.govSubjects) == 0 {
		return "—"
	}
	return s.govSubjects[s.secSubjCur%len(s.govSubjects)].Name
}

func (s *MocksScreen) updateCreateFocus() {
	for i := range s.cInputs {
		s.cInputs[i].Blur()
	}
	if s.cFocus >= 1 && s.cFocus <= len(s.cInputs) {
		s.cInputs[s.cFocus-1].Focus()
	}
}

// createFocusLine returns the 0-based content line of the focused widget
// inside createRightBody output, derived from the same structured layout
// the renderer uses (never hard-coded guesses). Multi-line controls
// (bordered buttons) resolve to their first line.
func (s *MocksScreen) createFocusLine() int {
	n := len(s.cInputs)
	switch {
	case s.cFocus <= 0:
		return 0
	case s.cFocus >= 1 && s.cFocus <= n:
		return s.cFocus // header(0) + inputs start at line 1
	case s.cFocus == n+1: // GOV subject selector
		return n + 3
	case s.cFocus == n+2: // ADD button box
		return n + 4
	default: // SAVE button box
		return n + 8
	}
}

// followCreateFocus keeps the focused create field visible: exam on top,
// SAVE on bottom, exact structured line otherwise.
func (s *MocksScreen) followCreateFocus() {
	total := len(strings.Split(s.createRightBody(s.createInnerW()), "\n"))
	_, h := s.app.contentViewport()
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	if s.cFocus <= 0 {
		s.formScroll.Reset()
		return
	}
	if s.cFocus >= s.nCreateFocus()-1 {
		s.formScroll.Offset = 1 << 30
		s.formScroll.Clamp(total, innerH)
		return
	}
	fl := s.createFocusLine()
	if fl < s.formScroll.Offset {
		s.formScroll.Offset = fl
	} else if fl > s.formScroll.Offset+innerH-1 {
		s.formScroll.Offset = fl - innerH + 1
	}
	s.formScroll.Clamp(total, innerH)
}

func (s *MocksScreen) updateCreate(msg tea.Msg) tea.Cmd {
	k, isKey := msg.(tea.KeyPressMsg)
	subjFocus := len(s.cInputs) + 1
	addFocus := len(s.cInputs) + 2
	if !isKey {
		// Forward to focused input.
		if s.cFocus >= 1 && s.cFocus <= len(s.cInputs) {
			var cmd tea.Cmd
			s.cInputs[s.cFocus-1], cmd = s.cInputs[s.cFocus-1].Update(msg)
			return cmd
		}
		return nil
	}
	// FORM rule: ↑/↓ or j/k moves field focus when a non-text widget owns
	// focus; arrow keys and letters belong to the text input otherwise.
	// The viewport always follows focus (followCreateFocus).
	isText := s.cFocus >= 1 && s.cFocus <= len(s.cInputs)
	ks := k.String()
	if !isText {
		if ks == "j" {
			ks = "down"
		} else if ks == "k" {
			ks = "up"
		}
	}
	switch ks {
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
			return nil
		}
		if s.cFocus == subjFocus && len(s.govSubjects) > 0 {
			s.secSubjCur = (s.secSubjCur - 1 + len(s.govSubjects)) % len(s.govSubjects)
			return nil
		}
	case "right", "l":
		if s.cFocus == 0 && len(s.exams) > 0 {
			s.cExamCur = (s.cExamCur + 1) % len(s.exams)
			return nil
		}
		if s.cFocus == subjFocus && len(s.govSubjects) > 0 {
			s.secSubjCur = (s.secSubjCur + 1) % len(s.govSubjects)
			return nil
		}
	case "enter":
		if s.cFocus == s.nCreateFocus()-1 {
			if s.saveCreate() {
				s.mode = "browse"
				s.Reload()
			}
			return nil
		}
		if s.cFocus == addFocus {
			s.addStagedSubject()
			return nil
		}
		if s.cFocus == 0 {
			s.cFocus = 1
			s.updateCreateFocus()
			s.followCreateFocus()
			return nil
		}
		// Enter in input moves next.
		s.cFocus = (s.cFocus + 1) % s.nCreateFocus()
		s.updateCreateFocus()
		s.followCreateFocus()
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
	// Strict full-consumption parsing: the entire input must be an
	// integer ("100abc", "10 20" and "1.5" are rejected, not truncated).
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("must be int >= 0")
	}
	return &n, nil
}

func parseOptFloat(v string) (*float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	// Strict full-consumption parsing: "100abc" is rejected, "100.5" is
	// accepted; non-finite values are never metrics.
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
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
	// Optional-metric semantics: omitted values stay unknown (nil) and are
	// never treated as numeric zero. Only enforce correct <= attempted when
	// both are supplied, and attempted <= total_questions only when the
	// total is actually supplied. Derived values are produced only when all
	// required inputs exist; otherwise they stay nil (never negative or
	// manufactured zeros).
	if correct != nil && attempted != nil && *correct > *attempted {
		s.cErr = "need correct <= attempted"
		return false
	}
	if attempted != nil && totalQ != nil && *attempted > *totalQ {
		s.cErr = "need attempted <= total questions"
		return false
	}
	// Derived metrics (never entered): incorrect, unattempted, accuracy.
	// incorrect requires both attempted and correct; unattempted requires
	// both total and attempted; accuracy requires attempted > 0 with both
	// attempted and correct known.
	var incorrect, unatt *int
	if attempted != nil && correct != nil {
		v := *attempted - *correct
		incorrect = &v
	}
	if totalQ != nil && attempted != nil {
		v := *totalQ - *attempted
		unatt = &v
	}
	var acc *float64
	if attempted != nil && correct != nil && *attempted > 0 {
		a := float64(*correct) / float64(*attempted) * 100
		acc = &a
	}
	exam := s.exams[s.cExamCur%len(s.exams)]
	id := db.ID(fmt.Sprintf("mock-%d", time.Now().UTC().UnixNano()))
	m := db.Mock{ID: id, ExamID: exam.ID, Name: name, MockDate: date, Platform: platform,
		Score: score, TotalScore: total, Attempted: attempted, TotalQuestions: totalQ,
		Correct: correct, Incorrect: incorrect, Unattempted: unatt, RankValue: rankV,
		Percentile: perc, Accuracy: acc, AvgSpeedSeconds: avg, CreatedAt: time.Now().UTC()}
	// One atomic engine path: mock + GOV-subject sections + weak topics in
	// ONE transaction (TUI input -> Engine CreateMockAtomic -> SQLite).
	// The UI never orchestrates the transaction itself. Storage section
	// names derive from the subject name; the UI never asks for an
	// arbitrary section name.
	secs := make([]engine.MockSectionInput, 0, len(s.stagedSecs))
	for _, st := range s.stagedSecs {
		secs = append(secs, engine.MockSectionInput{
			Name: s.govSubjectName(st.GovSubjectID), GovSubjectID: st.GovSubjectID, WeakTopics: st.Weak,
		})
	}
	if _, err := engine.CreateMockAtomic(ctx, s.app.DB, m, secs, time.Now().UTC()); err != nil {
		s.cErr = err.Error()
		return false
	}
	s.stagedSecs = nil
	return true
}

// addStagedSubject stages one real GOV subject for the new mock and opens
// the weak-topic editor for it so its 0-3 ordered weak topics are picked
// before the atomic save. No arbitrary section layer exists.
func (s *MocksScreen) addStagedSubject() {
	if len(s.govSubjects) == 0 {
		s.cErr = "no GOV subjects"
		return
	}
	subj := s.govSubjects[s.secSubjCur%len(s.govSubjects)]
	for _, st := range s.stagedSecs {
		if st.GovSubjectID == subj.ID {
			s.cErr = "subject already added"
			return
		}
	}
	s.stagedSecs = append(s.stagedSecs, stagedSubject{GovSubjectID: subj.ID})
	s.cErr = ""
	s.updateCreateFocus()
	s.openWeakForStaged(len(s.stagedSecs) - 1)
}

// addStagedSection is kept for compat (legacy name); new product UI uses
// direct GOV-subject selection via addStagedSubject.
func (s *MocksScreen) addStagedSection() {
	s.addStagedSubject()
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
	s.wStagedMode = false
	s.openWeakFor(*m, s.sections[s.secCur])
}

// openWeakFor opens the weak-topic editor for one concrete section.
// Choices are real gov topics filtered to the section's explicit GOV subject
// mapping AND the exam syllabus. The mapping is never inferred from the name
// when gov_subject_id is present; legacy unmapped sections fall back to
// name matching.
func (s *MocksScreen) openWeakFor(m db.Mock, sec db.MockSection) {
	ctx := context.Background()
	choices, err := s.sectionTopicChoicesFor(ctx, m.ExamID, sec)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.wMockID = m.ID
	s.wSection = sec
	s.wExamID = m.ExamID
	s.wChoices = choices
	s.wSubjName = map[string]string{}
	subjs, err := catalog.GetGovSubjects(ctx, s.app.DB)
	if err != nil {
		s.err = "data error: " + err.Error()
		return
	}
	for _, g := range subjs {
		s.wSubjName[string(g.ID)] = g.Name
	}
	exist, err := runtime.ListWeakTopics(ctx, s.app.DB, sec.ID)
	if err != nil {
		s.err = "data error: " + err.Error()
		return
	}
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
	s.wStagedMode = false
	s.mode = "weak"
	s.refreshWeakList()
}

// openWeakForStaged opens the weak-topic editor for a not-yet-persisted
// staged GOV subject of the create flow. Choices are filtered to the staged
// subject AND the selected exam syllabus; saving stages the ordered weak
// topics back into the create form (no DB write).
func (s *MocksScreen) openWeakForStaged(idx int) {
	if idx < 0 || idx >= len(s.stagedSecs) {
		return
	}
	if len(s.exams) == 0 {
		s.cErr = "no exams"
		return
	}
	st := s.stagedSecs[idx]
	exam := s.exams[s.cExamCur%len(s.exams)]
	sec := db.MockSection{
		ID:   db.ID(fmt.Sprintf("staged-%d", idx)),
		Name: s.govSubjectName(st.GovSubjectID), GovSubjectID: &st.GovSubjectID,
	}
	ctx := context.Background()
	choices, err := s.sectionTopicChoicesFor(ctx, exam.ID, sec)
	if err != nil {
		s.cErr = err.Error()
		return
	}
	s.wMockID = ""
	s.wSection = sec
	s.wExamID = exam.ID
	s.wChoices = choices
	s.wSubjName = map[string]string{}
	subjs, err := catalog.GetGovSubjects(ctx, s.app.DB)
	if err != nil {
		s.cErr = "data error: " + err.Error()
		return
	}
	for _, g := range subjs {
		s.wSubjName[string(g.ID)] = g.Name
	}
	s.wStaged = append([]db.ID(nil), st.Weak...)
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
	s.wStagedMode = true
	s.wStagedIdx = idx
	s.wScroll.Reset()
	s.mode = "weak"
	s.refreshWeakList()
}

// saveWeakTopics persists the staged ordered weak topics. For stored
// sections it uses the atomic engine path with full subject+syllabus
// validation and returns to browsing. For staged create-flow sections it
// validates the same way and stages the topics back into the create form
// (no DB write; the atomic engine save happens on mock SAVE).
func (s *MocksScreen) saveWeakTopics() {
	ctx := context.Background()
	subj := db.ID("")
	if s.wSection.GovSubjectID != nil {
		subj = *s.wSection.GovSubjectID
	} else {
		// Legacy unmapped section: resolve via name for validation.
		if resolved, err := s.resolveSubjectForLegacy(ctx, s.wExamID, s.wSection.Name); err == nil {
			subj = resolved
		}
	}
	if subj == "" {
		s.wErr = "section has no GOV subject mapping"
		return
	}
	if s.wStagedMode {
		if s.wStagedIdx < 0 || s.wStagedIdx >= len(s.stagedSecs) {
			s.wErr = "staged section gone"
			s.mode = "create"
			s.wStagedMode = false
			return
		}
		if err := engine.ValidateMockSection(ctx, s.app.DB, s.wExamID, engine.MockSectionInput{
			Name: s.govSubjectName(s.stagedSecs[s.wStagedIdx].GovSubjectID), GovSubjectID: subj, WeakTopics: s.wStaged,
		}); err != nil {
			s.wErr = err.Error()
			return
		}
		s.stagedSecs[s.wStagedIdx].Weak = append([]db.ID(nil), s.wStaged...)
		s.mode = "create"
		s.wStagedMode = false
		s.wErr = ""
		s.cFocus = len(s.cInputs) + 1 // back at the GOV subject selector
		s.updateCreateFocus()
		s.followCreateFocus()
		return
	}
	if err := engine.ReplaceWeakTopicsAtomic(ctx, s.app.DB, s.wMockID, s.wSection.ID, subj, s.wStaged); err != nil {
		s.wErr = err.Error()
		return
	}
	s.mode = "browse"
	s.Reload()
}

// sectionTopicChoicesFor returns real gov topics for one section: the exam
// topics belonging to the section's explicit subject mapping. It falls back
// to name matching only for legacy sections without a mapping, and to every
// exam topic when the section does not map to an exam subject.
func (s *MocksScreen) sectionTopicChoicesFor(ctx context.Context, examID db.ID, sec db.MockSection) ([]db.GovTopic, error) {
	all, err := runtime.ExamTopicChoices(ctx, s.app.DB, examID)
	if err != nil {
		return nil, err
	}
	if sec.GovSubjectID != nil && *sec.GovSubjectID != "" {
		out := make([]db.GovTopic, 0)
		for _, tp := range all {
			if tp.SubjectID == *sec.GovSubjectID {
				out = append(out, tp)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
		return all, nil
	}
	return s.sectionTopicChoices(ctx, examID, sec.Name)
}

func (s *MocksScreen) resolveSubjectForLegacy(ctx context.Context, examID db.ID, section string) (db.ID, error) {
	cov, err := ComputeExamCoverage(ctx, s.app.DB, examID)
	if err != nil {
		return "", err
	}
	for _, sc := range cov.Subjects {
		if strings.EqualFold(sc.Subject, section) {
			return sc.SubjectID, nil
		}
	}
	return "", fmt.Errorf("no subject mapping for legacy section %q", section)
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

// mockBrowseLeftPct is the MOCKS browse split: the mock list needs room
// for number + name + date + exam on one row, so it takes ~41% (right
// ~59%). Uses the shared TwoPaneLayout calculation, not hardcoded widths.
const mockBrowseLeftPct = 41

func (s *MocksScreen) browseView() string {
	w, h := s.app.contentViewport()
	lw, rw := TwoPaneLayout(w, mockBrowseLeftPct, 26, 40)
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
	// Pure mock list. The N NEW action lives permanently in the global
	// footer legend; no redundant action row here.
	leftBody := SectionHeader("MOCK LIST") + "\n" + s.list.View()
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
		sb.WriteString(thHelp.Render("No subjects yet.") + "\n")
	}
	for i, sec := range s.sections {
		head := SectionHeader(strings.ToUpper(TruncateToWidth(sec.Name, inner-4)))
		if s.focusR && i == s.secCur {
			head = thFocusMark.Render("▶ ") + thPanelTitle.Render(strings.ToUpper(sec.Name))
		}
		sb.WriteString(head + "\n")
		sb.WriteString("    " + thMeta.Render("maps to "+s.sectionSubjectLabel(sec)) + "\n")
		ws := s.weak[string(sec.ID)]
		if len(ws) == 0 {
			sb.WriteString("    " + thHelp.Render("—") + "\n")
		}
		for j, w := range ws {
			nm := s.topNames[string(w.TopicID)]
			if nm == "" {
				nm = string(w.TopicID)
			}
			sb.WriteString("    " + thPrimary.Render(fmt.Sprintf("%02d  %s", j+1, TruncateToWidth(nm, inner-10))) + "\n")
			if j < len(ws)-1 {
				sb.WriteString("    " + RowDivider(inner-4) + "\n")
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

// govSubjectName resolves a GOV subject ID to its display name purely from
// the reload-time cache (plus the create-flow govSubjects slice). It never
// performs database access, so it is safe to call from View.
func (s *MocksScreen) govSubjectName(id db.ID) string {
	if nm, ok := s.subjNames[string(id)]; ok && nm != "" {
		return nm
	}
	for _, g := range s.govSubjects {
		if g.ID == id {
			return g.Name
		}
	}
	if s.wSubjName != nil {
		if nm, ok := s.wSubjName[string(id)]; ok && nm != "" {
			return nm
		}
	}
	return string(id)
}

// stagedSummary renders the selected GOV subjects for the create form:
// real subject names + weak-topic count. No arbitrary section names.
func (s *MocksScreen) stagedSummary(inner int) string {
	if len(s.stagedSecs) == 0 {
		return thHelp.Render("no subjects yet — select below")
	}
	var sb strings.Builder
	for i, st := range s.stagedSecs {
		nm := s.govSubjectName(st.GovSubjectID)
		head := fmt.Sprintf("%02d  %s", i+1, TruncateToWidth(nm, inner-14))
		sb.WriteString(thPrimary.Render(head) + "\n")
		sb.WriteString("    " + thMeta.Render(fmt.Sprintf("%d/3 weak", len(st.Weak))) + "\n")
		if i < len(s.stagedSecs)-1 {
			sb.WriteString(RowDivider(inner) + "\n")
		}
	}
	return sb.String()
}

func (s *MocksScreen) sectionSubjectLabel(sec db.MockSection) string {
	// Render-pure: subject names come from the reload-time cache only.
	if sec.GovSubjectID != nil && *sec.GovSubjectID != "" {
		if nm, ok := s.subjNames[string(*sec.GovSubjectID)]; ok && nm != "" {
			return nm
		}
		for _, g := range s.govSubjects {
			if g.ID == *sec.GovSubjectID {
				return g.Name
			}
		}
		return string(*sec.GovSubjectID)
	}
	return "unmapped legacy"
}

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
	// Direct GOV-subject mapping: real subjects represented in this mock.
	sb.WriteString(RowDivider(lw-6) + "\n")
	sb.WriteString(SectionHeader("GOV SUBJECTS IN THIS MOCK") + "\n")
	sb.WriteString(s.stagedSummary(lw-6) + "\n")
	left := PanelFixed("EXAM", sb.String(), lw, bodyH, s.cFocus == 0)
	// The create form scrolls independently when it overflows.
	right := PanelFixed("DETAILS", ViewportView(s.createRightBody(rw-6), &s.formScroll, bodyH-4), rw, bodyH, s.cFocus > 0)
	return lipglossJoin(left, right)
}

// createRightBody renders the right create-form body. Its line structure is
// the structured field model createFocusLine/followCreateFocus derive from:
// header(1) + inputs + divider + GOV SUBJECT header + subject selector +
// ADD + divider + SAVE (+ optional error + help). No section-name layer.
func (s *MocksScreen) createRightBody(inner int) string {
	if inner < 18 {
		inner = 18
	}
	n := len(s.cInputs)
	subjFocus := n + 1
	addFocus := n + 2
	var rb strings.Builder
	rb.WriteString(SectionHeader("NEW MOCK") + "\n")
	for i, lb := range s.cLabels {
		focused := s.cFocus == i+1
		// Fixed label column + fixed-width inputs: every input starts
		// at the same column with the same usable width.
		ff := FormField{Label: lb, Input: s.cInputs[i]}
		rb.WriteString(ff.View(focused, 15) + "\n")
	}
	rb.WriteString(RowDivider(inner) + "\n")
	rb.WriteString(SectionHeader("GOV SUBJECTS IN THIS MOCK") + "\n")
	subjLine := "  " + thMeta.Render(padRight("GOV subject", 15)+" ") + thPrimary.Render(TruncateToWidth(s.secSubjName(), 20))
	if s.cFocus == subjFocus {
		subjLine = thFocusMark.Render("▶ ") + thMeta.Render(padRight("GOV subject", 15)+" ") + thPrimary.Render(TruncateToWidth(s.secSubjName(), 20))
	}
	rb.WriteString(subjLine + "\n")
	rb.WriteString(Button{Label: "ADD SUBJECT", Focused: s.cFocus == addFocus}.View() + "\n")
	rb.WriteString(RowDivider(inner) + "\n")
	save := Button{Label: "SAVE", Focused: s.cFocus == s.nCreateFocus()-1}
	rb.WriteString(save.View() + "\n")
	if s.cErr != "" {
		rb.WriteString(thError.Render(s.cErr) + "\n")
	}
	rb.WriteString(thHelp.Render("↑/↓ move · H/L exam+subject · Enter ADD/SAVE · Esc back"))
	return rb.String()
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
	// Render-pure topic names: prefer the editor choices (already loaded),
	// then the browse-time topNames cache. Never queries SQLite in View.
	choiceNames := make(map[string]string, len(s.wChoices))
	for _, tp := range s.wChoices {
		choiceNames[string(tp.ID)] = tp.Name
	}
	for i, tid := range s.wStaged {
		nm := choiceNames[string(tid)]
		if nm == "" {
			nm = s.topNames[string(tid)]
		}
		if nm == "" {
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
	rb.WriteString(thHelp.Render("↑/↓ move · Space toggle · Enter save"))
	// The staged summary scrolls independently when it overflows: the
	// SAVE row follows focus (bottom when focused, top otherwise).
	rightH := bodyH - 4
	if rightH < 4 {
		rightH = 4
	}
	total := len(strings.Split(rb.String(), "\n"))
	if s.wFocus == 2 {
		s.wScroll.Offset = 1 << 30
	} else {
		s.wScroll.Reset()
	}
	s.wScroll.Clamp(total, rightH)
	right := PanelFixed("WEAK TOPICS", ViewportView(rb.String(), &s.wScroll, rightH), rw, bodyH, s.wFocus == 2)
	return lipglossJoin(left, right)
}
