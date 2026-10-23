package chatd

import (
	"database/sql"
	"time"

	"charm.land/fantasy"
)

type runnerActionKind string

const (
	runnerActionKindEnterRequiresAction runnerActionKind = "enter_requires_action"
	runnerActionKindFinishTurn          runnerActionKind = "finish_turn"
	runnerActionKindFinishError         runnerActionKind = "finish_error"
	runnerActionKindFinishInterruption  runnerActionKind = "finish_interruption"
)

// stepData is the durable content produced by one provider attempt.
type stepData struct {
	Content            []fantasy.Content
	Usage              fantasy.Usage
	ContextLimit       sql.NullInt64
	Runtime            time.Duration
	ProviderResponseID string

	// BatchRuntime is the local-tool batch window. Model steps use Runtime.
	BatchRuntime time.Duration
	// BatchBilledCalls counts the calls whose intervals produced
	// BatchRuntime. Audit metadata for the batch usage record.
	BatchBilledCalls int

	ToolCallCreatedAt    map[string]time.Time
	ToolResultCreatedAt  map[string]time.Time
	ReasoningStartedAt   []time.Time
	ReasoningCompletedAt []time.Time
}

// pendingDynamicToolCall describes a dynamic tool call parked for a user.
type pendingDynamicToolCall struct {
	ToolCallID string
	ToolName   string
	Args       string
}

type compactionStatus int

const (
	compactionStatusNotNeeded compactionStatus = iota
	compactionStatusNeeded
	compactionStatusAfterCompaction
	compactionStatusStillOverLimit
)
