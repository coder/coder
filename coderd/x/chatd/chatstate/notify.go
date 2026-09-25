package chatstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

// Publisher is the minimal interface chatstate needs to publish
// pubsub messages. It is intentionally compatible with
// database/pubsub.Pubsub: real callers pass the live pubsub directly
// and tests pass a recording fake.
type Publisher interface {
	Publish(event string, message []byte) error
}

// PublishBuffer is a [Publisher] that records each Publish call in
// order without forwarding it until [PublishBuffer.Flush] is called.
// It is an internal primitive used by chatstate entry points to
// hold pubsub messages until the surrounding transaction commits,
// and by tests that need to observe buffered output. Normal callers
// do not construct a PublishBuffer themselves and do not invoke
// Flush or Discard; chatstate's entry points own that lifecycle.
type PublishBuffer struct {
	inner Publisher

	mu       sync.Mutex
	pending  []bufferedMessage
	flushed  bool
	disabled bool
}

type bufferedMessage struct {
	Channel string
	Payload []byte
}

// NewPublishBuffer constructs a PublishBuffer that, when flushed, will
// forward messages in order to inner.
func NewPublishBuffer(inner Publisher) *PublishBuffer {
	return &PublishBuffer{inner: inner}
}

// Publish records a message. It never forwards to the inner publisher
// until [PublishBuffer.Flush] is called. Returns an error if Flush has
// already happened to make accidental reuse obvious.
func (b *PublishBuffer) Publish(channel string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.flushed {
		return xerrors.Errorf("publish buffer flushed; cannot accept message for %q", channel)
	}
	if b.disabled {
		return nil
	}
	b.pending = append(b.pending, bufferedMessage{Channel: channel, Payload: slices.Clone(payload)})
	return nil
}

// Flush forwards every pending message to the inner publisher in the
// order it was buffered, then marks the buffer flushed. Joined publish
// errors are returned with channel names annotated after every pending
// message has been attempted.
func (b *PublishBuffer) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.flushed {
		return nil
	}
	b.flushed = true
	var errs []error
	for _, msg := range b.pending {
		if err := b.inner.Publish(msg.Channel, msg.Payload); err != nil {
			errs = append(errs, xerrors.Errorf("publish %s: %w", msg.Channel, err))
		}
	}
	return errors.Join(errs...)
}

// Discard clears the buffered messages without forwarding them. It
// is safe to call multiple times and is harmless after [PublishBuffer.Flush]:
// once Flush has marked the buffer flushed and forwarded its
// pending messages, a subsequent Discard simply clears the (now
// empty) pending slice and sets the buffer to drop any future
// Publish calls. This makes `defer buf.Discard()` a safe pattern
// after a successful flush, including the one chatstate entry
// points use to own the buffer lifecycle.
func (b *PublishBuffer) Discard() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = nil
	b.disabled = true
}

// chatStateSnapshot carries the chat columns the state-update and
// ownership publications read, so both the full chat row and the lean
// post-transition row can feed them.
type chatStateSnapshot struct {
	ID                uuid.UUID
	SnapshotVersion   int64
	HistoryVersion    int64
	QueueVersion      int64
	RetryStateVersion int64
	GenerationAttempt int64
	Status            database.ChatStatus
	Archived          bool
	WorkerID          uuid.NullUUID
	RunnerID          uuid.NullUUID
}

func snapshotFromChat(chat database.Chat) chatStateSnapshot {
	return chatStateSnapshot{
		ID:                chat.ID,
		SnapshotVersion:   chat.SnapshotVersion,
		HistoryVersion:    chat.HistoryVersion,
		QueueVersion:      chat.QueueVersion,
		RetryStateVersion: chat.RetryStateVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            chat.Status,
		Archived:          chat.Archived,
		WorkerID:          chat.WorkerID,
		RunnerID:          chat.RunnerID,
	}
}

func snapshotFromTransitionState(row database.GetChatTransitionStateRow) chatStateSnapshot {
	return chatStateSnapshot{
		ID:                row.ID,
		SnapshotVersion:   row.SnapshotVersion,
		HistoryVersion:    row.HistoryVersion,
		QueueVersion:      row.QueueVersion,
		RetryStateVersion: row.RetryStateVersion,
		GenerationAttempt: row.GenerationAttempt,
		Status:            row.Status,
		Archived:          row.Archived,
		WorkerID:          row.WorkerID,
		RunnerID:          row.RunnerID,
	}
}

// buildChatUpdateMessage produces the JSON payload for a
// `chat:update` publication from a full chat row.
func buildChatUpdateMessage(chat database.Chat) []byte {
	return chatUpdateMessage(snapshotFromChat(chat))
}

func chatUpdateMessage(s chatStateSnapshot) []byte {
	msg := coderdpubsub.ChatStateUpdateMessage{
		SnapshotVersion:   s.SnapshotVersion,
		HistoryVersion:    s.HistoryVersion,
		QueueVersion:      s.QueueVersion,
		RetryStateVersion: s.RetryStateVersion,
		GenerationAttempt: s.GenerationAttempt,
		Status:            string(s.Status),
		Archived:          s.Archived,
	}
	if s.WorkerID.Valid {
		id := s.WorkerID.UUID
		msg.WorkerID = &id
	}
	if s.RunnerID.Valid {
		id := s.RunnerID.UUID
		msg.RunnerID = &id
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		// json.Marshal on this struct is total; panic is acceptable
		// because the only failure mode would be a bug in this
		// package, not user input.
		panic(fmt.Sprintf("marshal chat state update: %v", err))
	}
	return payload
}

// buildChatOwnershipMessage produces the JSON payload for the global
// `chat:ownership` ownership hint from a full chat row.
func buildChatOwnershipMessage(chat database.Chat) []byte {
	return chatOwnershipMessage(snapshotFromChat(chat))
}

func chatOwnershipMessage(s chatStateSnapshot) []byte {
	payload, err := json.Marshal(coderdpubsub.ChatStateOwnershipMessage{
		ChatID:          s.ID,
		SnapshotVersion: s.SnapshotVersion,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal chat state ownership: %v", err))
	}
	return payload
}
