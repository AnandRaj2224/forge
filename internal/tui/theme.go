package tui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Forge color system — ocean night + neon purple/pink/cyan/peach palette.
//
// EXACT PALETTE (use exact vivid colors, restraint comes from placement):
//
//	#0B0F2B ocean night — main application background/foundation
//	#BC6CFF neon purple — primary Forge accent (nav, focus, selection)
//	#FF5CA8 sunset pink — secondary accent, SWE domain
//	#00F0FF vice cyan — secondary accent, DSA domain, COMPLETE signal
//	#FFB86B miami peach — secondary accent, GOV domain, ACTIVE signal
//
// TEXT HIERARCHY (easy on the eyes):
//
//	normal text → muted neutral gray
//	secondary/supporting → darker gray
//	focused/important → white
//
// The ocean night background stays dominant; palette colors appear as small
// intentional accents, never large solid fields.
//
// Semantic theme values (centralized; screens must not hardcode hex):
//
//	background surface border focus
//	text textMuted textHighlight
//	primary secondary light
//	oceanNight neonPurple sunsetPink viceCyan miamiPeach
//	gov dsa swe
//	todo active complete
//
// NOTE (Charm v2): lipgloss.Color is a func returning color.Color, so palette
// entries are vars, not consts.
var (
	background    = lipgloss.Color("#0B0F2B")
	surface       = lipgloss.Color("#0B0F2B")
	border        = lipgloss.Color("#0B0F2B")
	focus         = lipgloss.Color("#BC6CFF")
	text          = lipgloss.Color("#FFFFFF")
	textMuted     = lipgloss.Color("#B9B9C9")
	textHighlight = lipgloss.Color("#FFFFFF")

	primary   = lipgloss.Color("#BC6CFF")
	secondary = lipgloss.Color("#FF5CA8")
	light     = lipgloss.Color("#FFFFFF")

	oceanNight = lipgloss.Color("#0B0F2B")
	neonPurple = lipgloss.Color("#BC6CFF")
	sunsetPink = lipgloss.Color("#FF5CA8")
	viceCyan   = lipgloss.Color("#00F0FF")
	miamiPeach = lipgloss.Color("#FFB86B")

	gov = lipgloss.Color("#FFB86B")
	dsa = lipgloss.Color("#00F0FF")
	swe = lipgloss.Color("#FF5CA8")

	todo     = lipgloss.Color("#8E8EA0")
	active   = lipgloss.Color("#FFB86B")
	complete = lipgloss.Color("#00F0FF")
)

var (
	// Base surfaces. Ocean night foundation; ordinary borders use the
	// dark palette treatment, never a full-panel fill.
	cBG      = lipgloss.Color("#0B0F2B")
	cSurface = lipgloss.Color("#0B0F2B")
	cBorder  = lipgloss.Color("#0B0F2B")
	cDimLine = lipgloss.Color("#0B0F2B")

	// Focused/important text is white.
	cWhite = lipgloss.Color("#FFFFFF")
	// Text hierarchy: comfortable muted neutrals for most of the UI.
	// Normal text is muted, secondary/supporting is darker,
	// helper text is darkest. White is reserved for focused/important.
	cFgPrimary   = lipgloss.Color("#B9B9C9")
	cFgSecondary = lipgloss.Color("#8E8EA0")
	cFgTertiary  = lipgloss.Color("#6E6E82")
	cMuted       = lipgloss.Color("#8E8EA0")

	// Primary Forge accent: neon purple. No neon variants beyond the palette.
	cPrimary       = lipgloss.Color("#BC6CFF")
	cPrimaryDim    = lipgloss.Color("#BC6CFF")
	cPrimaryBright = lipgloss.Color("#BC6CFF")

	// Active: miami peach. Secondary side is sunset pink.
	cActive     = lipgloss.Color("#FFB86B")
	cActiveBlue = lipgloss.Color("#FFB86B")

	// Semantic, palette-only. Completed is vice cyan; warning is
	// miami peach; validation-only error is sunset pink.
	cSuccess = lipgloss.Color("#00F0FF")
	cWarning = lipgloss.Color("#FFB86B")
	cError   = lipgloss.Color("#FF5CA8")

	// TODO status: subdued neutral treatment (deliberately low emphasis).
	cTodo = lipgloss.Color("#8E8EA0")

	// Domain accents: small TRACK identifiers only.
	// GOV peach, DSA cyan, SWE pink.
	cGov = lipgloss.Color("#FFB86B")
	cDSA = lipgloss.Color("#00F0FF")
	cSWE = lipgloss.Color("#FF5CA8")

	// LeetCode difficulty language (DSA screen difficulty labels only):
	// bright deep green for easy, bright deep yellow for medium,
	// bright deep red for hard.
	cEasy   = lipgloss.Color("#00E676")
	cMedium = lipgloss.Color("#FFD60A")
	cHard   = lipgloss.Color("#FF3131")

	// Live session / logo stops: palette only.
	cGrad0 = lipgloss.Color("#0B0F2B")
	cGrad1 = lipgloss.Color("#BC6CFF")
	cGrad2 = lipgloss.Color("#FF5CA8")
	cGrad3 = lipgloss.Color("#FFB86B")
	cGrad4 = lipgloss.Color("#00F0FF")
)

