package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Forge layout primitives. Explicit Rect/Bounds calculations keep the shell
// composition viewport-driven instead of content-driven.

// Rect is an explicit panel bound inside the terminal.
type Rect struct {
	X, Y int
	W, H int
}

// Contains reports whether (x,y) is inside the rect.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Inner shrinks the rect by horizontal/vertical chrome (borders+padding).
func (r Rect) Inner(dx, dy int) Rect {
	return Rect{X: r.X + dx, Y: r.Y + dy, W: max(0, r.W-2*dx), H: max(0, r.H-2*dy)}
}

// Shell geometry: multi-line header holds the 6-line Forge wordmark plus a
// bottom divider (7 lines) plus 1 join gap; footer is 1 command row plus a
// top divider (2 lines) plus 1 join gap.
const (
	ShellHeaderH = 8
	ShellFooterH = 3
)

// ShellLayout computes header/main/footer rects from the terminal size.
type ShellLayout struct {
	TermW, TermH int
	Header       Rect
	Main         Rect
	Footer       Rect
}

// NewShellLayout builds explicit bounds: MAIN = term - header - footer.
func NewShellLayout(termW, termH int) ShellLayout {
	if termW < 40 {
		termW = 40
	}
	if termH < 20 {
		termH = 20
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

// ThreePaneLayout splits total width into three panel widths by percentages.
// Accounts for three borders (+2 each) and two 1-cell gaps (total chrome 8).
func ThreePaneLayout(totalW int, pctA, pctB int) (a, b, c int) {
	if totalW < 40 {
		totalW = 40
	}
	chrome := 8
	avail := totalW - chrome
	if avail < 30 {
		avail = 30
	}
	a = avail * pctA / 100
	b = avail * pctB / 100
	c = avail - a - b
	if a < 10 {
		a = 10
	}
	if b < 10 {
		b = 10
	}
	if c < 10 {
		c = 10
	}
	return a, b, c
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

// BentoGrid lays out pre-rendered cards into responsive rows with a gap.
// Cards in the same row are joined horizontally; rows stacked vertically.
func BentoGrid(cards []string, cols int, gap string) string {
	if len(cards) == 0 {
		return ""
	}
	if cols < 1 {
		cols = 1
	}
	var rows []string
	for i := 0; i < len(cards); i += cols {
		end := i + cols
		if end > len(cards) {
			end = len(cards)
		}
		parts := make([]string, 0, (end-i)*2-1)
		for j := i; j < end; j++ {
			if j > i {
				parts = append(parts, gap)
			}
			parts = append(parts, cards[j])
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// HorizontalStack joins blocks left-to-right at the top edge with 1-cell gaps.
func HorizontalStack(blocks ...string) string {
	if len(blocks) == 0 {
		return ""
	}
	if len(blocks) == 1 {
		return blocks[0]
	}
	withGaps := make([]string, 0, len(blocks)*2-1)
	for i, b := range blocks {
		if i > 0 {
			withGaps = append(withGaps, " ")
		}
		withGaps = append(withGaps, b)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, withGaps...)
}

// VerticalStack joins blocks top-to-bottom, left-aligned.
func VerticalStack(blocks ...string) string {
	if len(blocks) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
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

// RowLayout is the explicit horizontal row geometry: number + mark + title +
// meta composed into a fixed-width row. It backs SelectableRow/TodayTaskRow
// so every navigable list shares one geometry contract instead of ad-hoc
// string padding.
type RowLayout struct {
	Num   string
	Mark  string
	Title string
	Meta  string
	Width int
}

// InnerWidth returns the usable content width inside padding.
func (r RowLayout) InnerWidth() int {
	w := r.Width - 2
	if w < 4 {
		w = 4
	}
	return w
}

// Prefix returns the plain-text row prefix (number + mark) for width math.
func (r RowLayout) Prefix() string {
	mark := r.Mark
	if mark == "" {
		mark = " "
	}
	if r.Num != "" {
		return r.Num + "  " + mark + "  "
	}
	return mark + "  "
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
// ---- Global app shell header: two real layout regions ----
//
// ShellHeader is ONE horizontal header composed of two side-by-side
// regions with deterministic widths and explicit vertical alignment:
//
//	LEFT (LogoRegion)     RIGHT (RightRegion: full remaining width)
//	FORGE ASCII logo      rows 0-1: QuoteBlock (full quote, right/top)
//	top-aligned           row 2:    NavBlock (centered horizontally)
//	                      rows 3-5: empty
//
// The quote and navigation share the RIGHT region vertically, so they never
// compete for horizontal width: the quote receives essentially the full
// remaining width after the logo. Widths are deterministic; vertical
// alignment uses lipgloss.Place (not manual spaces). Header content height
// is always HeaderLogoH (6); the logo determines the natural height.
const HeaderLogoH = 6

// LogoRegion is the LEFT header region: full Forge ASCII wordmark,
// top-aligned, with internal padding so it never touches the border.
type LogoRegion struct {
	Lines []string // plain ASCII (gradient applied in View)
	Width int      // outer width including left pad + content
}

// View renders exactly Width x HeaderLogoH, logo at the top-left.
func (r LogoRegion) View() string {
	w := r.Width
	if w < 10 {
		w = 10
	}
	grad := make([]string, 0, len(r.Lines))
	for _, ln := range r.Lines {
		grad = append(grad, gradientLogoLine(ln))
	}
	logoBlock := lipgloss.JoinVertical(lipgloss.Left, grad...)
	style := lipgloss.NewStyle().Width(w).PaddingLeft(2).Align(lipgloss.Left)
	block := style.Render(logoBlock)
	return lipgloss.Place(w, HeaderLogoH, lipgloss.Left, lipgloss.Top, block)
}

// fitHeaderLine keeps the full line when it fits and only clips (with
// ellipsis) when the terminal is genuinely too narrow.
func fitHeaderLine(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return TruncateToWidth(s, w)
}

// QuoteBlock renders exactly Width x 2: two quote lines, right-aligned,
// top-aligned. Full text is preserved whenever it fits.
type QuoteBlock struct {
	Line1 string
	Line2 string
	Width int // content width (excludes the outer right pad)
}

// View renders the two-line quote forced to Width x 2.
func (b QuoteBlock) View() string {
	w := b.Width
	if w < 8 {
		w = 8
	}
	l1 := fitHeaderLine(b.Line1, w)
	l2 := fitHeaderLine(b.Line2, w)
	return lipgloss.Place(w, 2, lipgloss.Right, lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Right, l1, l2))
}

// NavBlock renders the one-line navigation centered horizontally.
type NavBlock struct {
	Line  string // single-line horizontal nav (already styled pills)
	Width int    // content width (excludes the outer right pad)
}

// View renders the nav line forced to Width x 1, centered.
func (b NavBlock) View() string {
	w := b.Width
	if w < 10 {
		w = 10
	}
	line := fitHeaderLine(b.Line, w)
	return lipgloss.Place(w, 1, lipgloss.Center, lipgloss.Center, line)
}

// RightRegion is the RIGHT header region: the full remaining terminal
// width after the logo. Rows 0-1 carry the quote, row 2 carries the nav
// (around the vertical middle of the 6-line header), rows 3-5 stay empty.
type RightRegion struct {
	Quote1  string // full first quote line (already styled)
	Quote2  string // attribution line (already styled)
	NavLine string // single-line horizontal nav (already styled pills)
	Width   int    // outer width including the 2-cell right pad
}

// View renders exactly Width x HeaderLogoH: quote top-right, nav centered
// on the middle row.
func (r RightRegion) View() string {
	w := r.Width
	if w < 12 {
		w = 12
	}
	contentW := w - 2 // reserve the outer right pad
	if contentW < 8 {
		contentW = 8
	}
	quote := QuoteBlock{Line1: r.Quote1, Line2: r.Quote2, Width: contentW}.View()
	nav := NavBlock{Line: r.NavLine, Width: contentW}.View()
	blanks := lipgloss.Place(contentW, HeaderLogoH-3, lipgloss.Left, lipgloss.Top, "")
	body := lipgloss.JoinVertical(lipgloss.Left, quote, nav, blanks)
	return lipgloss.NewStyle().Width(w).PaddingRight(2).Align(lipgloss.Left).Render(body)
}

// ShellHeader composes the logo and right regions in a real horizontal
// layout with deterministic widths. Content is always HeaderLogoH lines;
// the quote fits into the top rows and the nav into the middle row, so
// the header never grows.
type ShellHeader struct {
	Logo  LogoRegion
	Right RightRegion
	Width int
}

// View joins the two regions side-by-side and wraps with a bottom
// divider. No extra height is introduced.
func (h ShellHeader) View() string {
	w := h.Width
	if w < 40 {
		w = 40
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, h.Logo.View(), h.Right.View())
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		lines[i] = FitLine(ln, w)
	}
	body = strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(cBorder).
		Width(w).
		Render(body)
}

// ComputeHeaderWidths returns deterministic outer widths for the two header
// regions: logoOuter + rightOuter == totalW. The right region receives
// essentially the full remaining width so the quote is never starved by
// the navigation.
func ComputeHeaderWidths(totalW, logoContentW int) (logoOuter, rightOuter int) {
	if totalW < 40 {
		totalW = 40
	}
	logoOuter = 2 + logoContentW // left pad + logo; no inter-region gap needed
	rightOuter = totalW - logoOuter
	if rightOuter < 12 {
		rightOuter = 12
	}
	return logoOuter, rightOuter
}
