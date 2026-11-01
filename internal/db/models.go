package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// ID is the typed identifier used across all Forge tables.
type ID string

// Domain identifies the preparation vertical.
type Domain string

const (
	DomainGOV Domain = "GOV"
	DomainDSA Domain = "DSA"
)

// SessionKind identifies the kind of Today task / session.
type SessionKind string

const (
	SessionKindLecture  SessionKind = "LECTURE"
	SessionKindPractice SessionKind = "PRACTICE"
	SessionKindDSA      SessionKind = "DSA"
)

// SessionStatus is ACTIVE or STOPPED.
type SessionStatus string

const (
	SessionStatusActive  SessionStatus = "ACTIVE"
	SessionStatusStopped SessionStatus = "STOPPED"
)

// DSAMode is NEW or REPEAT.
type DSAMode string

const (
	DSAModeNew    DSAMode = "NEW"
	DSAModeRepeat DSAMode = "REPEAT"
)

// GovPracticeSelectionMode selects how a practice slot resolves subjects.
type GovPracticeSelectionMode string

const (
	GovSelectionFixed      GovPracticeSelectionMode = "FIXED_SUBJECT"
	GovSelectionRoundRobin GovPracticeSelectionMode = "ROUND_ROBIN"
)

// ForgeSettings is the singleton settings row (id = 1).
type ForgeSettings struct {
	ID             int
	Timezone       string
	DayStartMinute int
	CreatedAt      time.Time
}

// GovSubject is a global subject with global order.
type GovSubject struct {
	ID        ID
	Name      string
	SortOrder int
}

// GovTopic belongs to exactly one subject.
type GovTopic struct {
	ID        ID
	SubjectID ID
	Name      string
	SortOrder int
}

// GovLecture belongs to exactly one topic.
type GovLecture struct {
	ID        ID
	TopicID   ID
	Title     string
	SortOrder int
}

// GovExam has no stages or hierarchy.
type GovExam struct {
	ID        ID
	Name      string
	SortOrder int
}

// GovExamSyllabus maps an exam+subject to a canonical topic.
// (exam_id, topic_id) is the primary membership; subject_id is the
// authoritative exam-subject the topic belongs to in this exam and must
// equal the topic's canonical subject. There is one answer to
// "which topics belong to this subject in this exam".
type GovExamSyllabus struct {
	ExamID    ID
	SubjectID ID
	TopicID   ID
	SortOrder int
}

// GovExamSubject records that a global subject belongs to an exam's
// preparation track, with the exam-specific order the exam works through
// its subjects. Subjects are never duplicated per exam: the same global
// subject row is shared across exams.
type GovExamSubject struct {
	ExamID    ID
	SubjectID ID
	SortOrder int
}

// GovLectureRoutineSlot is one lecture routine slot.
type GovLectureRoutineSlot struct {
	SlotOrder       int
	DurationSeconds int
}

// GovPracticeRoutineSlot is one practice routine slot.
type GovPracticeRoutineSlot struct {
	SlotOrder       int
	DurationSeconds int
	SelectionMode   GovPracticeSelectionMode
	FixedSubjectID  *ID
}

// DSAProblem is one catalogue entry in global DSA order.
type DSAProblem struct {
	ID         ID
	Title      string
	Pattern    string
	Difficulty string
	SortOrder  int
}

// DSARoutine is the singleton DSA block configuration (id = 1).
type DSARoutine struct {
	ID             int
	PlannedSeconds int
}

// GovSubjectActivation marks a subject as entered preparation.
type GovSubjectActivation struct {
	SubjectID   ID
	ActivatedAt time.Time
}

// GovLectureCompletion records a lecture completed once.
type GovLectureCompletion struct {
	LectureID   ID
	SessionID   ID
	CompletedAt time.Time
}

// GovPracticeRecord is one topic practiced during one session.
// ExamID attributes the result to the focused exam (exam-specific practice
// history). NULL/empty preserves legacy rows, which remain readable as
// global history and are never reinterpreted under a new focus.
type GovPracticeRecord struct {
	ID                      ID
	SessionID               ID
	TopicID                 ID
	TestID                  *ID
	ExamID                  *ID
	TimeSpentSeconds        int
	Attempted               int
	Correct                 int
	AvgQuestionSpeedSeconds *float64
	RecordedAt              time.Time
}

