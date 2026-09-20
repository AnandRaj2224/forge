package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Focus zones inside the Today workspace.
const (
	tzTasks = iota
	tzActive
	tzRight
)

// TodayScreen is the three-part workspace: task list | active session /
// selected-task-or-result. Selection drives the detail panel; no separate
// detail screen exists.
type TodayScreen struct {
	app      *App
	tbl      table.Model
	tasks    []db.TodayTask
	status   []TaskStatus
	bundles  []Bundle
	snap     *db.TodaySnapshot
	active   *db.Session
	zone     int
	stopBtn  Button
	startBtn Button
	right    string // "detail" or "result"
	result   *ResultForm
	err      string
	ready    bool
}

func NewTodayScreen(a *App) *TodayScreen {
	cols := []table.Column{
		{Title: "#", Width: 3},
		{Title: "DOMAIN", Width: 6},
		{Title: "TASK", Width: 30},
		{Title: "DUR", Width: 5},
		{Title: "STATUS", Width: 7},
	}
	t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(12))
	t.SetStyles(tableStyles())
	return &TodayScreen{app: a, tbl: t, right: "detail",
		stopBtn: Button{Label: "STOP"}, startBtn: Button{Label: "START SESSION"}}
}

func tableStyles() table.Styles {
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).Foreground(lipgloss.Color("12"))
	st.Selected = st.Selected.Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("24"))
	st.Cell = st.Cell.Foreground(lipgloss.Color("7"))
	return st
}

func (s *TodayScreen) latestSnapshot(ctx context.Context) *db.TodaySnapshot {
	rows, err := s.app.DB.QueryContext(ctx, `SELECT local_date FROM today_snapshots ORDER BY local_date DESC LIMIT 1`)
	if err != nil {
		return nil
	}
	var d string
	if rows.Next() {
		_ = rows.Scan(&d)
	}
	_ = rows.Close()
	if d == "" {
		return nil
	}
	snap, err := runtime.GetLatestTodaySnapshot(ctx, s.app.DB, d)
	if err != nil {
		return nil
	}
	return &snap
}

func (s *TodayScreen) Reload() {
	ctx := context.Background()
	s.err, s.ready = "", false
	snap := s.latestSnapshot(ctx)
	if snap == nil {
		s.err = "no today snapshots"
		return
	}
	keepCursor := ""
	if len(s.tasks) > 0 && s.tbl.Cursor() < len(s.tasks) {
		keepCursor = string(s.tasks[s.tbl.Cursor()].ID)
	}
	s.snap = snap
	tasks, err := runtime.GetTodayTasks(ctx, s.app.DB, snap.ID)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.tasks = tasks
	s.status = make([]TaskStatus, len(tasks))
	s.bundles = make([]Bundle, len(tasks))
	rows := make([]table.Row, 0, len(tasks))
	for i, t := range tasks {
		st := ComputeTaskStatus(ctx, s.app.DB, t.ID)
		s.status[i] = st
		s.bundles[i] = ResolveBundle(ctx, s.app.DB, t)
		rows = append(rows, TodayTaskRow(i, t, st))
	}
	s.tbl.SetRows(rows)
	if keepCursor != "" {
		for i, t := range tasks {
			if string(t.ID) == keepCursor {
				s.tbl.SetCursor(i)
			}
		}
	}
	if sess, err := runtime.GetActiveSession(ctx, s.app.DB); err == nil {
		c := sess
		s.active = &c
	} else {
		s.active = nil
	}
	s.syncButtons()
	s.ready = true
}

func (s *TodayScreen) selected() (db.TodayTask, TaskStatus, Bundle, bool) {
	if len(s.tasks) == 0 || s.tbl.Cursor() >= len(s.tasks) {
		return db.TodayTask{}, TaskTodo, Bundle{}, false
	}
	i := s.tbl.Cursor()
	return s.tasks[i], s.status[i], s.bundles[i], true
}

func (s *TodayScreen) syncButtons() {
	_, aerr := runtime.GetActiveSession(context.Background(), s.app.DB)
	hasActive := aerr == nil
	s.stopBtn.Disabled = !hasActive
	s.stopBtn.Focused = s.zone == tzActive && hasActive
	_, st, _, ok := s.selected()
	s.startBtn.Disabled = hasActive || !ok || st == TaskCompleted || s.right != "detail"
	s.startBtn.Focused = s.zone == tzRight && s.right == "detail" && !s.startBtn.Disabled
}

