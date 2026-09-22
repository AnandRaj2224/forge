package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
type RowList struct {
	Items    []RowItem
	Cursor   int
	Offset   int // first visible index
	Width    int // content width for divider span
	Height   int // max visible rows (viewport)
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
}

// SetHeight sets the viewport row count.
func (r *RowList) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	r.Height = h
}

// FitHeight sizes the viewport to content up to maxLines visible lines.
// Each row plus its divider costs ~2 lines, so rows = (lines+1)/2.
func (r *RowList) FitHeight(maxLines int) {
	if maxLines < 3 {
		maxLines = 3
	}
	rows := (maxLines + 1) / 2
	if rows < 1 {
		rows = 1
	}
	if rows > len(r.Items) {
		rows = len(r.Items)
	}
	if rows < 1 {
		rows = 1
	}
	r.Height = rows
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
	if r.Cursor < r.Offset {
		r.Offset = r.Cursor
	}
	if r.Cursor >= r.Offset+r.Height {
		r.Offset = r.Cursor - r.Height + 1
	}
	if r.Offset < 0 {
		r.Offset = 0
	}
}

// View renders visible rows joined by thin dividers.
func (r RowList) View() string {
	if len(r.Items) == 0 {
		return EmptyState("Nothing here yet.")
	}
	h := r.Height
	if h < 1 {
		h = 1
	}
	end := r.Offset + h
	if end > len(r.Items) {
		end = len(r.Items)
	}
	var sb strings.Builder
	for i := r.Offset; i < end; i++ {
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
		}.View())
		if i < end-1 {
			sb.WriteString("\n" + RowDivider(r.Width) + "\n")
		} else {
			sb.WriteString("\n")
		}
	}
	// Scroll hints stay subtle and single-line.
	if r.Offset > 0 {
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

// SelectableRow is one structured row: number + mark + title plus an
// optional prominent right value (Meta) with a dimmer suffix (MetaSub,
// e.g. actual time plus planned time). Full-width tint in every state;
// the divider is rendered by the owning list.
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

// View renders the row forced to Width cells.
func (r SelectableRow) View() string {
	w := r.Width
	if w < 10 {
		w = 10
	}
	mark := r.Mark
	if mark == "" {
		mark = " "
	}
	pre := prefixPlain(r.Num, mark)
	pw := len([]rune(pre))
	inner := w - 2
	if inner < 4 {
		inner = 4
	}
	// Right block: prominent value + optional dimmer suffix, capped at half.
	budget := inner / 2
	meta := r.Meta
	sub := ""
	if r.MetaSub != "" {
		sub = "  " + r.MetaSub
	}
	rightW := lipgloss.Width(meta) + lipgloss.Width(sub)
	if rightW > budget {
		// Shrink the dimmer suffix first so the prominent value survives.
		subAllow := budget - min(lipgloss.Width(meta), budget)
		if subAllow < 0 {
			subAllow = 0
		}
		sub = TruncateToWidth(sub, subAllow)
		metaAllow := budget - lipgloss.Width(sub)
		if lipgloss.Width(meta) > metaAllow {
			meta = TruncateToWidth(meta, metaAllow)
		}
		rightW = lipgloss.Width(meta) + lipgloss.Width(sub)
	}
	titleW := w - 1 - pw - 2 - rightW
	if rightW == 0 {
		titleW = w - 1 - pw
	}
	if titleW < 1 {
		titleW = 1
	}
	title := PadToWidth(TruncateToWidth(r.Title, titleW), titleW)
	used := 1 + pw + titleW
	if rightW > 0 {
		used += 2 + rightW
	}
	trailW := w - used
	if trailW < 0 {
		trailW = 0
	}
	th, edged := rowThemeFor(r.Selected, r.Focused)
	lead := " "
	if edged {
		lead = "▌"
	}
	segs := []rowCell{
		{lead, th.edge},
		{pre, th.main},
		{title, th.main},
	}
	if rightW > 0 {
		segs = append(segs, rowCell{"  ", th.gap}, rowCell{meta, th.strong}, rowCell{sub, th.faint})
	}
	if trailW > 0 {
		segs = append(segs, rowCell{strings.Repeat(" ", trailW), th.gap})
	}
	out := paintCells(segs...)
	if lipgloss.Width(out) > w {
		out = ansi.Truncate(out, w, "…")
	}
	return out
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
// # | DOMAIN | AREA | TYPE | TASK | DUR | STATUS with the global row
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

// View renders the row forced to Width cells.
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
	num := PadToWidth(fmt.Sprint(r.Num), numW)
	dom := PadToWidth(TruncateToWidth(r.Domain, domW), domW)
	are := PadToWidth(TruncateToWidth(r.Area, areaW), areaW)
	ty := PadToWidth(TruncateToWidth(r.Typ, typW), typW)
	title := PadToWidth(TruncateToWidth(r.Title, taskW), taskW)
	dur := PadToWidth(TruncateToWidth(r.Duration, durW), durW)
	stTxt := "TODO"
	switch r.Status {
	case TaskActive:
		stTxt = "ACTIVE"
	case TaskCompleted:
		stTxt = "DONE"
	}
	stCell := PadToWidth(stTxt, stW)
	th, edged := rowThemeFor(r.Selected, r.Focused)
	lead := " "
	if edged {
		lead = "▌"
	}
	out := paintCells(
		rowCell{lead, th.edge},
		rowCell{num, th.main},
		rowCell{"  ", th.gap},
		th.domain(r.Domain, dom),
		rowCell{"  ", th.gap},
		rowCell{are, th.main},
		rowCell{"  ", th.gap},
		rowCell{ty, th.main},
		rowCell{"  ", th.gap},
		rowCell{title, th.main},
		rowCell{"  ", th.gap},
		rowCell{dur, th.main},
		rowCell{"  ", th.gap},
		rowCell{stCell, th.main},
	)
	if lipgloss.Width(out) > w {
		out = ansi.Truncate(out, w, "…")
	}
	return out
}

// wrapCell word-wraps s to width w on word boundaries, padding every line
// to exactly w. It never returns empty and never ellipsizes: overlong
// single words are hard-split. Widths are rune-based (cells are plain).
func wrapCell(s string, w int) []string {
	if w < 4 {
		w = 4
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
// # | DOMAIN | TASK | DUR | STATUS with badges.
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

// lineCost returns the rendered line count of logical row i (wrapped TASK
// lines, dividers excluded).
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
	_, _, label := TaskColumns(t.Tasks[i], b)
	n := len(wrapCell(label, taskW))
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

// View renders header + rows with dividers, forced to Width.
func (t TodayTaskTable) View() string {
	w := t.Width
	if w < 30 {
		w = 30
	}
	if len(t.Tasks) == 0 {
		return EmptyState("No tasks.")
	}
	// Structured columns: # 3, DOMAIN 6, AREA 10, TYPE 8, TASK flex,
	// DUR 6, STATUS 7; six 2-space gaps plus the 1-cell lead.
	numW, domW, areaW, typW, durW, stW := measureTodayCols(w)
	taskW := w - 1 - (numW + domW + areaW + typW + durW + stW) - 6*2
	if taskW < 4 {
		taskW = 4
	}
	header := FitLine(fmt.Sprintf(" %s  %s  %s  %s  %s  %s  %s",
		PadToWidth("#", numW),
		PadToWidth("DOMAIN", domW),
		PadToWidth("AREA", areaW),
		PadToWidth("TYPE", typW),
		PadToWidth("TASK", taskW),
		PadToWidth("DUR", durW),
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
// columns: # 3, DOMAIN 6, AREA 10, TYPE 8, DUR 6, STATUS 7, TASK flexes.
func measureTodayCols(w int) (numW, domW, areaW, typW, durW, stW int) {
	return 3, 6, 10, 8, 6, 7
}

// joinPlus joins bundle item names the product way: A + B.
func joinPlus(xs []string) string { return strings.Join(xs, " + ") }

// TaskColumns derives AREA/TYPE/TASK display strings from a Today task and
// its resolved bundle. Pure UI formatting over existing structured fields
// (TodayTask domain/kind plus domain target metadata); no storage change.
func TaskColumns(task db.TodayTask, b Bundle) (area, typ, label string) {
	switch {
	case b.Lecture != nil:
		return b.Lecture.Subject, "Lecture", b.Lecture.Topic
	case len(b.Practice) > 0:
		topics := make([]string, 0, len(b.Practice))
		for _, pr := range b.Practice {
			topics = append(topics, pr.Topic)
		}
		return b.Practice[0].Subject, "Practice", joinPlus(topics)
	case len(b.DSA) > 0:
		seen := map[string]bool{}
		var patterns, titles []string
		allNew, allRepeat := true, true
		for _, d := range b.DSA {
			if !seen[d.Pattern] {
				seen[d.Pattern] = true
				patterns = append(patterns, d.Pattern)
			}
			titles = append(titles, d.Title)
			if d.Mode != db.DSAModeNew {
				allNew = false
			}
			if d.Mode != db.DSAModeRepeat {
				allRepeat = false
			}
		}
		area = patterns[0]
		if len(patterns) > 1 {
			area = "Mixed"
		}
		typ = "Mixed"
		if allNew {
			typ = "New"
		} else if allRepeat {
			typ = "Repeat"
		}
		return area, typ, joinPlus(titles)
	case len(b.SWE) > 0:
		seen := map[string]bool{}
		var areas, names []string
		allTask, anyTask := true, false
		for _, sw := range b.SWE {
			if !seen[sw.Area] {
				seen[sw.Area] = true
				areas = append(areas, sw.Area)
			}
			if sw.TaskID != nil {
				anyTask = true
				names = append(names, sw.Task)
			} else {
				allTask = false
				names = append(names, sw.Area)
			}
		}
		area = areas[0]
		if len(areas) > 1 {
			area = "Mixed"
		}
		typ = "Mixed"
		if allTask {
			typ = "Task"
		} else if !anyTask {
			typ = "Area"
		}
		return area, typ, joinPlus(names)
	default:
		return "—", "—", task.Title
	}
}

// rowBlock renders logical row i as one or more physical lines. The TASK
// column word-wraps; DUR and STATUS stay right-aligned on the first line;
// continuation lines carry the wrapped task text. A selected row paints
// every line of the block, so the highlight covers the full expanded width
// and height as ONE component. j/k move between logical rows.
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
	// Plain fixed-width cells; color is applied per segment below so every
	// state paints identical column geometry edge to edge.
	num := PadToWidth(fmt.Sprint(i+1), numW)
	dom := PadToWidth(TruncateToWidth(string(task.Domain), domW), domW)
	are := PadToWidth(TruncateToWidth(area, areaW), areaW)
	ty := PadToWidth(TruncateToWidth(typ, typW), typW)
	tasks := wrapCell(label, taskW)
	dur := PadToWidth(TruncateToWidth(FormatDurationLong(task.PlannedSeconds), durW), durW)
	stTxt := "TODO"
	switch st {
	case TaskActive:
		stTxt = "ACTIVE"
	case TaskCompleted:
		stTxt = "DONE"
	}
	stCell := PadToWidth(stTxt, stW)
	selected := i == t.Cursor
	th, edged := rowThemeFor(selected, t.Focus && selected)
	lead := " "
	if edged {
		lead = "▌"
	}
	domCell := rowCell{text: dom, style: th.main}
	if string(task.Domain) == "GOV" || string(task.Domain) == "DSA" || string(task.Domain) == "SWE" {
		domCell = th.domain(string(task.Domain), dom)
	}
	gap := rowCell{"  ", th.gap}
	blank := func(n int) rowCell {
		if n < 0 {
			n = 0
		}
		return rowCell{strings.Repeat(" ", n), th.gap}
	}
	lines := make([]string, 0, len(tasks))
	for li, tl := range tasks {
		var segs []rowCell
		if li == 0 {
			segs = []rowCell{
				{lead, th.edge},
				{num, th.main},
				{"  ", th.gap},
				domCell,
				{"  ", th.gap},
				{are, th.main},
				{"  ", th.gap},
				{ty, th.main},
				{"  ", th.gap},
				{tl, th.main},
				{"  ", th.gap},
				{dur, th.main},
				{"  ", th.gap},
				{stCell, th.main},
			}
		} else {
			// Continuation: wrapped task text under the TASK column;
			// DUR/STATUS stay on the first line, aligned right.
			segs = []rowCell{
				{lead, th.edge},
				blank(numW),
				gap,
				blank(domW),
				gap,
				blank(areaW),
				gap,
				blank(typW),
				gap,
				{tl, th.main},
				gap,
				blank(durW),
				gap,
				blank(stW),
			}
		}
		out := paintCells(segs...)
		if lipgloss.Width(out) > w {
			out = ansi.Truncate(out, w, "…")
		}
		lines = append(lines, out)
	}
	_ = gap
	return lines
}
