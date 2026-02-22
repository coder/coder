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

func HandleChatEvent(cb func(ctx context.Context, payload ChatEvent, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, ChatEvent{}, xerrors.Errorf("chat event pubsub: %w", err))
			return
		}
		var payload ChatEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, ChatEvent{}, xerrors.Errorf("unmarshal chat event"))
			return
		}

		cb(ctx, payload, err)
	}
}

type ChatEvent struct {
	Kind ChatEventKind `json:"kind"`
	Chat codersdk.Chat `json:"chat"`
}

type ChatEventKind string

const (
	ChatEventKindStatusChange ChatEventKind = "status_change"
	ChatEventKindTitleChange  ChatEventKind = "title_change"
	ChatEventKindCreated      ChatEventKind = "created"
	ChatEventKindDeleted      ChatEventKind = "deleted"
)
