package engine

import (
	"errors"
	"fmt"
	"strings"
)

// Domain errors. Engine returns these for expected business states so the
// TUI can render neutral states instead of red error banners. Unexpected
// DB failures are wrapped separately and must surface as data errors.
var (
	ErrTaskCompleted     = errors.New("task already complete")
	ErrTaskActive        = errors.New("task already has an active session")
	ErrActiveSessionBusy = errors.New("another session is active")
	ErrNoActiveSession   = errors.New("no active session")
	ErrInvalidTarget     = errors.New("invalid task target")
	ErrValidation        = errors.New("validation failed")
	ErrAlreadySatisfied  = errors.New("target item already satisfied")
	ErrDuplicate         = errors.New("duplicate target result")
	ErrNotFound          = errors.New("not found")
	ErrLegacyTarget      = errors.New("legacy target is read-only")
)

// DomainError wraps a sentinel with context for a specific operation.
type DomainError struct {
	Op     string
	Err    error
	Detail string
}

func (e *DomainError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s: %s", e.Op, e.Err.Error(), e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Err.Error())
}

func (e *DomainError) Unwrap() error { return e.Err }

func wrapErr(op string, sentinel error, detail string) *DomainError {
	return &DomainError{Op: op, Err: sentinel, Detail: detail}
}

// isUniqueConflict reports whether err is a SQLite uniqueness violation
// (single-active-session index, snapshot revision uniqueness, DSA NEW
// uniqueness, ...). The DB stays the final concurrency guard; the engine
// translates its conflict into the appropriate domain error instead of
// leaking a raw storage message.
func isUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "UNIQUE CONSTRAINT FAILED") ||
		strings.Contains(msg, "UNIQUE CONSTRAINT") ||
		strings.Contains(msg, "PRIMARY KEY")
}
