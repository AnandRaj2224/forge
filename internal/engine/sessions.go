package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// StartTodayTask atomically starts a Today task: load, evaluate, reject
// COMPLETED/ACTIVE/global-busy, validate target, create session with
// snapshotted title/target/planned/domain/kind, return the new session.
// It is the only normal path to starting; TUI must use it.
func StartTodayTask(ctx context.Context, database *db.DB, taskID db.ID, now time.Time) (db.Session, error) {
	task, err := runtime.GetTodayTask(ctx, database, taskID)
	if err != nil {
		return db.Session{}, fmt.Errorf("engine: load task: %w", err)
	}
	ev, err := EvaluateTodayTask(ctx, database, taskID)
	if err != nil {
		return db.Session{}, err
	}
	if ev.ConfigError != "" {
		return db.Session{}, wrapErr("start", ErrInvalidTarget, ev.ConfigError)
	}
	if ev.State == StateCompleted {
		return db.Session{}, wrapErr("start", ErrTaskCompleted, string(taskID))
	}
	if ev.State == StateActive {
		return db.Session{}, wrapErr("start", ErrTaskActive, string(taskID))
	}
	if _, err := runtime.GetActiveSession(ctx, database); err == nil {
		return db.Session{}, wrapErr("start", ErrActiveSessionBusy, "stop the active session first")
	} else if !errors.Is(err, sql.ErrNoRows) && !isNotFoundErr(err) {
		return db.Session{}, fmt.Errorf("engine: check active session: %w", err)
	}
	// Semantic target validation before creating the session.
	if err := validateTaskTarget(ctx, database, task); err != nil {
		return db.Session{}, err
	}
	if task.PlannedSeconds <= 0 {
		return db.Session{}, wrapErr("start", ErrInvalidTarget, "planned_seconds must be > 0")
	}
	sid := db.ID(fmt.Sprintf("sess-%d", now.UnixNano()))
	sess := db.Session{
		ID: sid, TodayTaskID: task.ID, Domain: task.Domain, Kind: task.Kind,
		TitleSnapshot: task.Title, TargetSnapshotJSON: task.TargetJSON,
		PlannedSeconds: task.PlannedSeconds, StartedAt: now, Status: db.SessionStatusActive,
	}
	if err := runtime.CreateSession(ctx, database, sess); err != nil {
		// The partial unique index on ACTIVE sessions is the final
		// concurrency guard: a race that slips past the check above
		// surfaces here as a uniqueness conflict, translated into the
		// domain-level busy error (never a raw SQLite message, never a
		// duplicate session).
		if isUniqueConflict(err) {
			return db.Session{}, wrapErr("start", ErrActiveSessionBusy, "stop the active session first")
		}
		return db.Session{}, fmt.Errorf("engine: create session: %w", err)
	}
	created, err := runtime.GetSession(ctx, database, sid)
	if err != nil {
		return db.Session{}, fmt.Errorf("engine: reload session: %w", err)
	}
	return created, nil
}

// StopActiveSession closes the global ACTIVE session: require ACTIVE, set
// stopped_at/status, preserve timestamps, never change task completion.
func StopActiveSession(ctx context.Context, database *db.DB, now time.Time) (db.Session, error) {
	sess, err := runtime.GetActiveSession(ctx, database)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isNotFoundErr(err) {
			return db.Session{}, wrapErr("stop", ErrNoActiveSession, "")
		}
		return db.Session{}, fmt.Errorf("engine: get active session: %w", err)
	}
	if sess.Status != db.SessionStatusActive {
		return db.Session{}, wrapErr("stop", ErrNoActiveSession, string(sess.ID))
	}
	if now.Before(sess.StartedAt) {
		now = sess.StartedAt
	}
	if err := runtime.StopSession(ctx, database, sess.ID, now); err != nil {
		return db.Session{}, fmt.Errorf("engine: stop session: %w", err)
	}
	closed, err := runtime.GetSession(ctx, database, sess.ID)
	if err != nil {
		return db.Session{}, fmt.Errorf("engine: reload stopped session: %w", err)
	}
	return closed, nil
}

// LoadActiveSession returns the persisted ACTIVE session across restarts
// without creating, resetting, or duplicating it. Returns (nil, nil) when
// no session is active; DB errors propagate.
func LoadActiveSession(ctx context.Context, q db.DBTX) (*db.Session, error) {
	sess, err := runtime.GetActiveSession(ctx, q)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("engine: load active session: %w", err)
	}
	return &sess, nil
}

// EnsureGovSubjectActive marks a subject as active. It is idempotent:
// repeated activation of an already-active subject is a no-op (never a
// primary-key failure through normal operation).
func EnsureGovSubjectActive(ctx context.Context, database *db.DB, subjectID db.ID, now time.Time) error {
	if err := runtime.ActivateGovSubject(ctx, database, subjectID, now); err != nil {
		return fmt.Errorf("engine: activate subject: %w", err)
	}
	return nil
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	// runtime wraps sql.ErrNoRows with %w in most getters.
	return errors.Is(err, sql.ErrNoRows)
}

func validateTaskTarget(ctx context.Context, q db.DBTX, task db.TodayTask) error {
	if task.TargetJSON == nil || *task.TargetJSON == "" {
		return wrapErr("start", ErrInvalidTarget, "task has no target")
	}
	switch task.Domain {
	case db.DomainGOV:
		if task.Kind == db.SessionKindLecture {
			lt, err := db.UnmarshalGovLectureTarget(*task.TargetJSON)
			if err != nil {
				return wrapErr("start", ErrInvalidTarget, err.Error())
			}
			return ValidateGovLectureTarget(ctx, q, lt)
		}
		pt, err := db.UnmarshalGovPracticeTarget(*task.TargetJSON)
		if err != nil {
			return wrapErr("start", ErrInvalidTarget, err.Error())
		}
		return ValidateGovPracticeTarget(ctx, q, pt)
	case db.DomainDSA:
		dt, err := db.UnmarshalDSATarget(*task.TargetJSON)
		if err != nil {
			return wrapErr("start", ErrInvalidTarget, err.Error())
		}
		return ValidateDSATarget(ctx, q, dt)
	default:
		return wrapErr("start", ErrInvalidTarget, "unknown domain")
	}
}
