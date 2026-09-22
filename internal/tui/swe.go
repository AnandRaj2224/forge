package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	"forge/internal/engine"
)

// ---------- SWE: persistent two-panel areas workspace ----------

type SWETaskDetail struct {
	Area    db.SWEArea
	Task    db.SWETask
	History []db.SWEExecution
	Done    bool
}

type SWEScreen struct {
	app          *App
	areas        []db.SWEArea
	areaList     Navigator
	taskList     Navigator
	focusR       bool
	detail       *SWETaskDetail
	detailScroll ScrollViewport // task pane scrolls independently when overflowing
	showing      string         // "area" or "task"
	err          string
	// areaProgress caches per-area progress loaded in Reload/reloadTasks so
	// View rendering performs no database access.
	areaProgress map[string]SWEAreaProgress
	// taskDone caches per-task completion marks for the selected area.
	taskDone map[string]bool
}

func NewSWEScreen(a *App) *SWEScreen {
	return &SWEScreen{app: a, areaList: NewNavigator(), taskList: NewNavigator(), showing: "area", areaProgress: map[string]SWEAreaProgress{}, taskDone: map[string]bool{}}
}

func (s *SWEScreen) Reload() {
	ctx := context.Background()
	areas, err := catalog.GetSWEAreas(ctx, s.app.DB)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.areas = areas
	items := make([]RowItem, 0, len(areas))
	for _, a := range areas {
		tasks, _ := catalog.GetSWETasks(ctx, s.app.DB, a.ID)
		items = append(items, RowItem{ID: string(a.ID), Title: a.Name, Meta: fmt.Sprintf("%d tasks", len(tasks)), Mark: "•"})
	}
	s.areaList.SetItems(items)
	s.showing = "area"
	// Batch area progress for all areas (one tasks query per area is
	// catalog-bounded; completion resolved via one map query per area inside
	// ComputeSWEProgressE). Cache for render-pure View.
	if s.areaProgress == nil {
		s.areaProgress = map[string]SWEAreaProgress{}
	}
	for _, a := range areas {
		if pr, err := ComputeSWEProgressE(ctx, s.app.DB, a); err == nil {
			s.areaProgress[string(a.ID)] = pr
		} else {
			s.err = "data error: " + err.Error()
			s.areaProgress[string(a.ID)] = SWEAreaProgress{Area: a}
		}
	}
	s.reloadTasks()
}

func (s *SWEScreen) selArea() *db.SWEArea {
	id := s.areaList.SelectedID()
	if id == "" {
		return nil
	}
	for i, a := range s.areas {
		if string(a.ID) == id {
			return &s.areas[i]
		}
	}
	return nil
}

