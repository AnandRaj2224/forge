package engine

import db "forge/internal/db"

// IsDSAPass is the single authoritative pass rule:
//
//	independent AND optimal AND NOT hint_used
//
// The TUI, History, Reviews, repeat engine and problem detail must all use
// this rule. Never duplicate the condition.
func IsDSAPass(a db.DSAAttempt) bool {
	return a.Independent && a.Optimal && !a.HintUsed
}

// AnyPass reports whether any attempt in the list passes.
func AnyPass(attempts []db.DSAAttempt) bool {
	for _, a := range attempts {
		if IsDSAPass(a) {
			return true
		}
	}
	return false
}

// Repeat progression model. Intervals are derived from the existing fixture
// semantics: stage 0->1d, 1->3d, 2->7d, 3->14d, 4+->30d. This isolates the
// progression decision instead of hiding magic constants in save paths.
// The stage/interval table is explicit here so tests pin it.
var repeatIntervals = []int{1, 3, 7, 14, 30}

// IntervalForStage returns the review interval in days for a repeat stage.
func IntervalForStage(stage int) int {
	if stage < 0 {
		stage = 0
	}
	if stage >= len(repeatIntervals) {
		return repeatIntervals[len(repeatIntervals)-1]
	}
	return repeatIntervals[stage]
}

// MaxRepeatStage is the highest stage before intervals saturate.
func MaxRepeatStage() int { return len(repeatIntervals) - 1 }

// NextRepeatOnNew computes the repeat state after saving a NEW attempt:
// initialize/refresh according to progression and enter repeat scheduling.
// A passed NEW attempt enters stage 1; a failed NEW attempt enters stage 0.
// nextReview is the Forge local date (YYYY-MM-DD) interval days after
// localDate. Falls back to localDate when it does not parse.
func NextRepeatOnNew(pass bool, localDate string) (stage int, interval int, nextReview string) {
	if pass {
		stage = 1
	} else {
		stage = 0
	}
	interval = IntervalForStage(stage)
	nextReview = localDate
	if s, err := DefaultCalendar().StartOfDay(localDate); err == nil {
		nextReview = s.AddDate(0, 0, interval).Format("2006-01-02")
	}
	return stage, interval, nextReview
}

// NextRepeatOnRepeat computes progression after a REPEAT attempt:
// PASS advances one stage (saturating), FAIL resets to stage 0 per the
// existing intended rules.
func NextRepeatOnRepeat(curStage int, pass bool) (stage int, interval int) {
	if pass {
		stage = curStage + 1
		if stage > MaxRepeatStage() {
			stage = MaxRepeatStage()
		}
	} else {
		stage = 0
	}
	return stage, IntervalForStage(stage)
}
