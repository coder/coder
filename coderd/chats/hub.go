package chats

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// Hub is an in-memory pub/sub for chat events (streaming parts and persisted
// messages).
//
// Important: Hub events are best-effort and are not persisted. The authoritative
// chat transcript is stored in the database via chat_messages.
//
// This exists to support real-time streaming (token deltas, tool call deltas,
// etc.) without having to poll the database.
type Hub struct {
	mu   sync.RWMutex
	subs map[uuid.UUID]map[chan StreamEvent]struct{}
}

type StreamEvent struct {
	RunID   string
	Part    any
	Message *database.ChatMessage
}

func NewHub() *Hub {
	return &Hub{subs: make(map[uuid.UUID]map[chan StreamEvent]struct{})}
}

func (h *Hub) Subscribe(ctx context.Context, chatID uuid.UUID) (<-chan StreamEvent, func()) {
	ch := make(chan StreamEvent, 32)

	h.mu.Lock()
	if h.subs[chatID] == nil {
		h.subs[chatID] = make(map[chan StreamEvent]struct{})
	}
	h.subs[chatID][ch] = struct{}{}
	h.mu.Unlock()

	// Use sync.Once to ensure the channel is only closed once, since cancel
	// can be called both explicitly by the caller and by the context goroutine.
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			if m := h.subs[chatID]; m != nil {
				delete(m, ch)
				if len(m) == 0 {
					delete(h.subs, chatID)
				}
			}
			h.mu.Unlock()
			close(ch)
		})
	}

	go func() {
		<-ctx.Done()
		cancel()
	}()

	return ch, cancel
}

func (h *Hub) Publish(chatID uuid.UUID, ev StreamEvent) {
	h.mu.RLock()
	m := h.subs[chatID]
	h.mu.RUnlock()
	for sub := range m {
		select {
		case sub <- ev:
		default:
			// Drop if the subscriber can't keep up.
		}
	}
}

func (h *Hub) PublishMessage(chatID uuid.UUID, message database.ChatMessage) {
	if h == nil {
		return
	}
	msg := message
	h.Publish(chatID, StreamEvent{Message: &msg})
}
