package chatdebug

import "github.com/google/uuid"

// RunKind identifies the kind of debug run being recorded.
type RunKind string

const (
	// KindChatTurn records a standard chat turn.
	KindChatTurn RunKind = "chat_turn"
	// KindTitleGeneration records title generation for a chat.
	KindTitleGeneration RunKind = "title_generation"
	// KindQuickgen records quick-generation workflows.
	KindQuickgen RunKind = "quickgen"
	// KindCompaction records history compaction workflows.
	KindCompaction RunKind = "compaction"
)

// AllRunKinds contains every RunKind value. Update this when
// adding new constants above.
var AllRunKinds = []RunKind{
	KindChatTurn,
	KindTitleGeneration,
	KindQuickgen,
	KindCompaction,
}

// Status identifies lifecycle state shared by runs and steps.
type Status string

const (
	// StatusInProgress indicates work is still running.
	StatusInProgress Status = "in_progress"
	// StatusCompleted indicates work finished successfully.
	StatusCompleted Status = "completed"
	// StatusError indicates work finished with an error.
	StatusError Status = "error"
	// StatusInterrupted indicates work was canceled or interrupted.
	StatusInterrupted Status = "interrupted"
)

// IsTerminal reports whether the status represents a final state
// that should not be overwritten by stale callbacks.
func (s Status) IsTerminal() bool {
	return s.Priority() > 0
}

// Priority returns a numeric ordering used to prevent stale callbacks
// from regressing a step's status. Higher values win over lower ones.
func (s Status) Priority() int {
	switch s {
	case StatusInProgress:
		return 0
	case StatusInterrupted:
		return 1
	case StatusError:
		return 2
	case StatusCompleted:
		return 3
	default:
		return 0
	}
}

// AllStatuses contains every Status value. Update this when
// adding new constants above.
var AllStatuses = []Status{
	StatusInProgress,
	StatusCompleted,
	StatusError,
	StatusInterrupted,
}

// Operation identifies the model operation a step performed.
type Operation string

const (
	// OperationStream records a streaming model operation.
	OperationStream Operation = "stream"
	// OperationGenerate records a non-streaming generation operation.
	OperationGenerate Operation = "generate"
)

// AllOperations contains every Operation value. Update this when
// adding new constants above.
var AllOperations = []Operation{
	OperationStream,
	OperationGenerate,
}

// RunContext carries identity and metadata for a debug run.
type RunContext struct {
	RunID               uuid.UUID
	ChatID              uuid.UUID
	RootChatID          uuid.UUID // Zero means not set.
	ParentChatID        uuid.UUID // Zero means not set.
	ModelConfigID       uuid.UUID // Zero means not set.
	TriggerMessageID    int64     // Zero means not set.
	HistoryTipMessageID int64     // Zero means not set.
	Kind                RunKind
	Provider            string
	Model               string
}

// StepContext carries identity and metadata for a debug step.
type StepContext struct {
	StepID              uuid.UUID
	RunID               uuid.UUID
	ChatID              uuid.UUID
	StepNumber          int32
	Operation           Operation
	HistoryTipMessageID int64 // Zero means not set.
}

// Attempt captures a single HTTP round trip made during a step.
type Attempt struct {
	Number              int               `json:"number"`
	Status              string            `json:"status,omitempty"`
	Method              string            `json:"method,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Path                string            `json:"path,omitempty"`
	StartedAt           string            `json:"started_at,omitempty"`
	FinishedAt          string            `json:"finished_at,omitempty"`
	RequestHeaders      map[string]string `json:"request_headers,omitempty"`
	RequestBody         []byte            `json:"request_body,omitempty"`
	ResponseStatus      int               `json:"response_status,omitempty"`
	ResponseHeaders     map[string]string `json:"response_headers,omitempty"`
	ResponseBody        []byte            `json:"response_body,omitempty"`
	Error               string            `json:"error,omitempty"`
	DurationMs          int64             `json:"duration_ms"`
	RetryClassification string            `json:"retry_classification,omitempty"`
	RetryDelayMs        int64             `json:"retry_delay_ms,omitempty"`
}

// EventKind identifies the type of pubsub debug event.
type EventKind string

const (
	// EventKindRunUpdate publishes a run mutation.
	EventKindRunUpdate EventKind = "run_update"
	// EventKindStepUpdate publishes a step mutation.
	EventKindStepUpdate EventKind = "step_update"
	// EventKindFinalize publishes a finalization signal.
	EventKindFinalize EventKind = "finalize"
	// EventKindDelete publishes a deletion signal.
	EventKindDelete EventKind = "delete"
)

// DebugEvent is the lightweight pubsub envelope for chat debug updates.
type DebugEvent struct {
	Kind   EventKind `json:"kind"`
	ChatID uuid.UUID `json:"chat_id"`
	RunID  uuid.UUID `json:"run_id"`
	StepID uuid.UUID `json:"step_id"`
}

// BroadcastPubsubChannel is the shared pubsub channel for chat-debug events
// that are not scoped to a single chat, such as stale finalization sweeps.
const BroadcastPubsubChannel = "chat_debug:broadcast"

// PubsubChannel returns the chat-scoped pubsub channel for debug events.
// Nil chat IDs use the shared broadcast channel so publishers and subscribers
// can coordinate through one discoverable helper.
func PubsubChannel(chatID uuid.UUID) string {
	if chatID == uuid.Nil {
		return BroadcastPubsubChannel
	}
	return "chat_debug:" + chatID.String()
}
