package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// GlobalKeyMap — the ONE Forge keyboard system for every screen.
//
// Vertical: j/k + ↓/↑ move. Horizontal: h/l + ←/→ switch pane.
// Enter selects/opens/confirms. q quits Forge in normal navigation.
// Esc backs out / cancels the current form or sub-state (never quits).
// s starts, x stops. Lowercase input works; footer labels are uppercase.
//
// TEXT / FORM MODE:
// q inserts the character q, Esc cancels/leaves the input/form,
// Enter submits/saves where appropriate, Ctrl+C quits.
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

// IsQuit reports q in normal navigation (Ctrl+C also quits).
// While a text field/form owns focus, q types instead of quitting
// (the app routes form keys before consulting IsQuit).
func IsQuit(ks string) bool { return normKey(ks) == "q" || ks == "ctrl+c" }

// IsBack reports Esc: back out of sub-states, cancel/leave inputs/forms.
// Esc never terminates the Forge process.
func IsBack(ks string) bool { return ks == "esc" }

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
// every screen in normal navigation mode. No per-screen footer language
// exists (form/editing modes use InputModeFooter instead).
func GlobalFooterKeys() []FooterKey {
	return []FooterKey{
		{"Q", "Quit"},
		{"ESC", "Back"},
		{"J/K", "Move"},
		{"↑/↓", "Move"},
		{"H/L", "Panel"},
		{"←/→", "Panel"},
		{"ENTER", "Select"},
		{"S", "Start"},
		{"X", "Stop"},
	}
}

// InputModeFooterKeys is the explicit text/form editing command list.
// q types as text here, so "Q Quit" is deliberately absent; Esc cancels
// instead of quitting.
func InputModeFooterKeys() []FooterKey {
	return []FooterKey{
		{"ESC", "Cancel"},
		{"ENTER", "Save"},
		{"↑/↓", "Move"},
		{"J/K", "Move"},
	}
}

// GlobalFooter renders the ONE shared normal-mode footer: a single centered
// row of uniform key badges with action labels. Actions only — no page
// metadata, no dates, no screen-specific hints. It never wraps vertically.
func GlobalFooter(w int) string {
	return renderFooterKeys(GlobalFooterKeys(), w)
}

// InputModeFooter renders the explicit text/form editing footer:
// Esc Cancel, Enter Save. No Q Quit while editing.
func InputModeFooter(w int) string {
	return renderFooterKeys(InputModeFooterKeys(), w)
}

// TodayDoneFooterKeys is the completed-task bar: no Start advertised, only
// relevant actions (Enter views the read-only result).
func TodayDoneFooterKeys() []FooterKey {
	return []FooterKey{
		{"Q", "Quit"},
		{"ESC", "Back"},
		{"J/K", "Move"},
		{"↑/↓", "Move"},
		{"H/L", "Panel"},
		{"←/→", "Panel"},
		{"ENTER", "View"},
		{"X", "Stop"},
	}
}

// TodayDoneFooter renders the completed-task bar (no Start advertised).
func TodayDoneFooter(w int) string {
	return renderFooterKeys(TodayDoneFooterKeys(), w)
}

func renderFooterKeys(keys []FooterKey, w int) string {
	var parts []string
	for _, u := range keys {
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
		line = KeyBadge("Q") + thFootDesc.Render(" Quit") + sep +
			KeyBadge("ENTER") + thFootDesc.Render(" Select") + sep +
			KeyBadge("ESC") + thFootDesc.Render(" Back")
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
