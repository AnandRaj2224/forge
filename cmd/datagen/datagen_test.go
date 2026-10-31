package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dataDir resolves the canonical data directory relative to this package.
func dataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "data"))
	if err != nil {
		t.Fatalf("resolve data dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("data dir not found: %v", err)
	}
	return dir
}

func loadData(t *testing.T) *generator {
	t.Helper()
	g, err := load(dataDir(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return g
}

// TestDataFilesAreConsistent asserts the invariants that the canonical data
// files must satisfy before they can be turned into a seed. It is the
// guardrail that keeps data/*.yaml and the database from drifting apart.
func TestDataFilesAreConsistent(t *testing.T) {
	g := loadData(t)
	if err := g.validate(); err != nil {
		t.Fatalf("data files are not consistent: %v", err)
	}
}

// TestSubjectAndExamNamesAreUpperCase locks the naming convention: every
// subject name and every exam name is UPPER CASE.
func TestSubjectAndExamNamesAreUpperCase(t *testing.T) {
	g := loadData(t)
	for _, s := range g.subjects {
		if s.Name != strings.ToUpper(s.Name) {
			t.Errorf("subject %q name %q is not upper case", s.ID, s.Name)
		}
	}
	for _, e := range g.exams {
		if e.Name != strings.ToUpper(e.Name) {
			t.Errorf("exam %q name %q is not upper case", e.ID, e.Name)
		}
	}
}

// TestTopicNamesAreTitleCase locks the topic naming convention.
func TestTopicNamesAreTitleCase(t *testing.T) {
	g := loadData(t)
	for _, s := range g.subjects {
		for _, tp := range g.topics[s.Slug] {
			if want := titleCase(tp.Name); want != tp.Name {
				t.Errorf("topic %q is not title case, want %q", tp.Name, want)
			}
		}
	}
}

// TestLectureNamesAreTitleCase locks the lecture naming convention.
func TestLectureNamesAreTitleCase(t *testing.T) {
	g := loadData(t)
	for _, s := range g.subjects {
		for _, tp := range g.topics[s.Slug] {
			for _, l := range tp.Lectures {
				if want := titleCase(l.Title); want != l.Title {
					t.Errorf("lecture %q (topic %q) is not title case, want %q", l.Title, tp.Name, want)
				}
			}
		}
	}
}

// TestEveryTopicIsReachableFromAnExam guards against dead content: a topic
// that belongs to no exam syllabus can never be scheduled by the app.
func TestEveryTopicIsReachableFromAnExam(t *testing.T) {
	g := loadData(t)
	reachable := map[string]bool{}
	for _, entries := range g.syllabus {
		for _, se := range entries {
			for _, tp := range se.Topics {
				reachable[tp.ID] = true
			}
		}
	}
	for _, s := range g.subjects {
		for _, tp := range g.topics[s.Slug] {
			if !reachable[tp.ID] {
				t.Errorf("topic %q (%s) is not in any exam syllabus", tp.Name, s.Name)
			}
		}
	}
}

// TestProgressMarkerIsCumulative verifies the "# done till here" convention:
// the marker completes a prefix of the file, and exactly one marker exists
// per file that carries one.
func TestProgressMarkerIsCumulative(t *testing.T) {
	g := loadData(t)
	for _, s := range g.subjects {
		lines, err := readLines(filepath.Join(dataDir(t), s.File))
		if err != nil {
			t.Fatalf("read %s: %v", s.File, err)
		}
		markers := 0
		for _, l := range lines {
			if l.marked {
				markers++
			}
		}
		if markers > 1 {
			t.Errorf("%s has %d progress markers, want at most 1", s.File, markers)
		}
		// Completed lectures must form a prefix of the topic's lectures.
		for _, tp := range g.topics[s.Slug] {
			seenIncomplete := false
			for _, l := range tp.Lectures {
				if l.complete && seenIncomplete {
					t.Errorf("%s/%s: completed lecture %q is not a prefix", s.File, tp.Name, l.Title)
				}
				if !l.complete {
					seenIncomplete = true
				}
			}
		}
	}
}

// TestDSAProgressMarkerIsCumulative applies the same rule to dsa.yaml.
func TestDSAProgressMarkerIsCumulative(t *testing.T) {
	g := loadData(t)
	seenIncomplete := false
	for _, p := range g.dsa {
		if p.attempted && seenIncomplete {
			t.Errorf("dsa: attempted problem %q is not a prefix", p.Title)
		}
		if !p.attempted {
			seenIncomplete = true
		}
	}
}

// TestProgressCountsMatchSeed pins the imported-progress volume so an
// accidental marker move is caught.
func TestProgressCountsMatchSeed(t *testing.T) {
	g := loadData(t)
	completed := 0
	for _, s := range g.subjects {
		for _, tp := range g.topics[s.Slug] {
			for _, l := range tp.Lectures {
				if l.complete {
					completed++
				}
			}
		}
	}
	if completed != 121 {
		t.Errorf("completed lectures = %d, want 121", completed)
	}
	attempted := 0
	for _, p := range g.dsa {
		if p.attempted {
			attempted++
		}
	}
	if attempted != 22 {
		t.Errorf("attempted DSA problems = %d, want 22", attempted)
	}
}

// TestCatalogueShape pins the catalogue size so a silent add or removal in
// the data files shows up as a test failure rather than a surprise in the
// planner.
func TestCatalogueShape(t *testing.T) {
	g := loadData(t)
	if len(g.subjects) != 10 {
		t.Errorf("subjects = %d, want 10", len(g.subjects))
	}
	if len(g.exams) != 16 {
		t.Errorf("exams = %d, want 16", len(g.exams))
	}
	ntopics, nlectures, nsyl := 0, 0, 0
	for _, s := range g.subjects {
		for _, tp := range g.topics[s.Slug] {
			ntopics++
			nlectures += len(tp.Lectures)
		}
	}
	for _, entries := range g.syllabus {
		for _, se := range entries {
			nsyl += len(se.Topics)
		}
	}
	if ntopics != 110 {
		t.Errorf("topics = %d, want 110", ntopics)
	}
	if nlectures != 464 {
		t.Errorf("lectures = %d, want 464", nlectures)
	}
	if nsyl != 1200 {
		t.Errorf("syllabus rows = %d, want 1200", nsyl)
	}

	totalSessions := 0
	totalTests := 0
	for _, sess := range g.practiceSessions {
		totalSessions += len(sess)
		for _, s := range sess {
			totalTests += len(s.Tests)
		}
	}
	if totalSessions != 391 {
		t.Errorf("practice sessions = %d, want 391", totalSessions)
	}
	if totalTests != 1062 {
		t.Errorf("practice tests = %d, want 1062", totalTests)
	}
}
