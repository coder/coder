package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func ChatEventChannel(ownerID uuid.UUID) string {
	return fmt.Sprintf("chat:owner:%s", ownerID)
}

func HandleChatEvent(cb func(ctx context.Context, payload codersdk.ChatWatchEvent, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, codersdk.ChatWatchEvent{}, xerrors.Errorf("chat event pubsub: %w", err))
			return
		}
		var payload codersdk.ChatWatchEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, codersdk.ChatWatchEvent{}, xerrors.Errorf("unmarshal chat event: %w", err))
			return
		}

		cb(ctx, payload, err)
	}
}
