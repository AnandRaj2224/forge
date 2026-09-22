package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// Focus zones inside the Today workspace.
const (
	tzTasks = iota
	tzActive
	tzRight
)

// TodayScreen is the three-part bento workspace: task list | active session /
// selected-task-or-result. Selection drives the detail panel; no separate
// detail screen exists. Detail and result panes scroll independently via
// shared ScrollViewports whenever they overflow.
type TodayScreen struct {
	app          *App
	tbl          TodayTaskTable
	tasks        []db.TodayTask
	status       []TaskStatus
	bundles      []Bundle
	snap         *db.TodaySnapshot
	active       *db.Session
	zone         int
	stopBtn      Button
	startBtn     Button
	right        string // "detail" or "result"
	result       *ResultForm
	detailScroll ScrollViewport
	resultScroll ScrollViewport
	notice       string // small neutral contextual note (never a red error banner)
	err          string
	ready        bool
}

func NewTodayScreen(a *App) *TodayScreen {
	return &TodayScreen{app: a, tbl: NewTodayTaskTable(), right: "detail",
		stopBtn: Button{Label: "STOP SESSION"}, startBtn: Button{Label: "START SESSION"}}
}

func (s *TodayScreen) latestSnapshot(ctx context.Context) *db.TodaySnapshot {
	// Prefer today's snapshot in the Forge calendar (Asia/Kolkata midnight),
	// so future planned History days never hijack the home workspace; fall
	// back to the latest snapshot on/before today, then to the global latest.
	cal := engine.DefaultCalendar()
	todayStr := cal.LocalDate(time.Now())
	if snap, err := runtime.GetLatestTodaySnapshot(ctx, s.app.DB, todayStr); err == nil {
		return &snap
	}
	var d string
	rows, err := s.app.DB.QueryContext(ctx, `SELECT local_date FROM today_snapshots WHERE local_date <= ? ORDER BY local_date DESC LIMIT 1`, todayStr)
	if err == nil {
		if rows.Next() {
			_ = rows.Scan(&d)
		}
		_ = rows.Close()
		if d != "" {
			if snap, err := runtime.GetLatestTodaySnapshot(ctx, s.app.DB, d); err == nil {
				return &snap
			}
		}
	}
	rows2, err := s.app.DB.QueryContext(ctx, `SELECT local_date FROM today_snapshots ORDER BY local_date DESC LIMIT 1`)
	if err != nil {
		return nil
	}
	var d2 string
	if rows2.Next() {
		_ = rows2.Scan(&d2)
	}
	_ = rows2.Close()
	if d2 == "" {
		return nil
	}
	snap, err := runtime.GetLatestTodaySnapshot(ctx, s.app.DB, d2)
	if err != nil {
		return nil
	}
	return &snap
}

