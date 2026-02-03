package chats

import (
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/coderd/database"
)

const (
	EnvelopeTypeSystemPrompt = "system_prompt"
	EnvelopeTypeMessage      = "message"
	EnvelopeTypeStreamEvent  = "stream_event"
	EnvelopeTypeToolCall     = "tool_call"
	EnvelopeTypeToolResult   = "tool_result"
)

// DefaultSystemPrompt is persisted to new chats as durable state.
const DefaultSystemPrompt = "You are a helpful assistant."

type SystemPromptEnvelope struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type MessageEnvelope struct {
	Type    string        `json:"type"`
	RunID   string        `json:"run_id,omitempty"`
	Message aisdk.Message `json:"message"`
}

type StreamEventEnvelope struct {
	Type  string `json:"type"`
	RunID string `json:"run_id"`

	Event string `json:"event"`

	// event=run_finished
	FinishReason string `json:"finish_reason,omitempty"`
	Steps        int    `json:"steps,omitempty"`

	// event=usage
	Usage aisdk.Usage `json:"usage,omitempty"`
	Step  int         `json:"step,omitempty"`

	// event=error
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

// ToolCallEnvelope is persisted with role=tool_call.
type ToolCallEnvelope struct {
	Type       string          `json:"type"`
	RunID      string          `json:"run_id"`
	Step       int             `json:"step"`
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Args       json.RawMessage `json:"args"`
}

// ToolResultEnvelope is persisted with role=tool_result.
type ToolResultEnvelope struct {
	Type       string          `json:"type"`
	RunID      string          `json:"run_id"`
	Step       int             `json:"step"`
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Result     json.RawMessage `json:"result"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

func MarshalEnvelope(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// MessagesFromDB reconstructs an aisdk conversation from chat_messages.
//
// Only durable message events are used. Stream lifecycle events are ignored.
func MessagesFromDB(rows []database.ChatMessage) ([]aisdk.Message, error) {
	msgs := make([]aisdk.Message, 0, len(rows))

	for _, row := range rows {
		switch row.Role {
		case "system":
			var base struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(row.Content, &base); err != nil {
				return nil, xerrors.Errorf("unmarshal system envelope: %w", err)
			}
			if base.Type != EnvelopeTypeSystemPrompt {
				continue
			}
			var env SystemPromptEnvelope
			if err := json.Unmarshal(row.Content, &env); err != nil {
				return nil, xerrors.Errorf("unmarshal system prompt envelope: %w", err)
			}
			msgs = append(msgs, aisdk.Message{
				Role: "system",
				Parts: []aisdk.Part{{
					Type: aisdk.PartTypeText,
					Text: env.Content,
				}},
			})

		case "user", "assistant":
			var env MessageEnvelope
			if err := json.Unmarshal(row.Content, &env); err != nil {
				return nil, xerrors.Errorf("unmarshal message envelope: %w", err)
			}
			if env.Type != EnvelopeTypeMessage {
				continue
			}
			// The role is authoritative from the row.
			env.Message.Role = row.Role
			msgs = append(msgs, env.Message)
		}
	}

	return msgs, nil
}
