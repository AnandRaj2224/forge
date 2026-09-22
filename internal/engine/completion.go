package engine

import (
	"context"
	"fmt"

	db "forge/internal/db"
	"forge/internal/db/runtime"
)

// Authoritative product predicates for track completion.
//
// The runtime package answers persistence questions ("what execution /
// completion rows exist?"). These Engine wrappers own the product notion
// ("is this SWE task / lecture complete?"): TUI screens and Engine
// planners must ask here, never consult runtime completion helpers
// directly. DB failures propagate as errors, never silent false.

// IsSWETaskComplete reports whether a SWE task has a COMPLETED execution.
// It is the product-wide meaning of SWE task completion; Today
// startability and bundle completion additionally require exact target
// satisfaction via EvaluateTodayTask.
func IsSWETaskComplete(ctx context.Context, q db.DBTX, taskID db.ID) (bool, error) {
	done, err := runtime.IsSWETaskComplete(ctx, q, taskID)
	if err != nil {
		return false, fmt.Errorf("engine: swe completion: %w", err)
	}
	return done, nil
}

// IsLectureComplete reports whether a lecture has a completion row. Exact
// Today completion additionally requires target membership via
// EvaluateTodayTask.
func IsLectureComplete(ctx context.Context, q db.DBTX, lectureID db.ID) (bool, error) {
	done, err := runtime.IsLectureComplete(ctx, q, lectureID)
	if err != nil {
		return false, fmt.Errorf("engine: lecture completion: %w", err)
	}
	return done, nil
}
