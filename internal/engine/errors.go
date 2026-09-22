package engine

import (
	"errors"
	"fmt"
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
)

// DomainError wraps a sentinel with context for a specific operation.
type DomainError struct {
	Op   string
	Err  error
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
