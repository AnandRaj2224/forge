package tui

import (
	"testing"

	db "forge/internal/db"
)

func TestFormatDuration(t *testing.T) {
	cases := map[int]string{3600: "1h", 7200: "2h", 9000: "2h30", 5400: "1h30", 2400: "40m", 90: "1m", 45: "45s"}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Fatalf("FormatDuration(%d)=%q want %q", in, got, want)
		}
	}
}

func TestValidatePracticeRow(t *testing.T) {
	if _, _, _, _, msg := ValidatePracticeRow("1380", "20", "16", "17.2"); msg != "" {
		t.Fatalf("valid row rejected: %s", msg)
	}
	if _, _, _, _, msg := ValidatePracticeRow("10", "5", "9", ""); msg == "" {
		t.Fatal("correct>attempted must fail")
	}
	if _, _, _, _, msg := ValidatePracticeRow("", "5", "3", ""); msg == "" {
		t.Fatal("empty time must fail")
	}
	if _, _, _, _, msg := ValidatePracticeRow("10", "5", "3", "0"); msg == "" {
		t.Fatal("zero avg must fail")
	}
	// Avg is optional.
	if _, _, _, avg, msg := ValidatePracticeRow("10", "5", "3", ""); msg != "" || avg != nil {
		t.Fatalf("empty avg should pass as nil: %v %v", avg, msg)
	}
}

func TestValidateSolveTime(t *testing.T) {
	if _, _, msg := ValidateSolveTime("300"); msg != "" {
		t.Fatalf("valid solve rejected: %s", msg)
	}
	if _, _, msg := ValidateSolveTime(""); msg == "" {
		t.Fatal("empty solve must fail (required evidence)")
	}
	if _, _, msg := ValidateSolveTime("-1"); msg == "" {
		t.Fatal("negative solve must fail")
	}
}

func TestDecideDSAAttemptMode(t *testing.T) {
	if got := DecideDSAAttemptMode(nil, db.DSAModeNew); got != db.DSAModeNew {
		t.Fatalf("first NEW should stay NEW, got %s", got)
	}
	existing := []db.DSAAttempt{{Mode: db.DSAModeNew}}
	if got := DecideDSAAttemptMode(existing, db.DSAModeNew); got != db.DSAModeRepeat {
		t.Fatalf("second NEW must become REPEAT, got %s", got)
	}
	if got := DecideDSAAttemptMode(nil, db.DSAModeRepeat); got != db.DSAModeRepeat {
		t.Fatalf("REPEAT stays REPEAT, got %s", got)
	}
}

func TestDSAPassUI(t *testing.T) {
	pass := []db.DSAAttempt{{Independent: true, Optimal: true, HintUsed: false}}
	if !DSAPassUI(pass) {
		t.Fatal("passing attempt not detected")
	}
	fail := []db.DSAAttempt{{Independent: true, Optimal: false, HintUsed: false}}
	if DSAPassUI(fail) {
		t.Fatal("non-optimal must not pass")
	}
	if DSAPassUI(nil) {
		t.Fatal("empty must not pass")
	}
}

func TestGroupDSAPatterns(t *testing.T) {
	probs := []db.DSAProblem{
		{ID: "a", Pattern: "Hash Map"}, {ID: "b", Pattern: "Stack"}, {ID: "c", Pattern: "Hash Map"},
	}
	groups := GroupDSAPatterns(probs)
	if len(groups) != 2 || groups[0].Name != "Hash Map" || len(groups[0].Problems) != 2 {
		t.Fatalf("bad grouping: %+v", groups)
	}
}

func TestProgressBarClamp(t *testing.T) {
	if got := (ProgressBar{Ratio: 2, Width: 4}.View()); got != "████" {
		t.Fatalf("clamp high: %q", got)
	}
	if got := (ProgressBar{Ratio: -1, Width: 4}.View()); got != "░░░░" {
		t.Fatalf("clamp low: %q", got)
	}
}

func TestButtonDisabledView(t *testing.T) {
	b := Button{Label: "SAVE", Disabled: true}
	if v := b.View(); v == "" {
		t.Fatal("empty button view")
	}
	r := NewRadio([]string{"YES", "NO"})
	if r.Selected != -1 {
		t.Fatal("radio must start unselected")
	}
	r.Move(1)
	r.Choose()
	if r.Selected != 1 {
		t.Fatal("radio choose failed")
	}
	c := Checkbox{Label: "x"}
	if c.Checked {
		t.Fatal("checkbox must start unchecked")
	}
}
