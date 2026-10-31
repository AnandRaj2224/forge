// Command datagen regenerates seed.sql from the canonical data files in
// data/. The YAML/JSON files are the source of truth; seed.sql is the
// generated SQL that populates a fresh Forge database.
//
// Usage:
//
//	go run ./cmd/datagen -data data -out seed.sql
//
// The generator is deliberately dependency-free: it implements the small YAML
// subset used by the data files instead of pulling in a YAML library, so
// `go.mod` stays free of a runtime dependency the app itself never needs.
//
// Progress markers
//
//	A lecture or DSA problem is considered complete when it appears at or
//	before the `# done till here` comment in its file. Everything after the
//	marker is not started. The marker is cumulative: it means "everything up
//	to and including this line is done".
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dataDir := flag.String("data", "data", "directory holding the canonical data files")
	out := flag.String("out", "seed.sql", "path of the generated SQL file")
	progressAt := flag.Int64("progress-at", 0, "unix seconds stamped on the imported-progress session (0 = 2026-10-19 12:00 IST)")
	flag.Parse()

	if *progressAt == 0 {
		*progressAt = defaultProgressAt
	}

	g, err := load(*dataDir)
	if err != nil {
		fatal(err)
	}
	if err := g.validate(); err != nil {
		fatal(err)
	}
	if err := g.emit(*out, *progressAt); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "datagen: wrote %s\n", *out)
	g.report()
}

// defaultProgressAt is 2026-10-19 12:00:00 Asia/Kolkata.
const defaultProgressAt int64 = 1792391400

// subjectSpec is the canonical identity of one GOV subject. The ID is stable
// and referenced from Go code (see internal/db/models.go), so it is declared
// explicitly rather than derived from the display name. The slug is the
// prefix used for topic and lecture IDs.
type subjectSpec struct {
	ID    string
	Name  string
	Slug  string
	Order int
	File  string
}

// subjects is the canonical subject catalogue, in display order. The display
// names are UPPER CASE by convention (see data/exams.yaml).
var subjects = []subjectSpec{
	{ID: "subj-quant", Name: "QUANT", Slug: "quant", Order: 1, File: "quant.yaml"},
	{ID: "subj-reasoning", Name: "REASONING", Slug: "reasoning", Order: 2, File: "reasoning.yaml"},
	{ID: "subj-english", Name: "ENGLISH", Slug: "english", Order: 3, File: "english.yaml"},
	{ID: "subj-history", Name: "HISTORY", Slug: "history", Order: 4, File: "history.yaml"},
	{ID: "subj-polity", Name: "POLITY", Slug: "polity", Order: 5, File: "polity.yaml"},
	{ID: "subj-geography", Name: "GEOGRAPHY", Slug: "geography", Order: 6, File: "geography.yaml"},
	{ID: "subj-general-science", Name: "GENERAL SCIENCE", Slug: "general-science", Order: 7, File: "General Science.yaml"},
	{ID: "subj-banking-fa", Name: "BANKING AND FINANCIAL AWARENESS", Slug: "banking-and-financial-awareness", Order: 8, File: "Banking_and_Financial_Awareness.yaml"},
	{ID: "subj-computer-awareness", Name: "COMPUTER AWARENESS", Slug: "computer-awareness", Order: 9, File: "Computer_Awareness.yaml"},
	{ID: "subj-it", Name: "PROFESSIONAL KNOWLEDGE", Slug: "it", Order: 10, File: "Professional_Knowledge.yaml"},
}

// examSpec is the canonical identity of one exam. The ID is stable, the
// display name is UPPER CASE, and the order drives the app's progression.
type examSpec struct {
	ID    string
	Name  string
	Order int
}

var exams = []examSpec{
	{ID: "exam-sbi-po", Name: "SBI PO", Order: 1},
	{ID: "exam-sbi-clerk", Name: "SBI CLERK", Order: 2},
	{ID: "exam-ibps-so-it", Name: "IBPS SO IT", Order: 3},
	{ID: "exam-ibps-clerk", Name: "IBPS CLERK", Order: 4},
	{ID: "exam-ibps-po", Name: "IBPS PO", Order: 5},
	{ID: "exam-rrb-clerk", Name: "RRB CLERK", Order: 6},
	{ID: "exam-rrb-po", Name: "RRB PO", Order: 7},
	{ID: "exam-rbi-assistant", Name: "RBI ASSISTANT", Order: 8},
	{ID: "exam-sbi-so-it", Name: "SBI SO IT", Order: 9},
	{ID: "exam-afcat", Name: "AFCAT", Order: 10},
	{ID: "exam-ssc-cgl", Name: "SSC CGL", Order: 11},
	{ID: "exam-ssc-chsl", Name: "SSC CHSL", Order: 12},
	{ID: "exam-ssc-selection-posts", Name: "SSC SELECTION POSTS", Order: 13},
	{ID: "exam-ssc-mts", Name: "SSC MTS", Order: 14},
	{ID: "exam-rrb-ntpc", Name: "RRB NTPC", Order: 15},
	{ID: "exam-cds", Name: "CDS", Order: 16},
}

