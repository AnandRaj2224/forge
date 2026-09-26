package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	db "forge/internal/db"
	"forge/internal/engine"
)

// PracticeRow is one editable topic row in the GOV practice result form.
// Raw facts only: Time spent, Questions attempted, Questions correct.
// Incorrect and Avg time/question are DERIVED (read-only), never entered.
type PracticeRow struct {
	TopicID db.ID
	Topic   string
	Time    FormField
	Att     FormField
	Corr    FormField
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

// ResultForm is the domain-specific evidence form embedded in Today's
// right-bottom panel. SAVE stays disabled until every bundled item validates.
// Focus order, rendered position, and scroll position derive from one
// structured field model (fieldDescriptors), never hard-coded line numbers.
// SWE blocks use no result form: stopping the session records actual time.
type ResultForm struct {
	app           *App
	sessionID     string
	task          *db.TodayTask
	bundle        Bundle
	kind          string
	lecture       RadioGroup
	prows         []PracticeRow
	drows         []DSARow
	focus         int
	save          Button
	err           string
	satisfiedNote string // already-recorded items from prior sessions (read-only)
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
	s.prows, s.drows = nil, nil
	s.satisfiedNote = ""
	s.lecture = NewRadio([]string{"YES", "NO"})
	// Multi-session partials: already-satisfied target items are shown
	// read-only; only missing items require new evidence.
	missing := map[string]bool{}
	var completed []string
	if ev, err := engine.EvaluateTodayTask(ctx, s.app.DB, task.ID); err == nil {
		for _, m := range ev.Missing {
			missing[m] = true
		}
		completed = ev.Completed
	}
	onlyMissing := len(missing) > 0 || len(completed) > 0
	if len(completed) > 0 {
		s.satisfiedNote = fmt.Sprintf("%d already recorded · %d still needed", len(completed), len(missing))
	}
	switch s.kind {
	case "PRACTICE":
		for _, p := range s.bundle.Practice {
			if onlyMissing && !missing[string(p.TopicID)] {
				continue
			}
			s.prows = append(s.prows, PracticeRow{TopicID: p.TopicID, Topic: p.Topic,
				Time: NewField("Time spent", "e.g. 20m", 10), Att: NewField("Attempted", "e.g. 20", 10),
				Corr: NewField("Correct", "e.g. 16", 10)})
		}
	case "DSA":
		for _, d := range s.bundle.DSA {
			if onlyMissing && !missing[string(d.ProblemID)] {
				continue
			}
			s.drows = append(s.drows, DSARow{ProblemID: d.ProblemID, Title: d.Title, Mode: d.Mode,
				Indep: Checkbox{Label: "Independent"}, Optimal: Checkbox{Label: "Optimal"},
				Hint: Checkbox{Label: "Hint used"}, Solve: NewField("Solve time", "e.g. 5m", 10)})
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
	// SWE blocks use no result form; stopping the session is sufficient.
	return "SWE"
}

func (s *ResultForm) nFocus() int {
	switch s.kind {
	case "LECTURE":
		return 2
	case "PRACTICE":
		return len(s.prows)*3 + 1
	case "DSA":
		return len(s.drows)*4 + 1
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
			// Raw facts only: Time spent + Attempted + Correct. Incorrect
			// and Avg time/question are derived, never entered.
			if _, msg := engine.ValidateGovPracticeInput(r.Topic, r.Time.Input.Value(), r.Att.Input.Value(), r.Corr.Input.Value(), "", false); msg != "" {
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
	}
	s.save.Focused = s.focus == s.nFocus()-1
}

func (s *ResultForm) updateFocusVisual() {
	s.revalidate()
	for i := range s.prows {
		s.prows[i].Time.Input.Blur()
		s.prows[i].Att.Input.Blur()
		s.prows[i].Corr.Input.Blur()
	}
	for i := range s.drows {
		s.drows[i].Solve.Input.Blur()
		s.drows[i].Indep.Focused, s.drows[i].Optimal.Focused, s.drows[i].Hint.Focused = false, false, false
	}
	s.lecture.Focused = false
	f := s.focus
	switch s.kind {
	case "LECTURE":
		if f == 0 {
			s.lecture.Focused = true
		}
	case "PRACTICE":
		if f < len(s.prows)*3 {
			r, c := f/3, f%3
			switch c {
			case 0:
				s.prows[r].Time.Input.Focus()
			case 1:
				s.prows[r].Att.Input.Focus()
			case 2:
				s.prows[r].Corr.Input.Focus()
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
	}
	s.save.Focused = f == s.nFocus()-1
}

// textFocus reports whether the focused widget is a text input. While a
// text field owns focus, letter keys (including q/s/x) type instead of
// acting as global keys. Esc cancels the form via the app (never quits);
// the app footer switches to Esc Cancel / Enter Save while editing.
func (s *ResultForm) textFocus() bool {
	switch s.kind {
	case "PRACTICE":
		return s.focus < len(s.prows)*3
	case "DSA":
		if s.focus >= len(s.drows)*4 {
			return false
		}
		return s.focus%4 == 3
	default:
		return false
	}
}

func (s *ResultForm) moveFocus(d int) {
	n := s.nFocus()
	if n <= 0 {
		return
	}
	s.focus = (s.focus + d + n) % n
	s.updateFocusVisual()
}

// fieldDescriptors is the single structured field model: focus order,
// rendered position, and scroll position all derive from it. Each entry is
// one focusable widget; the SAVE button is always last.
func (s *ResultForm) fieldDescriptors() []string {
	var out []string
	switch s.kind {
	case "LECTURE":
		out = []string{"lecture", "save"}
	case "PRACTICE":
		for i := range s.prows {
			out = append(out,
				fmt.Sprintf("p%d-time", i), fmt.Sprintf("p%d-att", i),
				fmt.Sprintf("p%d-corr", i))
		}
		out = append(out, "save")
	case "DSA":
		for i := range s.drows {
			out = append(out,
				fmt.Sprintf("d%d-indep", i), fmt.Sprintf("d%d-opt", i),
				fmt.Sprintf("d%d-hint", i), fmt.Sprintf("d%d-solve", i))
		}
		out = append(out, "save")
	default:
		out = []string{"save"}
	}
	return out
}

// rowHeights returns per-item rendered heights (matching ViewFor) so
// focusLine derives from the same layout as rendering.
func (s *ResultForm) rowHeights() []int {
	switch s.kind {
	case "PRACTICE":
		out := make([]int, len(s.prows))
		for i := range out {
			out[i] = 5 // divider + title + 2 field lines + validation
		}
		return out
	case "DSA":
		out := make([]int, len(s.drows))
		for i := range out {
			out[i] = 7 // divider + title + 3 checks + solve + validation
		}
		return out
	}
	return nil
}

// focusLine returns the 0-based content line of the focused widget inside
// ViewFor output, derived from the structured field model + row heights.
func (s *ResultForm) focusLine() int {
	f := s.focus
	descs := s.fieldDescriptors()
	if f < 0 || f >= len(descs) {
		return 0
	}
	id := descs[f]
	// Header lines before the first item: section header = 1.
	header := 1
	if id == "lecture" {
		return header + 1 // divider + first option
	}
	if id == "save" {
		total := header
		for _, h := range s.rowHeights() {
			total += h
		}
		return total + 1 // divider + save box
	}
	switch s.kind {
	case "PRACTICE":
		// p{r}-{field}: base = header + sum of prior row heights.
		// Time/Attempted share the first field line, Correct + derived second.
		var r int
		fmt.Sscanf(id, "p%d-", &r)
		base := header
		for i := 0; i < r; i++ {
			base += s.rowHeights()[i]
		}
		if strings.HasSuffix(id, "-time") || strings.HasSuffix(id, "-att") {
			return base + 2
		}
		return base + 3
	case "DSA":
		var r int
		fmt.Sscanf(id, "d%d-", &r)
		base := header
		for i := 0; i < r; i++ {
			base += s.rowHeights()[i]
		}
		switch {
		case strings.HasSuffix(id, "-indep"):
			return base + 2
		case strings.HasSuffix(id, "-opt"):
			return base + 3
		case strings.HasSuffix(id, "-hint"):
			return base + 4
		default:
			return base + 5
		}
	}
	return 0
}

// ensureFocusVisible scrolls vp just enough to keep the focused field
// visible. The focused item/field is never hidden below the viewport.
func (s *ResultForm) ensureFocusVisible(vp *ScrollViewport, height int) {
	if vp == nil {
		return
	}
	total := len(strings.Split(s.ViewFor(30), "\n"))
	fl := s.focusLine()
	if fl < vp.Offset {
		vp.Offset = fl
	} else if fl > vp.Offset+height-1 {
		vp.Offset = fl - height + 1
	}
	vp.Clamp(total, height)
}

// HandleKey processes a key when the result panel owns focus.
// Returns a command and whether the key was consumed.
func (s *ResultForm) HandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	ks := msg.String()
	switch ks {
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
			case "left", "h":
				s.lecture.Move(-1)
			case "right", "l":
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
		if f < len(s.prows)*3 {
			r, c := f/3, f%3
			var fld *FormField
			switch c {
			case 0:
				fld = &s.prows[r].Time
			case 1:
				fld = &s.prows[r].Att
			case 2:
				fld = &s.prows[r].Corr
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
	now := time.Now()
	taskID := s.task.ID
	sessionID := db.ID(s.sessionID)
	switch s.kind {
	case "LECTURE":
		yes := s.lecture.Selected == 0
		if s.lecture.Selected < 0 {
			return fmt.Errorf("lecture outcome required")
		}
		_, err := engine.SaveGovLectureEvidence(ctx, s.app.DB, taskID, sessionID, yes, now)
		if err != nil {
			// NO without evidence is not an error path: engine returns the
			// re-evaluated TODO state with nil error. Real failures propagate.
			if errors.Is(err, engine.ErrAlreadySatisfied) {
				return fmt.Errorf("lecture already complete — result is read-only")
			}
			return err
		}
		return nil
	case "PRACTICE":
		var inputs []engine.GovPracticeInput
		for _, r := range s.prows {
			in, msg := engine.ValidateGovPracticeInput(r.Topic, r.Time.Input.Value(), r.Att.Input.Value(), r.Corr.Input.Value(), "", false)
			if msg != "" {
				return fmt.Errorf("%s: %s", r.Topic, msg)
			}
			in.TopicID = r.TopicID
			inputs = append(inputs, in)
		}
		_, err := engine.SaveGovPracticeEvidence(ctx, s.app.DB, taskID, sessionID, inputs, now)
		return err
	case "DSA":
		var inputs []engine.DSAAttemptInput
		for _, r := range s.drows {
			_, solve, msg := ValidateSolveTime(r.Solve.Input.Value())
			if msg != "" {
				return fmt.Errorf("%s: %s", r.Title, msg)
			}
			if solve == nil || *solve <= 0 {
				return fmt.Errorf("%s: solve time must be > 0", r.Title)
			}
			inputs = append(inputs, engine.DSAAttemptInput{
				ProblemID: r.ProblemID, Independent: r.Indep.Checked,
				Optimal: r.Optimal.Checked, HintUsed: r.Hint.Checked, SolveSecs: *solve,
			})
		}
		_, err := engine.SaveDSAEvidence(ctx, s.app.DB, taskID, sessionID, inputs, now)
		return err
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
	if s.satisfiedNote != "" {
		sb.WriteString(thMeta.Render(s.satisfiedNote) + "\n")
		sb.WriteString(div + "\n")
	}
	switch s.kind {
	case "LECTURE":
		sb.WriteString(SectionHeader("LECTURE COMPLETED?") + "\n")
		sb.WriteString(div + "\n")
		sb.WriteString(s.lecture.View() + "\n")
	case "PRACTICE":
		sb.WriteString(SectionHeader(fmt.Sprintf("EVIDENCE · %d TOPICS", len(s.prows))) + "\n")
		for i, r := range s.prows {
			fbase := i * 3
			sb.WriteString(div + "\n")
			sb.WriteString(thPrimary.Render(fmt.Sprintf("%d. %s", i+1, TruncateToWidth(r.Topic, inner-4))) + "\n")
			sb.WriteString("  " + r.Time.View(s.focus == fbase, 11) + "  " + r.Att.View(s.focus == fbase+1, 11) + "\n")
			sb.WriteString("  " + r.Corr.View(s.focus == fbase+2, 11) + "\n")
			if in, msg := engine.ValidateGovPracticeInput(r.Topic, r.Time.Input.Value(), r.Att.Input.Value(), r.Corr.Input.Value(), "", false); msg != "" {
				sb.WriteString("  " + thError.Render("✕ "+msg) + "\n")
			} else {
				incorrect := in.Attempted - in.Correct
				avg := "—"
				if in.AvgSpeed != nil {
					avg = fmt.Sprintf("%.1fs", *in.AvgSpeed)
				}
				sb.WriteString("  " + thSuccess.Render("✓ "+engine.FormatDurationLong(in.TimeSecs)+" · Incorrect "+fmt.Sprint(incorrect)+" · Avg time/question "+avg) + "\n")
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
			if _, solveSecs, msg := ValidateSolveTime(r.Solve.Input.Value()); msg != "" {
				sb.WriteString("  " + thError.Render("✕ "+msg) + "\n")
			} else {
				dur := ""
				if solveSecs != nil {
					dur = FormatDurationLong(*solveSecs)
				}
				sb.WriteString("  " + thSuccess.Render("✓ "+dur) + "  " + thMeta.Render("e.g. 30s, 5m, 1h 15m") + "\n")
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
	sb.WriteString(thHelp.Render("↑/↓·j/k move · Space toggle · Enter SAVE"))
	return sb.String()
}
