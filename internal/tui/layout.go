package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Forge layout primitives. Explicit Rect/Bounds calculations keep the shell
// composition viewport-driven instead of content-driven.

// Rect is an explicit panel bound inside the terminal.
type Rect struct {
	X, Y int
	W, H int
}

// Shell geometry: fixed header/footer heights, main viewport derived.
const (
	ShellHeaderH = 2
	ShellFooterH = 2
)

// ShellLayout computes header/main/footer rects from the terminal size.
type ShellLayout struct {
	TermW, TermH int
	Header      Rect
	Main        Rect
	Footer      Rect
}

// NewShellLayout builds explicit bounds: MAIN = term - header - footer.
func NewShellLayout(termW, termH int) ShellLayout {
	if termW < 40 {
		termW = 40
	}
	if termH < 16 {
		termH = 16
	}
	header := Rect{X: 0, Y: 0, W: termW, H: ShellHeaderH}
	footer := Rect{X: 0, Y: termH - ShellFooterH, W: termW, H: ShellFooterH}
	main := Rect{X: 0, Y: header.H, W: termW, H: termH - header.H - footer.H}
	if main.H < 5 {
		main.H = 5
	}
	return ShellLayout{TermW: termW, TermH: termH, Header: header, Main: main, Footer: footer}
}

// TwoPaneLayout splits a main width into left/right panel widths.
// Accounts for two boxy borders (+2 each) and a 1-cell gap.
func TwoPaneLayout(totalW, leftPct, leftMin, rightMin int) (leftW, rightW int) {
	leftW, rightW = splitW(totalW, leftPct, leftMin, rightMin)
	return leftW, rightW
}

// BentoGridCardW computes a uniform card width for a responsive grid.
func BentoGridCardW(totalW, cols, gap int) int {
	if cols < 1 {
		cols = 1
	}
	w := (totalW - (cols-1)*gap) / cols
	if w < 20 {
		w = 20
	}
	return w
}

// BentoCols picks responsive column counts: 3 wide, 2 medium, 1 narrow.
func BentoCols(totalW int) int {
	if totalW >= 130 {
		return 3
	}
	if totalW >= 80 {
		return 2
	}
	return 1
}

// TruncateToWidth cuts a string to a visible cell width.
func TruncateToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// PadToWidth pads a string with spaces to exactly w visible cells.
func PadToWidth(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-sw)
}

// FitLine forces a single line to exactly w cells (truncate or pad).
func FitLine(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return PadToWidth(s, w)
}

// JoinPanels places two pre-rendered panels side by side with a gap.
// Both panels should already carry explicit Widths; this only joins.
func JoinPanels(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left+" ", right)
}

// ClipToHeight forces a rendered block to exactly h lines: truncates overflow
// or pads with blanks. Keeps bento panels inside the main viewport.
func ClipToHeight(s string, h int) string {
	if h < 1 {
		h = 1
	}
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// PanelFixed renders a bento card forced to an exact total height (including
// borders). Content is clipped or padded so side-by-side panels stay aligned
// and never push the footer out of the viewport.
func PanelFixed(title, content string, width, height int, focused bool) string {
	if height < 5 {
		height = 5
	}
	// Chrome: top border + title + divider + bottom border = 4 lines.
	innerH := height - 4
	if innerH < 1 {
		innerH = 1
	}
	content = ClipToHeight(content, innerH)
	return Panel(title, content, width, focused)
}

// StackPanels stacks pre-rendered panels vertically.
func StackPanels(panels ...string) string {
	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}

// Clamp confines v to [lo, hi].
func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