func (s *TodayScreen) Reload() {
	ctx := context.Background()
	s.err, s.ready = "", false
	s.notice = ""
	snap := s.latestSnapshot(ctx)
	if snap == nil {
		// First run on a fresh DB: generate today's snapshot exactly once
		// via EnsureToday (no-op when a snapshot already exists, so
		// routine reloads never create revisions).
		cal := engine.DefaultCalendar()
		if nsnap, _, err := engine.EnsureToday(ctx, s.app.DB, cal.LocalDate(time.Now()), time.Now()); err == nil {
			snap = &nsnap
		}
	}
	if snap == nil {
		s.err = "no today snapshots"
		return
	}
	keepCursor := ""
	if len(s.tasks) > 0 && s.tbl.CursorPos() < len(s.tasks) {
		keepCursor = string(s.tasks[s.tbl.CursorPos()].ID)
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
	for i, t := range tasks {
		// Authoritative engine evaluation; DB failures surface as a
		// data-error state instead of a silent TODO fallback.
		ev, err := engine.EvaluateTodayTask(ctx, s.app.DB, t.ID)
		if err != nil {
			s.err = "data error: " + err.Error()
			s.status[i] = TaskTodo
		} else {
			switch ev.State {
			case engine.StateCompleted:
				s.status[i] = TaskCompleted
			case engine.StateActive:
				s.status[i] = TaskActive
			default:
				s.status[i] = TaskTodo
			}
			if ev.ConfigError != "" {
				s.err = "target error: " + ev.ConfigError
			}
		}
		s.bundles[i] = ResolveBundle(ctx, s.app.DB, t)
	}
	s.tbl.SetData(s.tasks, s.status, s.bundles)
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
	if len(s.tasks) == 0 || s.tbl.CursorPos() >= len(s.tasks) {
		return db.TodayTask{}, TaskTodo, Bundle{}, false
	}
	i := s.tbl.CursorPos()
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

// startSelected creates the persisted ACTIVE session for the selected task
// via the authoritative engine operation. Completed tasks never start again.
func (s *TodayScreen) startSelected() {
	_, _, _, ok := s.selected()
	if !ok {
		return
	}
	task := s.tasks[s.tbl.CursorPos()]
	ctx := context.Background()
	ev, err := engine.EvaluateTodayTask(ctx, s.app.DB, task.ID)
	if err != nil {
		s.app.err = "data error: " + err.Error()
		return
	}
	if ev.State == engine.StateCompleted {
		// Normal state, not an error: a DONE task is simply non-actionable.
		s.notice = "Completed · read-only"
		s.syncButtons()
		return
	}
	if ev.State == engine.StateActive {
		s.notice = "Already active"
		s.syncButtons()
		return
	}
	if _, err := engine.StartTodayTask(ctx, s.app.DB, task.ID, time.Now()); err != nil {
		// Expected busy/completed states stay neutral; real failures surface.
		msg := err.Error()
		if isExpectedStartErr(err) {
			s.notice = "Completed · read-only"
			if containsBusy(msg) {
				s.app.err = "another session is active — stop it first (x)"
			}
			s.syncButtons()
			return
		}
		s.app.err = msg
		return
	}
	s.right = "detail"
	s.Reload()
	s.app.refreshActiveIndicator()
}

func isExpectedStartErr(err error) bool {
	if err == nil {
		return false
	}
	return containsBusy(err.Error()) || containsDone(err.Error())
}

func containsBusy(msg string) bool {
	l := strings.ToLower(msg)
	return strings.Contains(l, "another session is active") || strings.Contains(l, "active")
}

func containsDone(msg string) bool {
	l := strings.ToLower(msg)
	return strings.Contains(l, "already complete") || strings.Contains(l, "already has an active")
}

// stopActive stops the global session via the engine and swaps right-bottom
// to the form.
func (s *TodayScreen) stopActive() {
	ctx := context.Background()
	sess, err := engine.StopActiveSession(ctx, s.app.DB, time.Now())
	if err != nil {
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
	if k, ok := msg.(tea.KeyPressMsg); ok {
		// While the in-place result form owns the right panel, the form owns
		// the keys: text fields take every keystroke (j/k/q/s/x type there);
		// j/k on non-text widgets moves between fields like ↓/↑; Enter
		// saves on SAVE; the viewport always keeps the focused field
		// visible. Global s/x/1-7/q never fire from inside the form — the
		// app routes them here.
		if s.zone == tzRight && s.right == "result" && s.result != nil {
			ks := k.String()
			if ks == "enter" {
				if s.result.focus == s.result.nFocus()-1 {
					if ok, _ := s.result.TrySave(); ok {
						s.right = "detail"
						s.zone = tzTasks
						s.resultScroll.Reset()
						s.Reload()
						s.app.refreshActiveIndicator()
					}
				} else if cmd, handled := s.result.HandleKey(k); handled {
					s.result.ensureFocusVisible(&s.resultScroll, s.resultViewportH())
					return cmd
				}
				return nil
			}
			if (ks == "j" || ks == "k") && !s.result.textFocus() {
				// Non-text widget: j/k moves between fields (never scrolls
				// the focused field out of view).
				if ks == "j" {
					s.result.moveFocus(1)
				} else {
					s.result.moveFocus(-1)
				}
				s.result.ensureFocusVisible(&s.resultScroll, s.resultViewportH())
				return nil
			}
			if cmd, handled := s.result.HandleKey(k); handled {
				s.result.ensureFocusVisible(&s.resultScroll, s.resultViewportH())
				return cmd
			}
			// Unhandled keys while recording evidence are ignored.
			return nil
		}
		switch k.String() {
		case "h", "left":
			s.stepZone(-1)
			return nil
		case "l", "right":
			s.stepZone(1)
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
				s.detailScroll.Reset()
				s.notice = ""
				s.syncButtons()
				return nil
			}
			if s.zone == tzRight && s.right == "detail" {
				// Scroll the detail pane independently; selection stays.
				innerH := s.detailViewportH()
				total := len(strings.Split(s.detailViewWH(30, 999), "\n"))
				if k.String() == "j" || k.String() == "down" {
					s.detailScroll.ScrollDown(total, innerH)
				} else {
					s.detailScroll.ScrollUp(total, innerH)
				}
				return nil
			}
			if s.zone == tzRight && s.right == "result" && s.result != nil {
				if cmd, handled := s.result.HandleKey(k); handled {
					s.result.ensureFocusVisible(&s.resultScroll, s.resultViewportH())
					return cmd
				}
			}
			return nil
		}
		// Typing into result inputs.
		if s.zone == tzRight && s.right == "result" && s.result != nil {
			if k, ok := msg.(tea.KeyPressMsg); ok {
				if cmd, handled := s.result.HandleKey(k); handled {
					s.result.ensureFocusVisible(&s.resultScroll, s.resultViewportH())
					return cmd
				}
			}
		}
	}
	return nil
}

// resultViewportH estimates the visible result lines from the current
// terminal size (same geometry as View).
func (s *TodayScreen) resultViewportH() int {
	_, h := s.app.contentViewport()
	bodyH := h
	if bodyH < 10 {
		bodyH = 10
	}
	activeH := bodyH/2 - 1
	if activeH < 6 {
		activeH = bodyH / 2
	}
	bottomH := bodyH - activeH
	innerH := bottomH - 4
	if innerH < 3 {
		innerH = 3
	}
	return innerH
}

// detailViewportH mirrors rightPanelFixed geometry for scrolling.
func (s *TodayScreen) detailViewportH() int { return s.resultViewportH() }

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

// View is the explicit three-part bento: LEFT tasks | RIGHT active/detail.
// Bounds come from the shell main viewport: LEFT ~58%, RIGHT ~42%, right
// split ~50/50 vertically. Panels are height-clipped to the viewport.
// No metadata/shortcut text lives in main content; that belongs in footer.
func (s *TodayScreen) View() string {
	w, h := s.app.contentViewport()
	if w < 60 {
		w = 60
	}
	leftW, rightW := TwoPaneLayout(w, 58, 30, 26)
	bodyH := h
	if bodyH < 10 {
		bodyH = 10
	}
	// Today table viewport in lines: panel chrome ≈ 4 lines.
	s.tbl.SetSize(leftW-4, bodyH-4)
	s.tbl.Focus = s.zone == tzTasks

	// Right column: TOP active session, BOTTOM detail/result, ~50/50.
	// Bottom gets one extra line so bundled START buttons stay visible.
	activeH := bodyH/2 - 1
	if activeH < 6 {
		activeH = bodyH / 2
	}
	bottomH := bodyH - activeH
	left := PanelFixed("TODAY TASKS", s.tbl.View(), leftW, bodyH, s.zone == tzTasks)
	activePanel := PanelFixed("ACTIVE SESSION", s.activeViewFor(rightW, activeH-4), rightW, activeH, s.zone == tzActive)
	rightBottom := s.rightPanelFixed(rightW, bottomH)
	rightCol := lipgloss.JoinVertical(lipgloss.Left, activePanel, rightBottom)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left+" ", rightCol)
	if s.err != "" {
		body += "\n" + thError.Render(s.err)
	}
	return body
}

func (s *TodayScreen) rightPanelFixed(w, h int) string {
	innerH := h - 4
	if innerH < 3 {
		innerH = 3
	}
	if s.right == "result" && s.result != nil && s.taskOfResult() {
		full := s.result.ViewFor(w - 4)
		return PanelFixed("RESULT — EVIDENCE", ViewportView(full, &s.resultScroll, innerH), w, h, s.zone == tzRight)
	}
	// Full detail (never compact-clipped here): the viewport scrolls instead.
	full := s.detailViewWH(w-4, 999)
	return PanelFixed("SELECTED TASK", ViewportView(full, &s.detailScroll, innerH), w, h, s.zone == tzRight)
}

// detailView is the test-compat wrapper (default inner width).
func (s *TodayScreen) detailView() string { return s.detailViewW(36) }

func (s *TodayScreen) taskOfResult() bool { return s.result != nil && s.result.task != nil }

// activeViewFor renders the session card constrained to maxLines inner lines.
// Compact mode at small heights preserves timer + progress + STOP.
func (s *TodayScreen) activeViewFor(w, maxLines int) string {
	return s.activeViewWH(w, maxLines)
}

func (s *TodayScreen) activeView(w int) string { return s.activeViewWH(w, 99) }

func (s *TodayScreen) activeViewWH(w, maxLines int) string {
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	barW := inner - 8
	if barW < 14 {
		barW = 14
	}
	if barW > 26 {
		barW = 26
	}
	if s.active == nil {
		return thMeta.Render("No active session.") + "\n" +
			RowDivider(inner) + "\n" +
			thHelp.Render("Select a task, then START.")
	}
	el := time.Since(s.active.StartedAt)
	if el < 0 {
		el = 0
	}
	ratio := 0.0
	if s.active.PlannedSeconds > 0 {
		ratio = float64(el.Seconds()) / float64(s.active.PlannedSeconds)
	}
	frame := int(el.Seconds())
	var sb strings.Builder
	// Compact mode for short viewports: timer + progress + STOP only.
	if maxLines < 8 {
		sb.WriteString(thScreenTitle.Render(FormatHMS(el)) + "  " + thMeta.Render(TruncateToWidth(s.active.TitleSnapshot, inner-10)) + "\n")
		sb.WriteString(SessionProgressBar{Ratio: ratio, Width: barW, Frame: frame, ShowPct: true}.View() + "\n")
		sb.WriteString(s.stopBtn.CompactView())
		return sb.String()
	}
	sb.WriteString(DomainBadge(string(s.active.Domain)) + "  " + thMeta.Render(string(s.active.Kind)) + "\n")
	sb.WriteString(thPrimary.Render(TruncateToWidth(s.active.TitleSnapshot, inner)) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(thScreenTitle.Render(FormatHMS(el)) + "\n")
	sb.WriteString(SessionProgressBar{Ratio: ratio, Width: barW, Frame: frame, ShowPct: true}.View() + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(MetaRow("Planned", FormatDurationLong(s.active.PlannedSeconds), 8) + "\n")
	if maxLines < 10 {
		sb.WriteString(s.stopBtn.CompactView())
	} else {
		sb.WriteString(s.stopBtn.View())
	}
	return sb.String()
}

func (s *TodayScreen) detailViewForTest() string { return s.detailViewW(36) }

func (s *TodayScreen) detailViewW(inner int) string { return s.detailViewWH(inner, 99) }

func (s *TodayScreen) detailViewWH(inner, maxLines int) string {
	if inner < 18 {
		inner = 18
	}
	div := RowDivider(inner)
	_, st, b, ok := s.selected()
	if !ok {
		return thMeta.Render("No tasks.")
	}
	t := s.tasks[s.tbl.CursorPos()]
	// Completed tasks are viewable but never startable: read-only evidence,
	// no Start action of any kind.
	if st == TaskCompleted {
		var sb strings.Builder
		sb.WriteString(DomainBadge(string(t.Domain)) + "  " + thMeta.Render(string(t.Kind)) + "  " + StatusPill(st) + "\n")
		sb.WriteString(thSuccess.Render("✓ DONE — READ-ONLY") + "\n")
		sb.WriteString(div + "\n")
		sb.WriteString(s.completedEvidenceView(inner, t, b) + "\n")
		sb.WriteString(div + "\n")
		if s.notice != "" {
			sb.WriteString(thMeta.Render(s.notice) + "\n")
		} else {
			sb.WriteString(thHelp.Render("Enter views · Completed tasks cannot be started again.") + "\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	}
	var sb strings.Builder
	// Compact mode for short viewports: preserve title + planned + action.
	if maxLines < 8 {
		sb.WriteString(DomainBadge(string(t.Domain)) + "  " + StatusPill(st) + "\n")
		sb.WriteString(thPrimary.Render(TruncateToWidth(t.Title, inner)) + "\n")
		sb.WriteString(div + "\n")
		sb.WriteString(MetaRow("Planned", FormatDurationLong(t.PlannedSeconds), 8) + "\n")
		sb.WriteString(s.startBtn.CompactView())
		return strings.TrimRight(sb.String(), "\n")
	}
	sb.WriteString(DomainBadge(string(t.Domain)) + "  " + thMeta.Render(string(t.Kind)) + "  " + StatusPill(st) + "\n")
	switch {
	case b.Lecture != nil:
		sb.WriteString(thScreenTitle.Render(strings.ToUpper(TruncateToWidth(b.Lecture.Subject, inner))) + "\n")
		sb.WriteString(SectionHeader("LECTURE") + "\n")
		sb.WriteString(thPrimary.Render(TruncateToWidth(b.Lecture.Topic, inner)) + "\n")
		if b.Lecture.Lecture != "" {
			sb.WriteString(thMeta.Render(TruncateToWidth(b.Lecture.Lecture, inner)) + "\n")
		}
	case len(b.Practice) > 0:
		subj := ""
		if len(b.Practice) > 0 {
			subj = b.Practice[0].Subject
		}
		sb.WriteString(thScreenTitle.Render(strings.ToUpper(TruncateToWidth(subj, inner))) + "\n")
		sb.WriteString(SectionHeader(fmt.Sprintf("TOPICS (%d)", len(b.Practice))) + "\n")
		for i, p := range b.Practice {
			sb.WriteString(bundleRow(i+1, p.Topic, inner) + "\n")
			if i < len(b.Practice)-1 {
				sb.WriteString(RowDivider(inner) + "\n")
			}
		}
	case len(b.DSA) > 0:
		sb.WriteString(MetaRow("Planned", FormatDurationLong(t.PlannedSeconds), 8) + "\n")
		var repeats, news []DSAItem
		for _, d := range b.DSA {
			if d.Mode == db.DSAModeNew {
				news = append(news, d)
			} else {
				repeats = append(repeats, d)
			}
		}
		n := 0
		if len(repeats) > 0 {
			sb.WriteString(SectionHeader("REPEAT") + "\n")
			for i, d := range repeats {
				n++
				sb.WriteString(bundleRow(n, d.Title, inner) + "\n")
				if i < len(repeats)-1 || len(news) > 0 {
					sb.WriteString(RowDivider(inner) + "\n")
				}
			}
		}
		if len(news) > 0 {
			sb.WriteString(SectionHeader("NEW") + "\n")
			for i, d := range news {
				n++
				sb.WriteString(bundleRow(n, d.Title, inner) + "\n")
				if i < len(news)-1 {
					sb.WriteString(RowDivider(inner) + "\n")
				}
			}
		}
		sb.WriteString(div + "\n")
		if maxLines < 13 {
			sb.WriteString(s.startBtn.CompactView())
		} else {
			sb.WriteString(s.startBtn.View())
			if s.startBtn.Disabled {
				sb.WriteString("\n" + thHelp.Render("Start unavailable while a session runs."))
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	case len(b.SWE) > 0:
		sb.WriteString(MetaRow("Planned", FormatDurationLong(t.PlannedSeconds), 8) + "\n")
		lastArea := ""
		n := 0
		for _, sw := range b.SWE {
			if sw.Area != lastArea {
				sb.WriteString(SectionHeader(strings.ToUpper(TruncateToWidth(sw.Area, inner-2))) + "\n")
				lastArea = sw.Area
			}
			if sw.Task != "" {
				n++
				sb.WriteString(bundleRow(n, sw.Task, inner) + "\n")
				sb.WriteString(RowDivider(inner) + "\n")
			}
		}
		if maxLines < 13 {
			sb.WriteString(s.startBtn.CompactView())
		} else {
			sb.WriteString(s.startBtn.View())
			if s.startBtn.Disabled {
				sb.WriteString("\n" + thHelp.Render("Start unavailable while a session runs."))
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	default:
		sb.WriteString(thPrimary.Render(TruncateToWidth(t.Title, inner)) + "\n")
	}
	sb.WriteString(div + "\n")
	sb.WriteString(MetaRow("Planned", FormatDurationLong(t.PlannedSeconds), 8) + "\n")
	if maxLines < 13 {
		sb.WriteString(s.startBtn.CompactView())
	} else {
		sb.WriteString(s.startBtn.View())
		if s.startBtn.Disabled {
			sb.WriteString("\n" + thHelp.Render("Stop active session first."))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// completedEvidenceView renders persisted evidence for a DONE task as
// human-readable read-only rows (no inputs, no SAVE, no Start). Durations
// always carry units; raw storage integers are never exposed.
func (s *TodayScreen) completedEvidenceView(inner int, task db.TodayTask, b Bundle) string {
	ctx := context.Background()
	var sb strings.Builder
	switch {
	case b.Lecture != nil:
		sb.WriteString(thScreenTitle.Render(strings.ToUpper(TruncateToWidth(b.Lecture.Subject, inner))) + "\n")
		sb.WriteString(SectionHeader("LECTURE") + "\n")
		sb.WriteString(thPrimary.Render(TruncateToWidth(b.Lecture.Topic, inner)) + "\n")
		if b.Lecture.Lecture != "" {
			sb.WriteString(thMeta.Render(TruncateToWidth(b.Lecture.Lecture, inner)) + "\n")
		}
		if comp, err := runtime.GetLectureCompletion(ctx, s.app.DB, b.Lecture.LectureID); err == nil {
			sb.WriteString(MetaRow("Completed", FormatDate(comp.CompletedAt), 10))
		} else {
			sb.WriteString(thSuccess.Render("✓ Completed"))
		}
	case len(b.Practice) > 0:
		sb.WriteString(thScreenTitle.Render(strings.ToUpper(TruncateToWidth(b.Practice[0].Subject, inner))) + "\n")
		sb.WriteString(SectionHeader(fmt.Sprintf("EVIDENCE · %d TOPICS", len(b.Practice))) + "\n")
		recs := s.practiceRecordsForTask(ctx, task.ID)
		byTopic := map[string]db.GovPracticeRecord{}
		for _, r := range recs {
			byTopic[string(r.TopicID)] = r
		}
		for i, p := range b.Practice {
			if i > 0 {
				sb.WriteString("\n" + RowDivider(inner) + "\n")
			}
			sb.WriteString(thPrimary.Render(fmt.Sprintf("%d. %s", i+1, TruncateToWidth(p.Topic, inner-4))) + "\n")
			if r, ok := byTopic[string(p.TopicID)]; ok {
				sb.WriteString(MetaRow("Time spent", FormatDurationLong(r.TimeSpentSeconds), 12) + "\n")
				sb.WriteString(MetaRow("Attempted", fmt.Sprint(r.Attempted), 12) + "\n")
				sb.WriteString(MetaRow("Correct", fmt.Sprint(r.Correct), 12) + "\n")
				avg := "—"
				if r.AvgQuestionSpeedSeconds != nil {
					avg = fmt.Sprintf("%.1fs", *r.AvgQuestionSpeedSeconds)
				}
				sb.WriteString(MetaRow("Avg speed", avg, 12))
			} else {
				sb.WriteString(thMeta.Render("—"))
			}
		}
	case len(b.DSA) > 0:
		sb.WriteString(SectionHeader(fmt.Sprintf("EVIDENCE · %d PROBLEMS", len(b.DSA))) + "\n")
		atts := s.dsaAttemptsForTask(ctx, task.ID)
		latest := map[string]db.DSAAttempt{}
		for _, a := range atts {
			k := string(a.ProblemID)
			if cur, ok := latest[k]; !ok || !a.AttemptedAt.Before(cur.AttemptedAt) {
				latest[k] = a
			}
		}
		for i, d := range b.DSA {
			if i > 0 {
				sb.WriteString("\n" + RowDivider(inner) + "\n")
			}
			sb.WriteString(thPrimary.Render(fmt.Sprintf("%d. %s", i+1, TruncateToWidth(d.Title, inner-8))) + "  " + thMeta.Render("["+string(d.Mode)+"]") + "\n")
			if a, ok := latest[string(d.ProblemID)]; ok {
				sb.WriteString(s.readOnlyCheckbox("Independent", a.Independent) + "\n")
				sb.WriteString(s.readOnlyCheckbox("Optimal", a.Optimal) + "\n")
				sb.WriteString(s.readOnlyCheckbox("Hint used", a.HintUsed) + "\n")
				dur := "—"
				if a.SolveTimeSeconds != nil {
					dur = FormatDurationLong(*a.SolveTimeSeconds)
				}
				sb.WriteString(MetaRow("Solve time", dur, 12))
			} else {
				sb.WriteString(thMeta.Render("—"))
			}
		}
	case len(b.SWE) > 0:
		sb.WriteString(SectionHeader(fmt.Sprintf("EVIDENCE · %d TASKS", len(b.SWE))) + "\n")
		execs := s.sweExecutionsForTask(ctx, task.ID)
		latest := map[string]string{}
		for _, e := range execs {
			latest[string(e.TaskID)] = string(e.Status)
		}
		for i, sw := range b.SWE {
			if i > 0 {
				sb.WriteString("\n" + RowDivider(inner) + "\n")
			}
			label := sw.Task
			if label == "" {
				label = sw.Area
			}
			sb.WriteString(thPrimary.Render(TruncateToWidth(label, inner-2)) + "\n")
			st := "—"
			if sw.TaskID != nil {
				if v, ok := latest[string(*sw.TaskID)]; ok {
					st = v
				}
			}
			sb.WriteString(thSuccess.Render("✓ " + st))
		}
	default:
		sb.WriteString(thPrimary.Render(TruncateToWidth(task.Title, inner)))
	}
	return sb.String()
}

func (s *TodayScreen) readOnlyCheckbox(label string, checked bool) string {
	glyph := thMeta.Render("□")
	if checked {
		glyph = thSuccess.Render("■")
	}
	return "  " + glyph + "  " + thPrimary.Render(label)
}

func (s *TodayScreen) taskSessionIDs(ctx context.Context, taskID db.ID) []db.ID {
	sessList, err := runtime.ListSessions(ctx, s.app.DB)
	if err != nil {
		return nil
	}
	var out []db.ID
	for _, sess := range sessList {
		if sess.TodayTaskID == taskID && sess.Status != db.SessionStatusActive {
			out = append(out, sess.ID)
		}
	}
	return out
}

func (s *TodayScreen) practiceRecordsForTask(ctx context.Context, taskID db.ID) []db.GovPracticeRecord {
	var out []db.GovPracticeRecord
	for _, sid := range s.taskSessionIDs(ctx, taskID) {
		if recs, err := runtime.GetGovPracticeRecordsBySession(ctx, s.app.DB, sid); err == nil {
			out = append(out, recs...)
		}
	}
	return out
}

func (s *TodayScreen) dsaAttemptsForTask(ctx context.Context, taskID db.ID) []db.DSAAttempt {
	var out []db.DSAAttempt
	for _, sid := range s.taskSessionIDs(ctx, taskID) {
		if atts, err := runtime.GetDSAAttemptsBySession(ctx, s.app.DB, sid); err == nil {
			out = append(out, atts...)
		}
	}
	return out
}

func (s *TodayScreen) sweExecutionsForTask(ctx context.Context, taskID db.ID) []db.SWEExecution {
	var out []db.SWEExecution
	for _, sid := range s.taskSessionIDs(ctx, taskID) {
		if execs, err := runtime.GetSWEExecutionsBySession(ctx, s.app.DB, sid); err == nil {
			out = append(out, execs...)
		}
	}
	return out
}

// stepZone moves pane focus left/right across tasks, session, detail.
// The session pane is skipped when no session is active.
func (s *TodayScreen) stepZone(d int) {
	z := s.zone + d
	if z < tzTasks {
		z = tzTasks
	}
	if z > tzRight {
		z = tzRight
	}
	if z == tzActive && s.active == nil {
		if d > 0 {
			z = tzRight
		} else {
			z = tzTasks
		}
	}
	s.zone = z
	s.syncButtons()
}

// bundleRow renders one bundled work item with the global row language:
// visible number, deliberate padding, thin divider owned by the caller.
func bundleRow(n int, title string, width int) string {
	if width < 10 {
		width = 10
	}
	num := fmt.Sprintf("%02d", n)
	tw := width - 4 - len(num) - 2
	if tw < 4 {
		tw = 4
	}
	return thPrimary.Render(num + "  " + TruncateToWidth(title, tw))
}

// onBack steps the right-bottom panel from result back to detail.
func (s *TodayScreen) onBack() bool {
	if s.right == "result" {
		s.right = "detail"
		s.zone = tzTasks
		s.resultScroll.Reset()
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