// focusExamID is the exam the app focuses on by default.
const focusExamID = "exam-sbi-po"

// generator holds the fully parsed model that will be emitted.
type generator struct {
	subjects []subjectSpec
	exams    []examSpec

	// topics[subjectSlug] -> ordered topics
	topics map[string][]*topic
	// syllabus[examID] -> ordered subject list, each with ordered topics
	syllabus map[string][]*syllabusEntry
	// dsa problems in file order
	dsa []*dsaProblem
	// swe areas and routine slots
	sweAreas []*sweArea
	sweSlots []*sweSlot
	// quotes in file order
	quotes []quote
	// practiceSessions[subjectSlug] -> ordered practice sessions
	practiceSessions map[string][]*practiceSession
}

type practiceSession struct {
	Subject *subjectSpec
	Code    string
	Order   int
	ID      string
	Tests   []*practiceTest
}

type practiceTest struct {
	Session *practiceSession
	Subject *subjectSpec
	Title   string
	Order   int
	ID      string
}

type topic struct {
	Subject  *subjectSpec
	Name     string
	ID       string
	Order    int
	Lectures []*lecture
}

type lecture struct {
	Topic    *topic
	Title    string
	ID       string
	Order    int
	complete bool
}

type syllabusEntry struct {
	Subject *subjectSpec
	Topics  []*topic
}

type dsaProblem struct {
	Title      string
	Difficulty string
	Pattern    string
	ID         string
	Order      int
	attempted  bool
}

type sweArea struct {
	ID    string
	Name  string
	Order int
}

type sweSlot struct {
	ID       string
	Weekday  int
	Position int
	AreaID   string
	Seconds  int
}

type quote struct {
	Author string
	Text   string
}

// load reads and parses every canonical data file.
func load(dir string) (*generator, error) {
	g := &generator{
		subjects: subjects,
		exams:    exams,
		topics:   map[string][]*topic{},
		syllabus: map[string][]*syllabusEntry{},
	}
	byName := map[string]*subjectSpec{}
	for i := range g.subjects {
		byName[g.subjects[i].Name] = &g.subjects[i]
	}

	for _, spec := range g.subjects {
		ts, err := parseSubjectFile(filepath.Join(dir, spec.File))
		if err != nil {
			return nil, err
		}
		for i, t := range ts {
			t.Subject = &spec
			t.Order = i + 1
			t.ID = "topic-" + spec.Slug + "-" + slug(t.Name)
			for j, l := range t.Lectures {
				l.Topic = t
				l.Order = j + 1
				l.ID = fmt.Sprintf("lec-%s-%s-%02d", spec.Slug, slug(t.Name), j+1)
			}
		}
		g.topics[spec.Slug] = ts
	}

	groups, err := parseExamsFile(filepath.Join(dir, "exams.yaml"))
	if err != nil {
		return nil, err
	}
	examByName := map[string]*examSpec{}
	for i := range g.exams {
		examByName[g.exams[i].Name] = &g.exams[i]
	}
	for _, grp := range groups {
		for _, name := range grp.Exams {
			spec, ok := examByName[name]
			if !ok {
				return nil, fmt.Errorf("exams.yaml: unknown exam %q in group %q", name, grp.Key)
			}
			for _, sl := range grp.Subjects {
				sub, ok := byName[sl.Subject]
				if !ok {
					return nil, fmt.Errorf("exams.yaml: unknown subject %q in group %q", sl.Subject, grp.Key)
				}
				known := map[string]*topic{}
				for _, t := range g.topics[sub.Slug] {
					known[t.Name] = t
				}
				se := &syllabusEntry{Subject: sub}
				for _, tn := range sl.Topics {
					t, ok := known[tn]
					if !ok {
						return nil, fmt.Errorf("exams.yaml: subject %q has no topic %q", sl.Subject, tn)
					}
					se.Topics = append(se.Topics, t)
				}
				g.syllabus[spec.ID] = append(g.syllabus[spec.ID], se)
			}
		}
	}

	g.dsa, err = parseDSAFile(filepath.Join(dir, "dsa.yaml"))
	if err != nil {
		return nil, err
	}
	g.sweAreas, g.sweSlots, err = parseSWERoutine(filepath.Join(dir, "swe_routine.yaml"))
	if err != nil {
		return nil, err
	}
	g.quotes, err = parseQuotes(filepath.Join(dir, "quotes.json"))
	if err != nil {
		return nil, err
	}

	testOrderFiles := map[string]string{
		"quant":     "quantTestOrder.yaml",
		"english":   "englishTestOrder.yaml",
		"reasoning": "reasoningTestOrder.yaml",
	}
	g.practiceSessions = map[string][]*practiceSession{}
	for _, spec := range g.subjects {
		if fname, ok := testOrderFiles[spec.Slug]; ok {
			pPath := filepath.Join(dir, fname)
			if _, err := os.Stat(pPath); err == nil {
				sCopy := spec
				sess, err := parsePracticeTestOrder(pPath, &sCopy)
				if err != nil {
					return nil, err
				}
				g.practiceSessions[spec.Slug] = sess
			}
		}
	}
	return g, nil
}

