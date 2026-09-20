package tui

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------- DSA ----------

type DSAProblemDetail struct {
	Problem db.DSAProblem
	Passed  bool
	Last    *db.DSAAttempt
	Review  *db.DSARepeatState
}

type DSAScreen struct {
	app      *App
	patterns []DSAPattern
	patList  Navigator
	qList    Navigator
	focusR   bool
	detail   *DSAProblemDetail
	showing  string // "pattern" or "problem"
	err      string
}

func NewDSAScreen(a *App) *DSAScreen {
	return &DSAScreen{app: a, patList: NewNavigator(), qList: NewNavigator(), showing: "pattern"}
}

func (s *DSAScreen) Reload() {
	ctx := context.Background()
	probs, err := catalog.GetDSAProblems(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.patterns = GroupDSAPatterns(probs)
	items := make([]listItem, 0, len(s.patterns))
	for _, p := range s.patterns {
		items = append(items, listItem{id: p.Name, title: p.Name, desc: fmt.Sprintf("%d questions", len(p.Problems))})
	}
	s.patList.SetItems(wrapItems(items))
	s.showing = "pattern"
	s.reloadQuestions()
}

func (s *DSAScreen) selPattern() *DSAPattern {
	id := s.patList.SelectedID()
	if id == "" {
		return nil
	}
	for i, p := range s.patterns {
		if p.Name == id {
			return &s.patterns[i]
		}
	}
	return nil
}

func (s *DSAScreen) reloadQuestions() {
	ctx := context.Background()
	p := s.selPattern()
	if p == nil {
		s.qList.SetItems(nil)
		return
	}
	items := make([]listItem, 0, len(p.Problems))
	for _, q := range p.Problems {
		att, _ := runtime.GetDSAAttempts(ctx, s.app.DB, q.ID)
		mark := "○"
		if DSAPassUI(att) {
			mark = "✓"
		}
		items = append(items, listItem{id: string(q.ID), title: mark + "  " + q.Title, desc: q.Difficulty})
	}
	s.qList.SetItems(wrapItems(items))
}

func (s *DSAScreen) openProblem(pid string) {
	ctx := context.Background()
	p, err := catalog.GetDSAProblem(ctx, s.app.DB, db.ID(pid))
	if err != nil {
		s.err = err.Error()
		return
	}
	att, _ := runtime.GetDSAAttempts(ctx, s.app.DB, p.ID)
	var rev *db.DSARepeatState
	if rs, err := runtime.GetDSARepeatState(ctx, s.app.DB, p.ID); err == nil {
		rev = &rs
	}
	s.detail = &DSAProblemDetail{Problem: p, Passed: DSAPassUI(att), Last: LastAttempt(att), Review: rev}
	s.showing = "problem"
}

// onBack pops the problem panel first. True = consumed.
func (s *DSAScreen) onBack() bool {
	if s.showing == "problem" {
		s.showing = "pattern"
		return true
	}
	if s.focusR {
		s.focusR = false
		return true
	}
	return false
}

func (s *DSAScreen) patternProgress(p *DSAPattern) (int, int) {
	ctx := context.Background()
	done := 0
	for _, q := range p.Problems {
		if att, _ := runtime.GetDSAAttempts(ctx, s.app.DB, q.ID); DSAPassUI(att) {
			done++
		}
	}
	return done, len(p.Problems)
}

func (s *DSAScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "tab":
			s.focusR = !s.focusR
			return nil
		case "enter":
			if s.showing == "problem" {
				return nil
			}
			if s.focusR {
				if id := s.qList.SelectedID(); id != "" {
					s.openProblem(id)
				}
				return nil
			}
			s.focusR = true
			return nil
		case "j", "down", "k", "up":
			down := k.String() == "j" || k.String() == "down"
			if s.focusR {
				if down {
					s.qList.List.CursorDown()
				} else {
					s.qList.List.CursorUp()
				}
				return nil
			}
			prev := s.patList.SelectedID()
			if down {
				s.patList.List.CursorDown()
			} else {
				s.patList.List.CursorUp()
			}
			if s.patList.SelectedID() != prev {
				s.reloadQuestions()
			}
			return nil
		}
	}
	if s.focusR {
		var c tea.Cmd
		s.qList.List, c = s.qList.List.Update(msg)
		return c
	}
	prev := s.patList.SelectedID()
	var c tea.Cmd
	s.patList.List, c = s.patList.List.Update(msg)
	if s.patList.SelectedID() != prev {
		s.reloadQuestions()
	}
	return c
}

