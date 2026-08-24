package chatd

import (
	"database/sql"
	"time"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/codersdk"
)

type runnerActionKind string

type runnerActionMessage struct {
	ID   int64
	Role codersdk.ChatMessageRole
}

const (
	runnerActionKindEnterRequiresAction runnerActionKind = "enter_requires_action"
	runnerActionKindFinishTurn          runnerActionKind = "finish_turn"
	runnerActionKindFinishError         runnerActionKind = "finish_error"
	runnerActionKindFinishInterruption  runnerActionKind = "finish_interruption"
)

// stepData is the durable content produced by one provider attempt.
type stepData struct {
	Content      []fantasy.Content
	Usage        fantasy.Usage
	ContextLimit sql.NullInt64
	Runtime      time.Duration

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

// compactionOutcome contains a generated context summary. It must stay
// field-compatible with chatloop.CompactionResult; generateCompaction
// converts between the two directly.
type compactionOutcome struct {
	SystemSummary    string
	SummaryReport    string
	Source           chatloop.CompactionSource
	ThresholdPercent int32
	UsagePercent     float64
	ContextTokens    int64
	ContextLimit     int64
	Runtime          time.Duration
}

type compactionStatus int

const (
	compactionStatusNotNeeded compactionStatus = iota
	compactionStatusNeeded
	compactionStatusAfterCompaction
	compactionStatusStillOverLimit
)
