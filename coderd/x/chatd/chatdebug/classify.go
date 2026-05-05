package chatdebug

import (
	"context"
	"errors"
)

// ClassifyError maps a run error to the appropriate debug status.
// nil -> StatusCompleted, context.Canceled -> StatusInterrupted,
// everything else -> StatusError. Callers with additional classification
// rules (e.g. ErrInterrupted, ErrDynamicToolCall) should handle those
// before falling back to this helper.
func ClassifyError(err error) Status {
	switch {
	case err == nil:
		return StatusCompleted
	case errors.Is(err, context.Canceled):
		return StatusInterrupted
	default:
		return StatusError
	}
}
