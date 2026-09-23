package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// Forge shared components. Boxy bento cards, thin elegant borders,
// consistent padding, real controls. No string-based layout.

// Panel renders a bento card: thin boxy border, consistent inner padding,
// title + divider + body. Focused cards get the accent border + marker.
func Panel(title, content string, width int, focused bool) string {
	if width < 20 {
		width = 20
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(cBorder).
		Padding(0, 1).
		Width(width)
	if focused {
		style = style.BorderForeground(cPrimary)
	}
	head := thPanelTitle.Render(title)
	if focused {
		head = thPanelTitle.Render(title) + thFocusDot.Render("  ●")
	}
	sep := Separator(maxInt(width-4, 4))
	return style.Render(head + "\n" + sep + "\n" + content)
}

// Card is the canonical Forge card alias (see BentoPanel/Panel).
func Card(title, content string, width int, focused bool) string {
	return Panel(title, content, width, focused)
}

// PanelPlain is a bento card without focus affordance (static info groups).
func PanelPlain(title, content string, width int) string {
	if width < 20 {
		width = 20
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(cBorder).
		Padding(0, 1).
		Width(width)
	return style.Render(thPanelTitle.Render(title) + "\n" + Separator(maxInt(width-4, 4)) + "\n" + content)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// splitW divides total into left/right panel content widths for two panels
// with +2 borders each and a 1-cell gap: l+r+5 == total (or best effort at
// tiny widths). Minimums rebalance instead of overflowing.
func splitW(total, leftPct, leftMin, rightMin int) (l, r int) {
	l = total * leftPct / 100
	r = total - l - 5
	if r < rightMin {
		r = rightMin
		l = total - 5 - r
	}
	if l < leftMin {
		l = leftMin
		r = total - 5 - l
	}
	if r < 10 {
		r = 10
	}
	if l < 10 {
		l = 10
	}
	return l, r
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ProgressBar is the NORMAL STATIC progress component for syllabus coverage,
// lecture progress, practice progression, pattern/area progress.
// Clean quiet bar: ████████░░░░░░ 60% with percentage/count. The fill stays
// neutral by default and may carry one restrained domain accent (GOV violet,
// DSA cyan, SWE rose) via Accent. Never a gradient.
type ProgressBar struct {
	Ratio   float64 // 0..1
	Width   int
	ShowPct bool
	Accent  color.Color // optional domain accent for the fill; nil = neutral
}

func (p ProgressBar) View() string {
	w := p.Width
	if w <= 0 {
		w = 22
	}
	r := p.Ratio
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	filled := int(r*float64(w) + 0.5)
	// v2 lipgloss renders empty strings with reset codes; skip empty runs
	// so clamped bars stay byte-clean for tests and alignment.
	fill := color.Color(cFgPrimary)
	if p.Accent != nil {
		fill = p.Accent
	}
	bar := ""
	if filled > 0 {
		bar += lipgloss.NewStyle().Foreground(fill).Render(strings.Repeat("█", filled))
	}
	if w-filled > 0 {
		bar += lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("░", w-filled))
	}
	if p.ShowPct {
		return fmt.Sprintf("%s %s", bar, thMeta.Render(fmt.Sprintf("%3.0f%%", r*100)))
	}
	return bar
}

// SessionProgressBar is the LIVE SESSION gradient treatment only.
// Violet → purple → blue → cyan → green, animated by Frame phase shift.
// Never use for static coverage bars.
type SessionProgressBar struct {
	Ratio   float64 // 0..1
	Width   int
	Frame   int
	ShowPct bool
}

func (p SessionProgressBar) View() string {
	w := p.Width
	if w <= 0 {
		w = 22
	}
	r := p.Ratio
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	filled := int(r*float64(w) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > w {
		filled = w
	}
	var sb strings.Builder
	for i := 0; i < w; i++ {
		if i < filled {
			// Position along the bar plus animation phase.
			t := 0.0
			if w > 1 {
				t = float64(i) / float64(w-1)
			}
			// Animate: shift hue phase slowly with frame.
			phase := float64(p.Frame%24) / 24.0 * 0.15
			t += phase
			if t > 1 {
				t -= 1
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(gradientAt(t)).Render("█"))
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(cBorder).Render("░"))
		}
	}
	if p.ShowPct {
		return fmt.Sprintf("%s %s", sb.String(), thPrimary.Render(fmt.Sprintf("%3.0f%%", r*100)))
	}
	return sb.String()
}

var gradStops = []string{"#A78BFA", "#E879F9", "#8B5CF6", "#60A5FA", "#22D3EE", "#34D399"}

func gradientAt(t float64) color.Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	segs := len(gradStops) - 1
	pos := t * float64(segs)
	i := int(pos)
	if i >= segs {
		return lipgloss.Color(gradStops[segs])
	}
	f := pos - float64(i)
	a := hexRGB(gradStops[i])
	b := hexRGB(gradStops[i+1])
	r := int(float64(a[0]) + f*(float64(b[0])-float64(a[0])) + 0.5)
	g := int(float64(a[1]) + f*(float64(b[1])-float64(a[1])) + 0.5)
	bl := int(float64(a[2]) + f*(float64(b[2])-float64(a[2])) + 0.5)
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", r, g, bl))
}

func hexRGB(h string) [3]int {
	h = strings.TrimPrefix(h, "#")
	if len(h) != 6 {
		return [3]int{128, 128, 128}
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseInt(h[i*2:i*2+2], 16, 32)
		if err != nil {
			return [3]int{128, 128, 128}
		}
		out[i] = int(v)
	}
	return out
}

// Button is a real boxy interactive control with focus + disabled states.
type Button struct {
	Label    string
	Focused  bool
	Disabled bool
}

func (b Button) View() string {
	border := lipgloss.NormalBorder()
	if b.Disabled {
		inner := thBtnDisabled.Render("  " + b.Label + "  ")
		return lipgloss.NewStyle().Border(border).BorderForeground(cBorder).Padding(0, 1).Render(inner)
	}
	inner := thBtnIdle.Render("  " + b.Label + "  ")
	if b.Focused {
		inner = thBtnFocus.Render("▶ " + b.Label + "  ")
		return lipgloss.NewStyle().Border(border).BorderForeground(cPrimary).Padding(0, 1).Render(inner)
	}
	return lipgloss.NewStyle().Border(border).BorderForeground(cBorder).Padding(0, 1).Render(inner)
}

// CompactView is the single-line button for short viewports: same real
// control (focus/disabled states) in one line to preserve actions.
func (b Button) CompactView() string {
	if b.Disabled {
		return thBtnDisabled.Render("▪ " + b.Label)
	}
	if b.Focused {
		return thBtnFocus.Render("▶ " + b.Label)
	}
	return thBtnIdle.Render("▪ " + b.Label)
}

// RadioGroup is a real mutually-exclusive selection control with stable
// glyphs (◉/○) that never depend on arrow rendering.
// Focus marker ▶ stays visible alongside the selection glyph:
// ▶ ◉ YES /   ○ NO  and  ▶ ○ NO /   ◉ YES patterns.
type RadioGroup struct {
	Options  []string
	Selected int // -1 = none
	Cursor   int
	Focused  bool
}

func NewRadio(options []string) RadioGroup { return RadioGroup{Options: options, Selected: -1} }

func (r *RadioGroup) Move(d int) {
	if len(r.Options) == 0 {
		return
	}
	r.Cursor = (r.Cursor + d + len(r.Options)) % len(r.Options)
}

func (r *RadioGroup) Choose() {
	if len(r.Options) == 0 {
		return
	}
	r.Selected = r.Cursor
}

func (r RadioGroup) View() string {
	var sb strings.Builder
	for i, o := range r.Options {
		glyph := "○"
		if i == r.Selected {
			glyph = "◉"
		}
		if r.Focused && i == r.Cursor {
			// Both focus arrow and selection glyph always visible.
			sb.WriteString(thFocusMark.Render("▶ ") + thPrimary.Render(glyph+"  "+o) + "\n")
		} else {
			sb.WriteString("  " + thPrimary.Render(glyph+"  "+o) + "\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Checkbox is a real toggle control with a strong checked state.
// Focus stays on the arrow ▶ (never replaced); the checked state uses a
// solid block ■ in a solid color vs the hollow □, clearly visible at a
// glance: ▶ ■ Independent /   □ Optimal.
type Checkbox struct {
	Label   string
	Checked bool
	Focused bool
}

func (c Checkbox) View() string {
	var glyph string
	var line string
	if c.Checked {
		glyph = thSuccess.Render("■")
		line = glyph + "  " + thPrimary.Render(c.Label)
	} else {
		glyph = thMeta.Render("□")
		line = glyph + "  " + thPrimary.Render(c.Label)
	}
	if c.Focused {
		return thFocusMark.Render("▶ ") + line
	}
	return "  " + line
}

// FormField wraps a real editable text input with an aligned label.
type FormField struct {
	Label string
	Input textinput.Model
	Error string
}

// NewField builds an aligned form field with an explicit input width.
// The width is required: a zero-width input collapses to its content and
// breaks form alignment.
func NewField(label, placeholder string, width int) FormField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 32
	ti.Prompt = "│ "
	if width < 6 {
		width = 6
	}
	ti.SetWidth(width)
	return FormField{Label: label, Input: ti}
}

// View renders label + input; label column width keeps forms aligned.
func (f FormField) View(focused bool, labelWidth int) string {
	if labelWidth <= 0 {
		labelWidth = 10
	}
	lbl := padRight(f.Label, labelWidth)
	if focused {
		return thFocusMark.Render(lbl+" ") + f.Input.View()
	}
	return thMeta.Render(lbl+" ") + f.Input.View()
}

// KeyBadge renders one compact [key] chip for the footer command bar.
// Single line, neutral badge: bright text on a dark neutral surface.
func KeyBadge(key string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary).Background(lipgloss.Color("#23232E")).Padding(0, 1).Render(key)
}

// lipglossJoin places two panels side by side with a gap.
func lipglossJoin(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left+" ", right)
}

// listItem is a real selectable list row (compat alias for RowItem construction).
type listItem struct {
	id    string
	title string
	desc  string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.desc }
func (i listItem) FilterValue() string { return i.title }

// Navigator is the shared Forge list: thin dividers, polished selection,
// viewport scrolling, single focus convention. Built on RowList — no Bubbles
// chrome, no pink defaults. All master navigators build on it.
type Navigator struct {
	rows RowList
}

// NewNavigator returns a chrome-free Forge selectable list.
func NewNavigator() Navigator {
	r := NewRowList()
	r.Focused = true
	return Navigator{rows: r}
}

// SetItems replaces rows (Forge RowItems).
func (n *Navigator) SetItems(items []RowItem) { n.rows.SetItems(items) }

// SetListItems is a compat bridge from legacy listItem rows.
func (n *Navigator) SetListItems(in []listItem) {
	out := make([]RowItem, 0, len(in))
	for _, it := range in {
		out = append(out, RowItem{ID: it.id, Title: it.title, Meta: it.desc})
	}
	n.rows.SetItems(out)
}

// FitHeight sizes the viewport to content so panels never float in padding.
func (n *Navigator) FitHeight(maxH int) { n.rows.FitHeight(maxH) }

// SetWidth sets divider span.
func (n *Navigator) SetWidth(w int) { n.rows.SetWidth(w) }

// SetHeight sets viewport rows.
func (n *Navigator) SetHeight(h int) { n.rows.SetHeight(h) }

// Select moves cursor.
func (n *Navigator) Select(i int) { n.rows.Select(i) }

// CursorDown moves down.
func (n *Navigator) CursorDown() { n.rows.CursorDown() }

// CursorUp moves up.
func (n *Navigator) CursorUp() { n.rows.CursorUp() }

// SelectedID returns the selected row id.
func (n Navigator) SelectedID() string { return n.rows.SelectedID() }

// View renders rows with dividers.
func (n Navigator) View() string { return n.rows.View() }

// SetFocused toggles focus treatment.
func (n *Navigator) SetFocused(f bool) { n.rows.Focused = f }

// Cursor returns cursor index.
func (n Navigator) Cursor() int { return n.rows.Cursor }

// Len returns row count.
func (n Navigator) Len() int { return len(n.rows.Items) }

// wrapItems converts legacy typed rows to Forge RowItems.
func wrapItems(in []listItem) []RowItem {
	out := make([]RowItem, 0, len(in))
	for _, it := range in {
		out = append(out, RowItem{ID: it.id, Title: it.title, Meta: it.desc})
	}
	return out
}

// ExamCard is a real selectable bento card for the GOV exam dashboard.
// Selected marks the derived current exam (first incomplete in exam
// order); it only changes the title suffix, never the card geometry.
type ExamCard struct {
	Name     string
	Done     int
	Total    int
	Focused  bool
	Selected bool
	Width    int
}

func (c ExamCard) Percent() float64 {
	if c.Total == 0 {
		return 0
	}
	return float64(c.Done) / float64(c.Total)
}

// View renders the card with structured sections. Fixed 8 content lines so
// grid rows stay uniform; see examCardHeight.
func (c ExamCard) View() string {
	w := c.Width
	if w < 28 {
		w = 28
	}
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	barW := inner - 6
	if barW < 12 {
		barW = 12
	}
	if barW > 26 {
		barW = 26
	}
	var sb strings.Builder
	title := c.Name
	if c.Selected {
		title += " · CURRENT"
	}
	sb.WriteString(thScreenTitle.Render(TruncateToWidth(title, inner)) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader("SYLLABUS COVERAGE") + "\n")
	sb.WriteString(ProgressBar{Ratio: c.Percent(), Width: barW, ShowPct: true, Accent: cGov}.View() + "\n")
	sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d topics", c.Done, c.Total)) + "\n")
	sb.WriteString(SectionHeader("PRACTICE READINESS") + "\n")
	sb.WriteString(thHelp.Render("not yet calculated") + "\n")
	sb.WriteString("" + "\n")
	lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	for len(lines) < 8 {
		lines = append(lines, "")
	}
	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cBorder).Padding(0, 1).Width(w)
	if c.Focused {
		style = style.BorderForeground(cPrimary)
		lines[0] = thPanelTitle.Render(TruncateToWidth(title, inner)) + thFocusDot.Render("  ●")
	} else if c.Selected {
		lines[0] = thPanelTitle.Render(TruncateToWidth(title, inner))
	}
	return style.Render(strings.Join(lines, "\n"))
}

// examCardHeight is the rendered ExamCard height including borders.
const examCardHeight = 10

// MetricCard is a compact bento metric: label + prominent value + sub.
// Used for PLANNED/ACTUAL/COMPLETION/SESSIONS/TASKS/OVER-UNDER, mock
// scores, history summaries. Fixed 3-line body keeps grids uniform.
type MetricCard struct {
	Label  string
	Value  string
	Sub    string
	Width  int
	Accent color.Color // optional label accent (domain cards); nil = neutral
}

func (c MetricCard) View() string {
	w := c.Width
	if w < 14 {
		w = 14
	}
	inner := w - 4
	if inner < 8 {
		inner = 8
	}
	label := SectionHeader(TruncateToWidth(c.Label, inner))
	if c.Accent != nil {
		label = lipgloss.NewStyle().Bold(true).Foreground(c.Accent).Render(TruncateToWidth(c.Label, inner))
	}
	value := thScreenTitle.Render(TruncateToWidth(c.Value, inner))
	sub := thMeta.Render(TruncateToWidth(c.Sub, inner))
	if c.Sub == "" {
		sub = ""
		return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cBorder).Padding(0, 1).Width(w).Render(label + "\n" + value)
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cBorder).Padding(0, 1).Width(w).Render(label + "\n" + value + "\n" + sub)
}

// MockCard is a dense selectable summary for the mock list.
// Number + name + date + exam; focused state uses the accent border.
type MockCard struct {
	Num     int
	Name    string
	Date    string
	Exam    string
	Focused bool
	Width   int
}

func (c MockCard) View() string {
	w := c.Width
	if w < 24 {
		w = 24
	}
	inner := w - 4
	if inner < 16 {
		inner = 16
	}
	title := thPrimary.Render(TruncateToWidth(c.Name, inner))
	meta := thMeta.Render(TruncateToWidth(c.Date+"  ·  "+c.Exam, inner))
	num := fmt.Sprintf("%02d", c.Num)
	head := thMeta.Render(num+"  ") + title
	body := head + "\n" + "      " + meta
	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cBorder).Padding(0, 1).Width(w)
	if c.Focused {
		style = style.BorderForeground(cPrimary)
		body = thPanelTitle.Render(num+"  "+TruncateToWidth(c.Name, inner-4)) + thFocusDot.Render("  ●") + "\n" + "      " + meta
	}
	return style.Render(body)
}