func (s *SWEScreen) reloadTasks() {
	ctx := context.Background()
	a := s.selArea()
	if a == nil {
		s.taskList.SetItems(nil)
		if s.taskDone == nil {
			s.taskDone = map[string]bool{}
		} else {
			for k := range s.taskDone {
				delete(s.taskDone, k)
			}
		}
		return
	}
	tasks, err := catalog.GetSWETasks(ctx, s.app.DB, a.ID)
	if err != nil {
		s.err = err.Error()
		return
	}
	// Batch completion marks for the selected area (one map query).
	ids := make([]db.ID, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	compMap, err := runtime.GetSWETaskCompletionMap(ctx, s.app.DB, ids)
	if err != nil {
		s.err = "data error: " + err.Error()
		compMap = map[db.ID]bool{}
	}
	if s.taskDone == nil {
		s.taskDone = map[string]bool{}
	} else {
		for k := range s.taskDone {
			delete(s.taskDone, k)
		}
	}
	items := make([]RowItem, 0, len(tasks))
	for _, t := range tasks {
		done := compMap[t.ID]
		s.taskDone[string(t.ID)] = done
		mark := "○"
		if done {
			mark = "✓"
		}
		items = append(items, RowItem{ID: string(t.ID), Title: t.Title, Mark: mark})
	}
	s.taskList.SetItems(items)
	// Keep the area progress cache consistent with the same batch result.
	if s.areaProgress == nil {
		s.areaProgress = map[string]SWEAreaProgress{}
	}
	done := 0
	for _, t := range tasks {
		if compMap[t.ID] {
			done++
		}
	}
	s.areaProgress[string(a.ID)] = SWEAreaProgress{Area: *a, Total: len(tasks), Done: done}
}

func (s *SWEScreen) openTask(areaID, taskID string) {
	ctx := context.Background()
	var area db.SWEArea
	for _, a := range s.areas {
		if string(a.ID) == areaID {
			area = a
		}
	}
	if area.ID == "" {
		if areas, err := catalog.GetSWEAreas(ctx, s.app.DB); err == nil {
			for _, a := range areas {
				if string(a.ID) == areaID {
					area = a
				}
			}
		}
	}
	tasks, err := catalog.GetSWETasks(ctx, s.app.DB, db.ID(areaID))
	if err != nil {
		s.err = err.Error()
		return
	}
	for _, t := range tasks {
		if string(t.ID) == taskID {
			hist, err := runtime.GetSWEExecutionHistory(ctx, s.app.DB, t.ID)
			if err != nil {
				s.err = "data error: " + err.Error()
			}
			done, err := engine.IsSWETaskComplete(ctx, s.app.DB, t.ID)
			if err != nil {
				s.err = "data error: " + err.Error()
			}
			s.detail = &SWETaskDetail{Area: area, Task: t, History: hist, Done: done}
			s.showing = "task"
			s.detailScroll.Reset()
			return
		}
	}
	s.err = "task not found"
}

// onBack pops the task panel first. True = consumed.
func (s *SWEScreen) onBack() bool {
	if s.showing == "task" {
		s.showing = "area"
		return true
	}
	if s.focusR {
		s.focusR = false
		return true
	}
	return false
}

func (s *SWEScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			if s.showing == "task" {
				return nil
			}
			if s.focusR {
				area := s.selArea()
				if id := s.taskList.SelectedID(); id != "" && area != nil {
					s.openTask(string(area.ID), id)
				}
				return nil
			}
			s.focusR = true
			return nil
		case "j", "down", "k", "up", "h", "left", "l", "right":
			down := k.String() == "j" || k.String() == "down"
			up := k.String() == "k" || k.String() == "up"
			left := k.String() == "h" || k.String() == "left"
			right := k.String() == "l" || k.String() == "right"
			if s.showing == "task" {
				// Scroll the task pane independently.
				if down || up {
					_, h := s.app.contentViewport()
					bodyH := h
					if bodyH < 8 {
						bodyH = 8
					}
					innerH := bodyH - 4
					if innerH < 4 {
						innerH = 4
					}
					total := len(strings.Split(s.taskBody(40), "\n"))
					if down {
						s.detailScroll.ScrollDown(total, innerH)
					} else {
						s.detailScroll.ScrollUp(total, innerH)
					}
				}
				return nil
			}
			if s.focusR {
				if down {
					s.taskList.CursorDown()
				} else if up {
					s.taskList.CursorUp()
				} else if left {
					s.focusR = false
				}
				return nil
			}
			prev := s.areaList.SelectedID()
			if down {
				s.areaList.CursorDown()
			} else if up {
				s.areaList.CursorUp()
			} else if right {
				s.focusR = true
				return nil
			} else {
				return nil
			}
			if s.areaList.SelectedID() != prev {
				s.reloadTasks()
			}
			return nil
		}
	}
	return nil
}

