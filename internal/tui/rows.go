package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	db "forge/internal/db"
)

// Forge row system: every navigable list item is a structured row separated
// by a thin horizontal divider. Selection is a background treatment owned by
// the row itself, never a bare ">" prefix.

// RowItem is one selectable row in a Forge list.
type RowItem struct {
	ID      string
	Title   string
	Meta    string // prominent right value (actual time, count, date)
	MetaSub string // dimmer suffix after Meta (planned time, context)
	Mark    string // leading glyph: ✓ ○ ● etc (empty = none)
}

// RowList is the Forge navigable list: real component with cursor, viewport
// scrolling, thin dividers, and polished selected states.
//
// Cursor is a LOGICAL ROW INDEX over Items. The viewport is PHYSICAL-LINE
// based: each row may wrap to multiple physical lines (see SelectableRow
// LineCount), dividers cost one line each, and Height is a visible line
// budget. The visible window is calculated from summed physical line cost
// so tall rows stay fully visible, never clipped or skipped.
type RowList struct {
	Items    []RowItem
	Cursor   int
	Offset   int // first visible logical index
	Width    int // content width for divider span
	Height   int // visible line budget (physical lines, incl. dividers)
	Focused  bool
	Numbered bool // global row rule: visible number/index where appropriate
}

// NewRowList returns an empty Forge list.
func NewRowList() RowList { return RowList{Width: 28, Height: 12, Numbered: true} }

// SetItems replaces rows and clamps the cursor.
func (r *RowList) SetItems(items []RowItem) {
	r.Items = items
	if r.Cursor >= len(items) {
		r.Cursor = len(items) - 1
	}
	if r.Cursor < 0 {
		r.Cursor = 0
	}
	if r.Offset > r.Cursor {
		r.Offset = r.Cursor
	}
	r.ensureVisible()
}

// SetWidth sets the divider/content width.
func (r *RowList) SetWidth(w int) {
	if w < 10 {
		w = 10
	}
	r.Width = w
	r.ensureVisible()
}

// SetHeight sets the viewport line budget (physical lines).
func (r *RowList) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	r.Height = h
}

// FitHeight sizes the viewport to content up to maxLines visible lines.
// The budget is physical lines (row heights + dividers). When content fits,
// the viewport shrinks to the exact physical cost so panels never float in
// padding; otherwise it caps at maxLines.
func (r *RowList) FitHeight(maxLines int) {
	if maxLines < 3 {
		maxLines = 3
	}
	if len(r.Items) == 0 {
		r.Height = 1
		return
	}
	total := r.TotalHeight()
	if total < 1 {
		total = 1
	}
	if total < maxLines {
		r.Height = total
		r.ensureVisible()
		return
	}
	r.Height = maxLines
	r.ensureVisible()
}

// Select moves the cursor to i.
func (r *RowList) Select(i int) {
	if len(r.Items) == 0 {
		return
	}
	r.Cursor = Clamp(i, 0, len(r.Items)-1)
	r.ensureVisible()
}

// CursorDown moves one row down.
func (r *RowList) CursorDown() {
	if len(r.Items) == 0 {
		return
	}
	if r.Cursor < len(r.Items)-1 {
		r.Cursor++
		r.ensureVisible()
	}
}

// CursorUp moves one row up.
func (r *RowList) CursorUp() {
	if len(r.Items) == 0 {
		return
	}
	if r.Cursor > 0 {
		r.Cursor--
		r.ensureVisible()
	}
}

// SelectedID returns the cursor row id.
func (r RowList) SelectedID() string {
	if len(r.Items) == 0 || r.Cursor < 0 || r.Cursor >= len(r.Items) {
		return ""
	}
	return r.Items[r.Cursor].ID
}

// Selected returns the cursor row.
func (r RowList) Selected() *RowItem {
	if len(r.Items) == 0 || r.Cursor < 0 || r.Cursor >= len(r.Items) {
		return nil
	}
	return &r.Items[r.Cursor]
}

func (r *RowList) ensureVisible() {
	if r.Height < 1 {
		r.Height = 1
	}
	if len(r.Items) == 0 {
		r.Offset = 0
		return
	}
	if r.Cursor >= len(r.Items) {
		r.Cursor = len(r.Items) - 1
	}
	if r.Cursor < 0 {
		r.Cursor = 0
	}
	if r.Cursor < r.Offset {
		r.Offset = r.Cursor
	}
	// Advance the offset until the cursor block fits the physical line
	// budget. Window cost = sum of row heights + one divider between
	// consecutive rows. A single oversize row is always shown (offset ==
	// cursor) even when taller than the viewport.
	for {
		total := 0
		for i := r.Offset; i <= r.Cursor; i++ {
			total += r.RowHeight(i)
			if i > r.Offset {
				total++ // divider
			}
		}
		if total <= r.Height || r.Offset >= r.Cursor {
			break
		}
		r.Offset++
	}
	if r.Offset < 0 {
		r.Offset = 0
	}
}

