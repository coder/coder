package confine

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type eventBatcher struct {
	client    AgentClient
	logger    slog.Logger
	sessionID uuid.UUID
	capacity  int

	mu     sync.Mutex
	events []agentsdk.AISandboxNetworkEvent
}

func newEventBatcher(client AgentClient, logger slog.Logger, sessionID uuid.UUID, capacity int) *eventBatcher {
	return &eventBatcher{
		client:    client,
		logger:    logger,
		sessionID: sessionID,
		capacity:  capacity,
		events:    make([]agentsdk.AISandboxNetworkEvent, 0, capacity),
	}
}

func (b *eventBatcher) Add(event NetworkEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.capacity <= 0 {
		return
	}
	if len(b.events) == b.capacity {
		copy(b.events, b.events[1:])
		b.events = b.events[:len(b.events)-1]
	}
	b.events = append(b.events, agentsdk.AISandboxNetworkEvent{
		SessionID:      b.sessionID,
		OccurredAt:     time.Now(),
		Protocol:       event.Protocol,
		Host:           event.Host,
		Port:           event.Port,
		Action:         event.Action,
		PolicyRevision: event.PolicyRevision,
	})
}

func (b *eventBatcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.Flush()
		}
	}
}

func (b *eventBatcher) Flush() {
	b.mu.Lock()
	if len(b.events) == 0 {
		b.mu.Unlock()
		return
	}
	events := append([]agentsdk.AISandboxNetworkEvent(nil), b.events...)
	b.events = b.events[:0]
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()
	err := b.client.PatchAISandboxNetworkEvents(ctx, agentsdk.PatchAISandboxNetworkEventsRequest{Events: events})
	if err == nil || isNotFound(err) {
		return
	}
	b.logger.Warn(ctx, "report AI sandbox network events", slog.Error(err), slog.F("event_count", len(events)))
}