// startSelected creates the persisted ACTIVE session for the selected task.
func (s *TodayScreen) startSelected() {
	_, _, _, ok := s.selected()
	if !ok {
		return
	}
	task := s.tasks[s.tbl.Cursor()]
	ctx := context.Background()
	if _, err := runtime.GetActiveSession(ctx, s.app.DB); err == nil {
		s.app.err = "another session is active — stop it first (x)"
		return
	}
	now := time.Now().UTC()
	sid := db.ID(fmt.Sprintf("sess-tui-%d", now.UnixNano()))
	var target *string
	if task.TargetJSON != nil {
		v := *task.TargetJSON
		target = &v
	}
	sess := db.Session{ID: sid, TodayTaskID: task.ID, Domain: task.Domain, Kind: task.Kind,
		TitleSnapshot: task.Title, TargetSnapshotJSON: target,
		PlannedSeconds: task.PlannedSeconds, StartedAt: now, Status: db.SessionStatusActive}
	if err := runtime.CreateSession(ctx, s.app.DB, sess); err != nil {
		s.app.err = err.Error()
		return
	}
	s.right = "detail"
	s.Reload()
	s.app.refreshActiveIndicator()
}

// stopActive stops the global session and swaps right-bottom to the form.
func (s *TodayScreen) stopActive() {
	ctx := context.Background()
	sess, err := runtime.GetActiveSession(ctx, s.app.DB)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	if err := runtime.StopSession(ctx, s.app.DB, sess.ID, now); err != nil {
		s.app.err = err.Error()
		return
	}
	task, terr := runtime.GetTodayTask(ctx, s.app.DB, sess.TodayTaskID)
	if terr != nil {
		task = db.TodayTask{ID: sess.TodayTaskID, Domain: sess.Domain, Kind: sess.Kind, Title: sess.TitleSnapshot, PlannedSeconds: sess.PlannedSeconds, TargetJSON: sess.TargetSnapshotJSON}
	}
	if s.result == nil {
		s.result = NewResultForm(s.app)
	}
	s.result.Reload(string(sess.ID), task)
	s.right = "result"
	s.zone = tzRight
	s.Reload()
	s.app.refreshActiveIndicator()
}

func (s *TodayScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		// While the in-place result form owns the right panel, it owns
		// Tab/Up/Down/Space/typing. Esc backs out (handled by the app).
		if s.zone == tzRight && s.right == "result" && s.result != nil && k.String() != "x" {
			if k.String() == "enter" {
				if s.result.focus == s.result.nFocus()-1 {
					if ok, _ := s.result.TrySave(); ok {
						s.right = "detail"
						s.zone = tzTasks
						s.Reload()
						s.app.refreshActiveIndicator()
					}
				} else if cmd, handled := s.result.HandleKey(k); handled {
					return cmd
				}
				return nil
			}
			if cmd, handled := s.result.HandleKey(k); handled {
				return cmd
			}
			// Unhandled keys (e.g. s) are ignored while recording evidence.
			return nil
		}
		switch k.String() {
		case "tab":
			s.zone = (s.zone + 1) % 3
			if s.zone == tzActive && s.active == nil {
				s.zone = tzRight
			}
			if s.result == nil && s.right == "result" {
				s.right = "detail"
			}
			s.syncButtons()
			if s.right == "result" && s.result != nil {
				s.result.updateFocusVisual()
			}
			return nil
		case "s":
			if s.right == "detail" {
				s.startSelected()
			}
			return nil
		case "x":
			s.stopActive()
			return nil
		case "enter":
			return s.pressEnter()
		case "j", "down", "k", "up":
			if s.zone == tzTasks {
				if k.String() == "j" || k.String() == "down" {
					s.tbl.MoveDown(1)
				} else {
					s.tbl.MoveUp(1)
				}
				s.syncButtons()
				return nil
			}
			if s.zone == tzRight && s.right == "result" && s.result != nil {
				if cmd, handled := s.result.HandleKey(k); handled {
					return cmd
				}
			}
			return nil
		}
		// Typing into result inputs.
		if s.zone == tzRight && s.right == "result" && s.result != nil {
			if k, ok := msg.(tea.KeyMsg); ok {
				ks := k.String()
				if ks != "tab" && ks != "shift+tab" {
					if cmd, handled := s.result.HandleKey(k); handled {
						return cmd
					}
				}
			}
		}
	}
	if s.zone == tzTasks {
		var cmd tea.Cmd
		prev := s.tbl.Cursor()
		s.tbl, cmd = s.tbl.Update(msg)
		if s.tbl.Cursor() != prev {
			s.syncButtons()
		}
		_ = prev
		return cmd
	}
	return nil
}

