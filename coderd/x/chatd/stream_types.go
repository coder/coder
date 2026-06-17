package chatd

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// StreamPartsDialer dials an episode-aware source of message parts.
type StreamPartsDialer func(ctx context.Context, input StreamPartsDialInput) (StreamPartsSession, error)

// StreamPartsDialInput carries the metadata needed to dial a parts source.
type StreamPartsDialInput struct {
	ChatID        uuid.UUID
	WorkerID      uuid.UUID
	RequestHeader http.Header
}

// StreamPartsSession streams message parts for selected episodes.
type StreamPartsSession interface {
	SelectEpisode(ctx context.Context, historyVersion, generationAttempt int64) error
	Parts() <-chan StreamPart
	Close() error
}

// StreamPart is a live preview part scoped to one chat history episode.
type StreamPart struct {
	HistoryVersion    int64
	GenerationAttempt int64
	Seq               int64
	Role              codersdk.ChatMessageRole
	Part              codersdk.ChatMessagePart
}

type streamPart = StreamPart

type streamRelayTarget struct {
	workerID          uuid.NullUUID
	historyVersion    int64
	generationAttempt int64
}
