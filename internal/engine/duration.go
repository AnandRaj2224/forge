package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatDurationLong renders seconds as product metrics: 3600->"1h",
// 9300->"1h 35m", 300->"5m", 90->"1m 30s", 42->"42s". Single canonical
// formatter for solve time, session elapsed, planned/actual, variance,
// time by domain, mock avg speed where appropriate. Raw seconds stay in storage.
func FormatDurationLong(secs int) string {
	if secs < 0 {
		secs = 0
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h == 0 {
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %02dm", h, m)
}

// FormatSigned renders signed durations: -600->"-10m", +660->"+11m".
func FormatSigned(secs int) string {
	if secs < 0 {
		return "-" + FormatDurationLong(-secs)
	}
	if secs > 0 {
		return "+" + FormatDurationLong(secs)
	}
	return FormatDurationLong(secs)
}

// ParseDurationSeconds parses human duration strings into seconds.
// Accepted: "30s", "5m", "1h", "1h 15m", "1h15m", "1m 30s".
// Bare integers are accepted as seconds for backward compatibility with
// already-persisted inputs and existing tests (storage meaning unchanged),
// but UI placeholders must make units explicit (e.g. "e.g. 5m") so users
// never type ambiguous bare numbers by default.
func ParseDurationSeconds(s string) (int, string) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, "required"
	}
	low := strings.ToLower(t)
	if v, err := strconv.Atoi(strings.TrimSpace(low)); err == nil {
		if v < 0 {
			return 0, "must be >= 0"
		}
		return v, ""
	}
	nospace := strings.ReplaceAll(low, " ", "")
	if nospace == "" {
		return 0, "required"
	}
	total := 0
	i := 0
	seen := false
	for i < len(nospace) {
		j := i
		for j < len(nospace) && nospace[j] >= '0' && nospace[j] <= '9' {
			j++
		}
		if j == i {
			return 0, "use like 30s, 5m, 1h 15m"
		}
		num, err := strconv.Atoi(nospace[i:j])
		if err != nil {
			return 0, "use like 30s, 5m, 1h 15m"
		}
		if j >= len(nospace) {
			return 0, "use like 30s, 5m, 1h 15m"
		}
		switch nospace[j] {
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		default:
			return 0, "use like 30s, 5m, 1h 15m"
		}
		seen = true
		i = j + 1
	}
	if !seen {
		return 0, "use like 30s, 5m, 1h 15m"
	}
	return total, ""
}

// ParseAvgSpeed parses an avg-question-speed input. Empty is allowed by the
// legacy form helper (nil, no error); engine evidence validation requires a
// positive value when the bundle demands it (see evidence.go).
func ParseAvgSpeed(s string) (*float64, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ""
	}
	// Allow trailing "s" (e.g. "17.2s") for consistency with duration UX.
	low := strings.ToLower(s)
	low = strings.TrimSuffix(low, "s")
	low = strings.TrimSpace(low)
	v, err := strconv.ParseFloat(low, 64)
	if err != nil || v <= 0 {
		return nil, "average speed must be > 0"
	}
	return &v, ""
}
