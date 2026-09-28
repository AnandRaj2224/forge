package tui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Forge color system — Charm-inspired, dark, restrained.
// Mostly dark/neutral + muted secondary + small vivid accent.
//
// BASE: very dark near-black with cool/blue character.
// PRIMARY: violet/purple/magenta family (navigation, selection, focus).
// ACTIVE: cyan/blue family (running, live).
// SUCCESS/WARNING/ERROR: restrained semantic colors, used sparingly.
// DOMAIN: subtle differentiation, low saturation.
//
// NOTE (Charm v2): lipgloss.Color is a func returning color.Color, so palette
// entries are vars, not consts.
var (
	// Base surfaces. Neutral cool-dark tones; borders are slate, never
	// tinted with the Forge accent except on focused/selected components.
	cBG      = lipgloss.Color("#0B0B13") // near-black, cool/blue
	cSurface = lipgloss.Color("#14141F") // panel/surface tone
	cBorder  = lipgloss.Color("#2E3340") // muted neutral slate border
	cDimLine = lipgloss.Color("#2E3340") // separators / unfocused borders (alias)

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
	cGov = lipgloss.Color("#A78BFA") // subtle violet
	cDSA = lipgloss.Color("#67E8F9") // subtle cyan/blue
	cSWE = lipgloss.Color("#F9A8D4") // subtle rose/pink

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
	// Screen + panel titles. Neutral by default; the Forge accent is
	// reserved for focus/selection markers, never for every heading.
	thScreenTitle = lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary)
	thPanelTitle  = lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary)
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
	// Restrained dark tint (not solid violet) keeps the UI neutral.
	thRowNormal   = lipgloss.NewStyle().Foreground(cFgSecondary)
	thRowSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#2A2547"))
	thRowCursor   = lipgloss.NewStyle().Foreground(cFgPrimary).Background(lipgloss.Color("#1E1B33"))
	// Table header: strong, compact, neutral, aligned to columns.
	thTableHead = lipgloss.NewStyle().Bold(true).Foreground(cFgSecondary)
	// Footer: keys pop, descriptions recede, groups spaced.
	thFootKey  = lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary)
	thFootDesc = lipgloss.NewStyle().Foreground(cFgSecondary)
	// Focus markers use the active/focus accent (cyan/blue), not violet.
	thFocusMark = lipgloss.NewStyle().Bold(true).Foreground(cActive)
	// Focus dot marks the single focused card/panel; small and selective.
	thFocusDot = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Controls. Idle buttons stay neutral; only the focused primary action
	// carries the accent fill.
	thBtnFocus    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B0B13")).Background(cPrimary)
	thBtnIdle     = lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary)
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

// StatusPill renders TODO / ACTIVE / COMPLETE with semantic color.
func StatusPill(st TaskStatus) string {
	switch st {
	case TaskActive:
		return thRunning.Render("▶ ACTIVE")
	case TaskCompleted:
		return thSuccess.Render("✓ COMPLETE")
	default:
		return thTodo.Render("TODO")
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

// DomainStyle returns the restrained accent style for a domain tag, label,
// or section in that domain's workspace. Neutral base is preserved;
// the domain accent appears only where it carries meaning.
func DomainStyle(domain string) lipgloss.Style {
	switch domain {
	case "GOV":
		return lipgloss.NewStyle().Bold(true).Foreground(cGov)
	case "DSA":
		return lipgloss.NewStyle().Bold(true).Foreground(cDSA)
	case "SWE":
		return lipgloss.NewStyle().Bold(true).Foreground(cSWE)
	default:
		return thPrimary
	}
}

// DomainFg returns the accent foreground for a domain tag or cell.
func DomainFg(domain string) color.Color {
	switch domain {
	case "GOV":
		return cGov
	case "DSA":
		return cDSA
	case "SWE":
		return cSWE
	default:
		return cFgSecondary
	}
}

// DomainLabel renders a padded domain label for aligned metadata rows
// (e.g. TIME BY DOMAIN sections).
func DomainLabel(domain string, w int) string {
	return DomainStyle(domain).Render(padRight(domain, w))
}

// DomainSection renders a section label tinted by its domain context
// (e.g. the selected pattern/area/subject sections).
func DomainSection(domain, label string) string {
	return DomainStyle(domain).Render(label)
}
