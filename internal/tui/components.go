package tui

import (
	"fmt"
	"strings"

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
