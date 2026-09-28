package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
)

// ---------- DSA: persistent two-panel patterns workspace ----------

type DSAProblemDetail struct {
	Problem db.DSAProblem
	Passed  bool
	Last    *db.DSAAttempt
	Review  *db.DSARepeatState
}

type DSAScreen struct {
	app          *App
	patterns     []DSAPattern
	patList      Navigator
	qList        Navigator
	focusR       bool
	detail       *DSAProblemDetail
	detailScroll ScrollViewport // problem pane scrolls independently when overflowing
	showing      string         // "pattern" or "problem"
	err          string
	// passCache caches per-problem pass marks loaded in Reload/reloadQuestions
	// so View rendering performs no database access.
	passCache map[string]bool
}

func NewDSAScreen(a *App) *DSAScreen {
	return &DSAScreen{app: a, patList: NewNavigator(), qList: NewNavigator(), showing: "pattern", passCache: map[string]bool{}}
}

func (s *DSAScreen) Reload() {
	ctx := context.Background()
	probs, err := catalog.GetDSAProblems(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.patterns = GroupDSAPatterns(probs)
	items := make([]RowItem, 0, len(s.patterns))
	for _, p := range s.patterns {
		items = append(items, RowItem{ID: p.Name, Title: p.Name, Meta: fmt.Sprintf("%d questions", len(p.Problems)), Mark: "•"})
	}
	s.patList.SetItems(items)
	s.showing = "pattern"
	// Batch pass marks for all problems (one query, pass computed in memory).
	s.refreshPassCache(ctx, probs)
	s.reloadQuestions()
}

// refreshPassCache loads pass marks for the given problems in ONE batch query.
// Pass is the authoritative engine.AnyPass rule computed in memory.
func (s *DSAScreen) refreshPassCache(ctx context.Context, probs []db.DSAProblem) {
	if s.passCache == nil {
		s.passCache = map[string]bool{}
	}
	if len(probs) == 0 {
		return
	}
	ids := make([]db.ID, 0, len(probs))
	for _, p := range probs {
		ids = append(ids, p.ID)
	}
	atts, err := runtime.GetDSAAttemptsForProblems(ctx, s.app.DB, ids)
	if err != nil {
		s.err = "data error: " + err.Error()
		return
	}
	byProblem := make(map[db.ID][]db.DSAAttempt, len(ids))
	for _, a := range atts {
		byProblem[a.ProblemID] = append(byProblem[a.ProblemID], a)
	}
	for _, p := range probs {
		s.passCache[string(p.ID)] = DSAPassUI(byProblem[p.ID])
	}
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
	p := s.selPattern()
	if p == nil {
		s.qList.SetItems(nil)
		return
	}
	// Render marks from the batch pass cache (populated in Reload). If a
	// problem is missing from the cache (e.g. pattern changed without a
	// full reload), fall back to the cached value defaulting to unpassed
	// without performing I/O; the next Reload refreshes the cache.
	items := make([]RowItem, 0, len(p.Problems))
	for _, q := range p.Problems {
		mark := "○"
		if s.passCache != nil && s.passCache[string(q.ID)] {
			mark = "✓"
		}
		items = append(items, RowItem{ID: string(q.ID), Title: q.Title, Meta: q.Difficulty, Mark: mark})
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
	att, err := runtime.GetDSAAttempts(ctx, s.app.DB, p.ID)
	if err != nil {
		s.err = "data error: " + err.Error()
	}
	var rev *db.DSARepeatState
	if rs, err := runtime.GetDSARepeatState(ctx, s.app.DB, p.ID); err == nil {
		rev = &rs
	}
	s.detail = &DSAProblemDetail{Problem: p, Passed: DSAPassUI(att), Last: LastAttempt(att), Review: rev}
	s.showing = "problem"
	s.detailScroll.Reset()
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
	// Render-pure: progress derives from the batch pass cache loaded in
	// Reload. Never queries SQLite and never mutates s.err during render.
	if p == nil {
		return 0, 0
	}
	done := 0
	for _, q := range p.Problems {
		if s.passCache != nil && s.passCache[string(q.ID)] {
			done++
		}
	}
	return done, len(p.Problems)
}

func (s *DSAScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
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
		case "j", "down", "k", "up", "h", "left", "l", "right":
			down := k.String() == "j" || k.String() == "down"
			up := k.String() == "k" || k.String() == "up"
			left := k.String() == "h" || k.String() == "left"
			right := k.String() == "l" || k.String() == "right"
			if s.showing == "problem" {
				// Scroll the problem pane independently.
				if down || up {
					_, h := s.app.contentViewport()
					bodyH := h
					if bodyH < 8 {
						bodyH = 8
					}
					innerH := bodyH - 4
					if innerH < 4 {
						innerH = 4
					}
					total := len(strings.Split(s.problemBody(s.problemInnerW()), "\n"))
					if down {
						s.detailScroll.ScrollDown(total, innerH)
					} else {
						s.detailScroll.ScrollUp(total, innerH)
					}
				}
				return nil
			}
			if s.focusR {
				if down {
					s.qList.CursorDown()
				} else if up {
					s.qList.CursorUp()
				} else if left {
					s.focusR = false
				}
				return nil
			}
			prev := s.patList.SelectedID()
			if down {
				s.patList.CursorDown()
			} else if up {
				s.patList.CursorUp()
			} else if right {
				s.focusR = true
				return nil
			} else {
				return nil
			}
			if s.patList.SelectedID() != prev {
				s.reloadQuestions()
			}
			return nil
		}
	}
	return nil
}