func (s *SWEScreen) View() string {
	w, h := s.app.contentViewport()
	lw, rw := TwoPaneLayout(w, 32, 24, 40)
	bodyH := h
	if bodyH < 8 {
		bodyH = 8
	}
	innerH := bodyH - 4
	if innerH < 4 {
		innerH = 4
	}
	s.areaList.SetWidth(lw - 4)
	s.taskList.SetWidth(rw - 4)
	s.areaList.FitHeight(innerH - 1)
	s.taskList.FitHeight(innerH - 7)
	s.areaList.SetFocused(!s.focusR)
	s.taskList.SetFocused(s.focusR)
	var right string
	if s.showing == "task" {
		right = PanelFixed("TASK", ViewportView(s.taskBody(rw-6), &s.detailScroll, innerH), rw, bodyH, true)
	} else if a := s.selArea(); a != nil {
		// Render-pure: progress comes from the reload-time cache. Never
		// queries SQLite and never mutates s.err during render.
		pr, ok := s.areaProgress[string(a.ID)]
		if !ok {
			pr = SWEAreaProgress{Area: *a}
		}
		r := 0.0
		if pr.Total > 0 {
			r = float64(pr.Done) / float64(pr.Total)
		}
		barW := rw - 12
		if barW < 14 {
			barW = 14
		}
		if barW > 28 {
			barW = 28
		}
		inner := rw - 6
		body := lipgloss.NewStyle().Bold(true).Foreground(cSWE).Render(strings.ToUpper(TruncateToWidth(a.Name, inner))) + "\n" +
			RowDivider(inner) + "\n" +
			SectionHeader("PROGRESS") + "\n" +
			ProgressBar{Ratio: r, Width: barW, ShowPct: true, Accent: cSWE}.View() + "\n" +
			thMeta.Render(fmt.Sprintf("%d / %d", pr.Done, pr.Total)) + "\n" +
			RowDivider(inner) + "\n" +
			DomainSection("SWE", "TASKS") + "\n" + s.taskList.View()
		right = PanelFixed("AREA", body, rw, bodyH, s.focusR)
	} else {
		right = PanelFixed("AREA", EmptyState("Select an area."), rw, bodyH, s.focusR)
	}
	return lipglossJoin(PanelFixed("AREAS", SectionHeader("AREAS")+"\n"+s.areaList.View(), lw, bodyH, !s.focusR), right)
}

func (s *SWEScreen) taskBody(inner int) string {
	d := s.detail
	if d == nil {
		return EmptyState("Task not found: " + s.err)
	}
	if inner < 24 {
		inner = 24
	}
	var sb strings.Builder
	sb.WriteString(thScreenTitle.Render(strings.ToUpper(TruncateToWidth(d.Task.Title, inner))) + "\n")
	sb.WriteString(thMeta.Render(d.Area.Name) + "\n")
	sb.WriteString(RowDivider(inner) + "\n")
	if d.Done {
		sb.WriteString(thSuccess.Render("✓  DONE") + "\n")
	} else {
		sb.WriteString(thTodo.Render("○  TODO") + "\n")
	}
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(SectionHeader(fmt.Sprintf("EXECUTIONS (%d)", len(d.History))) + "\n")
	if len(d.History) == 0 {
		sb.WriteString(thHelp.Render("No executions yet.") + "\n")
	}
	for i, e := range d.History {
		st := thMeta.Render(string(e.Status))
		switch e.Status {
		case db.SWEStatusCompleted:
			st = thSuccess.Render(string(e.Status))
		case db.SWEStatusBlocked:
			st = thError.Render(string(e.Status))
		case db.SWEStatusPartial:
			st = thWarn.Render(string(e.Status))
		}
		sb.WriteString(SelectableRow{Mark: "•", Title: string(st) + "  " + e.RecordedAt.Format("02 Jan 15:04"), Width: inner}.View() + "\n")
		if i < len(d.History)-1 {
			sb.WriteString(RowDivider(inner) + "\n")
		}
	}
	sb.WriteString(RowDivider(inner) + "\n")
	sb.WriteString(thHelp.Render("Work starts from Today only."))
	return sb.String()
}
