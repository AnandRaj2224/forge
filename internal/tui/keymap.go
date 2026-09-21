package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// GlobalKeyMap — the ONE Forge keyboard system for every screen.
//
// Vertical: j/k + ↓/↑ move. Horizontal: h/l + ←/→ switch pane.
// Enter selects/opens/confirms. Esc quits the application. q goes back.
// s starts, x stops. Lowercase input works; footer labels are uppercase.
func normKey(ks string) string {
	if len([]rune(ks)) == 1 {
		return strings.ToLower(ks)
	}
	return ks
}

// IsMoveDown reports j / ↓.
func IsMoveDown(ks string) bool { ks = normKey(ks); return ks == "j" || ks == "down" }

// IsMoveUp reports k / ↑.
func IsMoveUp(ks string) bool { ks = normKey(ks); return ks == "k" || ks == "up" }

// IsMoveLeft reports h / ←.
func IsMoveLeft(ks string) bool { ks = normKey(ks); return ks == "h" || ks == "left" }

// IsMoveRight reports l / →.
func IsMoveRight(ks string) bool { ks = normKey(ks); return ks == "l" || ks == "right" }

// IsConfirm reports Enter.
func IsConfirm(ks string) bool { return ks == "enter" }

// IsQuit reports Esc (Ctrl+C also quits).
func IsQuit(ks string) bool { return ks == "esc" || ks == "ctrl+c" }

// IsBack reports q.
func IsBack(ks string) bool { return normKey(ks) == "q" }

// IsStart reports s.
func IsStart(ks string) bool { return normKey(ks) == "s" }

// IsStop reports x.
func IsStop(ks string) bool { return normKey(ks) == "x" }

// FooterKey is one global footer badge + action label.
type FooterKey struct {
	Key  string // uppercase display label, e.g. J/K
	Desc string // action, e.g. Move
}

// GlobalFooterKeys is the single Forge command list shown identically on
// every screen. No per-screen footer language exists.
func GlobalFooterKeys() []FooterKey {
	return []FooterKey{
		{"ESC", "Quit"},
		{"Q", "Back"},
		{"J/K", "Move"},
		{"↑/↓", "Move"},
		{"H/L", "Panel"},
		{"←/→", "Panel"},
		{"ENTER", "Select"},
		{"S", "Start"},
		{"X", "Stop"},
	}
}

// GlobalFooter renders the ONE shared footer: a single centered row of
// uniform key badges with action labels. Actions only — no page metadata,
// no dates, no screen-specific hints. It never wraps vertically.
func GlobalFooter(w int) string {
	var parts []string
	for _, u := range GlobalFooterKeys() {
		parts = append(parts, KeyBadge(u.Key)+thFootDesc.Render(" "+u.Desc))
	}
	sep := "  "
	line := strings.Join(parts, sep)
	// Shrink gracefully from the right but NEVER wrap: keep one row.
	for lipgloss.Width(line) > w-2 && len(parts) > 4 {
		parts = parts[:len(parts)-1]
		line = strings.Join(parts, sep)
	}
	if lipgloss.Width(line) > w-2 {
		line = KeyBadge("ESC") + thFootDesc.Render(" Quit") + sep +
			KeyBadge("ENTER") + thFootDesc.Render(" Select") + sep +
			KeyBadge("Q") + thFootDesc.Render(" Back")
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
