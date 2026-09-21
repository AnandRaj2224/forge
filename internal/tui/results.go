package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// PracticeRow is one editable topic row in the GOV practice result form.
type PracticeRow struct {
	TopicID db.ID
	Topic   string
	Time    FormField
	Att     FormField
	Corr    FormField
	Avg     FormField
}

// DSARow is one problem block in the DSA result form.
type DSARow struct {
	ProblemID db.ID
	Title     string
	Mode      db.DSAMode
	Indep     Checkbox
	Optimal   Checkbox
	Hint      Checkbox
	Solve     FormField
}

// SWERow is one task outcome in the SWE result form.
type SWERow struct {
	TaskID db.ID
	Area   string
	Task   string
	Radio  RadioGroup
}

// ResultForm is the domain-specific evidence form embedded in Today's
// right-bottom panel. SAVE stays disabled until every bundled item validates.
type ResultForm struct {
	app       *App
	sessionID string
	task      *db.TodayTask
	bundle    Bundle
	kind      string
	lecture   RadioGroup
	prows     []PracticeRow
	drows     []DSARow
	srows     []SWERow
	focus     int
	save      Button
	err       string
}

func NewResultForm(a *App) *ResultForm {
	return &ResultForm{app: a, lecture: NewRadio([]string{"YES", "NO"}), save: Button{Label: "SAVE", Disabled: true}}
}

func (s *ResultForm) Reload(sessionID string, task db.TodayTask) {
	ctx := context.Background()
	s.sessionID = sessionID
	s.task = &task
	s.bundle = ResolveBundle(ctx, s.app.DB, task)
	s.kind = resultKindForTask(task)
	s.focus, s.err = 0, ""
	s.prows, s.drows, s.srows = nil, nil, nil
	s.lecture = NewRadio([]string{"YES", "NO"})
	switch s.kind {
	case "PRACTICE":
		for _, p := range s.bundle.Practice {
			s.prows = append(s.prows, PracticeRow{TopicID: p.TopicID, Topic: p.Topic,
				Time: NewField("Time", "1380"), Att: NewField("Attempted", "20"),
				Corr: NewField("Correct", "16"), Avg: NewField("Avg", "17.2")})
		}
	case "DSA":
		for _, d := range s.bundle.DSA {
			s.drows = append(s.drows, DSARow{ProblemID: d.ProblemID, Title: d.Title, Mode: d.Mode,
				Indep: Checkbox{Label: "Independent"}, Optimal: Checkbox{Label: "Optimal"},
				Hint: Checkbox{Label: "Hint used"}, Solve: NewField("Solve time", "300")})
		}
	case "SWE":
		for _, sw := range s.bundle.SWE {
			label := sw.Task
			if label == "" {
				label = sw.Area
			}
			tid := db.ID("")
			if sw.TaskID != nil {
				tid = *sw.TaskID
			}
			s.srows = append(s.srows, SWERow{TaskID: tid, Area: sw.Area, Task: label, Radio: NewRadio([]string{"COMPLETED", "PARTIAL", "BLOCKED"})})
		}
	}
	s.updateFocusVisual()
}

func resultKindForTask(t db.TodayTask) string {
	if t.Domain == db.DomainGOV && t.Kind == db.SessionKindLecture {
		return "LECTURE"
	}
	if t.Domain == db.DomainGOV {
		return "PRACTICE"
	}
	if t.Domain == db.DomainDSA {
		return "DSA"
	}
	return "SWE"
}

func (s *ResultForm) nFocus() int {
	switch s.kind {
	case "LECTURE":
		return 2
	case "PRACTICE":
		return len(s.prows)*4 + 1
	case "DSA":
		return len(s.drows)*4 + 1
	case "SWE":
		return len(s.srows) + 1
	}
	return 1
}