// EnsureVisible is the shared scrolling contract: after any size change or
// cursor move, the focused row stays inside the visible viewport. Screens
// call it after FitHeight/SetWidth and before View so focus never sits
// outside the viewport (application-level fix, not per-panel).
func (r *RowList) EnsureVisible() { r.ensureVisible() }

// RowHeight returns the physical line count of logical row i (wrapped
// title/meta lines, dividers excluded). Narrow widths never panic and never
// truncate: heights grow as content wraps.
func (r RowList) RowHeight(i int) int {
	if i < 0 || i >= len(r.Items) {
		return 1
	}
	num := ""
	if r.Numbered {
		num = fmt.Sprintf("%02d", i+1)
	}
	n := SelectableRow{
		Num:     num,
		Mark:    r.Items[i].Mark,
		Title:   r.Items[i].Title,
		Meta:    r.Items[i].Meta,
		MetaSub: r.Items[i].MetaSub,
		Width:   r.Width,
	}.LineCount()
	if n < 1 {
		n = 1
	}
	return n
}

// TotalHeight returns the full physical content height: summed row heights
// plus one divider line between consecutive rows.
func (r RowList) TotalHeight() int {
	if len(r.Items) == 0 {
		return 0
	}
	total := 0
	for i := range r.Items {
		total += r.RowHeight(i)
		if i > 0 {
			total++ // divider
		}
	}
	return total
}

