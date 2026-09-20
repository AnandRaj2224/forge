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

// SWEExecutionStatus is the outcome of one SWE execution.
type SWEExecutionStatus string

const (
	SWEStatusCompleted SWEExecutionStatus = "COMPLETED"
	SWEStatusPartial   SWEExecutionStatus = "PARTIAL"
	SWEStatusBlocked   SWEExecutionStatus = "BLOCKED"
)

// PracticeStatus is an alias for SWE execution status vocabulary.
type PracticeStatus = SWEExecutionStatus

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

// SWETask belongs to exactly one area.
type SWETask struct {
	ID        ID
	AreaID    ID
	Title     string
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

// SWEExecution preserves one execution outcome.
type SWEExecution struct {
	ID         ID
	SessionID  ID
	TaskID     ID
	Status     SWEExecutionStatus
	RecordedAt time.Time
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

type SWETarget struct {
	// Legacy singular task assignment.
	AreaID ID  `json:"area_id,omitempty"`
	TaskID *ID `json:"task_id,omitempty"`
	// Bundle assignment: the daily task combination.
	Tasks []SWETaskTarget `json:"tasks,omitempty"`
}

// SWETaskTarget is one task inside a SWE block bundle.
type SWETaskTarget struct {
	AreaID ID  `json:"area_id"`
	TaskID *ID `json:"task_id,omitempty"`
}

// Items normalizes legacy singular and bundle shapes into one list.
func (t SWETarget) Items() []SWETaskTarget {
	if len(t.Tasks) > 0 {
		return t.Tasks
	}
	if t.AreaID != "" {
		return []SWETaskTarget{{AreaID: t.AreaID, TaskID: t.TaskID}}
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