// GovExamProfile holds lifecycle state for one exam, independent of
// syllabus completion. Main-goal is separate from focus.
type GovExamProfile struct {
	ExamID     ID
	Status     string // ACTIVE, PAUSED, ARCHIVED
	IsMainGoal bool
	Priority   int
	TargetDate *string // YYYY-MM-DD, optional
	UpdatedAt  time.Time
}

// Exam profile statuses.
const (
	ExamStatusActive   = "ACTIVE"
	ExamStatusPaused   = "PAUSED"
	ExamStatusArchived = "ARCHIVED"
)

// GovFocusState is the singleton explicit focus exam (id = 1).
// Nil FocusedExamID means no focus selected yet.
type GovFocusState struct {
	ID            int
	FocusedExamID *ID
	UpdatedAt     time.Time
}

// GovPracticeSession is one numbered session in a subject's test order track.
type GovPracticeSession struct {
	ID           ID
	SubjectID    ID
	SessionCode  string
	SessionOrder int
}

// GovPracticeTest is one specific test belonging to a practice session.
type GovPracticeTest struct {
	ID        ID
	SessionID ID
	SubjectID ID
	Title     string
	SortOrder int
}

// GovPracticeTrack tracks the persistent session order cursor for a subject.
type GovPracticeTrack struct {
	SubjectID           ID
	CurrentSessionOrder int
	UpdatedAt           time.Time
}

// GovPracticeRotation is the deterministic per-exam+subject topic cursor.
// NextIndex is the offset into the stable eligible order for the next plan.
type GovPracticeRotation struct {
	ExamID    ID
	SubjectID ID
	NextIndex int
	UpdatedAt time.Time
}

// Stable canonical practice subject IDs for the V2 fixed routine.
// Resolved by ID, never by display name.
const (
	PracticeSubjectQuant     ID = "subj-quant"
	PracticeSubjectEnglish   ID = "subj-english"
	PracticeSubjectReasoning ID = "subj-reasoning"
)

// V2FixedPracticeSubjects is the authoritative fixed practice order.
var V2FixedPracticeSubjects = []ID{
	PracticeSubjectQuant,
	PracticeSubjectEnglish,
	PracticeSubjectReasoning,
}

// DSAAttempt is historical evidence of one attempt.
type DSAAttempt struct {
	ID               ID
	SessionID        ID
	ProblemID        ID
	Mode             DSAMode
	AttemptedAt      time.Time
	Independent      bool
	Optimal          bool
	HintUsed         bool
	SolveTimeSeconds *int
}

// DSARepeatState is current scheduling state for a problem.
type DSARepeatState struct {
	ProblemID           ID
	RepeatStage         int
	CurrentIntervalDays int
	NextReviewDate      string // YYYY-MM-DD
	UpdatedAt           time.Time
}

// TodaySnapshot is an immutable generated projection revision.
type TodaySnapshot struct {
	ID          ID
	LocalDate   string // YYYY-MM-DD
	Revision    int
	GeneratedAt time.Time
}

// TodayTask is one presented task inside a snapshot.
type TodayTask struct {
	ID                   ID
	SnapshotID           ID
	Position             int
	ScheduledStartMinute *int
	Domain               Domain
	Kind                 SessionKind
	Title                string
	PlannedSeconds       int
	TargetJSON           *string
}

// Session is one concrete Today task execution.
type Session struct {
	ID                 ID
	TodayTaskID        ID
	Domain             Domain
	Kind               SessionKind
	TitleSnapshot      string
	TargetSnapshotJSON *string
	PlannedSeconds     int
	StartedAt          time.Time
	StoppedAt          *time.Time
	Status             SessionStatus
}

// ElapsedSeconds derives stopped_at - started_at; -1 when still active.
func (s Session) ElapsedSeconds() int64 {
	if s.StoppedAt == nil {
		return -1
	}
	return s.StoppedAt.Unix() - s.StartedAt.Unix()
}

// WeeklyReview is one immutable-per-week snapshot plus reflections.
type WeeklyReview struct {
	ID               string // ISO week YYYY-Www (stored as week PK)
	WeekStart        string // YYYY-MM-DD
	WeekEnd          string // YYYY-MM-DD
	GeneratedAt      time.Time
	UpdatedAt        time.Time
	PlannedSeconds   int
	ElapsedSeconds   int
	OverrunSeconds   int
	ReportJSON       string
	ReflectionWorked string
	ReflectionChange string
}