func (s *TodayScreen) pressEnter() tea.Cmd {
	switch s.zone {
	case tzActive:
		s.stopActive()
	case tzRight:
		if s.right == "detail" {
			s.startSelected()
		} else if s.result != nil && s.result.focus == s.result.nFocus()-1 {
			// Enter on SAVE attempts the save; widget keys are handled
			// by the form itself while it owns focus.
			if ok, _ := s.result.TrySave(); ok {
				s.right = "detail"
				s.zone = tzTasks
				s.Reload()
				s.app.refreshActiveIndicator()
			}
		}
	case tzTasks:
		// Selection already drives detail; Enter jumps focus right.
		s.zone = tzRight
		s.syncButtons()
	}
	return nil
}

// Viewport-proportioned three-part bento.
func (s *TodayScreen) View() string {
	w := s.app.width
	h := s.app.height
	if w < 60 {
		w = 60
	}
	leftW, rightW := splitW(w, 58, 30, 26)
	contentH := h - 7 // header(2) + title(1) + footer(2) + margins
	if contentH < 10 {
		contentH = 10
	}
	// Task column flexes; headers never wrap because the fixed columns
	// exactly fit their longest values (#/6/5/6 + spacing).
	taskCol := (leftW - 4) - (3 + 6 + 5 + 7) - 8
	if taskCol < 6 {
		taskCol = 6
	}
	s.tbl.SetColumns([]table.Column{
		{Title: "#", Width: 3},
		{Title: "DOMAIN", Width: 6},
		{Title: "TASK", Width: taskCol},
		{Title: "DUR", Width: 5},
		{Title: "STATUS", Width: 7},
	})
	s.tbl.SetHeight(contentH - 6)
	s.tbl.SetWidth(leftW - 4)

	title := "TODAY"
	if s.snap != nil {
		title = fmt.Sprintf("TODAY  ·  %s  ·  revision %d", s.snap.LocalDate, s.snap.Revision)
	}
	titleBar := thScreenTitle.Render(title)

	left := Panel("TASKS", s.tbl.View(), leftW, s.zone == tzTasks)
	rightH := (contentH - 1) / 2
	activePanel := Panel("ACTIVE SESSION", s.activeView(rightW), rightW, s.zone == tzActive)
	rightBottom := s.rightPanel(rightW, contentH-rightH-1)
	rightCol := lipgloss.JoinVertical(lipgloss.Left, activePanel, rightBottom)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left+" ", rightCol)
	if s.err != "" {
		body += "\n" + thError.Render(s.err)
	}
	return titleBar + "\n" + body
}

func (s *TodayScreen) rightPanel(w, _ int) string {
	if s.right == "result" && s.result != nil && s.taskOfResult() {
		return Panel("RESULT — EVIDENCE", s.result.View(), w, s.zone == tzRight)
	}
	return Panel("SELECTED TASK", s.detailView(), w, s.zone == tzRight)
}

func (s *TodayScreen) taskOfResult() bool { return s.result != nil && s.result.task != nil }

