package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
)

type TopTab int

const (
	TabToday TopTab = iota
	TabGOV
	TabDSA
	TabSWE
	TabMocks
	TabHistory
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
	case TabMocks:
		return "MOCKS"
	case TabHistory:
		return "HISTORY"
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
	mocks   *MocksScreen
	history *HistoryScreen
	reviews *ReviewsScreen

	activeInfo string
}

func NewApp(database *db.DB) *App {
	a := &App{DB: database, now: time.Now(), width: 120, height: 36}
	a.today = NewTodayScreen(a)
	a.gov = NewGovScreen(a)
	a.dsa = NewDSAScreen(a)
	a.swe = NewSWEScreen(a)
	a.mocks = NewMocksScreen(a)
	a.history = NewHistoryScreen(a)
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
	case TabMocks:
		a.mocks.Reload()
	case TabHistory:
		a.history.Reload()
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
	case tea.KeyPressMsg:
		// While the in-place evidence form owns focus, it owns every key
		// except Quit and Back. In particular digits, r and x must
		// reach text inputs instead of triggering global navigation.
		inForm := a.tab == TabToday && a.today.zone == tzRight && a.today.right == "result"
		inMockForm := a.tab == TabMocks && a.mocks.ownsKeys()
		if (inForm || inMockForm) && m.String() != "q" && m.String() != "ctrl+c" && m.String() != "esc" {
			break
		}
		switch m.String() {
		case "q", "ctrl+c":
			return a, func() tea.Msg { return tea.Quit() }
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
			a.goTab(TabMocks)
			return a, nil
		case "6":
			a.goTab(TabHistory)
			return a, nil
		case "7":
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
			case TabMocks:
				if a.mocks.onBack() {
					return a, nil
				}
			case TabHistory:
				if a.history.onBack() {
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
	case TabMocks:
		cmd = a.mocks.Update(msg)
	case TabHistory:
		cmd = a.history.Update(msg)
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

func (a *App) View() tea.View {
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
	case TabMocks:
		body = a.mocks.View()
	case TabHistory:
		body = a.history.View()
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
	return tea.NewView(out)
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

// headerView is the Forge shell header: LEFT 6-line gradient ASCII
// wordmark + RIGHT region carrying the full two-line quote on rows 0-1 and
// the one-line navigation centered on the middle row. Two real layout
// regions (LogoRegion/RightRegion) joined via ShellHeader. NO date/time,
// NO clock. Header height stays at the 6-line logo (no growth).
func (a *App) headerView(w int) string {
	names := []string{"TODAY", "GOV", "DSA", "SWE", "MOCKS", "HISTORY", "REVIEWS"}
	// Compact pills so all 7 items fit; active keeps an accent fill.
	navActive := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B0B13")).Background(cPrimary).Padding(0, 1)
	navInactive := lipgloss.NewStyle().Foreground(cFgSecondary).Padding(0, 1)
	pills := make([]string, 0, len(names))
	for i, n := range names {
		if TopTab(i) == a.tab {
			pills = append(pills, navActive.Render(n))
		} else {
			pills = append(pills, navInactive.Render(n))
		}
	}
	navLine := strings.Join(pills, " ")
	logoContentW := 0
	for _, ln := range forgeLogoLines {
		if lw := lipgloss.Width(ln); lw > logoContentW {
			logoContentW = lw
		}
	}
	logoOuter, rightOuter := ComputeHeaderWidths(w, logoContentW)
	// The full quote text goes to the bounded right-side block, which wraps
	// it naturally (styling owned by QuoteRegion). Never truncated here.
	sh := ShellHeader{
		Logo: LogoRegion{Lines: forgeLogoLines, Width: logoOuter},
		Right: RightRegion{
			QuoteText: "“Everyone sees what you appear to be, few experience what you really are.”",
			QuoteAttr: "— Niccolò Machiavelli, The Prince",
			NavLine:   navLine,
			Width:     rightOuter,
		},
		Width: w,
	}
	return sh.View()
}

// gradientLogoLine renders one wordmark line with a smooth horizontal
// gradient: violet → magenta → purple → blue → cyan → green. Controlled and
// deliberate; the logo is the one place where expressive color is allowed.
func gradientLogoLine(line string) string {
	runes := []rune(line)
	n := len(runes)
	if n == 0 {
		return ""
	}
	var sb strings.Builder
	for i, r := range runes {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(gradientAt(t)).Render(string(r)))
	}
	return sb.String()
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
// Actions only: no date, no metadata, no titles. The bar is one controlled
// horizontal composition, visually centered in the footer area. Each key/action
// pair is a real horizontal unit; the bar never wraps vertically.
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
	line = TruncateToWidth(line, w-2)
	// Center the whole bar horizontally within the footer area.
	lineW := lipgloss.Width(line)
	avail := w - 2
	pad := (avail - lineW) / 2
	if pad < 0 {
		pad = 0
	}
	line = strings.Repeat(" ", pad) + line
	line = FitLine(line, w-2)
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
// Neutral badge keeps the footer quiet; only descriptions recede further.
func footChip(key string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary).Background(lipgloss.Color("#23232E")).Padding(0, 1).Render(key)
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
	case TabMocks:
		return a.mocks.Hints(), true
	case TabHistory:
		return a.history.Hints(), true
	case TabReviews:
		return a.reviews.Hints(), true
	}
	return "", false
}