func (s *ResultForm) revalidate() {
	switch s.kind {
	case "LECTURE":
		s.save.Disabled = s.lecture.Selected < 0
	case "PRACTICE":
		if len(s.prows) == 0 {
			s.save.Disabled = true
			return
		}
		for _, r := range s.prows {
			if _, _, _, _, msg := ValidatePracticeRow(r.Time.Input.Value(), r.Att.Input.Value(), r.Corr.Input.Value(), r.Avg.Input.Value()); msg != "" {
				s.save.Disabled = true
				return
			}
		}
		s.save.Disabled = false
	case "DSA":
		if len(s.drows) == 0 {
			s.save.Disabled = true
			return
		}
		for _, r := range s.drows {
			if _, _, msg := ValidateSolveTime(r.Solve.Input.Value()); msg != "" {
				s.save.Disabled = true
				return
			}
		}
		s.save.Disabled = false
	case "SWE":
		if len(s.srows) == 0 {
			s.save.Disabled = true
			return
		}
		for _, r := range s.srows {
			if r.Radio.Selected < 0 {
				s.save.Disabled = true
				return
			}
		}
		s.save.Disabled = false
	}
	s.save.Focused = s.focus == s.nFocus()-1
}

func (s *ResultForm) updateFocusVisual() {
	s.revalidate()
	for i := range s.prows {
		s.prows[i].Time.Input.Blur()
		s.prows[i].Att.Input.Blur()
		s.prows[i].Corr.Input.Blur()
		s.prows[i].Avg.Input.Blur()
	}
	for i := range s.drows {
		s.drows[i].Solve.Input.Blur()
		s.drows[i].Indep.Focused, s.drows[i].Optimal.Focused, s.drows[i].Hint.Focused = false, false, false
	}
	s.lecture.Focused = false
	for i := range s.srows {
		s.srows[i].Radio.Focused = false
	}
	f := s.focus
	switch s.kind {
	case "LECTURE":
		if f == 0 {
			s.lecture.Focused = true
		}
	case "PRACTICE":
		if f < len(s.prows)*4 {
			r, c := f/4, f%4
			switch c {
			case 0:
				s.prows[r].Time.Input.Focus()
			case 1:
				s.prows[r].Att.Input.Focus()
			case 2:
				s.prows[r].Corr.Input.Focus()
			case 3:
				s.prows[r].Avg.Input.Focus()
			}
		}
	case "DSA":
		if f < len(s.drows)*4 {
			r, c := f/4, f%4
			switch c {
			case 0:
				s.drows[r].Indep.Focused = true
			case 1:
				s.drows[r].Optimal.Focused = true
			case 2:
				s.drows[r].Hint.Focused = true
			case 3:
				s.drows[r].Solve.Input.Focus()
			}
		}
	case "SWE":
		if f < len(s.srows) {
			s.srows[f].Radio.Focused = true
		}
	}
	s.save.Focused = f == s.nFocus()-1
}

func (s *ResultForm) moveFocus(d int) {
	n := s.nFocus()
	if n <= 0 {
		return
	}
	s.focus = (s.focus + d + n) % n
	s.updateFocusVisual()
}

// HandleKey processes a key when the result panel owns focus.
// Returns a command and whether the key was consumed.
func (s *ResultForm) HandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	ks := msg.String()
	switch ks {
	case "tab":
		s.moveFocus(1)
		return nil, true
	case "shift+tab":
		s.moveFocus(-1)
		return nil, true
	case "down":
		s.moveFocus(1)
		return nil, true
	case "up":
		s.moveFocus(-1)
		return nil, true
	}
	f := s.focus
	switch s.kind {
	case "LECTURE":
		if f == 0 {
			switch ks {
			case "left", "h", "k":
				s.lecture.Move(-1)
			case "right", "l", "j":
				s.lecture.Move(1)
			case " ", "space", "enter":
				s.lecture.Choose()
			default:
				return nil, false
			}
			s.updateFocusVisual()
			return nil, true
		}
		return nil, false
	case "PRACTICE":
		if f < len(s.prows)*4 {
			r, c := f/4, f%4
			var fld *FormField
			switch c {
			case 0:
				fld = &s.prows[r].Time
			case 1:
				fld = &s.prows[r].Att
			case 2:
				fld = &s.prows[r].Corr
			case 3:
				fld = &s.prows[r].Avg
			}
			var cmd tea.Cmd
			fld.Input, cmd = fld.Input.Update(msg)
			s.updateFocusVisual()
			return cmd, true
		}
		return nil, false
	case "DSA":
		if f < len(s.drows)*4 {
			r, c := f/4, f%4
			switch c {
			case 0:
				if ks == " " || ks == "space" || ks == "enter" {
					s.drows[r].Indep.Checked = !s.drows[r].Indep.Checked
				} else {
					return nil, false
				}
			case 1:
				if ks == " " || ks == "space" || ks == "enter" {
					s.drows[r].Optimal.Checked = !s.drows[r].Optimal.Checked
				} else {
					return nil, false
				}
			case 2:
				if ks == " " || ks == "space" || ks == "enter" {
					s.drows[r].Hint.Checked = !s.drows[r].Hint.Checked
				} else {
					return nil, false
				}
			case 3:
				var cmd tea.Cmd
				s.drows[r].Solve.Input, cmd = s.drows[r].Solve.Input.Update(msg)
				s.updateFocusVisual()
				return cmd, true
			}
			s.updateFocusVisual()
			return nil, true
		}
		return nil, false
	case "SWE":
		if f < len(s.srows) {
			switch ks {
			case "left", "h":
				s.srows[f].Radio.Move(-1)
			case "right", "l":
				s.srows[f].Radio.Move(1)
			case " ", "space", "enter":
				s.srows[f].Radio.Choose()
			default:
				return nil, false
			}
			s.updateFocusVisual()
			return nil, true
		}
		return nil, false
	}
	return nil, false
}

