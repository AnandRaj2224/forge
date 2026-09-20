package tui

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"github.com/charmbracelet/bubbles/list"
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
	patList  list.Model
	qList    list.Model
	focusR   bool
	detail   *DSAProblemDetail
	showing  string // "pattern" or "problem"
	err      string
}

func NewDSAScreen(a *App) *DSAScreen {
	mk := func() list.Model {
		l := list.New(nil, list.NewDefaultDelegate(), 28, 14)
		l.SetShowHelp(false)
		l.SetShowTitle(false)
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(false)
		return l
	}
	return &DSAScreen{app: a, patList: mk(), qList: mk(), showing: "pattern"}
}

func (s *DSAScreen) Reload() {
	ctx := context.Background()
	probs, err := catalog.GetDSAProblems(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.patterns = GroupDSAPatterns(probs)
	items := make([]list.Item, 0, len(s.patterns))
	for _, p := range s.patterns {
		items = append(items, listItem{id: p.Name, title: p.Name, desc: fmt.Sprintf("%d questions", len(p.Problems))})
	}
	s.patList.SetItems(items)
	s.showing = "pattern"
	s.reloadQuestions()
}

func (s *DSAScreen) selPattern() *DSAPattern {
	it, ok := s.patList.SelectedItem().(listItem)
	if !ok {
		return nil
	}
	for i, p := range s.patterns {
		if p.Name == it.id {
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
	items := make([]list.Item, 0, len(p.Problems))
	for _, q := range p.Problems {
		att, _ := runtime.GetDSAAttempts(ctx, s.app.DB, q.ID)
		mark := "○"
		if DSAPassUI(att) {
			mark = "✓"
		}
		items = append(items, listItem{id: string(q.ID), title: mark + "  " + q.Title, desc: q.Difficulty})
	}
	s.qList.SetItems(items)
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
				if it, ok := s.qList.SelectedItem().(listItem); ok && it.id != "" {
					s.openProblem(it.id)
				}
				return nil
			}
			s.focusR = true
			return nil
		case "j", "down", "k", "up":
			down := k.String() == "j" || k.String() == "down"
			if s.focusR {
				if down {
					s.qList.CursorDown()
				} else {
					s.qList.CursorUp()
				}
				return nil
			}
			prev := ""
			if it, ok := s.patList.SelectedItem().(listItem); ok {
				prev = it.id
			}
			if down {
				s.patList.CursorDown()
			} else {
				s.patList.CursorUp()
			}
			if it, ok := s.patList.SelectedItem().(listItem); ok && it.id != prev {
				s.reloadQuestions()
			}
			return nil
		}
	}
	if s.focusR {
		var c tea.Cmd
		s.qList, c = s.qList.Update(msg)
		return c
	}
	prev := ""
	if it, ok := s.patList.SelectedItem().(listItem); ok {
		prev = it.id
	}
	var c tea.Cmd
	s.patList, c = s.patList.Update(msg)
	if it, ok := s.patList.SelectedItem().(listItem); ok && it.id != prev {
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
	s.patList.SetWidth(lw - 4)
	s.qList.SetWidth(rw - 4)
	s.patList.SetHeight(fitListHeight(len(s.patList.Items()), listH))
	s.qList.SetHeight(fitListHeight(len(s.qList.Items()), listH-8))
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
			SectionHeader("QUESTIONS") + "\n" + s.qList.View()
		right = Panel("PATTERN", body, rw, s.focusR)
	} else {
		right = Panel("PATTERN", thMeta.Render("Select a pattern."), rw, s.focusR)
	}
	return lipglossJoin(Panel("PATTERNS", s.patList.View(), lw, !s.focusR), right)
}

func (s *DSAScreen) problemBody() string {
	d := s.detail
	if d == nil {
		return thMeta.Render("Problem not found.")
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
