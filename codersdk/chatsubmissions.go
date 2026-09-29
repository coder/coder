package codersdk

import "github.com/google/uuid"

// ChatExactSettings is the immutable model selection admitted for a turn.
// ReasoningEffort is the value Coder will send, not proof the provider ran.
// A nil value leaves provider/configuration defaults unspecified.
type ChatExactSettings struct {
	ModelConfigID   uuid.UUID `json:"model_config_id" format:"uuid"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	ReasoningEffort *string   `json:"reasoning_effort,omitempty"`
}

// ChatSubmissionReceipt identifies an accepted submission independently of
// message queue promotion. QueuedMessageID is the original queue identity.
type ChatSubmissionReceipt struct {
	ID              uuid.UUID          `json:"id" format:"uuid"`
	RequestID       uuid.UUID          `json:"request_id" format:"uuid"`
	ChatID          uuid.UUID          `json:"chat_id" format:"uuid"`
	State           string             `json:"state" enums:"reserved,accepted,uncertain,rejected"`
	MessageID       int64              `json:"message_id,omitempty"`
	QueuedMessageID int64              `json:"queued_message_id,omitempty"`
	Error           string             `json:"error,omitempty"`
	Settings        *ChatExactSettings `json:"settings,omitempty"`
}