func (s *TodayScreen) activeView(_ int) string {
	if s.active == nil {
		return thMeta.Render("No active session.") + "\n\n" + thHelp.Render("Select a task, then START.")
	}
	el := time.Since(s.active.StartedAt)
	if el < 0 {
		el = 0
	}
	ratio := 0.0
	if s.active.PlannedSeconds > 0 {
		ratio = float64(el.Seconds()) / float64(s.active.PlannedSeconds)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s  ·  %s\n", DomainBadge(string(s.active.Domain)), thPrimary.Render(string(s.active.Kind)))
	sb.WriteString(thPrimary.Render(s.active.TitleSnapshot) + "\n\n")
	sb.WriteString(thScreenTitle.Render(FormatHMS(el)) + "\n")
	sb.WriteString(ProgressBar{Ratio: ratio, Width: 22, ShowPct: true}.View() + "\n\n")
	sb.WriteString(MetaRow("Planned", FormatDuration(s.active.PlannedSeconds), 8) + "\n")
	sb.WriteString(MetaRow("Started", s.active.StartedAt.Local().Format("15:04"), 8) + "\n\n")
	sb.WriteString(s.stopBtn.View())
	return sb.String()
}

func (s *TodayScreen) detailView() string {
	_, st, b, ok := s.selected()
	if !ok {
		return thMeta.Render("No tasks.")
	}
	t := s.tasks[s.tbl.Cursor()]
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s  ·  %s\n\n", DomainBadge(string(t.Domain)), thPrimary.Render(string(t.Kind)))
	switch {
	case b.Lecture != nil:
		sb.WriteString(thSectionHead.Render(b.Lecture.Subject) + "\n")
		sb.WriteString(thPrimary.Render(b.Lecture.Topic) + "\n")
		sb.WriteString(thMeta.Render(b.Lecture.Lecture) + "\n")
	case len(b.Practice) > 0:
		subj := ""
		if len(b.Practice) > 0 {
			subj = b.Practice[0].Subject
		}
		sb.WriteString(thSectionHead.Render(subj) + "\n")
		sb.WriteString(SectionHeader(fmt.Sprintf("TOPICS (%d)", len(b.Practice))) + "\n")
		for _, p := range b.Practice {
			sb.WriteString("  " + thPrimary.Render(p.Topic) + "\n")
		}
	case len(b.DSA) > 0:
		sb.WriteString(MetaRow("Planned", FormatDuration(t.PlannedSeconds), 8) + "\n")
		var repeats, news []DSAItem
		for _, d := range b.DSA {
			if d.Mode == db.DSAModeNew {
				news = append(news, d)
			} else {
				repeats = append(repeats, d)
			}
		}
		if len(repeats) > 0 {
			sb.WriteString(SectionHeader("REPEAT") + "\n")
			for _, d := range repeats {
				sb.WriteString("  " + thPrimary.Render(d.Title) + "\n")
			}
		}
		if len(news) > 0 {
			sb.WriteString(SectionHeader("NEW") + "\n")
			for _, d := range news {
				sb.WriteString("  " + thPrimary.Render(d.Title) + "\n")
			}
		}
		sb.WriteString("\n" + MetaRow("Status", string(st), 8) + "\n\n")
		sb.WriteString(s.startBtn.View())
		if s.startBtn.Disabled {
			sb.WriteString("\n" + thHelp.Render("Start unavailable while a session runs."))
		}
		return sb.String()
	case len(b.SWE) > 0:
		sb.WriteString(MetaRow("Planned", FormatDuration(t.PlannedSeconds), 8) + "\n")
		lastArea := ""
		for _, sw := range b.SWE {
			if sw.Area != lastArea {
				sb.WriteString(thSectionHead.Render(sw.Area) + "\n")
				lastArea = sw.Area
			}
			if sw.Task != "" {
				sb.WriteString("  " + thPrimary.Render(sw.Task) + "\n")
			}
		}
		sb.WriteString("\n" + MetaRow("Status", string(st), 8) + "\n\n")
		sb.WriteString(s.startBtn.View())
		if s.startBtn.Disabled {
			sb.WriteString("\n" + thHelp.Render("Start unavailable while a session runs."))
		}
		return sb.String()
	default:
		sb.WriteString(thPrimary.Render(t.Title) + "\n")
	}
	sb.WriteString("\n" + MetaRow("Planned", FormatDuration(t.PlannedSeconds), 8) + "\n")
	sb.WriteString(MetaRow("Status", string(st), 8) + "\n\n")
	sb.WriteString(s.startBtn.View())
	if s.startBtn.Disabled {
		sb.WriteString("\n" + thHelp.Render("Start unavailable while a session runs."))
	}
	_ = st
	return sb.String()
}

func (s *TodayScreen) Hints() string { return "Tab Focus   s Start   x Stop" }

// onBack steps the right-bottom panel from result back to detail.
func (s *TodayScreen) onBack() bool {
	if s.right == "result" {
		s.right = "detail"
		s.zone = tzTasks
		s.Reload()
		return true
	}
	if s.zone != tzTasks {
		s.zone = tzTasks
		s.syncButtons()
		return true
	}
	return false
}