// TrySave validates and persists; returns true on success.
func (s *ResultForm) TrySave() (bool, string) {
	s.revalidate()
	if s.save.Disabled {
		s.err = "complete all required evidence first"
		return false, s.err
	}
	if err := s.saveAll(); err != nil {
		s.err = err.Error()
		return false, s.err
	}
	return true, ""
}

func (s *ResultForm) saveAll() error {
	ctx := context.Background()
	now := time.Now().UTC()
	switch s.kind {
	case "LECTURE":
		if s.lecture.Selected == 0 { // YES
			lt, err := db.UnmarshalGovLectureTarget(*s.task.TargetJSON)
			if err != nil {
				return err
			}
			return runtime.RecordLectureCompletion(ctx, s.app.DB, db.GovLectureCompletion{LectureID: lt.LectureID, SessionID: db.ID(s.sessionID), CompletedAt: now})
		}
		return nil
	case "PRACTICE":
		return s.app.DB.WithTx(ctx, func(q db.DBTX) error {
			for _, r := range s.prows {
				tSecs, att, corr, avg, msg := ValidatePracticeRow(r.Time.Input.Value(), r.Att.Input.Value(), r.Corr.Input.Value(), r.Avg.Input.Value())
				if msg != "" {
					return fmt.Errorf("%s: %s", r.Topic, msg)
				}
				id := db.ID(fmt.Sprintf("gpr-tui-%d-%s", now.UnixNano(), r.TopicID))
				if err := runtime.InsertGovPracticeRecord(ctx, q, db.GovPracticeRecord{ID: id, SessionID: db.ID(s.sessionID), TopicID: r.TopicID, TimeSpentSeconds: tSecs, Attempted: att, Correct: corr, AvgQuestionSpeedSeconds: avg, RecordedAt: now}); err != nil {
					return err
				}
			}
			return nil
		})
	case "DSA":
		return s.app.DB.WithTx(ctx, func(q db.DBTX) error {
			for _, r := range s.drows {
				_, solve, msg := ValidateSolveTime(r.Solve.Input.Value())
				if msg != "" {
					return fmt.Errorf("%s: %s", r.Title, msg)
				}
				existing, _ := runtime.GetDSAAttempts(ctx, q, r.ProblemID)
				mode := DecideDSAAttemptMode(existing, r.Mode)
				id := db.ID(fmt.Sprintf("datt-tui-%d-%s", now.UnixNano(), r.ProblemID))
				if err := runtime.InsertDSAAttempt(ctx, q, db.DSAAttempt{ID: id, SessionID: db.ID(s.sessionID), ProblemID: r.ProblemID, Mode: mode, AttemptedAt: now, Independent: r.Indep.Checked, Optimal: r.Optimal.Checked, HintUsed: r.Hint.Checked, SolveTimeSeconds: solve}); err != nil {
					return err
				}
			}
			return nil
		})
	case "SWE":
		return s.app.DB.WithTx(ctx, func(q db.DBTX) error {
			for _, r := range s.srows {
				if r.Radio.Selected < 0 {
					return fmt.Errorf("%s: outcome required", r.Task)
				}
				st := db.SWEExecutionStatus(r.Radio.Options[r.Radio.Selected])
				id := db.ID(fmt.Sprintf("swex-tui-%d-%s", now.UnixNano(), r.TaskID))
				if err := runtime.InsertSWEExecution(ctx, q, db.SWEExecution{ID: id, SessionID: db.ID(s.sessionID), TaskID: r.TaskID, Status: st, RecordedAt: now}); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return fmt.Errorf("unknown result kind")
}

// View renders the structured in-panel evidence form with sections,
// dividers, stable controls, and a real SAVE button.
func (s *ResultForm) View() string { return s.ViewFor(30) }

// ViewFor renders the form constrained to an inner width (panel-aware).
func (s *ResultForm) ViewFor(inner int) string {
	if inner < 18 {
		inner = 18
	}
	div := RowDivider(inner)
	var sb strings.Builder
	switch s.kind {
	case "LECTURE":
		sb.WriteString(SectionHeader("LECTURE COMPLETED?") + "\n")
		sb.WriteString(div + "\n")
		sb.WriteString(s.lecture.View() + "\n")
	case "PRACTICE":
		sb.WriteString(SectionHeader(fmt.Sprintf("EVIDENCE · %d TOPICS · 1H BLOCK", len(s.prows))) + "\n")
		for i, r := range s.prows {
			fbase := i * 4
			sb.WriteString(div + "\n")
			sb.WriteString(thPrimary.Render(fmt.Sprintf("%d. %s", i+1, TruncateToWidth(r.Topic, inner-4))) + "\n")
			sb.WriteString("  " + r.Time.View(s.focus == fbase, 5) + "  " + r.Att.View(s.focus == fbase+1, 5) + "\n")
			sb.WriteString("  " + r.Corr.View(s.focus == fbase+2, 5) + "  " + r.Avg.View(s.focus == fbase+3, 5) + "\n")
			if _, _, _, _, msg := ValidatePracticeRow(r.Time.Input.Value(), r.Att.Input.Value(), r.Corr.Input.Value(), r.Avg.Input.Value()); msg != "" {
				sb.WriteString("  " + thError.Render("✕ "+msg) + "\n")
			} else {
				sb.WriteString("  " + thSuccess.Render("✓ valid") + "\n")
			}
		}
	case "DSA":
		sb.WriteString(SectionHeader(fmt.Sprintf("EVIDENCE · %d PROBLEMS", len(s.drows))) + "\n")
		for i, r := range s.drows {
			fbase := i * 4
			sb.WriteString(div + "\n")
			sb.WriteString(thPrimary.Render(fmt.Sprintf("%d. %s", i+1, TruncateToWidth(r.Title, inner-8))) + "  " + thMeta.Render("["+string(r.Mode)+"]") + "\n")
			sb.WriteString("  " + r.Indep.View() + "\n")
			sb.WriteString("  " + r.Optimal.View() + "\n")
			sb.WriteString("  " + r.Hint.View() + "\n")
			sb.WriteString("  " + r.Solve.View(s.focus == fbase+3, 10) + "\n")
			_ = fbase
			if _, _, msg := ValidateSolveTime(r.Solve.Input.Value()); msg != "" {
				sb.WriteString("  " + thError.Render("✕ "+msg) + "\n")
			} else {
				sb.WriteString("  " + thSuccess.Render("✓ valid") + "\n")
			}
		}
	case "SWE":
		sb.WriteString(SectionHeader(fmt.Sprintf("EVIDENCE · %d TASKS", len(s.srows))) + "\n")
		for i, r := range s.srows {
			sb.WriteString(div + "\n")
			_ = i
			sb.WriteString(thPrimary.Render(TruncateToWidth(r.Area+"  ·  "+r.Task, inner-2)) + "\n")
			sb.WriteString(r.Radio.View() + "\n")
			if r.Radio.Selected < 0 {
				sb.WriteString("  " + thWarn.Render("○ outcome required") + "\n")
			} else {
				sb.WriteString("  " + thSuccess.Render("✓ "+r.Radio.Options[r.Radio.Selected]) + "\n")
			}
		}
	}
	sb.WriteString(div + "\n")
	sb.WriteString(s.save.View() + "\n")
	if s.save.Disabled {
		sb.WriteString(thMeta.Render("SAVE needs every item valid.") + "\n")
	}
	if s.err != "" {
		sb.WriteString(thError.Render(s.err) + "\n")
	}
	sb.WriteString(thHelp.Render("Tab move · Space toggle · Enter SAVE"))
	return sb.String()
}
