package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// GlobalKeyMap — the ONE Forge keyboard system for every screen.
//
// Vertical: j/k + ↓/↑ move (selectable) or scroll (detail).
// Horizontal: h/l + ←/→ switch pane.
// Enter selects/opens/confirms. Ctrl+C quits Forge everywhere.
// q backs out in normal navigation; Esc backs out / cancels (never quits).
// s starts, x stops, n creates mocks. Lowercase input works.
//
// TEXT INPUT (a text field owns the keyboard):
// q inserts the character q, Esc cancels/leaves the input/form,
// Enter submits/saves where appropriate, Ctrl+C quits.
// The rule is based only on whether a text-input component currently owns
// the keyboard — never screen-specific q rules.
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

// IsQuit reports Ctrl+C only — the single quit command everywhere.
// q is Back in normal navigation (or a typed character in text inputs),
// never Quit. Esc never quits.
func IsQuit(ks string) bool { return ks == "ctrl+c" }

// IsBack reports q / Esc in normal navigation: back out of sub-states,
// cancel/leave inputs/forms. Esc never terminates Forge. While a text
// input owns the keyboard, q types instead of backing (the app routes
// text keys before consulting IsBack for q).
func IsBack(ks string) bool { return ks == "esc" || normKey(ks) == "q" }

// IsBackEsc reports Esc only (valid even inside text inputs).
func IsBackEsc(ks string) bool { return ks == "esc" }

// IsStart reports s.
func IsStart(ks string) bool { return normKey(ks) == "s" }

// IsStop reports x.
func IsStop(ks string) bool { return normKey(ks) == "x" }

// FooterKey is one footer badge + action label.
type FooterKey struct {
	Key  string // display label, e.g. Q, CTRL+C, ENTER
	Desc string // action, e.g. BACK, QUIT
}

// globalFooterModel is the ONE source of truth for the Forge footer.
// Static app-wide keyboard legend. Identical on every screen, never
// changes by screen or focus. Display labels are canonical UPPERCASE;
// keyboard handling stays case-tolerant (q/j/k/h/l/n/s/x work lowercase).
// The legend lists ALL established application-level commands, even when
// a given command is not valid on the current screen.
func globalFooterModel() []FooterKey {
	return []FooterKey{
		{"Q", "BACK"},
		{"ESC", "BACK/CANCEL"},
		{"CTRL+C", "QUIT"},
		{"J/K", "MOVE"},
		{"↑/↓", "MOVE"},
		{"H/L", "PANEL"},
		{"ENTER", "VIEW/SAVE"},
		{"SPACE", "TOGGLE"},
		{"N", "NEW"},
		{"S", "START"},
		{"X", "STOP"},
	}
}

// renderGlobalFooter renders the static legend with fixed geometry:
// single centered row, same badges/spacing/typography everywhere.
func renderGlobalFooter(w int) string {
	return renderFooterKeys(globalFooterModel(), w)
}

// GlobalFooter renders the ONE static app-wide footer.
func GlobalFooter(w int) string {
	return renderGlobalFooter(w)
}

func renderFooterKeys(keys []FooterKey, w int) string {
	var parts []string
	for _, u := range keys {
		parts = append(parts, KeyBadge(u.Key)+thFootDesc.Render(" "+u.Desc))
	}
	sep := "  "
	line := strings.Join(parts, sep)
	// Single row, never wrapped. Never drop commands to fit: the legend
	// is complete by definition. At narrow widths the row is clipped
	// (existing layout strategy) but the command definition stays whole
	// and context-free.
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

// isDifficulty reports whether s is a LeetCode difficulty label.
func isDifficulty(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "EASY", "MEDIUM", "HARD":
		return true
	}
	return false
}

// DifficultyBadge renders EASY/MEDIUM/HARD with palette-only colors:
// cyan / peach / pink. Only the badge carries color; selection
// backgrounds are applied by the owning row.
func DifficultyBadge(diff string) string {
	switch strings.ToUpper(strings.TrimSpace(diff)) {
	case "EASY":
		return lipgloss.NewStyle().Bold(true).Foreground(cSuccess).Render(strings.ToUpper(strings.TrimSpace(diff)))
	case "MEDIUM":
		return lipgloss.NewStyle().Bold(true).Foreground(cActiveBlue).Render(strings.ToUpper(strings.TrimSpace(diff)))
	case "HARD":
		return lipgloss.NewStyle().Bold(true).Foreground(cError).Render(strings.ToUpper(strings.TrimSpace(diff)))
	default:
		return thMeta.Render(diff)
	}
}

// DifficultyBadgeOnBG renders the difficulty badge over an explicit row
// background so selected rows keep their dark tint plus focus edge while
// the difficulty stays semantically colored.
func DifficultyBadgeOnBG(diff string, bg interface{}) string {
	_ = bg
	return DifficultyBadge(diff)
}
