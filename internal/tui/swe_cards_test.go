package tui

import (
	"strings"
	"testing"

	db "forge/internal/db"
)

func TestSWEDisplayNames(t *testing.T) {
	cases := map[string]string{
		"golang":                     "Go",
		"docker":                     "Docker",
		"prometheus":                 "Prometheus",
		"postgres/sql":               "PostgreSQL",
		"postgres":                   "PostgreSQL",
		"grafana":                    "Grafana",
		"curl":                       "cURL",
		"bash":                       "Bash",
		"linux(arch) + my own setup": "Linux (Arch) + My Own Setup",
		"CI/CD(github actions)":      "CI/CD (GitHub Actions)",
	}
	for in, want := range cases {
		if got := SWEDisplayName(in); got != want {
			t.Fatalf("SWEDisplayName(%q)=%q want %q", in, got, want)
		}
	}
	// Intentional technical casing preserved (no blind Title Case).
	for _, keep := range []string{"SWE", "DSA", "GOV", "CN", "SQL", "HTTP", "HTTPS", "API", "CI/CD", "PostgreSQL", "JavaScript", "TypeScript", "MySQL", "cURL", "K8s", "OS", "DBMS", "Project — Sentinel"} {
		if got := SWEDisplayName(keep); got != keep {
			t.Fatalf("preserve %q got %q", keep, got)
		}
	}
}

func TestSWEGroupSingleWeekdaySingleSlot(t *testing.T) {
	slots := []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 5400},
	}
	groups := groupSWESlotsByDay(slots)
	if len(groups) != 1 {
		t.Fatalf("groups=%d want 1", len(groups))
	}
	if groups[0].DayName != "Monday" {
		t.Fatalf("day=%q want Monday", groups[0].DayName)
	}
	if len(groups[0].Slots) != 1 {
		t.Fatalf("slots=%d want 1", len(groups[0].Slots))
	}
}

func TestSWEGroupMultipleWeekdays(t *testing.T) {
	slots := []db.SWERoutineSlot{
		{ID: "s1", Weekday: 3, Position: 0, AreaID: "a", DurationSeconds: 100},
		{ID: "s2", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 100},
		{ID: "s3", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 100},
	}
	// Input order preserved as given (first-appearance); production feeds
	// ORDER BY weekday so groups arrive weekday-sorted. Here we feed
	// weekday order 3,1,2 to verify no hardcoding and deterministic grouping.
	groups := groupSWESlotsByDay(slots)
	if len(groups) != 3 {
		t.Fatalf("groups=%d want 3", len(groups))
	}
	// Each weekday represented once.
	seen := map[int]bool{}
	for _, g := range groups {
		if seen[g.Weekday] {
			t.Fatalf("duplicate weekday %d", g.Weekday)
		}
		seen[g.Weekday] = true
		if len(g.Slots) != 1 {
			t.Fatalf("weekday %d slots=%d want 1", g.Weekday, len(g.Slots))
		}
	}
}

func TestSWEGroupMultipleSlotsSameWeekday(t *testing.T) {
	slots := []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 5400},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "a", DurationSeconds: 1800},
		{ID: "s3", Weekday: 2, Position: 0, AreaID: "a", DurationSeconds: 3600},
	}
	groups := groupSWESlotsByDay(slots)
	if len(groups) != 2 {
		t.Fatalf("groups=%d want 2", len(groups))
	}
	if groups[0].Weekday != 1 || len(groups[0].Slots) != 2 {
		t.Fatalf("first group must be Monday with 2 slots: %+v", groups[0])
	}
	if groups[0].Slots[0].ID != "s1" || groups[0].Slots[1].ID != "s2" {
		t.Fatalf("slot order not preserved: %+v", groups[0].Slots)
	}
	if groups[1].Weekday != 2 || len(groups[1].Slots) != 1 {
		t.Fatalf("second group wrong: %+v", groups[1])
	}
}

func TestSWERoutineCardContent(t *testing.T) {
	card := SWERoutineCard{
		Day:   "MONDAY",
		Total: "1h 30m",
		Tasks: []SWECardTask{{Name: "Go", Dur: "1h 30m"}},
		Width: 34,
	}
	v := stripANSIForTest(card.View())
	if !strings.Contains(v, "MONDAY") {
		t.Fatalf("card missing day:\n%s", v)
	}
	if !strings.Contains(v, "Go") {
		t.Fatalf("card must show area Go:\n%s", v)
	}
	if !strings.Contains(v, "1h 30m") {
		t.Fatalf("card must preserve duration 5400s as 1h 30m:\n%s", v)
	}
	// Read-only: no slot chrome, no selection, no cursor.
	for _, bad := range []string{"SLOT", "Slot 1", "Slot 2", "DURATION", "SELECTED", "▶", "●", "▌"} {
		if strings.Contains(v, bad) {
			t.Fatalf("read-only card must not contain %q:\n%s", bad, v)
		}
	}
	// Day-card language: bordered card with day header, divider, and the
	// day total top-right.
	for _, want := range []string{"┌", "─", "1h 30m"} {
		if !strings.Contains(v, want) {
			t.Fatalf("card must contain %q:\n%s", want, v)
		}
	}
	// Compact: area and duration share one line.
	found := false
	for _, ln := range strings.Split(v, "\n") {
		if strings.Contains(ln, "Go") && strings.Contains(ln, "1h 30m") {
			found = true
		}
	}
	if !found {
		t.Fatalf("compact entry must show area + duration on one line:\n%s", v)
	}
	// Must not be the old flat row.
	if strings.Contains(v, "Mon · slot 0") || strings.Contains(v, "01 • Mon") {
		t.Fatalf("card must not be flat row:\n%s", v)
	}
}

