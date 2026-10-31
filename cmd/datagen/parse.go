package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// progressMarker is the comment that marks the end of the completed portion
// of a file. Everything up to and including the marked line is done.
const progressMarker = "# done till here"

// yamlLine is one meaningful line of a data file.
type yamlLine struct {
	indent int
	key    string // for "key:" lines
	value  string // for "key: value" lines
	item   string // for "- value" lines
	isItem bool
	marked bool // this line carries the "# done till here" marker
	number int
}

// readLines parses a data file into its meaningful lines. Blank lines and
// whole-line comments are dropped; trailing "# ..." comments are stripped
// from values and recorded on the line so the progress marker can be found.
func readLines(path string) ([]yamlLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []yamlLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	n := 0
	for sc.Scan() {
		n++
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			// A whole-line comment. It can still carry the marker, but a
			// marker on its own line carries no progress information.
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		marked := false
		if i := strings.Index(raw, "#"); i >= 0 {
			if strings.Contains(strings.ToLower(raw[i:]), progressMarker) {
				marked = true
			}
			raw = raw[:i]
			trimmed = strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
		}
		l := yamlLine{indent: indent, marked: marked, number: n}
		if strings.HasPrefix(trimmed, "- ") {
			l.isItem = true
			l.item = unquote(strings.TrimSpace(trimmed[2:]))
		} else {
			k, v, ok := strings.Cut(trimmed, ":")
			if !ok {
				return nil, fmt.Errorf("%s:%d: unsupported line %q", path, n, trimmed)
			}
			l.key = strings.TrimSpace(k)
			l.value = unquote(strings.TrimSpace(v))
		}
		out = append(out, l)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// unquote removes matching surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// markerIndex returns the index of the first line carrying the progress
// marker, or -1.
func markerIndex(lines []yamlLine) int {
	for i, l := range lines {
		if l.marked {
			return i
		}
	}
	return -1
}

// parseSubjectFile reads a subject catalogue file:
//
//	Topic Name:
//	  Lectures:
//	    - Lecture A
//	Other Topic:
//	  - Lecture B
//
// The second form (no "Lectures:" key) is also supported. The progress
// marker is cumulative: every lecture at or before the marked line is
// complete.
func parseSubjectFile(path string) ([]*topic, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var topics []*topic
	var cur *topic
	mark := markerIndex(lines)
	lectureNo := 0
	for i, l := range lines {
		switch {
		case l.isItem && l.indent >= 2 && cur != nil:
			lectureNo++
			lec := &lecture{Title: l.item}
			if mark >= 0 && i <= mark {
				lec.complete = true
			}
			cur.Lectures = append(cur.Lectures, lec)
		case !l.isItem && l.indent == 0:
			cur = &topic{Name: l.key}
			topics = append(topics, cur)
		}
	}
	return topics, nil
}

// examGroup is one top-level block in exams.yaml.
type examGroup struct {
	Key      string
	Exams    []string
	Subjects []subjectList
}

type subjectList struct {
	Subject string
	Topics  []string
}

// parseExamsFile reads exams.yaml:
//
//	EXAM A,EXAM B:
//	  SUBJECT:
//	    - Topic
func parseExamsFile(path string) ([]examGroup, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var groups []examGroup
	var cur *examGroup
	var curSubj *subjectList
	for _, l := range lines {
		switch {
		case l.isItem:
			if curSubj != nil {
				curSubj.Topics = append(curSubj.Topics, l.item)
			}
		case l.indent == 0:
			groups = append(groups, examGroup{Key: l.key})
			cur = &groups[len(groups)-1]
			for _, e := range strings.Split(l.key, ",") {
				e = strings.TrimSpace(e)
				if e != "" {
					cur.Exams = append(cur.Exams, e)
				}
			}
			curSubj = nil
		case l.indent == 2 && cur != nil:
			cur.Subjects = append(cur.Subjects, subjectList{Subject: l.key})
			curSubj = &cur.Subjects[len(cur.Subjects)-1]
		}
	}
	return groups, nil
}

// parseDSAFile reads dsa.yaml:
//
//	dsa:
//	  - pattern: "Arrays & Hashing"
//	    problems:
//	      - title: "Two Sum"
//	        difficulty: Easy
//
// The progress marker is cumulative over the problem list.
func parseDSAFile(path string) ([]*dsaProblem, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	mark := markerIndex(lines)
	var out []*dsaProblem
	pat := ""
	problemNo := 0
	for i, l := range lines {
		if l.isItem && strings.HasPrefix(l.item, "pattern:") {
			pat = unquote(strings.TrimSpace(strings.TrimPrefix(l.item, "pattern:")))
			continue
		}
		if l.isItem && strings.HasPrefix(l.item, "title:") {
			problemNo++
			p := &dsaProblem{
				Title:     unquote(strings.TrimSpace(strings.TrimPrefix(l.item, "title:"))),
				Pattern:   pat,
				Order:     problemNo,
				ID:        fmt.Sprintf("dsa-%03d", problemNo),
				attempted: mark >= 0 && i <= mark,
			}
			out = append(out, p)
			continue
		}
		if !l.isItem && l.key == "difficulty" && len(out) > 0 {
			out[len(out)-1].Difficulty = l.value
		}
	}
	return out, nil
}

// parseSWERoutine reads swe_routine.yaml:
//
//	routine:
//	  monday:
//	    - area: golang
//	      duration: 120 mins
func parseSWERoutine(path string) ([]*sweArea, []*sweSlot, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, nil, err
	}
	weekdays := map[string]int{
		"monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4,
		"friday": 5, "saturday": 6, "sunday": 7,
	}
	var areas []*sweArea
	var slots []*sweSlot
	areaByName := map[string]*sweArea{}
	curDay := 0
	var curArea string
	var curDur int
	posByDay := map[int]int{}

	flush := func() {
		if curArea == "" {
			return
		}
		a, ok := areaByName[curArea]
		if !ok {
			a = &sweArea{ID: "swe-area-" + slug(curArea), Name: curArea, Order: len(areas) + 1}
			areaByName[curArea] = a
			areas = append(areas, a)
		}
		pos := posByDay[curDay]
		posByDay[curDay] = pos + 1
		slots = append(slots, &sweSlot{
			ID:       fmt.Sprintf("swe-slot-%d-%02d", curDay, pos),
			Weekday:  curDay,
			Position: pos,
			AreaID:   a.ID,
			Seconds:  curDur,
		})
		curArea = ""
		curDur = 0
	}
	for _, l := range lines {
		if l.isItem && strings.HasPrefix(l.item, "area:") {
			flush()
			curArea = unquote(strings.TrimSpace(strings.TrimPrefix(l.item, "area:")))
			continue
		}
		if !l.isItem && l.key == "duration" {
			curDur = parseDuration(l.value)
			continue
		}
		if !l.isItem && l.indent == 2 {
			if d, ok := weekdays[strings.ToLower(l.key)]; ok {
				flush()
				curDay = d
			}
		}
	}
	flush()
	return areas, slots, nil
}

var durRe = regexp.MustCompile(`^(\d+)\s*min`)

// parseDuration turns "120 mins" / "30mins" into seconds.
func parseDuration(s string) int {
	m := durRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n * 60
}

// quoteJSON is the on-disk shape of data/quotes.json.
type quoteJSON struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}