// validate enforces the invariants the data files must satisfy before they
// are allowed to become a seed.
func (g *generator) validate() error {
	seenTopic := map[string]bool{}
	for _, spec := range g.subjects {
		for _, t := range g.topics[spec.Slug] {
			if seenTopic[t.ID] {
				return fmt.Errorf("duplicate topic id %q", t.ID)
			}
			seenTopic[t.ID] = true
			if len(t.Lectures) == 0 {
				return fmt.Errorf("topic %q (%s) has no lectures", t.Name, spec.Name)
			}
			seen := map[string]bool{}
			for _, l := range t.Lectures {
				if seen[l.Title] {
					return fmt.Errorf("duplicate lecture %q in topic %q", l.Title, t.Name)
				}
				seen[l.Title] = true
			}
		}
	}
	for _, e := range g.exams {
		entries := g.syllabus[e.ID]
		if len(entries) == 0 {
			return fmt.Errorf("exam %q has no subjects", e.Name)
		}
		seen := map[string]bool{}
		for _, se := range entries {
			for _, t := range se.Topics {
				if seen[t.ID] {
					return fmt.Errorf("exam %q lists topic %q twice", e.Name, t.Name)
				}
				seen[t.ID] = true
			}
		}
	}
	// Every catalogue topic must be reachable from at least one exam,
	// otherwise it can never be scheduled by the app.
	reachable := map[string]bool{}
	for _, entries := range g.syllabus {
		for _, se := range entries {
			for _, t := range se.Topics {
				reachable[t.ID] = true
			}
		}
	}
	for _, spec := range g.subjects {
		for _, t := range g.topics[spec.Slug] {
			if !reachable[t.ID] {
				return fmt.Errorf("topic %q (%s) is not part of any exam syllabus", t.Name, spec.Name)
			}
		}
	}
	for slug, sess := range g.practiceSessions {
		seenOrder := map[int]bool{}
		for i, s := range sess {
			if s.Order != i+1 {
				return fmt.Errorf("%s: non-sequential session order %d at index %d", slug, s.Order, i)
			}
			if seenOrder[s.Order] {
				return fmt.Errorf("%s: duplicate session order %d", slug, s.Order)
			}
			seenOrder[s.Order] = true
			if len(s.Tests) == 0 {
				return fmt.Errorf("%s session %s has no tests", slug, s.Code)
			}
			seenTest := map[string]bool{}
			for _, t := range s.Tests {
				if t.Title == "" {
					return fmt.Errorf("%s session %s has empty test title", slug, s.Code)
				}
				if seenTest[t.Title] {
					return fmt.Errorf("%s session %s has duplicate test title %q", slug, s.Code, t.Title)
				}
				seenTest[t.Title] = true
			}
		}
	}
	return nil
}

// report prints a short summary of what was generated.
func (g *generator) report() {
	ntopics, nlectures, ncompleted := 0, 0, 0
	for _, spec := range g.subjects {
		for _, t := range g.topics[spec.Slug] {
			ntopics++
			nlectures += len(t.Lectures)
			for _, l := range t.Lectures {
				if l.complete {
					ncompleted++
				}
			}
		}
	}
	nsyl := 0
	for _, entries := range g.syllabus {
		for _, se := range entries {
			nsyl += len(se.Topics)
		}
	}
	nattempts := 0
	for _, p := range g.dsa {
		if p.attempted {
			nattempts++
		}
	}
	fmt.Fprintf(os.Stderr, "datagen: %d subjects, %d exams, %d topics, %d lectures, %d syllabus rows\n",
		len(g.subjects), len(g.exams), ntopics, nlectures, nsyl)
	fmt.Fprintf(os.Stderr, "datagen: progress imported: %d lectures completed, %d DSA problems attempted\n",
		ncompleted, nattempts)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "datagen:", err)
	os.Exit(1)
}

// nowUnix is used for rows that must carry a timestamp but carry no meaning
// of their own.
func nowUnix() int64 { return time.Now().UTC().Unix() }
