package pubsub

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// ChatStreamNotifyChannel returns the pubsub channel for per-chat
// stream notifications. Subscribers receive lightweight notifications
// and read actual content from the database.
func ChatStreamNotifyChannel(chatID uuid.UUID) string {
	return fmt.Sprintf("chat:stream:%s", chatID)
}

// ChatStreamNotifyMessage is the payload published on the per-chat
// stream notification channel. Durable message content is still read
// from the database, while transient control events can be carried
// inline for cross-replica delivery.
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

	// Retry carries a structured retry event for cross-replica live
	// delivery. This is transient stream state and is not read back
	// from the database.
	Retry *codersdk.ChatStreamRetry `json:"retry,omitempty"`

	// ErrorPayload carries a structured error event for cross-replica
	// live delivery. Keep Error for backward compatibility with older
	// replicas during rolling deploys.
	ErrorPayload *codersdk.ChatError `json:"error_payload,omitempty"`

	// Error is the legacy string-only error payload kept for mixed-
	// version compatibility during rollout.
	Error string `json:"error,omitempty"`

	// QueueUpdate is set when the queued messages change.
	QueueUpdate bool `json:"queue_update,omitempty"`

	// FullRefresh signals that subscribers should re-fetch all
	// messages from the beginning (e.g. after an edit that
	// truncates message history).
	FullRefresh bool `json:"full_refresh,omitempty"`

	// WakeOnly marks a status notification as a lifecycle wake
	// (e.g. SendMessage transitioning a waiting chat to pending so
	// the processor acquires it) rather than a request to interrupt
	// any in-flight processing. Control subscribers must not cancel
	// ongoing work when WakeOnly is true even if Status would
	// otherwise be a cancel-trigger (pending, waiting, error).
	//
	// Defaulting to false preserves the historical interrupt-on-
	// cancelable-status behavior for rolling deploys where older
	// replicas publish without this field set.
	WakeOnly bool `json:"wake_only,omitempty"`
}
