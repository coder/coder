package chatd

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/quartz"
)

const streamSyncInterval = 10 * time.Second

type streamSyncPoller struct {
	ctx    context.Context
	cancel context.CancelFunc
	db     database.Store
	clock  quartz.Clock
	logger slog.Logger

	mu          sync.Mutex
	subscribers map[uuid.UUID]map[*streamSyncPollerSubscriber]struct{}
}

type streamSyncPollerSubscriber struct {
	chatID uuid.UUID
	hints  chan streamSyncHint
}

func newStreamSyncPoller(
	ctx context.Context,
	db database.Store,
	clock quartz.Clock,
	logger slog.Logger,
) *streamSyncPoller {
	if clock == nil {
		clock = quartz.NewReal()
	}
	//nolint:gocritic // The poller is internal chatd infrastructure. Each
	// registered stream was already authorized before subscription, and this
	// batch query only fetches synchronization metadata for subscribed chats.
	pollerCtx, cancel := context.WithCancel(dbauthz.AsChatd(ctx))
	return &streamSyncPoller{
		ctx:         pollerCtx,
		cancel:      cancel,
		db:          db,
		clock:       clock,
		logger:      logger,
		subscribers: make(map[uuid.UUID]map[*streamSyncPollerSubscriber]struct{}),
	}
}

func (p *streamSyncPoller) Start() {
	if p == nil {
		return
	}
	go p.loop()
}

func (p *streamSyncPoller) Close() {
	if p == nil {
		return
	}
	p.cancel()
}

func (p *streamSyncPoller) Register(chatID uuid.UUID) (<-chan streamSyncHint, func()) {
	if p == nil {
		ch := make(chan streamSyncHint)
		close(ch)
		return ch, func() {}
	}
	subscriber := &streamSyncPollerSubscriber{
		chatID: chatID,
		hints:  make(chan streamSyncHint, 1),
	}
	p.mu.Lock()
	if p.subscribers[chatID] == nil {
		p.subscribers[chatID] = make(map[*streamSyncPollerSubscriber]struct{})
	}
	p.subscribers[chatID][subscriber] = struct{}{}
	p.mu.Unlock()

	return subscriber.hints, func() {
		p.unregister(subscriber)
	}
}

func (p *streamSyncPoller) unregister(subscriber *streamSyncPollerSubscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	chatSubscribers := p.subscribers[subscriber.chatID]
	if chatSubscribers == nil {
		return
	}
	delete(chatSubscribers, subscriber)
	if len(chatSubscribers) == 0 {
		delete(p.subscribers, subscriber.chatID)
	}
	close(subscriber.hints)
}

func (p *streamSyncPoller) loop() {
	ticker := p.clock.NewTicker(streamSyncInterval, "chatd", "stream-sync-poller")
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce()
		}
	}
}

func (p *streamSyncPoller) pollOnce() {
	chatIDs, subscribers := p.snapshotSubscribers()
	if len(chatIDs) == 0 {
		return
	}
	rows, err := p.db.GetChatStreamSyncRows(p.ctx, chatIDs)
	if err != nil {
		if p.ctx.Err() == nil {
			p.logger.Warn(p.ctx, "failed to poll chat streams", slog.Error(err))
		}
		return
	}
	for _, row := range rows {
		hint := streamSyncHintFromPollRow(row)
		for _, subscriber := range subscribers[row.ID] {
			select {
			case subscriber.hints <- hint:
			default:
			}
		}
	}
}

func (p *streamSyncPoller) snapshotSubscribers() ([]uuid.UUID, map[uuid.UUID][]*streamSyncPollerSubscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	chatIDs := make([]uuid.UUID, 0, len(p.subscribers))
	subscribers := make(map[uuid.UUID][]*streamSyncPollerSubscriber, len(p.subscribers))
	for chatID, chatSubscribers := range p.subscribers {
		chatIDs = append(chatIDs, chatID)
		for subscriber := range chatSubscribers {
			subscribers[chatID] = append(subscribers[chatID], subscriber)
		}
	}
	return chatIDs, subscribers
}

func streamSyncHintFromPollRow(row database.GetChatStreamSyncRowsRow) streamSyncHint {
	return streamSyncHint{
		snapshotVersion:   row.SnapshotVersion,
		historyVersion:    row.HistoryVersion,
		queueVersion:      row.QueueVersion,
		retryVersion:      row.RetryStateVersion,
		status:            row.Status,
		workerID:          row.WorkerID,
		generationAttempt: row.GenerationAttempt,
	}
}