func TestSWECardFollowsForgeConventions(t *testing.T) {
	// SWE day cards keep the Forge card language: borders + divider.
	swe := stripANSIForTest(SWERoutineCard{Day: "MONDAY", Total: "1h", Tasks: []SWECardTask{{Name: "Go", Dur: "1h"}}, Width: 34}.View())
	for _, want := range []string{"┌", "─", "MONDAY"} {
		if !strings.Contains(swe, want) {
			t.Fatalf("day card must contain %q:\n%s", want, swe)
		}
	}
	// Read-only: no focus marker even though GOV keeps its own.
	govF := stripANSIForTest(ExamCard{Name: "X", Done: 0, Total: 1, Focused: true, Width: 34}.View())
	if !strings.Contains(govF, "●") {
		t.Fatalf("GOV focused card must still show ●")
	}
	if strings.Contains(swe, "●") {
		t.Fatalf("SWE read-only card must never show ●:\n%s", swe)
	}
	if strings.Contains(swe, "SELECTED") {
		t.Fatalf("SWE read-only card must never show SELECTED:\n%s", swe)
	}
}

func TestSWECardReadOnlyNoCursor(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.goTab(TabSWE)
	// Seed two slots same weekday via direct slot injection (display-only).
	s := app.swe
	s.areas = []db.SWEArea{{ID: "area-go", Name: "golang"}}
	s.slots = []db.SWERoutineSlot{
		{ID: "s1", Weekday: 1, Position: 0, AreaID: "area-go", DurationSeconds: 3600},
		{ID: "s2", Weekday: 1, Position: 1, AreaID: "area-go", DurationSeconds: 1800},
	}
	// Both same-day slots share one Monday card, rendered read-only.
	view := stripANSIForTest(s.routineCardsView(120, 30))
	if strings.Count(view, "MONDAY") != 1 {
		t.Fatalf("day card must appear once:\n%s", view)
	}
	if !strings.Contains(view, "Go") {
		t.Fatalf("day card must show cased area Go:\n%s", view)
	}
	for _, bad := range []string{"Slot 1", "Slot 2", "SLOT", "SELECTED", "▶", "●", "▌"} {
		if strings.Contains(view, bad) {
			t.Fatalf("read-only grid must not contain %q:\n%s", bad, view)
		}
	}
	// Navigation keys are no-ops on the read-only overview.
	before := view
	for _, k := range []string{"j", "k", "h", "l", "up", "down", "enter"} {
		s.Update(testKey(k))
	}
	after := stripANSIForTest(s.routineCardsView(120, 30))
	if before != after {
		t.Fatalf("Update must be no-op on read-only grid:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestSWEViewCasing(t *testing.T) {
	app := newFlowApp(t)
	app.width, app.height = 120, 36
	app.goTab(TabSWE)
	v := stripANSIForTest(app.swe.View())
	// Compact grid header keeps professional casing (not raw caps).
	for _, want := range []string{"SWE ROUTINE"} {
		if !strings.Contains(v, want) {
			t.Fatalf("SWE view missing casing %q:\n%s", want, v)
		}
	}
	for _, bad := range []string{"SWE ROUTINE SLOTS", "SCHEDULED TIME", "ROUTINE SLOTS", "SWE AREAS"} {
		if strings.Contains(v, bad) {
			t.Fatalf("SWE view must not show two-panel %q:\n%s", bad, v)
		}
	}
	// Area display names inside day cards keep deliberate casing.
	s := app.swe
	s.areas = []db.SWEArea{{ID: "a", Name: "golang"}}
	s.slots = []db.SWERoutineSlot{{ID: "s1", Weekday: 1, Position: 0, AreaID: "a", DurationSeconds: 3600}}
	vc := stripANSIForTest(s.routineCardsView(120, 30))
	if !strings.Contains(vc, "Go") {
		t.Fatalf("day card must show cased area Go:\n%s", vc)
	}
}
