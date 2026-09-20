package tui

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/db/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------- SWE ----------

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
	items := make([]listItem, 0, len(areas))
	for _, a := range areas {
		items = append(items, listItem{id: string(a.ID), title: a.Name})
	}
	s.areaList.SetItems(wrapItems(items))
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
	items := make([]listItem, 0, len(tasks))
	for _, t := range tasks {
		done, _ := runtime.IsSWETaskComplete(ctx, s.app.DB, t.ID)
		mark := "○"
		if done {
			mark = "✓"
		}
		items = append(items, listItem{id: string(t.ID), title: mark + "  " + t.Title})
	}
	s.taskList.SetItems(wrapItems(items))
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
	if k, ok := msg.(tea.KeyMsg); ok {
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
		case "j", "down", "k", "up":
			down := k.String() == "j" || k.String() == "down"
			if s.focusR {
				if down {
					s.taskList.List.CursorDown()
				} else {
					s.taskList.List.CursorUp()
				}
				return nil
			}
			prev := s.areaList.SelectedID()
			if down {
				s.areaList.List.CursorDown()
			} else {
				s.areaList.List.CursorUp()
			}
			if s.areaList.SelectedID() != prev {
				s.reloadTasks()
			}
			return nil
		}
	}
	if s.focusR {
		var c tea.Cmd
		s.taskList.List, c = s.taskList.List.Update(msg)
		return c
	}
	prev := s.areaList.SelectedID()
	var c tea.Cmd
	s.areaList.List, c = s.areaList.List.Update(msg)
	if s.areaList.SelectedID() != prev {
		s.reloadTasks()
	}
	return c
}

func (s *SWEScreen) View() string {
	w, h := s.app.contentViewport()
	lw, rw := splitW(w, 30, 22, 40)
	listH := h - 6
	if listH < 8 {
		listH = 8
	}
	s.areaList.List.SetWidth(lw - 4)
	s.taskList.List.SetWidth(rw - 4)
	s.areaList.FitHeight(listH)
	s.taskList.FitHeight(listH - 8)
	var right string
	if s.showing == "task" {
		right = Panel("TASK", s.taskBody(), rw, true)
	} else if a := s.selArea(); a != nil {
		pr := ComputeSWEProgress(context.Background(), s.app.DB, *a)
		r := 0.0
		if pr.Total > 0 {
			r = float64(pr.Done) / float64(pr.Total)
		}
		body := thScreenTitle.Render(a.Name) + "\n\n" +
			SectionHeader("PROGRESS") + "\n" +
			ProgressBar{Ratio: r, Width: 26, ShowPct: true}.View() + "\n" +
			thMeta.Render(fmt.Sprintf("%d / %d", pr.Done, pr.Total)) + "\n\n" +
			SectionHeader("TASKS") + "\n" + s.taskList.List.View()
		right = Panel("AREA", body, rw, s.focusR)
	} else {
		right = Panel("AREA", EmptyState("Select an area."), rw, s.focusR)
	}
	return lipglossJoin(Panel("AREAS", s.areaList.List.View(), lw, !s.focusR), right)
}

func (s *SWEScreen) taskBody() string {
	d := s.detail
	if d == nil {
		return EmptyState("Task not found: " + s.err)
	}
	var sb string
	sb += thScreenTitle.Render(d.Task.Title) + "\n"
	sb += thMeta.Render(d.Area.Name) + "\n\n"
	if d.Done {
		sb += thSuccess.Render("✓  DONE") + "\n"
	} else {
		sb += thTodo.Render("○  TODO") + "\n"
	}
	sb += "\n" + SectionHeader(fmt.Sprintf("EXECUTIONS (%d)", len(d.History))) + "\n"
	if len(d.History) == 0 {
		sb += thHelp.Render("No executions yet.") + "\n"
	}
	for _, e := range d.History {
		st := thMeta.Render(string(e.Status))
		switch e.Status {
		case db.SWEStatusCompleted:
			st = thSuccess.Render(string(e.Status))
		case db.SWEStatusBlocked:
			st = thError.Render(string(e.Status))
		case db.SWEStatusPartial:
			st = thWarn.Render(string(e.Status))
		}
		sb += "  " + st + "   " + thMeta.Render(e.RecordedAt.Format("02 Jan 15:04")) + "\n"
	}
	sb += "\n" + thHelp.Render("Work starts from Today only.")
	return sb
}

func (s *SWEScreen) Hints() string { return "Tab Focus   Enter Open" }