func (s *DSAScreen) View() string {
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
	s.patList.SetWidth(lw - 4)
	s.qList.SetWidth(rw - 4)
	s.patList.FitHeight(innerH - 1)
	s.qList.FitHeight(innerH - 7)
	s.patList.SetFocused(!s.focusR)
	s.qList.SetFocused(s.focusR)
	p := s.selPattern()
	var right string
	if s.showing == "problem" {
		right = PanelFixed("PROBLEM", ViewportView(s.problemBody(rw-6), &s.detailScroll, innerH), rw, bodyH, true)
	} else if p != nil {
		done, total := s.patternProgress(p)
		r := 0.0
		if total > 0 {
			r = float64(done) / float64(total)
		}
		barW := rw - 12
		if barW < 14 {
			barW = 14
		}
		if barW > 28 {
			barW = 28
		}
		inner := rw - 6
		body := lipgloss.NewStyle().Bold(true).Foreground(cDSA).Render(strings.ToUpper(TruncateToWidth(p.Name, inner))) + "\n" +
			RowDivider(inner) + "\n" +
			SectionHeader("PROGRESSION") + "\n" +
			ProgressBar{Ratio: r, Width: barW, ShowPct: true, Accent: cDSA}.View() + "\n" +
			thMeta.Render(fmt.Sprintf("%d / %d", done, total)) + "\n" +
			RowDivider(inner) + "\n" +
			DomainSection("DSA", "QUESTIONS") + "\n" + s.qList.View()
		right = PanelFixed("PATTERN", body, rw, bodyH, s.focusR)
	} else {
		right = PanelFixed("PATTERN", EmptyState("Select a pattern."), rw, bodyH, s.focusR)
	}
	return lipglossJoin(PanelFixed("PATTERNS", SectionHeader("PATTERNS")+"\n"+s.patList.View(), lw, bodyH, !s.focusR), right)
}

func (s *DSAScreen) problemBody(inner int) string {
	d := s.detail
	if d == nil {
		return EmptyState("Problem not found.")
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
	sb.WriteString(thScreenTitle.Render(strings.ToUpper(TruncateToWidth(d.Problem.Title, inner))) + "\n")
	sb.WriteString(DifficultyBadge(d.Problem.Difficulty) + "  " + thMeta.Render("·  "+d.Problem.Pattern) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("PROGRESS") + "\n")
	if d.Last == nil {
		sb.WriteString(thHelp.Render("Not reached yet") + "\n")
	} else if d.Passed {
		sb.WriteString(thSuccess.Render("✓  Passed") + "\n")
	} else {
		sb.WriteString(thWarn.Render("○  Attempted") + "\n")
	}
	if d.Last != nil {
		yn := map[bool]string{true: thSuccess.Render("✓"), false: thMeta.Render("No")}
		sb.WriteString(RowDivider(inner) + "\n")
		sb.WriteString(SectionHeader("LAST ATTEMPT") + "\n")
		sb.WriteString(MetaRow("Independent", yn[d.Last.Independent], 12) + "\n")
		sb.WriteString(MetaRow("Optimal", yn[d.Last.Optimal], 12) + "\n")
		sb.WriteString(MetaRow("Hint used", yn[d.Last.HintUsed], 12) + "\n")
		if d.Last.SolveTimeSeconds != nil {
			sb.WriteString(MetaRow("Solve time", FormatDurationLong(*d.Last.SolveTimeSeconds), 12) + "\n")
		} else {
			sb.WriteString(MetaRow("Solve time", "—", 12) + "\n")
		}
	}
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("NEXT REVIEW") + "\n")
	if d.Review != nil {
		sb.WriteString(thPrimary.Render(d.Review.NextReviewDate) + "\n")
	} else {
		sb.WriteString(thMeta.Render("—") + "\n")
	}
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(thHelp.Render("Work starts from Today only."))
	return sb.String()
}

// problemInnerW returns the actual problem inner width (same geometry as View).
func (s *DSAScreen) problemInnerW() int {
	w, _ := s.app.contentViewport()
	_, rw := TwoPaneLayout(w, 32, 24, 40)
	inner := rw - 6
	if inner < 24 {
		inner = 24
	}
	return inner
}
