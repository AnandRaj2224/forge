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
)

// ---------- SWE: persistent two-panel areas workspace ----------

type SWETaskDetail struct {
	Area    db.SWEArea
	Task    db.SWETask
	History []db.SWEExecution
	Done    bool
}

type SWEScreen struct {
	app      *App
	areas    []db.SWEArea
	areaList Navigator
	taskList Navigator
	focusR   bool
	detail   *SWETaskDetail
	showing  string // "area" or "task"
	err      string
}

func NewSWEScreen(a *App) *SWEScreen {
	return &SWEScreen{app: a, areaList: NewNavigator(), taskList: NewNavigator(), showing: "area"}
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
		return
	}
	tasks, err := catalog.GetSWETasks(ctx, s.app.DB, a.ID)
	if err != nil {
		s.err = err.Error()
		return
	}
	items := make([]RowItem, 0, len(tasks))
	for _, t := range tasks {
		done, _ := runtime.IsSWETaskComplete(ctx, s.app.DB, t.ID)
		mark := "○"
		if done {
			mark = "✓"
		}
		items = append(items, RowItem{ID: string(t.ID), Title: t.Title, Mark: mark})
	}
	s.taskList.SetItems(items)
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
			hist, _ := runtime.GetSWEExecutionHistory(ctx, s.app.DB, t.ID)
			done, _ := runtime.IsSWETaskComplete(ctx, s.app.DB, t.ID)
			s.detail = &SWETaskDetail{Area: area, Task: t, History: hist, Done: done}
			s.showing = "task"
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
		case "tab":
			s.focusR = !s.focusR
			return nil
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
		right = PanelFixed("TASK", s.taskBody(rw-6), rw, bodyH, true)
	} else if a := s.selArea(); a != nil {
		pr := ComputeSWEProgress(context.Background(), s.app.DB, *a)
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
			ProgressBar{Ratio: r, Width: barW, ShowPct: true}.View() + "\n" +
			thMeta.Render(fmt.Sprintf("%d / %d", pr.Done, pr.Total)) + "\n" +
			RowDivider(inner) + "\n" +
			SectionHeader("TASKS") + "\n" + s.taskList.View()
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

func (s *SWEScreen) Hints() string { return "Tab Focus   Enter Open" }
