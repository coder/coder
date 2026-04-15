package pubsub

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// ChatConfigEventChannel is the pubsub channel for chat config
// changes (providers, model configs, user prompts). All replicas
// subscribe to this channel to invalidate their local caches.
const ChatConfigEventChannel = "chat:config_change"

// HandleChatConfigEvent wraps a typed callback for ChatConfigEvent
// messages, following the same pattern as HandleChatWatchEvent.
func HandleChatConfigEvent(cb func(ctx context.Context, payload ChatConfigEvent, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, ChatConfigEvent{}, xerrors.Errorf("chat config event pubsub: %w", err))
			return
		}
		var payload ChatConfigEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, ChatConfigEvent{}, xerrors.Errorf("unmarshal chat config event: %w", err))
			return
		}

		cb(ctx, payload, err)
	}
}

// ChatConfigEvent is published when chat configuration changes
// (provider CRUD, model config CRUD, or user prompt updates).
// Subscribers use this to invalidate their local caches.
type ChatConfigEvent struct {
	Kind ChatConfigEventKind `json:"kind"`
	// EntityID carries context for the invalidation:
	//   - For providers: uuid.Nil (all providers are invalidated).
	//   - For model configs: the specific config ID.
	//   - For user prompts: the user ID.
	EntityID uuid.UUID `json:"entity_id"`
}

type ChatConfigEventKind string

const (
	ChatConfigEventProviders   ChatConfigEventKind = "providers"
	ChatConfigEventModelConfig ChatConfigEventKind = "model_config"
	ChatConfigEventUserPrompt  ChatConfigEventKind = "user_prompt"
)