func (s *DSAScreen) View() string {
	w, h := s.app.contentViewport()
	lw, rw := splitW(w, 30, 22, 40)
	listH := h - 6
	if listH < 8 {
		listH = 8
	}
	s.patList.List.SetWidth(lw - 4)
	s.qList.List.SetWidth(rw - 4)
	s.patList.FitHeight(listH)
	s.qList.FitHeight(listH - 8)
	p := s.selPattern()
	var right string
	if s.showing == "problem" {
		right = Panel("PROBLEM", s.problemBody(), rw, true)
	} else if p != nil {
		done, total := s.patternProgress(p)
		r := 0.0
		if total > 0 {
			r = float64(done) / float64(total)
		}
		body := thScreenTitle.Render(p.Name) + "\n\n" +
			SectionHeader("PROGRESSION") + "\n" +
			ProgressBar{Ratio: r, Width: 26, ShowPct: true}.View() + "\n" +
			thMeta.Render(fmt.Sprintf("%d / %d", done, total)) + "\n\n" +
			SectionHeader("QUESTIONS") + "\n" + s.qList.List.View()
		right = Panel("PATTERN", body, rw, s.focusR)
	} else {
		right = Panel("PATTERN", EmptyState("Select a pattern."), rw, s.focusR)
	}
	return lipglossJoin(Panel("PATTERNS", s.patList.List.View(), lw, !s.focusR), right)
}

func (s *DSAScreen) problemBody() string {
	d := s.detail
	if d == nil {
		return EmptyState("Problem not found.")
	}
	var sb string
	sb += thScreenTitle.Render(d.Problem.Title) + "\n"
	sb += thMeta.Render(d.Problem.Difficulty+"  ·  "+d.Problem.Pattern) + "\n\n"
	sb += SectionHeader("PROGRESS") + "\n"
	if d.Last == nil {
		sb += thHelp.Render("Not reached yet") + "\n"
	} else if d.Passed {
		sb += thSuccess.Render("✓  Passed") + "\n"
	} else {
		sb += thWarn.Render("○  Attempted") + "\n"
	}
	if d.Last != nil {
		yn := map[bool]string{true: thSuccess.Render("✓"), false: thMeta.Render("No")}
		sb += "\n" + SectionHeader("LAST ATTEMPT") + "\n"
		sb += MetaRow("Independent", yn[d.Last.Independent], 12) + "\n"
		sb += MetaRow("Optimal", yn[d.Last.Optimal], 12) + "\n"
		sb += MetaRow("Hint used", yn[d.Last.HintUsed], 12) + "\n"
		if d.Last.SolveTimeSeconds != nil {
			sb += MetaRow("Solve time", FormatDuration(*d.Last.SolveTimeSeconds), 12) + "\n"
		} else {
			sb += MetaRow("Solve time", "—", 12) + "\n"
		}
	}
	sb += "\n" + SectionHeader("NEXT REVIEW") + "\n"
	if d.Review != nil {
		sb += thPrimary.Render(d.Review.NextReviewDate) + "\n"
	} else {
		sb += thMeta.Render("—") + "\n"
	}
	sb += "\n" + thHelp.Render("Work starts from Today only.")
	return sb
}

func (s *DSAScreen) Hints() string { return "Tab Focus   Enter Open" }