var (
	// Application identity.
	thAppTitle = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Forge wordmark: largest element in header.
	thLogo = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Navigation: active is a controlled purple pill with white text;
	// inactive stays muted gray.
	thNavActive   = lipgloss.NewStyle().Bold(true).Foreground(cWhite).Background(cPrimary).Padding(0, 2)
	thNavInactive = lipgloss.NewStyle().Foreground(cFgPrimary).Padding(0, 2)
	thClock       = lipgloss.NewStyle().Foreground(cFgSecondary)
	// Screen + panel titles are important → white; section labels are
	// supporting → darker gray. The Forge accent stays reserved for
	// focus/selection markers, never for every heading.
	thScreenTitle = lipgloss.NewStyle().Bold(true).Foreground(cWhite)
	thPanelTitle  = lipgloss.NewStyle().Bold(true).Foreground(cWhite)
	thSectionHead = lipgloss.NewStyle().Bold(true).Foreground(cFgSecondary)
	// Content levels: normal muted, secondary darker,
	// helper darkest italic. White is for focused/important only.
	thPrimary = lipgloss.NewStyle().Foreground(cFgPrimary)
	thMeta    = lipgloss.NewStyle().Foreground(cFgSecondary)
	thHelp    = lipgloss.NewStyle().Foreground(cFgTertiary).Italic(true)
	thError   = lipgloss.NewStyle().Foreground(cError).Bold(true)
	// Status: TODO subdued neutral, ACTIVE peach, COMPLETE cyan.
	thSuccess = lipgloss.NewStyle().Foreground(cSuccess).Bold(true)
	thRunning = lipgloss.NewStyle().Foreground(cActive).Bold(true)
	thTodo    = lipgloss.NewStyle().Foreground(cTodo)
	thWarn    = lipgloss.NewStyle().Foreground(cWarning)
	// Rows: no solid selection rectangle. The focused row is identified by
	// a thin purple edge plus brighter/white text on a transparent background.
	// Normal rows stay muted gray.
	thRowNormal   = lipgloss.NewStyle().Foreground(cFgPrimary)
	thRowSelected = lipgloss.NewStyle().Bold(true).Foreground(cWhite)
	thRowCursor   = lipgloss.NewStyle().Foreground(cFgPrimary)
	// Table header: muted gray, compact, aligned to columns.
	thTableHead = lipgloss.NewStyle().Bold(true).Foreground(cFgPrimary)
	// Footer: keys pop white bold, descriptions recede gray.
	thFootKey  = lipgloss.NewStyle().Bold(true).Foreground(cWhite)
	thFootDesc = lipgloss.NewStyle().Foreground(cFgSecondary)
	// Focus markers use the primary Forge accent (purple).
	thFocusMark = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Focus dot marks the single focused card/panel; small and selective.
	thFocusDot = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	// Controls. Idle buttons stay muted gray; only the focused primary
	// action carries the accent fill with white text.
	thBtnFocus    = lipgloss.NewStyle().Bold(true).Foreground(cWhite).Background(cPrimary)
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
// COMPLETE defaults to cyan (legacy); use StatusPillForDomain for
// track-tinted COMPLETE in the Today workspace.
func StatusPill(st TaskStatus) string {
	return StatusPillForDomain(st, "")
}

// StatusPillForDomain renders TODO / ACTIVE / COMPLETE where COMPLETE
// takes the track color (GOV peach, DSA cyan, SWE pink) so the detail
// header matches the Today table STATUS cell. ACTIVE stays peach (global
// active signal), TODO stays neutral. Unknown/empty domain falls back to
// cyan to preserve legacy behavior.
func StatusPillForDomain(st TaskStatus, domain string) string {
	switch st {
	case TaskActive:
		return thRunning.Render("▶ ACTIVE")
	case TaskCompleted:
		switch domain {
		case "GOV":
			return lipgloss.NewStyle().Bold(true).Foreground(cGov).Render("✓ COMPLETE")
		case "DSA":
			return lipgloss.NewStyle().Bold(true).Foreground(cDSA).Render("✓ COMPLETE")
		case "SWE":
			return lipgloss.NewStyle().Bold(true).Foreground(cSWE).Render("✓ COMPLETE")
		default:
			return thSuccess.Render("✓ COMPLETE")
		}
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
		return cFgPrimary
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
