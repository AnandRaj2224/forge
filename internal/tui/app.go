package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TopTab int

const (
	TabToday TopTab = iota
	TabGOV
	TabDSA
	TabSWE
	TabReviews
)

func (t TopTab) Title() string {
	switch t {
	case TabToday:
		return "TODAY"
	case TabGOV:
		return "GOV"
	case TabDSA:
		return "DSA"
	case TabSWE:
		return "SWE"
	default:
		return "REVIEWS"
	}
}

type TickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return TickMsg(t) })
}

// Screen is implemented by each top-level workspace view.
type Screen interface {
	Reload()
	Update(msg tea.Msg) tea.Cmd
	View() string
	Hints() string
}

type App struct {
	DB     *db.DB
	tab    TopTab
	width  int
	height int
	now    time.Time
	err    string

	today   *TodayScreen
	gov     *GovScreen
	dsa     *DSAScreen
	swe     *SWEScreen
	reviews *ReviewsScreen

	activeInfo string
}

func NewApp(database *db.DB) *App {
	a := &App{DB: database, now: time.Now(), width: 120, height: 36}
	a.today = NewTodayScreen(a)
	a.gov = NewGovScreen(a)
	a.dsa = NewDSAScreen(a)
	a.swe = NewSWEScreen(a)
	a.reviews = NewReviewsScreen(a)
	return a
}

func (a *App) ctx() context.Context { return context.Background() }

func (a *App) goTab(t TopTab) {
	a.tab = t
	a.reloadCurrent()
}

func (a *App) reloadCurrent() {
	a.err = ""
	a.refreshActiveIndicator()
	switch a.tab {
	case TabToday:
		a.today.Reload()
	case TabGOV:
		a.gov.Reload()
	case TabDSA:
		a.dsa.Reload()
	case TabSWE:
		a.swe.Reload()
	case TabReviews:
		a.reviews.Reload()
	}
}

func (a *App) refreshActiveIndicator() {
	sess, err := runtime.GetActiveSession(a.ctx(), a.DB)
	if err != nil {
		a.activeInfo = ""
		return
	}
	el := time.Since(sess.StartedAt)
	if el < 0 {
		el = 0
	}
	a.activeInfo = fmt.Sprintf("● %s %s %s", sess.Domain, sess.Kind, FormatHMS(el))
}

func (a *App) Init() tea.Cmd { return tea.Batch(tickCmd(), func() tea.Msg { return ReloadMsg{} }) }

type ReloadMsg struct{}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		return a, nil
	case TickMsg:
		a.now = time.Time(m)
		a.refreshActiveIndicator()
		return a, tickCmd()
	case ReloadMsg:
		a.reloadCurrent()
		return a, nil
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "1":
			a.goTab(TabToday)
			return a, nil
		case "2":
			a.goTab(TabGOV)
			return a, nil
		case "3":
			a.goTab(TabDSA)
			return a, nil
		case "4":
			a.goTab(TabSWE)
			return a, nil
		case "5":
			a.goTab(TabReviews)
			return a, nil
		case "r":
			a.reloadCurrent()
			return a, nil
		case "esc":
			// Sub-state back is owned by each workspace; at root it is a no-op.
			switch a.tab {
			case TabToday:
				if a.today.onBack() {
					return a, nil
				}
			case TabGOV:
				if a.gov.onBack() {
					return a, nil
				}
			case TabDSA:
				if a.dsa.onBack() {
					return a, nil
				}
			case TabSWE:
				if a.swe.onBack() {
					return a, nil
				}
			}
			return a, nil
		case "x":
			// Global stop: end the session and surface its form on Today.
			if _, err := runtime.GetActiveSession(a.ctx(), a.DB); err == nil {
				a.goTab(TabToday)
				a.today.stopActive()
			}
			return a, nil
		}
	}
	var cmd tea.Cmd
	switch a.tab {
	case TabToday:
		cmd = a.today.Update(msg)
	case TabGOV:
		cmd = a.gov.Update(msg)
	case TabDSA:
		cmd = a.dsa.Update(msg)
	case TabSWE:
		cmd = a.swe.Update(msg)
	case TabReviews:
		cmd = a.reviews.Update(msg)
	}
	return a, cmd
}

// contentViewport returns the central area between header and footer.
func (a *App) contentViewport() (w, h int) {
	w, h = a.width, a.height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 30
	}
	h -= 5 // header(2) + footer(2) + breathing room
	if h < 8 {
		h = 8
	}
	return w, h
}

