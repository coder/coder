package autostart

import (
	"time"

	"github.com/google/uuid"
)

// RunResult captures timing and outcome information for a single autostart
// test run.
type RunResult struct {
	// WorkspaceID is the ID of the workspace that was tested.
	WorkspaceID uuid.UUID
	// WorkspaceName is the name of the workspace that was tested.
	WorkspaceName string

	// ConfigTime is when UpdateWorkspaceAutostart was called to set the
	// autostart schedule.
	ConfigTime time.Time
	// ScheduledTime is the time the workspace was scheduled to autostart.
	ScheduledTime time.Time
	// CompletionTime is when the autostart build completed successfully.
	CompletionTime time.Time

	// Success indicates whether the autostart build completed successfully.
	Success bool
	// Error contains the error message if Success is false.
	Error string
}

// EndToEndLatency returns the total time from setting the autostart config
// to the autostart build completing.
func (r RunResult) EndToEndLatency() time.Duration {
	if r.ConfigTime.IsZero() || r.CompletionTime.IsZero() {
		return 0
	}
	return r.CompletionTime.Sub(r.ConfigTime)
}

// TriggerToCompletionLatency returns the time from the scheduled autostart
// time to completion. This includes queueing time plus build execution time.
func (r RunResult) TriggerToCompletionLatency() time.Duration {
	if r.ScheduledTime.IsZero() || r.CompletionTime.IsZero() {
		return 0
	}
	return r.CompletionTime.Sub(r.ScheduledTime)
}
