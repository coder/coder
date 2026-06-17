package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// ChatStateUpdateChannel returns the pubsub channel that receives one
// `chat:update:{chat_id}` message every time the chatstate state
// machine commits a transition for the chat.
func ChatStateUpdateChannel(chatID uuid.UUID) string {
	return fmt.Sprintf("chat:update:%s", chatID)
}

// ChatStateOwnershipChannel is the global pubsub channel that
// receives ownership hints when a chat is runnable but currently has
// missing or stale ownership. Workers listen on this channel to know
// when to attempt acquisition.
const ChatStateOwnershipChannel = "chat:ownership"

// ChatStateUpdateMessage is the JSON payload published on
// [ChatStateUpdateChannel] after every successful CreateChat or
// ChatMachine.Update commit. It carries the committed post-transition
// versions and ownership identifiers so stream loops and workers can
// decide whether to refetch state.
type ChatStateUpdateMessage struct {
	SnapshotVersion   int64      `json:"snapshot_version"`
	WorkerID          *uuid.UUID `json:"worker_id"`
	RunnerID          *uuid.UUID `json:"runner_id"`
	HistoryVersion    int64      `json:"history_version"`
	QueueVersion      int64      `json:"queue_version"`
	RetryStateVersion int64      `json:"retry_state_version"`
	GenerationAttempt int64      `json:"generation_attempt"`
	Status            string     `json:"status"`
	Archived          bool       `json:"archived"`
}

// ChatStateOwnershipMessage is the JSON payload published on
// [ChatStateOwnershipChannel] when ownership is missing or stale for
// a runnable chat. Subscribers should reload the chat row to confirm
// ownership before acting.
type ChatStateOwnershipMessage struct {
	ChatID          uuid.UUID `json:"chat_id"`
	SnapshotVersion int64     `json:"snapshot_version"`
}

// HandleChatStateUpdate wraps a typed callback for
// [ChatStateUpdateMessage] consumption, following the same pattern as
// HandleChatWatchEvent.
func HandleChatStateUpdate(cb func(ctx context.Context, payload ChatStateUpdateMessage, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, ChatStateUpdateMessage{}, xerrors.Errorf("chat state update pubsub: %w", err))
			return
		}
		var payload ChatStateUpdateMessage
		if uerr := json.Unmarshal(message, &payload); uerr != nil {
			cb(ctx, ChatStateUpdateMessage{}, xerrors.Errorf("unmarshal chat state update: %w", uerr))
			return
		}
		cb(ctx, payload, err)
	}
}

// HandleChatStateOwnership wraps a typed callback for
// [ChatStateOwnershipMessage] consumption.
func HandleChatStateOwnership(cb func(ctx context.Context, payload ChatStateOwnershipMessage, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, ChatStateOwnershipMessage{}, xerrors.Errorf("chat state ownership pubsub: %w", err))
			return
		}
		var payload ChatStateOwnershipMessage
		if uerr := json.Unmarshal(message, &payload); uerr != nil {
			cb(ctx, ChatStateOwnershipMessage{}, xerrors.Errorf("unmarshal chat state ownership: %w", uerr))
			return
		}
		cb(ctx, payload, err)
	}
}
