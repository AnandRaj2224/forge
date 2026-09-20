package tui

import (
	"fmt"
	"strings"

	db "forge/internal/db"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Panel renders a bento card: styled title bar, consistent inner padding,
// subtle or accent border. Content is pre-rendered panel body.
func Panel(title, content string, width int, focused bool) string {
	border := lipgloss.RoundedBorder()
	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(cDimLine).
		Padding(0, 1).
		Width(width)
	if focused {
		style = style.BorderForeground(cPrimary)
	}
	head := thPanelTitle.Render(title)
	if focused {
		head = thPanelTitle.Render(title + "  ●")
	}
	sep := Separator(maxInt(width-4, 4))
	return style.Render(head + "\n" + sep + "\n" + content)
}

// PanelPlain is a bento card without focus affordance (static info groups).
func PanelPlain(title, content string, width int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cDimLine).
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
	return l, r
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ProgressBar is the single reusable Forge progress component:
// ratio-backed, compact, clean block texture with optional percent.
type ProgressBar struct {
	Ratio   float64 // 0..1
	Width   int
	ShowPct bool
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
	bar := lipgloss.NewStyle().Foreground(cPrimary).Render(strings.Repeat("█", filled)) +
		thMeta.Render(strings.Repeat("░", w-filled))
	if p.ShowPct {
		return fmt.Sprintf("%s %3.0f%%", bar, r*100)
	}
	return bar
}

// Button is a real boxy interactive control with focus + disabled states.
type Button struct {
	Label    string
	Focused  bool
	Disabled bool
}

func (b Button) View() string {
	if b.Disabled {
		inner := thBtnDisabled.Render("  " + b.Label + "  ")
		return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDimLine).Padding(0, 1).Render(inner)
	}
	inner := thBtnIdle.Render("  " + b.Label + "  ")
	if b.Focused {
		inner = thBtnFocus.Render("  " + b.Label + "  ")
		return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cPrimary).Padding(0, 1).Render(inner)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDimLine).Padding(0, 1).Render(inner)
}

// RadioGroup is a real mutually-exclusive selection control with stable
// glyphs (◉/○) that never depend on arrow rendering.
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
		line := glyph + "  " + o
		if r.Focused && i == r.Cursor {
			line = thBtnIdle.Render("▸ ") + thPrimary.Render(line)
		} else {
			line = "  " + thPrimary.Render(line)
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Checkbox is a real toggle control with stable glyphs (☑/☐).
type Checkbox struct {
	Label   string
	Checked bool
	Focused bool
}

func (c Checkbox) View() string {
	glyph := "☐"
	if c.Checked {
		glyph = "☑"
	}
	line := glyph + "  " + c.Label
	if c.Focused {
		return thBtnIdle.Render("▸ ") + thPrimary.Render(line)
	}
	return "  " + thPrimary.Render(line)
}

// FormField wraps a real editable text input with an aligned label.
type FormField struct {
	Label string
	Input textinput.Model
	Error string
}

func NewField(label, placeholder string) FormField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 16
	ti.Prompt = "│ "
	return FormField{Label: label, Input: ti}
}

// View renders label + input; label column width keeps forms aligned.
func (f FormField) View(focused bool, labelWidth int) string {
	if labelWidth <= 0 {
		labelWidth = 10
	}
	lbl := padRight(f.Label, labelWidth)
	if focused {
		return thBtnIdle.Render(lbl+" ") + f.Input.View()
	}
	return thMeta.Render(lbl+" ") + f.Input.View()
}

// KeyMsg helpers shared by screens.
func IsBack(msg tea.KeyMsg) bool { return msg.String() == "esc" }

// lipglossJoin places two panels side by side with a gap.
func lipglossJoin(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left+" ", right)
}

// Navigator is the shared Forge list wrapper: no duplicated list chrome,
// content-fitted height, single focus convention. All master navigators
// (exams, subjects, patterns, areas, weeks) build on it.
type Navigator struct {
	List list.Model
}

// NewNavigator returns a chrome-free selectable list.
func NewNavigator() Navigator {
	l := list.New(nil, list.NewDefaultDelegate(), 28, 12)
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	return Navigator{List: l}
}

// SetItems replaces rows.
func (n *Navigator) SetItems(items []list.Item) { n.List.SetItems(items) }

// FitHeight sizes the viewport to content so panels never float in padding.
func (n *Navigator) FitHeight(maxH int) {
	h := len(n.List.Items())*2 + 1
	if h < 4 {
		h = 4
	}
	if h > maxH {
		h = maxH
	}
	n.List.SetHeight(h)
}

// SelectedID returns the selected row id for listItem rows.
func (n Navigator) SelectedID() string {
	if it, ok := n.List.SelectedItem().(listItem); ok {
		return it.id
	}
	return ""
}

// wrapItems converts typed rows to list items.
func wrapItems(in []listItem) []list.Item {
	out := make([]list.Item, 0, len(in))
	for _, it := range in {
		out = append(out, it)
	}
	return out
}

// ExamCard is a real selectable bento card for the GOV exam dashboard.
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

// View renders the card (7 content lines + border). Keep in sync with
// examCardHeight: dashboard grid math depends on uniform card height.
func (c ExamCard) View() string {
	w := c.Width
	if w < 28 {
		w = 28
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(c.Name) + "\n")
	sb.WriteString(SectionHeader("SYLLABUS COVERAGE") + "\n")
	sb.WriteString(ProgressBar{Ratio: c.Percent(), Width: 20, ShowPct: true}.View() + "\n")
	sb.WriteString(thMeta.Render(fmt.Sprintf("%d / %d topics", c.Done, c.Total)) + "\n")
	sb.WriteString(SectionHeader("PRACTICE READINESS") + "\n")
	sb.WriteString(thHelp.Render("not yet calculated") + "\n")
	// Pad to exactly 7 content lines so grid rows stay uniform.
	lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	for len(lines) < 7 {
		lines = append(lines, "")
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDimLine).Padding(0, 1).Width(w)
	if c.Focused {
		style = style.BorderForeground(cPrimary)
	}
	return style.Render(strings.Join(lines, "\n"))
}

// examCardHeight is the rendered ExamCard height including borders.
const examCardHeight = 9

// EmptyState renders a consistent placeholder for empty content.
func EmptyState(text string) string { return thHelp.Render(text) }

// ValidationMessage renders a form error, or nothing when valid.
func ValidationMessage(msg string) string {
	if msg == "" {
		return ""
	}
	return thError.Render(msg)
}

// TodayTaskRow builds one aligned Today table row with domain badge text
// and status pill text. The table owns column alignment; this owns content.
func TodayTaskRow(i int, t db.TodayTask, st TaskStatus) table.Row {
	status := "TODO"
	if st == TaskActive {
		status = "ACTIVE"
	} else if st == TaskCompleted {
		status = "DONE"
	}
	return table.Row{fmt.Sprint(i + 1), string(t.Domain), t.Title, FormatDuration(t.PlannedSeconds), status}
}