// MetricGrid renders compact MetricCards responsively inside inner width.
// cols: 3 when wide, 2 medium, 1 narrow. Card widths are derived so the
// grid never overflows its panel.
func MetricGrid(cards []MetricCard, inner int) string {
	if len(cards) == 0 {
		return ""
	}
	cols := 3
	if inner < 52 {
		cols = 2
	}
	if inner < 30 {
		cols = 1
	}
	if cols > len(cards) {
		cols = len(cards)
	}
	cw := (inner - (cols-1)*1) / cols
	if cw < 10 {
		cw = 10
	}
	rendered := make([]string, 0, len(cards))
	for _, c := range cards {
		c.Width = cw
		rendered = append(rendered, c.View())
	}
	return BentoGrid(rendered, cols, " ")
}

// StatPair is one labeled value inside an evidence row.
type StatPair struct {
	Label string
	Value string
}

// EvidenceRow renders one domain evidence row: a domain-colored label plus
// aligned full-word stat pairs (Lectures, Questions, Accuracy — never
// lect/q/acc shorthand).
// Kept for compat; new information-design prefers DomainDetailBlock
// (structured sections/cards/rows) over single-line prose.
func EvidenceRow(domain string, pairs []StatPair) string {
	var sb strings.Builder
	sb.WriteString(DomainLabel(domain, 5))
	for _, pr := range pairs {
		sb.WriteString("   " + thMeta.Render(padRight(pr.Label, 9)) + " " + thPrimary.Render(pr.Value))
	}
	return sb.String()
}

