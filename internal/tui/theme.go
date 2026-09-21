package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Forge color system — Charm-inspired, dark, restrained.
// Mostly dark/neutral + muted secondary + small vivid accent.
//
// BASE: very dark near-black with cool/blue character.
// PRIMARY: violet/purple/magenta family (navigation, selection, focus).
// ACTIVE: cyan/blue family (running, live).
// SUCCESS/WARNING/ERROR: restrained semantic colors, used sparingly.
// DOMAIN: subtle differentiation, low saturation.
const (
	// Base surfaces.
	cBG      = lipgloss.Color("#0B0B13") // near-black, cool/blue
	cSurface = lipgloss.Color("#14141F") // panel/surface tone
	cBorder  = lipgloss.Color("#2B2B3D") // muted cool border
	cDimLine = lipgloss.Color("#2B2B3D") // separators / unfocused borders (alias)

	// Text.
	cFgPrimary   = lipgloss.Color("#E8E8F0") // soft bright foreground
	cFgSecondary = lipgloss.Color("#9A9AB0") // muted cool-gray
	cFgTertiary  = lipgloss.Color("#62627A") // subdued help gray
	cMuted       = lipgloss.Color("#9A9AB0") // compat alias (secondary)

	// Primary Forge accent: violet.
	cPrimary       = lipgloss.Color("#A78BFA") // vivid violet
	cPrimaryDim    = lipgloss.Color("#7C3AED") // deep violet for fills
	cPrimaryBright = lipgloss.Color("#C4B5FD") // headings / key emphasis

	// Active: cyan/blue.
	cActive     = lipgloss.Color("#22D3EE") // running / in-progress
	cActiveBlue = lipgloss.Color("#60A5FA") // blue side of live range

	// Semantic (restrained).
	cSuccess = lipgloss.Color("#34D399") // completed
	cWarning = lipgloss.Color("#FBBF24") // attention, sparingly
	cError   = lipgloss.Color("#F87171") // validation only

	// Domain accents (subtle, not rainbow).
	cGov = lipgloss.Color("#93A8F5") // cool periwinkle
	cDSA = lipgloss.Color("#C4B5FD") // soft lavender
	cSWE = lipgloss.Color("#67E8F9") // soft cyan

	// Live session gradient stops: violet → purple → blue → cyan → green.
	cGrad0 = lipgloss.Color("#A78BFA")
	cGrad1 = lipgloss.Color("#8B5CF6")
	cGrad2 = lipgloss.Color("#60A5FA")
	cGrad3 = lipgloss.Color("#22D3EE")
	cGrad4 = lipgloss.Color("#34D399")
)

var (
	// Application identity.
	thAppTitle = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Forge wordmark: largest element in header.
	thLogo = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Navigation: active is obvious accent pill, inactive quiet but readable.
	thNavActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B0B13")).Background(cPrimary).Padding(0, 2)
	thNavInactive = lipgloss.NewStyle().Foreground(cFgSecondary).Padding(0, 2)
	thClock       = lipgloss.NewStyle().Foreground(cFgTertiary)
	// Screen + panel titles.
	thScreenTitle = lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary)
	thPanelTitle  = lipgloss.NewStyle().Bold(true).Foreground(cPrimaryBright)
	thSectionHead = lipgloss.NewStyle().Bold(true).Foreground(cFgSecondary)
	// Content levels.
	thPrimary = lipgloss.NewStyle().Foreground(cFgPrimary)
	thMeta    = lipgloss.NewStyle().Foreground(cFgSecondary)
	thHelp    = lipgloss.NewStyle().Foreground(cFgTertiary).Italic(true)
	thError   = lipgloss.NewStyle().Foreground(cError).Bold(true)
	// Status.
	thSuccess = lipgloss.NewStyle().Foreground(cSuccess).Bold(true)
	thRunning = lipgloss.NewStyle().Foreground(cActive).Bold(true)
	thTodo    = lipgloss.NewStyle().Foreground(cFgTertiary)
	thWarn    = lipgloss.NewStyle().Foreground(cWarning)
	// Rows: structured list rows with thin dividers. Selected rows own a
	// full-width background; cursor-without-focus stays visible but quiet.
	thRowNormal   = lipgloss.NewStyle().Foreground(cFgSecondary)
	thRowSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(cPrimaryDim)
	thRowCursor   = lipgloss.NewStyle().Foreground(cFgPrimary).Background(lipgloss.Color("#1E1B33"))
	// Table header: strong, compact, aligned to columns.
	thTableHead = lipgloss.NewStyle().Bold(true).Foreground(cPrimaryBright)
	// Footer: keys pop, descriptions recede, groups spaced.
	thFootKey  = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	thFootDesc = lipgloss.NewStyle().Foreground(cFgSecondary)
	// Controls.
	thBtnFocus    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B0B13")).Background(cPrimary)
	thBtnIdle     = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	thBtnDisabled = lipgloss.NewStyle().Foreground(cFgTertiary)
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
	return lipgloss.NewStyle().Foreground(cBorder).Render(stringRepeat("─", width))
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