// parseQuotes reads data/quotes.json.
func parseQuotes(path string) ([]quote, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw []quoteJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]quote, 0, len(raw))
	for _, q := range raw {
		out = append(out, quote{Author: q.Author, Text: q.Text})
	}
	return out, nil
}

// parsePracticeTestOrder reads a test order file:
//
//	ses_1:
//	  - "Test Title A"
//	  - "Test Title B"
//
func parsePracticeTestOrder(path string, spec *subjectSpec) ([]*practiceSession, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var sessions []*practiceSession
	var cur *practiceSession
	for _, l := range lines {
		switch {
		case !l.isItem && strings.HasPrefix(l.key, "ses_"):
			code := l.key
			order, err := strconv.Atoi(strings.TrimPrefix(code, "ses_"))
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid session code %q", path, l.number, code)
			}
			cur = &practiceSession{
				Subject: spec,
				Code:    code,
				Order:   order,
				ID:      fmt.Sprintf("prac-ses-%s-%d", spec.Slug, order),
			}
			sessions = append(sessions, cur)
		case l.isItem && cur != nil:
			tOrder := len(cur.Tests) + 1
			test := &practiceTest{
				Session: cur,
				Subject: spec,
				Title:   l.item,
				Order:   tOrder,
				ID:      fmt.Sprintf("test-%s-%s-%d", spec.Slug, cur.Code, tOrder),
			}
			cur.Tests = append(cur.Tests, test)
		}
	}
	return sessions, nil
}

// slug converts a display name into a stable lowercase identifier fragment.
func slug(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", " and ")
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
