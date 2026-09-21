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
		// While the in-place evidence form owns focus, it owns every key
		// except Quit and Back. In particular digits (1-5), r and x must
		// reach text inputs instead of triggering global navigation.
		inForm := a.tab == TabToday && a.today.zone == tzRight && a.today.right == "result"
		if inForm && m.String() != "q" && m.String() != "ctrl+c" && m.String() != "esc" {
			break
		}
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

// contentViewport returns the central area between header and footer via
// the explicit shell layout: MAIN = term - header - footer.
func (a *App) contentViewport() (w, h int) {
	w, h = a.width, a.height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 30
	}
	sh := NewShellLayout(w, h)
	return sh.Main.W, sh.Main.H
}

func (a *App) View() string {
	w, h := a.width, a.height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 30
	}
	sh := NewShellLayout(w, h)
	header := a.headerView(sh.Header.W)
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
	footer := a.footerView(sh.Footer.W)
	contentH := sh.Main.H
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

// Forge ASCII wordmark — largest element in the header, anchored LEFT.
var forgeLogoLines = []string{
	"███████╗ ██████╗ ██████╗  ██████╗ ███████╗",
	"██╔════╝██╔═══██╗██╔══██╗██╔════╝ ██╔════╝",
	"█████╗  ██║   ██║██████╔╝██║  ███╗█████╗",
	"██╔══╝  ██║   ██║██╔══██╗██║   ██║██╔══╝",
	"██║     ╚██████╔╝██║  ██║╚██████╔╝███████╗",
	"╚═╝      ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚══════╝",
}

// headerView is the Forge shell header: LEFT ASCII wordmark + MID→RIGHT
// top-level navigation. NO date/time, NO clock, NO numbers in labels.
// The wordmark is intentionally larger than the navigation text. The header
// is spatially composed (not tab/space-faked single line): the 6-line logo
// sits left, navigation is vertically centered to its right.
func (a *App) headerView(w int) string {
	names := []string{"TODAY", "GOV", "DSA", "SWE", "REVIEWS"}
	var nav []string
	for i, n := range names {
		if TopTab(i) == a.tab {
			nav = append(nav, thNavActive.Render(n))
		} else {
			nav = append(nav, thNavInactive.Render(n))
		}
	}
	navLine := strings.Join(nav, "  ")
	// Style logo lines with the Forge accent.
	logo := make([]string, len(forgeLogoLines))
	for i, ln := range forgeLogoLines {
		logo[i] = thLogo.Render(ln)
	}
	logoW := lipgloss.Width(logo[0])
	gap := "    "
	avail := w - logoW - lipgloss.Width(gap)
	if avail < 20 {
		avail = 20
	}
	// Navigation stays on ONE line, truncated to fit, vertically centered
	// against the 6-line logo (row 2 of 0..5).
	navLine = TruncateToWidth(navLine, avail)
	rows := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		line := logo[i] + gap
		if i == 2 {
			line += navLine
		}
		// Force exact width so the divider below spans the viewport.
		line = FitLine(line, w)
		rows = append(rows, line)
	}
	block := strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(cBorder).
		Width(w).
		Render(block)
}

func joinHeader(brand string, nav []string, active, clock string, w int, gap string) string {
	// Kept for compat: LEFT brand+nav, RIGHT active/clock. New header no
	// longer uses clock, but external callers/tests may reference it.
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

// footerView is EXACTLY ONE horizontal command bar with compact key badges.
// It never becomes vertical: children are horizontal units, the bar is
// truncated to fit and occupies a controlled fixed height.
func (a *App) footerView(w int) string {
	units := []struct{ key, desc string }{
		{"q", "Quit"},
		{"j/k", "Navigate"},
		{"Enter", "Select"},
		{"Esc", "Back"},
		{"r", "Refresh"},
		{"s", "Start"},
		{"x", "Stop"},
	}
	var parts []string
	for _, u := range units {
		parts = append(parts, footChip(u.key)+thFootDesc.Render(" "+u.desc))
	}
	if s, ok := a.screenHints(); ok && s != "" {
		parts = append(parts, thFootDesc.Render("· "+TruncateToWidth(s, 36)))
	}
	sep := "  "
	line := strings.Join(parts, sep)
	// Shrink gracefully from the right but NEVER wrap: keep one row.
	for lipgloss.Width(line) > w-2 && len(parts) > 4 {
		parts = parts[:len(parts)-1]
		line = strings.Join(parts, sep)
	}
	if lipgloss.Width(line) > w-2 {
		line = footChip("q") + thFootDesc.Render(" Quit") + sep +
			footChip("Enter") + thFootDesc.Render(" Select") + sep +
			footChip("Esc") + thFootDesc.Render(" Back")
	}
	line = FitLine(TruncateToWidth(line, w-2), w-2)
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(cBorder).
		Width(w).
		MaxHeight(2).
		Render(line)
}

func (a *App) footerExtra() string {
	// Screen-specific hints already flow via screenHints(); no duplication.
	return ""
}

// footChip renders a compact single-line key badge. It must stay exactly
// one line tall so the footer remains ONE horizontal command bar.
func footChip(key string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(cPrimary).Padding(0, 1).Render(key)
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
