package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

// ChatWatchEventChannel returns the pubsub channel for chat
// lifecycle events scoped to a single user.
func ChatWatchEventChannel(ownerID uuid.UUID) string {
	return fmt.Sprintf("chat:owner:%s", ownerID)
}

// chatWatchEventPayloadBudgetBytes stays below PostgreSQL's NOTIFY
// payload limit so chat watch events fail here instead of inside pubsub.
const chatWatchEventPayloadBudgetBytes = 6000

// PublishChatWatchEvent publishes a bounded chat watch event to the
// owner-scoped chat watch channel.
func PublishChatWatchEvent(pubsub dbpubsub.Publisher, ownerID uuid.UUID, event codersdk.ChatWatchEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return xerrors.Errorf("marshal chat watch event: %w", err)
	}
	if len(payload) > chatWatchEventPayloadBudgetBytes {
		return xerrors.Errorf(
			"chat watch event payload exceeds budget: %d > %d bytes",
			len(payload), chatWatchEventPayloadBudgetBytes,
		)
	}
	if err := pubsub.Publish(ChatWatchEventChannel(ownerID), payload); err != nil {
		return xerrors.Errorf("publish chat watch event: %w", err)
	}
	return nil
}

// HandleChatWatchEvent wraps a typed callback for
// ChatWatchEvent messages delivered via pubsub.
func HandleChatWatchEvent(cb func(ctx context.Context, payload codersdk.ChatWatchEvent, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, codersdk.ChatWatchEvent{}, xerrors.Errorf("chat watch event pubsub: %w", err))
			return
		}
		var payload codersdk.ChatWatchEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, codersdk.ChatWatchEvent{}, xerrors.Errorf("unmarshal chat watch event: %w", err))
			return
		}

		cb(ctx, payload, err)
	}
}
