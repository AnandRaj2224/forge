package main

import (
	"regexp"
	"strings"
)

// titleCase applies Forge's conservative title-case policy to a display name.
//
// The policy is deliberately conservative: it capitalizes ordinary
// mid-sentence words but leaves alone anything that carries meaning beyond
// capitalization:
//
//   - all-caps acronyms: DI, PM, NBFC, EM, CPU, OR, OSI, TCP/IP
//   - file extensions and dotted names: .docx, .xlsx, IPv4.x
//   - "etc."
//   - a capitalized "The" that introduces a proper noun
//   - single-letter brand fragments: e-Drive
//
// Small words (of, and, to, ...) stay lowercase unless they open or close
// the title. "of" and "vs" are always lowercased because they are never
// proper nouns.
func titleCase(s string) string {
	toks := strings.Split(s, " ")
	out := make([]string, len(toks))
	for i, t := range toks {
		if strings.Contains(t, ".") {
			out[i] = t
			continue
		}
		first := i == 0
		last := i == len(toks)-1
		out[i] = wordRun.ReplaceAllStringFunc(t, func(w string) string {
			low := strings.ToLower(w)
			if len(w) > 1 && isAlpha(w) && w == strings.ToUpper(w) {
				return w // acronym
			}
			if alwaysLowerWords[low] {
				return low
			}
			if low == "the" {
				if w == strings.ToLower(w) {
					return "the"
				}
				return "The"
			}
			if smallWords[low] && !first && !last {
				if w == strings.ToLower(w) {
					return low
				}
				return w
			}
			if len(w) == 1 {
				return w
			}
			return strings.ToUpper(w[:1]) + w[1:]
		})
	}
	return strings.Join(out, " ")
}

var (
	wordRun = regexp.MustCompile(`[A-Za-z]+`)

	// smallWords may appear lowercase inside a title.
	smallWords = map[string]bool{
		"and": true, "or": true, "to": true, "in": true, "on": true,
		"for": true, "a": true, "an": true, "at": true, "by": true,
		"with": true, "from": true, "vs": true, "the": true,
	}

	// alwaysLowerWords are never proper nouns.
	alwaysLowerWords = map[string]bool{"of": true, "vs": true}
)

// isAlpha reports whether s is all ASCII letters.
func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return len(s) > 0
}
