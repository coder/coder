package pubsub

import (
	"fmt"

	"github.com/google/uuid"
)

// ChatStreamNotifyChannel returns the pubsub channel for per-chat
// stream notifications. Subscribers receive lightweight notifications
// and read actual content from the database.
func ChatStreamNotifyChannel(chatID uuid.UUID) string {
	return fmt.Sprintf("chat:stream:%s", chatID)
}

// ChatStreamNotifyMessage is the payload published on the per-chat
// stream notification channel. The actual message content is read
// from the database by subscribers.
type ChatStreamNotifyMessage struct {
	// AfterMessageID tells subscribers to query messages after this
	// ID. Set when a new message is persisted.
	AfterMessageID int64 `json:"after_message_id,omitempty"`

	// Status is set when the chat status changes. Subscribers use
	// this to update clients and to manage relay lifecycle.
	Status string `json:"status,omitempty"`

	// WorkerID identifies which replica is running the chat. Used
	// by enterprise relay to know where to connect.
	WorkerID string `json:"worker_id,omitempty"`

	// Error is set when a processing error occurs.
	Error string `json:"error,omitempty"`

	// QueueUpdate is set when the queued messages change.
	QueueUpdate bool `json:"queue_update,omitempty"`
}
