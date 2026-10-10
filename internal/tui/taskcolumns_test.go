package tui

import (
	"strings"
	"testing"

	db "forge/internal/db"
)

// Lectures sharing one topic must render distinguishably in the TODAY task
// list: the visible label carries the lecture title, not just the topic.
func TestTaskColumnsLectureLabelDistinguishesLectures(t *testing.T) {
	mkLecture := func(lectureID, lectureTitle string) Bundle {
		return Bundle{Lecture: &LectureItem{
			SubjectID: "subj-quant", TopicID: "topic-ns", LectureID: db.ID(lectureID),
			Subject: "Quant", Topic: "Number System", Lecture: lectureTitle,
		}}
	}
	task := db.TodayTask{Domain: db.DomainGOV, Kind: db.SessionKindLecture}
	_, typ1, label1 := TaskColumns(task, mkLecture("lec-ns-1", "Core Concepts"))
	_, typ2, label2 := TaskColumns(task, mkLecture("lec-ns-2", "Worked Examples"))
	if typ1 != "LECTURE" || typ2 != "LECTURE" {
		t.Fatalf("types=%q,%q want LECTURE", typ1, typ2)
	}
	if label1 == label2 {
		t.Fatalf("same-topic lectures render identically: %q", label1)
	}
	if !strings.Contains(label1, "Core Concepts") || !strings.Contains(label2, "Worked Examples") {
		t.Fatalf("labels must carry lecture titles: %q vs %q", label1, label2)
	}
	if !strings.Contains(label1, "Number System") || !strings.Contains(label2, "Number System") {
		t.Fatalf("labels must keep the topic context: %q vs %q", label1, label2)
	}
}