// View renders the visible window joined by thin dividers. The window is
// selected from summed physical line cost (not row count): rows are added
// while the budget allows, tall rows expand, and the cursor row is always
// included even when oversize. Selection highlight covers every physical
// line of the selected row.
func (r *RowList) View() string {
	if len(r.Items) == 0 {
		return EmptyState("Nothing here yet.")
	}
	// Shared scrolling fix: re-sync before rendering so a resize or width
	// change cannot leave focus outside the viewport.
	r.ensureVisible()
	h := r.Height
	if h < 1 {
		h = 1
	}
	off := r.Offset
	if off < 0 {
		off = 0
	}
	if off >= len(r.Items) {
		off = len(r.Items) - 1
	}
	var sb strings.Builder
	used := 0
	end := off
	for i := off; i < len(r.Items); i++ {
		cost := r.RowHeight(i)
		add := cost
		if i > off {
			add++ // divider before this row
		}
		if used+add > h && i > off {
			break
		}
		if i > off {
			sb.WriteString(RowDivider(r.Width) + "\n")
			used++
		}
		num := ""
		if r.Numbered {
			num = fmt.Sprintf("%02d", i+1)
		}
		sb.WriteString(SelectableRow{
			Num:      num,
			Mark:     r.Items[i].Mark,
			Title:    r.Items[i].Title,
			Meta:     r.Items[i].Meta,
			MetaSub:  r.Items[i].MetaSub,
			Selected: i == r.Cursor,
			Focused:  r.Focused && i == r.Cursor,
			Width:    r.Width,
		}.View() + "\n")
		used += cost
		end = i + 1
	}
	// Scroll hints stay subtle and single-line.
	if off > 0 {
		sb.WriteString(thHelp.Render("↑ more above") + "\n")
	}
	if end < len(r.Items) {
		sb.WriteString(thHelp.Render("↓ more below") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// rowCell is one exact-width cell with explicit colors.
type rowCell struct {
	text  string
	style lipgloss.Style
}

// paintCells concatenates pre-sized cells. Widths are exact by
// construction; every segment owns its colors so background tint stays
// seamless edge to edge. Pre-styled text (containing ANSI) passes through
// untouched to preserve its colors.
func paintCells(cells ...rowCell) string {
	var sb strings.Builder
	for _, c := range cells {
		if strings.Contains(c.text, "\x1b[") {
			sb.WriteString(c.text)
			continue
		}
		sb.WriteString(c.style.Render(c.text))
	}
	return sb.String()
}

// rowTheme is the explicit style set for one row state: every cell carries
// its background, so tint reaches the panel's right inner edge.
type rowTheme struct {
	bg     color.Color // nil = transparent
	edge   lipgloss.Style
	main   lipgloss.Style
	strong lipgloss.Style
	faint  lipgloss.Style
	gap    lipgloss.Style
}

// rowThemeFor returns the style set for normal / cursor / focused-selected.
func rowThemeFor(selected, focused bool) (rowTheme, bool) {
	switch {
	case selected && focused:
		bg := lipgloss.Color("#2A2547")
		return rowTheme{
			bg:     bg,
			edge:   lipgloss.NewStyle().Foreground(cActive).Background(bg),
			main:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(bg),
			strong: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(bg),
			faint:  lipgloss.NewStyle().Foreground(cFgSecondary).Background(bg),
			gap:    lipgloss.NewStyle().Background(bg),
		}, true
	case selected:
		bg := lipgloss.Color("#1E1B33")
		return rowTheme{
			bg:     bg,
			edge:   lipgloss.NewStyle().Background(bg),
			main:   lipgloss.NewStyle().Foreground(cFgPrimary).Background(bg),
			strong: lipgloss.NewStyle().Foreground(cFgPrimary).Background(bg),
			faint:  lipgloss.NewStyle().Foreground(cFgSecondary).Background(bg),
			gap:    lipgloss.NewStyle().Background(bg),
		}, false
	default:
		return rowTheme{
			bg:     nil,
			edge:   lipgloss.NewStyle(),
			main:   thRowNormal,
			strong: thPrimary,
			faint:  lipgloss.NewStyle().Foreground(cFgTertiary),
			gap:    lipgloss.NewStyle(),
		}, false
	}
}

// domainCell styles a padded domain cell: domain accent foreground over the
// row background (transparent when the row has none).
func (t rowTheme) domain(domain, text string) rowCell {
	s := lipgloss.NewStyle().Bold(true).Foreground(DomainFg(domain))
	if t.bg != nil {
		s = s.Background(t.bg)
	}
	return rowCell{text: text, style: s}
}

// statusCellFor styles the STATUS cell with restrained semantic color over
// the row background: TODO muted, ACTIVE accent, COMPLETE green. Selection
// background is preserved; only the status text carries semantics.
func statusCellFor(th rowTheme, st TaskStatus, text string) rowCell {
	var s lipgloss.Style
	switch st {
	case TaskActive:
		s = lipgloss.NewStyle().Bold(true).Foreground(cActive)
	case TaskCompleted:
		s = lipgloss.NewStyle().Bold(true).Foreground(cSuccess)
	default:
		s = lipgloss.NewStyle().Foreground(cFgTertiary)
	}
	if th.bg != nil {
		s = s.Background(th.bg)
	}
	return rowCell{text: text, style: s}
}

// SelectableRow is one structured row: number + mark + title plus an
// optional prominent right value (Meta) with a dimmer suffix (MetaSub,
// e.g. actual time plus planned time). Full-width tint in every state;
// the divider is rendered by the owning list.
//
// Long cells wrap instead of truncating: the row grows to the tallest
// wrapped cell, every physical line carries the selection background, and
// no content is replaced with ellipsis. The logical cursor still moves one
// row at a time; only the physical height changes.
type SelectableRow struct {
	Num      string
	Mark     string
	Title    string
	Meta     string
	MetaSub  string
	Selected bool
	Focused  bool
	Width    int
}

// LineCount returns the physical line count of the row at its Width.
// Short rows stay one line; overflow wraps to more lines; multiple wrapped
// cells use the tallest height. Never panics on narrow widths.
func (r SelectableRow) LineCount() int {
	_, _, _, _, _, titleLines, rightLines, hasRight, fitsSingle := r.layout()
	if !hasRight {
		if len(titleLines) < 1 {
			return 1
		}
		return len(titleLines)
	}
	if fitsSingle {
		if len(titleLines) < 1 {
			return 1
		}
		return len(titleLines)
	}
	n := len(titleLines)
	if len(rightLines) > n {
		n = len(rightLines)
	}
	if n < 1 {
		n = 1
	}
	return n
}

// layout computes column widths and wrapped lines without rendering.
// hasRight reports whether a right block exists; fitsSingle reports whether
// the right block fits on one line (preserving distinct meta/sub styles).
func (r SelectableRow) layout() (w, pw, titleW, rightW int, pre string, titleLines, rightLines []string, hasRight, fitsSingle bool) {
	w = r.Width
	if w < 10 {
		w = 10
	}
	mark := r.Mark
	if mark == "" {
		mark = " "
	}
	pre = prefixPlain(r.Num, mark)
	pw = len([]rune(pre))
	inner := w - 2
	if inner < 4 {
		inner = 4
	}
	budget := inner / 2
	if budget < 1 {
		budget = 1
	}
	meta := r.Meta
	// Difficulty labels keep semantic meaning in uppercase for scannability.
	if isDifficulty(meta) {
		meta = strings.ToUpper(strings.TrimSpace(meta))
	}
	sub := ""
	if r.MetaSub != "" {
		sub = "  " + r.MetaSub
	}
	hasRight = meta != "" || sub != ""
	if !hasRight {
		titleW = w - 1 - pw
		if titleW < 1 {
			titleW = 1
		}
		titleLines = wrapCell(r.Title, titleW)
		return w, pw, titleW, 0, pre, titleLines, nil, false, false
	}
	totalRight := lipgloss.Width(meta) + lipgloss.Width(sub)
	if totalRight <= budget {
		rightW = totalRight
		if rightW < 1 {
			rightW = 1
		}
		titleW = w - 1 - pw - 2 - rightW
		if titleW < 1 {
			titleW = 1
		}
		titleLines = wrapCell(r.Title, titleW)
		fitsSingle = true
		return w, pw, titleW, rightW, pre, titleLines, nil, true, true
	}
	rightW = budget
	if rightW < 1 {
		rightW = 1
	}
	titleW = w - 1 - pw - 2 - rightW
	if titleW < 1 {
		titleW = 1
	}
	titleLines = wrapCell(r.Title, titleW)
	rightLines = wrapCell(meta+sub, rightW)
	return w, pw, titleW, rightW, pre, titleLines, rightLines, true, false
}

// View renders the row forced to Width cells, wrapping to multiple physical
// lines when content overflows. Selection highlight covers every physical
// line; no ellipsis is ever emitted.
func (r SelectableRow) View() string {
	w, pw, titleW, rightW, pre, titleLines, rightLines, hasRight, fitsSingle := r.layout()
	th, edged := rowThemeFor(r.Selected, r.Focused)
	lead := " "
	if edged {
		lead = "▌"
	}
	// Difficulty labels keep semantic color (green/amber/red) on any row
	// background; selection tint + focus edge stay independent.
	metaStyle := th.strong
	meta := r.Meta
	if isDifficulty(meta) {
		meta = strings.ToUpper(strings.TrimSpace(meta))
		var fg color.Color
		switch meta {
		case "EASY":
			fg = cSuccess
		case "MEDIUM":
			fg = cWarning
		default:
			fg = cError
		}
		metaStyle = lipgloss.NewStyle().Bold(true).Foreground(fg)
		if th.bg != nil {
			metaStyle = metaStyle.Background(th.bg)
		}
	}
	sub := ""
	if r.MetaSub != "" {
		sub = "  " + r.MetaSub
	}
	height := len(titleLines)
	if !hasRight {
		if height < 1 {
			height = 1
		}
	} else if fitsSingle {
		if height < 1 {
			height = 1
		}
	} else {
		if len(rightLines) > height {
			height = len(rightLines)
		}
		if height < 1 {
			height = 1
		}
	}
	blank := func(n int) string {
		if n < 0 {
			n = 0
		}
		return strings.Repeat(" ", n)
	}
	blankPre := blank(pw)
	lines := make([]string, 0, height)
	for li := 0; li < height; li++ {
		var tl string
		if li < len(titleLines) {
			tl = titleLines[li]
		} else {
			tl = blank(titleW)
			// wrapCell already pads to width; ensure exact.
			tl = PadToWidth("", titleW)
		}
		curPre := pre
		if li > 0 {
			curPre = blankPre
		}
		segs := []rowCell{
			{lead, th.edge},
			{curPre, th.main},
			{tl, th.main},
		}
		if hasRight {
			segs = append(segs, rowCell{"  ", th.gap})
			if fitsSingle {
				if li == 0 {
					segs = append(segs, rowCell{meta, metaStyle}, rowCell{sub, th.faint})
				} else {
					segs = append(segs, rowCell{PadToWidth("", rightW), th.gap})
				}
			} else {
				var rl string
				if li < len(rightLines) {
					rl = rightLines[li]
				} else {
					rl = PadToWidth("", rightW)
				}
				segs = append(segs, rowCell{rl, th.strong})
			}
		}
		used := 1 + pw + titleW
		if hasRight {
			used += 2 + rightW
		}
		trailW := w - used
		if trailW < 0 {
			trailW = 0
		}
		if trailW > 0 {
			segs = append(segs, rowCell{strings.Repeat(" ", trailW), th.gap})
		}
		lines = append(lines, paintCells(segs...))
	}
	return strings.Join(lines, "\n")
}

func prefixPlain(num, mark string) string {
	if mark == "" {
		mark = " "
	}
	if num != "" {
		return num + "  " + mark + "  "
	}
	return mark + "  "
}

// RowDivider is the thin horizontal separator spanning the list width.
func RowDivider(width int) string {
	if width < 4 {
		width = 4
	}
	return lipgloss.NewStyle().Foreground(cDimLine).Render(strings.Repeat("─", width))
}

// Divider renders a thin separator for panel sections (alias used by cards).
func Divider(width int) string { return RowDivider(width) }

// Row is the global Forge row alias: every navigable list item uses the
// same row language (number/index, padding, divider, selected state).
type Row = SelectableRow

// PanelTitle renders a consistent card title.
func PanelTitle(s string) string { return thPanelTitle.Render(s) }

// BentoPanel is the canonical Forge card: thin boxy border, consistent
// padding, title + divider + body, optional focus accent.
func BentoPanel(title, content string, width int, focused bool) string {
	return Panel(title, content, width, focused)
}

// Header renders the shell header block (logo + nav) at width w.
// Compat wrapper over the real ShellHeader layout (empty quote).
func Header(logo []string, navLine string, w int) string {
	logoContentW := 0
	for _, ln := range logo {
		if lw := lipgloss.Width(ln); lw > logoContentW {
			logoContentW = lw
		}
	}
	logoOuter, rightOuter := ComputeHeaderWidths(w, logoContentW)
	sh := ShellHeader{
		Logo: LogoRegion{Lines: logo, Width: logoOuter},
		Right: RightRegion{
			QuoteText: "",
			QuoteAttr: "",
			NavLine:   navLine,
			Width:     rightOuter,
		},
		Width: w,
	}
	return sh.View()
}

// Footer renders the single-row shell command bar at width w.
func Footer(line string, w int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(cBorder).
		Width(w).
		MaxHeight(2).
		Render(FitLine(TruncateToWidth(line, w-2), w-2))
}

// TodayTaskRow is one Today task table row as a reusable component:
// # | TRACK | AREA | TYPE | TASK | DURATION | STATUS with the global row
// treatment. Single component, full-width tint in every state.
type TodayTaskRow struct {
	Num      int
	Domain   string
	Area     string
	Typ      string
	Title    string
	Duration string
	Status   TaskStatus
	Selected bool
	Focused  bool
	Width    int
}

// View renders the row forced to Width cells, wrapping long cells instead
// of truncating. Row height equals the tallest wrapped cell; selection
// covers every physical line; no ellipsis is emitted.
func (r TodayTaskRow) View() string {
	w := r.Width
	if w < 30 {
		w = 30
	}
	numW, domW, areaW, typW, durW, stW := measureTodayCols(w)
	taskW := w - 1 - (numW + domW + areaW + typW + durW + stW) - 6*2
	if taskW < 4 {
		taskW = 4
	}
	numBase := fmt.Sprint(r.Num)
	domBase := string(r.Domain)
	areaBase := r.Area
	typBase := r.Typ
	titleBase := r.Title
	durBase := r.Duration
	stTxt := "TODO"
	switch r.Status {
	case TaskActive:
		stTxt = "ACTIVE"
	case TaskCompleted:
		stTxt = "COMPLETE"
	}
	// Wrap every potentially-long cell; height is the tallest.
	numLines := wrapCell(numBase, numW)
	domLines := wrapCell(domBase, domW)
	areaLines := wrapCell(areaBase, areaW)
	typLines := wrapCell(typBase, typW)
	titleLines := wrapCell(titleBase, taskW)
	durLines := wrapCell(durBase, durW)
	stLines := wrapCell(stTxt, stW)
	height := len(numLines)
	for _, n := range []int{len(domLines), len(areaLines), len(typLines), len(titleLines), len(durLines), len(stLines)} {
		if n > height {
			height = n
		}
	}
	if height < 1 {
		height = 1
	}
	at := func(lines []string, i, width int) string {
		if i < len(lines) {
			return lines[i]
		}
		return PadToWidth("", width)
	}
	th, edged := rowThemeFor(r.Selected, r.Focused)
	lead := " "
	if edged {
		lead = "▌"
	}
	lines := make([]string, 0, height)
	for li := 0; li < height; li++ {
		nl := at(numLines, li, numW)
		dl := at(domLines, li, domW)
		al := at(areaLines, li, areaW)
		tl := at(typLines, li, typW)
		tal := at(titleLines, li, taskW)
		du := at(durLines, li, durW)
		sl := at(stLines, li, stW)
		domCell := rowCell{dl, th.main}
		if r.Domain == "GOV" || r.Domain == "DSA" || r.Domain == "SWE" {
			domCell = th.domain(r.Domain, dl)
		}
		statusCell := statusCellFor(th, r.Status, sl)
		out := paintCells(
			rowCell{lead, th.edge},
			rowCell{nl, th.main},
			rowCell{"  ", th.gap},
			domCell,
			rowCell{"  ", th.gap},
			rowCell{al, th.main},
			rowCell{"  ", th.gap},
			rowCell{tl, th.main},
			rowCell{"  ", th.gap},
			rowCell{tal, th.main},
			rowCell{"  ", th.gap},
			rowCell{du, th.main},
			rowCell{"  ", th.gap},
			statusCell,
		)
		lines = append(lines, out)
	}
	return strings.Join(lines, "\n")
}

// LineCount returns the physical height of the row at its Width.
func (r TodayTaskRow) LineCount() int {
	w := r.Width
	if w < 30 {
		w = 30
	}
	numW, domW, areaW, typW, durW, stW := measureTodayCols(w)
	taskW := w - 1 - (numW + domW + areaW + typW + durW + stW) - 6*2
	if taskW < 4 {
		taskW = 4
	}
	n := len(wrapCell(fmt.Sprint(r.Num), numW))
	if m := len(wrapCell(string(r.Domain), domW)); m > n {
		n = m
	}
	if m := len(wrapCell(r.Area, areaW)); m > n {
		n = m
	}
	if m := len(wrapCell(r.Typ, typW)); m > n {
		n = m
	}
	if m := len(wrapCell(r.Title, taskW)); m > n {
		n = m
	}
	if m := len(wrapCell(r.Duration, durW)); m > n {
		n = m
	}
	stTxt := "TODO"
	switch r.Status {
	case TaskActive:
		stTxt = "ACTIVE"
	case TaskCompleted:
		stTxt = "COMPLETE"
	}
	if m := len(wrapCell(stTxt, stW)); m > n {
		n = m
	}
	if n < 1 {
		n = 1
	}
	return n
}

// wrapCell word-wraps s to width w on word boundaries, padding every line
// to exactly w. It never returns empty and never ellipsizes: overlong
// single words are hard-split. Widths are rune-based (cells are plain).
// Narrow widths (>=1) are supported without panic; no content is discarded.
func wrapCell(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{strings.Repeat(" ", w)}
	}
	var lines []string
	cur := ""
	flush := func() {
		if cur != "" {
			lines = append(lines, PadToWidth(cur, w))
			cur = ""
		}
	}
	for _, wd := range words {
		runes := []rune(wd)
		if len(runes) > w {
			// Hard-split pathological long words without ellipsis.
			flush()
			for len(runes) > w {
				lines = append(lines, string(runes[:w]))
				runes = runes[w:]
			}
			cur = string(runes)
			continue
		}
		if cur == "" {
			cur = wd
			continue
		}
		if len([]rune(cur))+1+len(runes) <= w {
			cur += " " + wd
			continue
		}
		flush()
		cur = wd
	}
	flush()
	if len(lines) == 0 {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return lines
}

// ---- Today task table: real aligned columns + thin dividers ----

// TodayRowView renders one Today task as aligned columns:
// # | TRACK | TASK | DURATION | STATUS with badges.
type TodayTaskTable struct {
	Tasks   []db.TodayTask
	Status  []TaskStatus
	Bundles []Bundle
	Cursor  int
	Offset  int // first visible logical row
	Width   int
	Lines   int // visible line budget (header + dividers + row lines)
	Focus   bool
}

// NewTodayTaskTable returns an empty table.
func NewTodayTaskTable() TodayTaskTable { return TodayTaskTable{Width: 60, Lines: 10} }

// SetData replaces rows with their resolved bundles for columns.
func (t *TodayTaskTable) SetData(tasks []db.TodayTask, status []TaskStatus, bundles []Bundle) {
	t.Tasks = tasks
	t.Status = status
	t.Bundles = bundles
	if t.Cursor >= len(tasks) {
		t.Cursor = len(tasks) - 1
	}
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	t.ensureVisible()
}

// SetSize sets content width and the visible line budget. The header costs
// 2 lines; every logical row costs 1 line per wrapped TASK line plus one
// divider, so multi-line rows consume the budget dynamically.
func (t *TodayTaskTable) SetSize(w, lines int) {
	if w < 30 {
		w = 30
	}
	if lines < 5 {
		lines = 5
	}
	t.Width = w
	t.Lines = lines
	t.ensureVisible()
}

// SetCursor moves selection.
func (t *TodayTaskTable) SetCursor(i int) {
	if len(t.Tasks) == 0 {
		return
	}
	t.Cursor = Clamp(i, 0, len(t.Tasks)-1)
	t.ensureVisible()
}

// MoveDown moves one row.
func (t *TodayTaskTable) MoveDown(n int) {
	if len(t.Tasks) == 0 {
		return
	}
	t.Cursor = Clamp(t.Cursor+n, 0, len(t.Tasks)-1)
	t.ensureVisible()
}

// MoveUp moves one row up.
func (t *TodayTaskTable) MoveUp(n int) {
	if len(t.Tasks) == 0 {
		return
	}
	t.Cursor = Clamp(t.Cursor-n, 0, len(t.Tasks)-1)
	t.ensureVisible()
}

// CursorPos returns the cursor.
func (t TodayTaskTable) CursorPos() int { return t.Cursor }

// lineCost returns the rendered line count of logical row i (tallest wrapped
// cell among AREA/TYPE/TASK, dividers excluded). Full content is preserved:
// overflow wraps and the row grows instead of truncating with ellipsis.
func (t *TodayTaskTable) lineCost(i int) int {
	if i < 0 || i >= len(t.Tasks) {
		return 1
	}
	numW, domW, areaW, typW, durW, stW := measureTodayCols(t.Width)
	taskW := t.Width - 1 - (numW + domW + areaW + typW + durW + stW) - 6*2
	if taskW < 4 {
		taskW = 4
	}
	var b Bundle
	if i < len(t.Bundles) {
		b = t.Bundles[i]
	}
	area, typ, label := TaskColumns(t.Tasks[i], b)
	n := len(wrapCell(area, areaW))
	if m := len(wrapCell(typ, typW)); m > n {
		n = m
	}
	if m := len(wrapCell(label, taskW)); m > n {
		n = m
	}
	// Short fixed columns (NUM/TRACK/DURATION/STATUS) stay single-line in
	// practice; include them for narrow-width safety (no panic, no clip).
	if m := len(wrapCell(fmt.Sprint(i+1), numW)); m > n {
		n = m
	}
	if m := len(wrapCell(string(t.Tasks[i].Domain), domW)); m > n {
		n = m
	}
	if m := len(wrapCell(FormatDurationLong(t.Tasks[i].PlannedSeconds), durW)); m > n {
		n = m
	}
	if n < 1 {
		n = 1
	}
	return n
}

func (t *TodayTaskTable) ensureVisible() {
	if t.Lines < 5 {
		t.Lines = 5
	}
	if len(t.Tasks) == 0 {
		t.Offset = 0
		return
	}
	if t.Cursor >= len(t.Tasks) {
		t.Cursor = len(t.Tasks) - 1
	}
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	if t.Cursor < t.Offset {
		t.Offset = t.Cursor
	}
	// Advance the offset until the cursor block fits the line budget.
	// Header (2) + row lines + one divider between consecutive rows.
	for {
		total := 2
		for i := t.Offset; i <= t.Cursor; i++ {
			total += t.lineCost(i)
			if i > t.Offset {
				total++ // divider
			}
		}
		if total <= t.Lines || t.Offset >= t.Cursor {
			break
		}
		t.Offset++
	}
	if t.Offset < 0 {
		t.Offset = 0
	}
}

// EnsureVisible is the shared scrolling contract for the Today task list:
// focus never sits outside the visible viewport.
func (t *TodayTaskTable) EnsureVisible() { t.ensureVisible() }

// View renders header + rows with dividers, forced to Width.
func (t *TodayTaskTable) View() string {
	return t.viewWithEnsure()
}

// viewWithEnsure re-syncs the viewport before rendering so the cursor is
// always visible even after a resize or width change (shared scrolling fix:
// focus never sits outside the visible viewport).
func (t *TodayTaskTable) viewWithEnsure() string {
	t.ensureVisible()
	w := t.Width
	if w < 30 {
		w = 30
	}
	if len(t.Tasks) == 0 {
		return EmptyState("No tasks.")
	}
	// Structured columns: # 3, TRACK 6, AREA 10, TYPE 8, TASK flex,
	// DURATION 8, STATUS 8; six 2-space gaps plus the 1-cell lead.
	numW, domW, areaW, typW, durW, stW := measureTodayCols(w)
	taskW := w - 1 - (numW + domW + areaW + typW + durW + stW) - 6*2
	if taskW < 4 {
		taskW = 4
	}
	header := FitLine(fmt.Sprintf(" %s  %s  %s  %s  %s  %s  %s",
		PadToWidth("#", numW),
		PadToWidth("TRACK", domW),
		PadToWidth("AREA", areaW),
		PadToWidth("TYPE", typW),
		PadToWidth("TASK", taskW),
		PadToWidth("DURATION", durW),
		PadToWidth("STATUS", stW),
	), w)
	var sb strings.Builder
	sb.WriteString(thTableHead.Render(header) + "\n")
	sb.WriteString(RowDivider(w) + "\n")
	used := 2
	end := t.Offset
	for i := t.Offset; i < len(t.Tasks); i++ {
		cost := t.lineCost(i)
		add := cost
		if i > t.Offset {
			add++ // divider before this row
		}
		if used+add > t.Lines && i > t.Offset {
			break
		}
		if i > t.Offset {
			sb.WriteString(RowDivider(w) + "\n")
			used++
		}
		for _, ln := range t.rowBlock(i, numW, domW, areaW, typW, taskW, durW, stW, w) {
			sb.WriteString(ln + "\n")
		}
		used += cost
		end = i + 1
	}
	if t.Offset > 0 {
		sb.WriteString(thHelp.Render("↑ more above") + "\n")
	}
	if end < len(t.Tasks) {
		sb.WriteString(thHelp.Render("↓ more below") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// measureTodayCols splits inner width w into the structured Today
// columns: # 3, TRACK 6, AREA 10, TYPE 8, DURATION 8, STATUS 8, TASK flexes.
// Fixed columns keep DURATION/STATUS vertically aligned; TASK takes the
// remaining width and wraps without pushing later columns.
func measureTodayCols(w int) (numW, domW, areaW, typW, durW, stW int) {
	return 3, 6, 10, 8, 8, 8
}

// joinPlus joins bundle item names the product way: A + B.
func joinPlus(xs []string) string { return strings.Join(xs, " + ") }

// TaskColumns derives AREA/TYPE/TASK display strings from a Today task and
// its resolved bundle. Pure UI formatting over existing structured fields
// (TodayTask domain/kind plus domain target metadata); no storage change.
//
// TYPE has exactly two values: LECTURE or PRACTICE. AREA is the subject/
// pattern/area, TASK is the description. DSA/SWE bundles map their legacy
// kinds to PRACTICE but display LECTURE when the task kind is LECTURE, so
// all tracks support both work types.
func TaskColumns(task db.TodayTask, b Bundle) (area, typ, label string) {
	switch {
	case b.Lecture != nil:
		return b.Lecture.Subject, "LECTURE", b.Lecture.Topic
	case len(b.Practice) > 0:
		topics := make([]string, 0, len(b.Practice))
		for _, pr := range b.Practice {
			topics = append(topics, pr.Topic)
		}
		return b.Practice[0].Subject, "PRACTICE", joinPlus(topics)
	case len(b.DSA) > 0:
		seen := map[string]bool{}
		var patterns, titles []string
		for _, d := range b.DSA {
			if !seen[d.Pattern] {
				seen[d.Pattern] = true
				patterns = append(patterns, d.Pattern)
			}
			titles = append(titles, d.Title)
		}
		area = ""
		if len(patterns) > 0 {
			area = patterns[0]
		}
		if len(patterns) > 1 {
			area = "Mixed"
		}
		return area, workTypeForTask(task), joinPlus(titles)
	case len(b.SWE) > 0:
		area := SWEDisplayName(b.SWE[0].Area)
		names := make([]string, 0, len(b.SWE))
		for _, sw := range b.SWE {
			names = append(names, SWEDisplayName(sw.Area))
		}
		if len(b.SWE) > 1 {
			seen := map[string]bool{}
			var uniq []string
			for _, sw := range b.SWE {
				disp := SWEDisplayName(sw.Area)
				if !seen[disp] {
					seen[disp] = true
					uniq = append(uniq, disp)
				}
			}
			if len(uniq) > 1 {
				area = "Mixed"
			}
		}
		return area, workTypeForTask(task), joinPlus(names)
	default:
		return "—", workTypeForTask(task), task.Title
	}
}

// workTypeForTask maps a task to its LECTURE/PRACTICE work type. GOV is
// decided by the bundle above; DSA/SWE legacy kinds (DSA/SWE) map to
// PRACTICE while explicit LECTURE/PRACTICE kinds display as-is, so every
// track supports both types without new TYPE values.
func workTypeForTask(task db.TodayTask) string {
	if task.Kind == db.SessionKindLecture {
		return "LECTURE"
	}
	if task.Kind == db.SessionKindPractice {
		return "PRACTICE"
	}
	// Legacy DSA/SWE kinds are practice-like work.
	return "PRACTICE"
}

// rowBlock renders logical row i as one or more physical lines. AREA, TYPE
// and TASK wrap independently; row height equals the tallest wrapped cell.
// DURATION and STATUS stay on the first line with blanks on continuation lines;
// fixed short columns (NUM/TRACK) behave the same. A selected row paints
// every line of the block, so the highlight covers the full expanded width
// and height as ONE component. j/k move between logical rows. No ellipsis.
func (t TodayTaskTable) rowBlock(i, numW, domW, areaW, typW, taskW, durW, stW, w int) []string {
	task := t.Tasks[i]
	st := TaskTodo
	if i < len(t.Status) {
		st = t.Status[i]
	}
	var b Bundle
	if i < len(t.Bundles) {
		b = t.Bundles[i]
	}
	area, typ, label := TaskColumns(task, b)
	// Wrap every variable cell; height is the tallest. Fixed short cells
	// are wrapped too for narrow-width safety (they stay one line normally).
	numLines := wrapCell(fmt.Sprint(i+1), numW)
	domLines := wrapCell(string(task.Domain), domW)
	areaLines := wrapCell(area, areaW)
	typLines := wrapCell(typ, typW)
	taskLines := wrapCell(label, taskW)
	durLines := wrapCell(FormatDurationLong(task.PlannedSeconds), durW)
	stTxt := "TODO"
	switch st {
	case TaskActive:
		stTxt = "ACTIVE"
	case TaskCompleted:
		stTxt = "COMPLETE"
	}
	stCellLines := wrapCell(stTxt, stW)
	height := len(numLines)
	for _, n := range []int{len(domLines), len(areaLines), len(typLines), len(taskLines), len(durLines), len(stCellLines)} {
		if n > height {
			height = n
		}
	}
	if height < 1 {
		height = 1
	}
	at := func(lines []string, idx, width int) string {
		if idx < len(lines) {
			return lines[idx]
		}
		return PadToWidth("", width)
	}
	selected := i == t.Cursor
	th, edged := rowThemeFor(selected, t.Focus && selected)
	lead := " "
	if edged {
		lead = "▌"
	}
	// First-line domain accent; continuation domain stays blank (aligned).
	firstDom := at(domLines, 0, domW)
	domCellFirst := rowCell{text: firstDom, style: th.main}
	if string(task.Domain) == "GOV" || string(task.Domain) == "DSA" || string(task.Domain) == "SWE" {
		domCellFirst = th.domain(string(task.Domain), firstDom)
	}
	firstStatus := at(stCellLines, 0, stW)
	statusFirst := statusCellFor(th, st, firstStatus)
	gap := rowCell{"  ", th.gap}
	blank := func(n int) rowCell {
		if n < 0 {
			n = 0
		}
		return rowCell{strings.Repeat(" ", n), th.gap}
	}
	lines := make([]string, 0, height)
	for li := 0; li < height; li++ {
		var segs []rowCell
		if li == 0 {
			segs = []rowCell{
				{lead, th.edge},
				{at(numLines, 0, numW), th.main},
				{"  ", th.gap},
				domCellFirst,
				{"  ", th.gap},
				{at(areaLines, 0, areaW), th.main},
				{"  ", th.gap},
				{at(typLines, 0, typW), th.main},
				{"  ", th.gap},
				{at(taskLines, 0, taskW), th.main},
				{"  ", th.gap},
				{at(durLines, 0, durW), th.main},
				{"  ", th.gap},
				statusFirst,
			}
		} else {
			// Continuation: wrapped AREA/TYPE/TASK text in place;
			// NUM/TRACK/DURATION/STATUS stay on the first line, aligned right.
			segs = []rowCell{
				{lead, th.edge},
				blank(numW),
				gap,
				blank(domW),
				gap,
				{at(areaLines, li, areaW), th.main},
				gap,
				{at(typLines, li, typW), th.main},
				gap,
				{at(taskLines, li, taskW), th.main},
				gap,
				blank(durW),
				gap,
				blank(stW),
			}
		}
		out := paintCells(segs...)
		lines = append(lines, out)
	}
	_ = gap
	return lines
}