// DomainDetailBlock renders one domain as a structured section: a
// domain-colored title plus one labeled row per metric (compact rows, never
// compressed prose). Labels explain the metric; values scan easily.
// Example:
//
//	GOV
//	Lectures completed      1
//	Practice questions     60
//	Accuracy             78.3%
func DomainDetailBlock(domain, title string, pairs []StatPair) string {
	var sb strings.Builder
	sb.WriteString(DomainStyle(domain).Render(title))
	for _, pr := range pairs {
		sb.WriteString("\n" + MetaRow(pr.Label, pr.Value, 18))
	}
	return sb.String()
}

// FitBlockWidth truncates every line of a pre-rendered block to w cells so
// side-by-side blocks never overflow their column.
func FitBlockWidth(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = TruncateToWidth(ln, w)
	}
	return strings.Join(lines, "\n")
}

// JoinDetailBlocks joins domain detail blocks side by side when wide enough
// to use the available space, else stacks them vertically with dividers.
// Widths are derived from inner so the row never overflows.
func JoinDetailBlocks(blocks []string, inner int) string {
	if len(blocks) == 0 {
		return ""
	}
	if inner < 60 || len(blocks) < 2 {
		div := ""
		var sb strings.Builder
		for i, b := range blocks {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(FitBlockWidth(b, inner))
			if i < len(blocks)-1 {
				sb.WriteString("\n" + RowDivider(inner))
			}
		}
		_ = div
		return sb.String()
	}
	n := len(blocks)
	cw := (inner - (n-1)*1) / n
	if cw < 10 {
		cw = 10
	}
	fitted := make([]string, 0, n)
	for _, b := range blocks {
		fitted = append(fitted, FitBlockWidth(b, cw))
	}
	parts := make([]string, 0, n*2-1)
	for i, b := range fitted {
		if i > 0 {
			parts = append(parts, " ")
		}
		parts = append(parts, b)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// EmptyState renders a consistent placeholder for empty content.
func EmptyState(text string) string { return thHelp.Render(text) }

// ValidationMessage renders a form error, or nothing when valid.
func ValidationMessage(msg string) string {
	if msg == "" {
		return ""
	}
	return thError.Render(msg)
}

// AppShell composes header + main + footer with explicit shell bounds.
// All screens render inside the main viewport; header/footer never shift.
func AppShell(header, body, footer string, sh ShellLayout) string {
	lines := strings.Split(body, "\n")
	if len(lines) > sh.Main.H {
		lines = lines[:sh.Main.H]
	}
	for len(lines) < sh.Main.H {
		lines = append(lines, "")
	}
	return header + "\n" + strings.Join(lines, "\n") + "\n" + footer
}
