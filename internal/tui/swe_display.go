package tui

import "strings"

// SWEDisplayName returns the deliberate human-readable display name for a
// stored SWE area name. Storage strings are unchanged; this is display-only.
// Explicit overrides cover known raw catalogue values; unknown values are
// returned trimmed (never blindly Title-cased) to preserve intentional
// technical casing (SWE, DSA, GOV, CN, SQL, HTTP, API, CI/CD, PostgreSQL,
// JavaScript, TypeScript, MySQL, cURL, K8s, OS, DBMS, ...).
func SWEDisplayName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}
	key := strings.ToLower(trimmed)
	// Collapse internal whitespace for robust matching (e.g. extra spaces).
	key = strings.Join(strings.Fields(key), " ")
	if v, ok := sweDisplayOverrides[key]; ok {
		return v
	}
	// Normalise spacing around parentheses/slashes for variant inputs.
	norm := strings.ReplaceAll(key, "( ", "(")
	norm = strings.ReplaceAll(norm, " )", ")")
	norm = strings.ReplaceAll(norm, " (", "(")
	norm = strings.ReplaceAll(norm, ") ", ")")
	if v, ok := sweDisplayOverrides[norm]; ok {
		return v
	}
	return trimmed
}

var sweDisplayOverrides = map[string]string{
	"golang":                     "Go",
	"go":                         "Go",
	"docker":                     "Docker",
	"prometheus":                 "Prometheus",
	"postgres/sql":               "PostgreSQL",
	"postgres / sql":             "PostgreSQL",
	"postgres":                   "PostgreSQL",
	"postgresql":                 "PostgreSQL",
	"grafana":                    "Grafana",
	"curl":                       "cURL",
	"bash":                       "Bash",
	"linux(arch)+my own setup":   "Linux (Arch) + My Own Setup",
	"linux(arch) + my own setup": "Linux (Arch) + My Own Setup",
	"linux (arch)+my own setup":  "Linux (Arch) + My Own Setup",
	"ci/cd(github actions)":      "CI/CD (GitHub Actions)",
	"ci/cd (github actions)":     "CI/CD (GitHub Actions)",
	"cicd(github actions)":       "CI/CD (GitHub Actions)",
}
