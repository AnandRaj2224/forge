package tui

import (
	"fmt"
	"strings"

	db "forge/internal/db"
	"github.com/charmbracelet/lipgloss"
)

// Forge row system: every navigable list item is a structured row separated
// by a thin horizontal divider. Selection is a background treatment owned by
// the row itself, never a bare ">" prefix.

// RowItem is one selectable row in a Forge list.
type RowItem struct {
	ID    string
	Title string
	Meta  string // secondary text shown right-aligned or dimmed
	Mark  string // leading glyph: ✓ ○ ● etc (empty = none)
}

// RowList is the Forge navigable list: real component with cursor, viewport
// scrolling, thin dividers, and polished selected states.
type RowList struct {
	Items   []RowItem
	Cursor  int
	Offset  int // first visible index
	Width   int // content width for divider span
	Height  int // max visible rows (viewport)
	Focused bool
}

// NewRowList returns an empty Forge list.
func NewRowList() RowList { return RowList{Width: 28, Height: 12} }

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
		sb.WriteString(SelectableRow{
			Mark:     r.Items[i].Mark,
			Title:    r.Items[i].Title,
			Meta:     r.Items[i].Meta,
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

// SelectableRow is one structured row: mark + title + meta, with a selected
// background treatment. The divider is rendered by the owning list.
type SelectableRow struct {
	Mark     string
	Title    string
	Meta     string
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
	inner := w - 2 // 1-cell padding each side
	if inner < 4 {
		inner = 4
	}
	mark := r.Mark
	if mark == "" {
		mark = " "
	}
	// Title/meta share the inner width: meta right-aligned when present.
	title := r.Title
	meta := r.Meta
	var line string
	if meta == "" {
		line = mark + "  " + TruncateToWidth(title, inner-3)
	} else {
		metaW := lipgloss.Width(meta)
		if metaW > inner/2 {
			meta = TruncateToWidth(meta, inner/2)
			metaW = lipgloss.Width(meta)
		}
		titleW := inner - 3 - metaW - 2
		if titleW < 4 {
			titleW = 4
		}
		line = mark + "  " + PadToWidth(TruncateToWidth(title, titleW), titleW) + "  " + meta
	}
	line = FitLine(" "+line, w)
	if r.Selected {
		if r.Focused {
			return thRowSelected.Render(line)
		}
		return thRowCursor.Render(line)
	}
	return thRowNormal.Render(line)
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

// PanelTitle renders a consistent card title.
func PanelTitle(s string) string { return thPanelTitle.Render(s) }

// BentoPanel is the canonical Forge card: thin boxy border, consistent
// padding, title + divider + body, optional focus accent.
func BentoPanel(title, content string, width int, focused bool) string {
	return Panel(title, content, width, focused)
}

// ---- Today task table: real aligned columns + thin dividers ----

// TodayRowView renders one Today task as aligned columns:
// # | DOMAIN | TASK | DUR | STATUS with badges.
type TodayTaskTable struct {
	Tasks  []db.TodayTask
	Status []TaskStatus
	Cursor int
	Offset int
	Width  int
	Height int // visible rows
	Focus  bool
}

// NewTodayTaskTable returns an empty table.
func NewTodayTaskTable() TodayTaskTable { return TodayTaskTable{Width: 60, Height: 10} }

// SetData replaces rows.
func (t *TodayTaskTable) SetData(tasks []db.TodayTask, status []TaskStatus) {
	t.Tasks = tasks
	t.Status = status
	if t.Cursor >= len(tasks) {
		t.Cursor = len(tasks) - 1
	}
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	t.ensureVisible()
}

// SetSize sets content width and viewport height in LINES.
// Header + divider cost 2 lines; each row + divider costs ~2 lines.
func (t *TodayTaskTable) SetSize(w, lines int) {
	if w < 30 {
		w = 30
	}
	if lines < 5 {
		lines = 5
	}
	t.Width = w
	rows := (lines - 2 + 1) / 2
	if rows < 2 {
		rows = 2
	}
	t.Height = rows
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

func (t *TodayTaskTable) ensureVisible() {
	if t.Height < 1 {
		t.Height = 1
	}
	if t.Cursor < t.Offset {
		t.Offset = t.Cursor
	}
	if t.Cursor >= t.Offset+t.Height {
		t.Offset = t.Cursor - t.Height + 1
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
	// Fixed columns: # 3, DOMAIN 6, DUR 5, STATUS 7; TASK flexes.
	const numW, domW, durW, stW = 3, 6, 6, 7
	gaps := 4 * 2 // two-space gaps between 5 columns
	taskW := w - 2 - (numW + domW + durW + stW) - gaps
	if taskW < 8 {
		taskW = 8
	}
	header := FitLine(fmt.Sprintf(" %s  %s  %s  %s  %s",
		PadToWidth("#", numW),
		PadToWidth("DOMAIN", domW),
		PadToWidth("TASK", taskW),
		PadToWidth("DUR", durW),
		PadToWidth("STATUS", stW),
	), w)
	var sb strings.Builder
	sb.WriteString(thTableHead.Render(header) + "\n")
	sb.WriteString(RowDivider(w) + "\n")
	end := t.Offset + t.Height
	if end > len(t.Tasks) {
		end = len(t.Tasks)
	}
	for i := t.Offset; i < end; i++ {
		sb.WriteString(t.rowLine(i, numW, domW, taskW, durW, stW, w))
		if i < end-1 {
			sb.WriteString("\n" + RowDivider(w) + "\n")
		} else {
			sb.WriteString("\n")
		}
	}
	if t.Offset > 0 {
		sb.WriteString(thHelp.Render("↑ more above") + "\n")
	}
	if end < len(t.Tasks) {
		sb.WriteString(thHelp.Render("↓ more below") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (t TodayTaskTable) rowLine(i, numW, domW, taskW, durW, stW, w int) string {
	task := t.Tasks[i]
	st := TaskTodo
	if i < len(t.Status) {
		st = t.Status[i]
	}
	num := PadToWidth(fmt.Sprint(i+1), numW)
	dom := PadToWidth(TruncateToWidth(string(task.Domain), domW), domW)
	title := PadToWidth(TruncateToWidth(task.Title, taskW), taskW)
	dur := PadToWidth(TruncateToWidth(FormatDuration(task.PlannedSeconds), durW), durW)
	stTxt := "TODO"
	switch st {
	case TaskActive:
		stTxt = "ACTIVE"
	case TaskCompleted:
		stTxt = "DONE"
	}
	stCell := PadToWidth(stTxt, stW)
	line := FitLine(fmt.Sprintf(" %s  %s  %s  %s  %s", num, dom, title, dur, stCell), w)
	selected := i == t.Cursor
	if selected {
		// Status tint keeps semantic color inside the selected row.
		var stStyled string
		switch st {
		case TaskActive:
			stStyled = thRunning.Render(stCell)
		case TaskCompleted:
			stStyled = thSuccess.Render(stCell)
		default:
			stStyled = thTodo.Render(stCell)
		}
		_ = stStyled
		if t.Focus {
			return thRowSelected.Render(line)
		}
		return thRowCursor.Render(line)
	}
	// Unselected rows: domain + status get quiet semantic color via
	// surrounding styles is not width-safe, so keep plain with markers.
	return thRowNormal.Render(line)
}