// Mock is one actual completed mock/exam attempt (runtime evidence).
// It belongs to exactly ONE gov exam. Weak topics are stored per section
// as gov_topic_id references (never free text) so the future GOV practice
// engine can weight recent weaknesses as a signal inside normal slots.
// Mock date preserves recency; no decay formula is implemented yet.
type Mock struct {
	ID              ID
	ExamID          ID
	Name            string
	MockDate        string // YYYY-MM-DD
	Platform        string
	Score           *int
	TotalScore      *int
	RankValue       *int
	Percentile      *float64
	Accuracy        *float64
	AvgSpeedSeconds *float64
	Attempted       *int
	TotalQuestions  *int
	Correct         *int
	Incorrect       *int
	Unattempted     *int
	CreatedAt       time.Time
}

// MockSection is one exam section inside a mock (e.g. QUANT, REASONING).
// Name is the actual user-entered section name. GovSubjectID is the explicit
// user-selected global GOV subject mapping (nullable for legacy rows; new
// sections must provide it). Never inferred from the name.
type MockSection struct {
	ID           ID
	MockID       ID
	Name         string
	SortOrder    int
	GovSubjectID *ID
}

// MockWeakTopic is one ordered weakness: 1 = strongest, up to 3 per section.
type MockWeakTopic struct {
	SectionID ID
	TopicID   ID
	RankOrder int
}

// Quote is one daily-quote catalogue entry.
// Author is never NULL; empty/missing upstream authors are stored as "Unknown".
// Used tracks the non-repeat cycle: 0 = not used in the current cycle,
// 1 = already shown in the current cycle. A quote repeats only after the
// complete quote set has been exhausted and the cycle resets.
type Quote struct {
	ID     int
	Quote  string
	Author string
	Used   int
}

// DailyQuoteState is the singleton daily selection state (id = 1).
// It persists the current quote's local date so restarting Forge on the
// same day reuses the same quote instead of picking another one.
type DailyQuoteState struct {
	ID        int
	LocalDate string // YYYY-MM-DD (Forge local date)
	QuoteID   *int   // nullable FK to quotes(id)
	QuoteText string
	Author    string
}

// Typed target payloads for today_tasks.target_json / sessions.target_snapshot_json.

type GovLectureTarget struct {
	SubjectID ID `json:"subject_id"`
	TopicID   ID `json:"topic_id"`
	LectureID ID `json:"lecture_id"`
}

type GovPracticeTarget struct {
	SubjectID    ID       `json:"subject_id"`
	TopicIDs     []ID     `json:"topic_ids"`
	SessionCode  string   `json:"session_code,omitempty"`
	SessionOrder int      `json:"session_order,omitempty"`
	TestNames    []string `json:"test_names,omitempty"`
}

type DSATarget struct {
	// Legacy singular block assignment.
	ProblemID ID      `json:"problem_id,omitempty"`
	Mode      DSAMode `json:"mode,omitempty"`
	// Bundle block assignment: 2 repeat + 2 new problems.
	Problems []DSAProblemTarget `json:"problems,omitempty"`
}

// DSAProblemTarget is one problem inside a DSA block bundle.
type DSAProblemTarget struct {
	ProblemID ID      `json:"problem_id"`
	Mode      DSAMode `json:"mode"`
}

// Items normalizes legacy singular and bundle shapes into one list.
func (t DSATarget) Items() []DSAProblemTarget {
	if len(t.Problems) > 0 {
		return t.Problems
	}
	if t.ProblemID != "" {
		return []DSAProblemTarget{{ProblemID: t.ProblemID, Mode: t.Mode}}
	}
	return nil
}

// MarshalTarget marshals a typed target payload strictly.
func MarshalTarget(v any) (string, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// UnmarshalGovLectureTarget parses a GOV lecture target strictly.
func UnmarshalGovLectureTarget(raw string) (GovLectureTarget, error) {
	var t GovLectureTarget
	if err := strictUnmarshal(raw, &t); err != nil {
		return GovLectureTarget{}, err
	}
	return t, nil
}

// UnmarshalGovPracticeTarget parses a GOV practice target strictly.
func UnmarshalGovPracticeTarget(raw string) (GovPracticeTarget, error) {
	var t GovPracticeTarget
	if err := strictUnmarshal(raw, &t); err != nil {
		return GovPracticeTarget{}, err
	}
	return t, nil
}

// UnmarshalDSATarget parses a DSA target strictly.
func UnmarshalDSATarget(raw string) (DSATarget, error) {
	var t DSATarget
	if err := strictUnmarshal(raw, &t); err != nil {
		return DSATarget{}, err
	}
	return t, nil
}

func strictUnmarshal(raw string, v any) error {
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("db: decode target: %w", err)
	}
	return nil
}
