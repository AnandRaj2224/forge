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
	DomainSWE Domain = "SWE"
)

// SessionKind identifies the kind of Today task / session.
type SessionKind string

const (
	SessionKindLecture  SessionKind = "LECTURE"
	SessionKindPractice SessionKind = "PRACTICE"
	SessionKindDSA      SessionKind = "DSA"
	SessionKindSWE      SessionKind = "SWE"
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

// PracticeMode is an alias kept for API symmetry with the spec.
type PracticeMode = DSAMode

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

// GovExamSyllabus maps an exam to a global topic.
type GovExamSyllabus struct {
	ExamID    ID
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

// SWEArea is a preparation area (CN, SQL, Docker, ...).
type SWEArea struct {
	ID        ID
	Name      string
	SortOrder int
}

// SWERoutineSlot maps a weekday/position to an area (not a task).
type SWERoutineSlot struct {
	ID              ID
	Weekday         int // 1 = Monday .. 7 = Sunday
	Position        int
	AreaID          ID
	DurationSeconds int
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

// GovPracticeRecord is one global topic practiced during one session.
type GovPracticeRecord struct {
	ID                      ID
	SessionID               ID
	TopicID                 ID
	TimeSpentSeconds        int
	Attempted               int
	Correct                 int
	AvgQuestionSpeedSeconds *float64
	RecordedAt              time.Time
}

// GovPracticeRotationState is singleton scheduler state (id = 1).
type GovPracticeRotationState struct {
	ID            int
	LastSubjectID *ID
	UpdatedAt     time.Time
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
	SubjectID ID   `json:"subject_id"`
	TopicIDs  []ID `json:"topic_ids"`
}

type DSATarget struct {
	// Legacy singular block assignment.
	ProblemID ID      `json:"problem_id,omitempty"`
	Mode      DSAMode `json:"mode,omitempty"`
	// Bundle block assignment: 2 repeat + 1 new problems.
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

// SWETarget identifies the SWE area for one Today SWE block.
// SWE is a time-allocation system: routine slot -> area + duration ->
// Today block -> generic session -> elapsed time. No tasks, no bundles.
// Completion is time-based: total SWE time spent >= PlannedSeconds means
// COMPLETED (engine evaluation, consistent across all screens).
type SWETarget struct {
	AreaID ID `json:"area_id"`
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

// UnmarshalSWETarget parses a SWE target strictly.
func UnmarshalSWETarget(raw string) (SWETarget, error) {
	var t SWETarget
	if err := strictUnmarshal(raw, &t); err != nil {
		return SWETarget{}, err
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