func (a *App) View() string {
	w, h := a.width, a.height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 30
	}
	header := a.headerView(w)
	var body string
	switch a.tab {
	case TabToday:
		body = a.today.View()
	case TabGOV:
		body = a.gov.View()
	case TabDSA:
		body = a.dsa.View()
	case TabSWE:
		body = a.swe.View()
	case TabReviews:
		body = a.reviews.View()
	}
	footer := a.footerView(w)
	contentH := h - lipgloss.Height(header) - lipgloss.Height(footer) - 1
	if contentH < 5 {
		contentH = 5
	}
	lines := strings.Split(body, "\n")
	if len(lines) > contentH {
		lines = lines[:contentH]
	}
	for len(lines) < contentH {
		lines = append(lines, "")
	}
	body = strings.Join(lines, "\n")
	out := header + "\n" + body + "\n" + footer
	if a.err != "" {
		out += "\n" + thError.Render("Error: "+a.err)
	}
	return out
}

// headerView is the application navigation bar. Numbers stay keyboard-only.
// At narrow widths it compresses: tighter gaps, short active pill, clock
// without seconds, and finally drops the clock — never wraps.
func (a *App) headerView(w int) string {
	brand := thAppTitle.Render("FORGE")
	names := []string{"TODAY", "GOV", "DSA", "SWE", "REVIEWS"}
	var nav []string
	for i, n := range names {
		if TopTab(i) == a.tab {
			nav = append(nav, thNavActive.Render(n))
		} else {
			nav = append(nav, thNavInactive.Render(n))
		}
	}
	clock := thClock.Render(a.now.Format("02 Jan  ·  15:04:05"))
	active := ""
	if a.activeInfo != "" {
		active = thRunning.Render(a.activeInfo)
	}
	bar := joinHeader(brand, nav, active, clock, w, "    ")
	if lipgloss.Width(bar) > w {
		bar = joinHeader(brand, nav, active, clock, w, "  ")
	}
	if lipgloss.Width(bar) > w {
		shortActive := ""
		if a.activeInfo != "" {
			shortActive = thRunning.Render("● ACTIVE")
		}
		bar = joinHeader(brand, nav, shortActive, thClock.Render(a.now.Format("15:04")), w, "  ")
	}
	if lipgloss.Width(bar) > w {
		bar = joinHeader(brand, nav, "", "", w, " ")
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(cDimLine).Width(w).Render(bar)
}

func joinHeader(brand string, nav []string, active, clock string, w int, gap string) string {
	left := brand + gap + strings.Join(nav, gap)
	right := clock
	if active != "" {
		if right != "" {
			right = active + gap + right
		} else {
			right = active
		}
	}
	sp := w - lipgloss.Width(left) - lipgloss.Width(right)
	if sp < 1 {
		sp = 1
	}
	return left + strings.Repeat(" ", sp) + right
}

// footerView is the grouped status/action bar.
func (a *App) footerView(w int) string {
	groups := [][]string{
		{"q", "Quit"},
		{"j/k", "Navigate"},
		{"Enter", "Select"},
		{"Esc", "Back"},
		{"r", "Refresh"},
	}
	if s, ok := a.screenHints(); ok && s != "" {
		groups = append(groups, []string{"·", s})
	}
	var parts []string
	for _, g := range groups {
		if g[0] == "·" {
			parts = append(parts, thFootDesc.Render(g[1]))
			continue
		}
		parts = append(parts, thFootKey.Render(g[0])+thFootDesc.Render("  "+g[1]))
	}
	line := strings.Join(parts, "     ")
	if lipgloss.Width(line) > w-2 {
		// Narrow terminals: keep primary groups only.
		line = thFootKey.Render("q") + thFootDesc.Render("  Quit") + "     " +
			thFootKey.Render("Enter") + thFootDesc.Render("  Select") + "     " +
			thFootKey.Render("Esc") + thFootDesc.Render("  Back")
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, false, false).BorderForeground(cDimLine).Width(w).Render(line)
}

func (a *App) screenHints() (string, bool) {
	switch a.tab {
	case TabToday:
		return a.today.Hints(), true
	case TabGOV:
		return a.gov.Hints(), true
	case TabDSA:
		return a.dsa.Hints(), true
	case TabSWE:
		return a.swe.Hints(), true
	case TabReviews:
		return a.reviews.Hints(), true
	}
	return "", false
}
