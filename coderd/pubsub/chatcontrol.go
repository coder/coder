package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// ChatControlReason identifies why a worker-scoped control message was sent.
type ChatControlReason string

const (
	// ChatControlReasonInterrupt requests that the current generation stop
	// running without fencing a newer generation. This is used for explicit stop
	// actions that still allow the interrupted run to persist partial output.
	ChatControlReasonInterrupt ChatControlReason = "interrupt"
	// ChatControlReasonRestart requests that an older generation stop because a
	// newer generation has been scheduled.
	ChatControlReasonRestart ChatControlReason = "restart"
	// ChatControlReasonArchive requests that the current generation stop because
	// the chat is being archived.
	ChatControlReasonArchive ChatControlReason = "archive"
	// ChatControlReasonRecoverStale requests that the current generation stop
	// because stale recovery fenced it off.
	ChatControlReasonRecoverStale ChatControlReason = "recover_stale"
)

// ChatControlChannel returns the pubsub channel for worker-scoped control
// messages. Each worker subscribes to exactly one control channel.
func ChatControlChannel(workerID uuid.UUID) string {
	return fmt.Sprintf("chat:control:%s", workerID)
}

// ChatControlMessage requests that a worker stop an active run for a chat.
// RunGeneration fences newer runs from stale control messages.
type ChatControlMessage struct {
	ChatID        uuid.UUID         `json:"chat_id"`
	RunGeneration int64             `json:"run_generation"`
	Reason        ChatControlReason `json:"reason,omitempty"`
}

// HandleChatControl wraps a typed callback for ChatControlMessage payloads.
func HandleChatControl(cb func(ctx context.Context, payload ChatControlMessage, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, ChatControlMessage{}, err)
			return
		}
		var payload ChatControlMessage
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, ChatControlMessage{}, err)
			return
		}
		cb(ctx, payload, nil)
	}
}
