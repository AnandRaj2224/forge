package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Forge color system. Restrained: one primary accent, semantic colors,
// subtle domain accents. No rainbow decoration.
const (
	cPrimary = lipgloss.Color("12") // Forge accent: selected / important
	cSuccess = lipgloss.Color("10") // completed / pass
	cActive  = lipgloss.Color("14") // in-progress / running
	cWarning = lipgloss.Color("11") // incomplete / attention
	cError   = lipgloss.Color("9")  // validation / errors
	cMuted   = lipgloss.Color("8")  // secondary metadata / help
	cDimLine = lipgloss.Color("8")  // separators / borders (unfocused)

	cGov = lipgloss.Color("12") // subtle domain accents
	cDSA = lipgloss.Color("13")
	cSWE = lipgloss.Color("14")
)

var (
	// Application identity.
	thAppTitle = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Navigation: active is a subtle filled pill, inactive is quiet.
	thNavActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(cPrimary).Padding(0, 1)
	thNavInactive = lipgloss.NewStyle().Foreground(cMuted).Padding(0, 1)
	thClock       = lipgloss.NewStyle().Foreground(cMuted)
	// Screen + panel titles.
	thScreenTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	thPanelTitle  = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	thSectionHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("7"))
	// Content levels.
	thPrimary = lipgloss.NewStyle()
	thMeta    = lipgloss.NewStyle().Foreground(cMuted)
	thHelp    = lipgloss.NewStyle().Foreground(cMuted).Italic(true)
	thError   = lipgloss.NewStyle().Foreground(cError).Bold(true)
	// Status.
	thSuccess = lipgloss.NewStyle().Foreground(cSuccess).Bold(true)
	thRunning = lipgloss.NewStyle().Foreground(cActive).Bold(true)
	thTodo    = lipgloss.NewStyle().Foreground(cMuted)
	thWarn    = lipgloss.NewStyle().Foreground(cWarning)
	// Rows: structured list rows with thin dividers. Selected rows own a
	// full-width background; cursor-without-focus stays visible but quiet.
	thRowNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	thRowSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("24"))
	thRowCursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("235"))
	// Table header: strong, compact, aligned to columns.
	thTableHead = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Footer: keys pop, descriptions recede, groups spaced.
	thFootKey  = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	thFootDesc = lipgloss.NewStyle().Foreground(cMuted)
	// Controls.
	thBtnFocus    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(cPrimary)
	thBtnIdle     = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	thBtnDisabled = lipgloss.NewStyle().Foreground(cMuted)
)

// DomainBadge renders a small domain tag in its accent color.
func DomainBadge(domain string) string {
	switch domain {
	case "GOV":
		return lipgloss.NewStyle().Bold(true).Foreground(cGov).Render(domain)
	case "DSA":
		return lipgloss.NewStyle().Bold(true).Foreground(cDSA).Render(domain)
	case "SWE":
		return lipgloss.NewStyle().Bold(true).Foreground(cSWE).Render(domain)
	default:
		return thPrimary.Render(domain)
	}
}

// StatusPill renders TODO / ACTIVE / DONE with semantic color.
func StatusPill(st TaskStatus) string {
	switch st {
	case TaskActive:
		return thRunning.Render("▶ ACTIVE")
	case TaskCompleted:
		return thSuccess.Render("✓ DONE")
	default:
		return thTodo.Render("○ TODO")
	}
}

// MetaRow renders an aligned label/value pair for compact metadata.
func MetaRow(label, value string, labelWidth int) string {
	if labelWidth <= 0 {
		labelWidth = 10
	}
	return fmt.Sprintf("%s  %s", thMeta.Render(padRight(label, labelWidth)), thPrimary.Render(value))
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + spaces(w-len(s))
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// Separator renders a subtle horizontal rule for panel sections.
func Separator(width int) string {
	if width < 4 {
		width = 4
	}
	return lipgloss.NewStyle().Foreground(cDimLine).Render(stringRepeat("─", width))
}

func stringRepeat(s string, n int) string {
	b := ""
	for i := 0; i < n; i++ {
		b += s
	}
	return b
}

// SectionHeader renders a small caps-ish section label inside panels.
func SectionHeader(s string) string { return thSectionHead.Render(s) }
