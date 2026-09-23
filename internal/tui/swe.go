package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	db "forge/internal/db"
	"forge/internal/db/catalog"
	"forge/internal/engine"
)

// ---------- SWE: areas + routine slots (time allocation, no tasks) ----------

type SWEScreen struct {
	app      *App
	areas    []db.SWEArea
	areaList Navigator
	slots    []db.SWERoutineSlot
	slotList Navigator
	focusR   bool
	err      string
}

func NewSWEScreen(a *App) *SWEScreen {
	return &SWEScreen{app: a, areaList: NewNavigator(), slotList: NewNavigator()}
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
		slots, _ := catalog.GetSWERoutineByArea(ctx, s.app.DB, a.ID)
		total := 0
		for _, sl := range slots {
			total += sl.DurationSeconds
		}
		meta := ""
		if len(slots) > 0 {
			meta = fmt.Sprintf("%d slots · %s/wk", len(slots), engine.FormatDurationLong(total))
		} else {
			meta = "no slots"
		}
		items = append(items, RowItem{ID: string(a.ID), Title: a.Name, Meta: meta, Mark: "•"})
	}
	s.areaList.SetItems(items)
	s.reloadSlots()
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

func (s *SWEScreen) reloadSlots() {
	ctx := context.Background()
	a := s.selArea()
	if a == nil {
		s.slots = nil
		s.slotList.SetItems(nil)
		return
	}
	slots, err := catalog.GetSWERoutineByArea(ctx, s.app.DB, a.ID)
	if err != nil {
		s.err = err.Error()
		s.slots = nil
		s.slotList.SetItems(nil)
		return
	}
	s.slots = slots
	items := make([]RowItem, 0, len(slots))
	for _, sl := range slots {
		items = append(items, RowItem{
			ID:    string(sl.ID),
			Title: weekdayName(sl.Weekday) + " · slot " + fmt.Sprint(sl.Position),
			Meta:  engine.FormatDurationLong(sl.DurationSeconds),
			Mark:  "•",
		})
	}
	s.slotList.SetItems(items)
}

// onBack pops the slot pane focus first. True = consumed.
func (s *SWEScreen) onBack() bool {
	if s.focusR {
		s.focusR = false
		return true
	}
	return false
}

func weekdayName(wd int) string {
	switch wd {
	case 1:
		return "Mon"
	case 2:
		return "Tue"
	case 3:
		return "Wed"
	case 4:
		return "Thu"
	case 5:
		return "Fri"
	case 6:
		return "Sat"
	case 7:
		return "Sun"
	default:
		return fmt.Sprintf("Day %d", wd)
	}
}

func (s *SWEScreen) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			s.focusR = true
			return nil
		case "j", "down", "k", "up", "h", "left", "l", "right":
			down := k.String() == "j" || k.String() == "down"
			up := k.String() == "k" || k.String() == "up"
			left := k.String() == "h" || k.String() == "left"
			right := k.String() == "l" || k.String() == "right"
			if s.focusR {
				if down {
					s.slotList.CursorDown()
				} else if up {
					s.slotList.CursorUp()
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
				s.reloadSlots()
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
	s.slotList.SetWidth(rw - 4)
	s.areaList.FitHeight(innerH - 1)
	s.slotList.FitHeight(innerH - 7)
	s.areaList.SetFocused(!s.focusR)
	s.slotList.SetFocused(s.focusR)
	var right string
	if a := s.selArea(); a != nil {
		inner := rw - 6
		total := 0
		for _, sl := range s.slots {
			total += sl.DurationSeconds
		}
		body := lipgloss.NewStyle().Bold(true).Foreground(cSWE).Render(strings.ToUpper(TruncateToWidth(a.Name, inner))) + "\n" +
			RowDivider(inner) + "\n" +
			SectionHeader("SCHEDULED TIME") + "\n" +
			thMeta.Render(fmt.Sprintf("%s / week · %d slots", engine.FormatDurationLong(total), len(s.slots))) + "\n" +
			RowDivider(inner) + "\n" +
			DomainSection("SWE", "ROUTINE SLOTS") + "\n" + s.slotList.View()
		right = PanelFixed("AREA", body, rw, bodyH, s.focusR)
	} else {
		right = PanelFixed("AREA", EmptyState("Select an area."), rw, bodyH, s.focusR)
	}
	return lipglossJoin(PanelFixed("AREAS", SectionHeader("SWE AREAS")+"\n"+s.areaList.View(), lw, bodyH, !s.focusR), right)
}
