package chats

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

const (
	// DefaultPollInterval is the default time between polling for pending chats.
	DefaultPollInterval = time.Second
	// DefaultStaleThreshold is the default time after which a running chat is
	// considered stale and should be recovered.
	DefaultStaleThreshold = 5 * time.Minute
)

// Processor handles background processing of pending chats.
type Processor struct {
	cancel context.CancelFunc
	closed chan struct{}

	db       database.Store
	workerID uuid.UUID
	logger   slog.Logger

	// Configuration
	pollInterval   time.Duration
	staleThreshold time.Duration
}

type Option func(*Processor)

// WithPollInterval sets the interval between polling for pending chats.
func WithPollInterval(interval time.Duration) Option {
	return func(p *Processor) {
		p.pollInterval = interval
	}
}

// WithStaleThreshold sets the time after which a running chat is considered stale.
func WithStaleThreshold(threshold time.Duration) Option {
	return func(p *Processor) {
		p.staleThreshold = threshold
	}
}

// NewProcessor creates a new chat processor. The processor polls for pending
// chats and processes them. It is the caller's responsibility to call Close
// on the returned instance.
func NewProcessor(logger slog.Logger, db database.Store, opts ...Option) *Processor {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Processor{
		cancel:         cancel,
		closed:         make(chan struct{}),
		db:             db,
		workerID:       uuid.New(),
		logger:         logger.Named("chat-processor"),
		pollInterval:   DefaultPollInterval,
		staleThreshold: DefaultStaleThreshold,
	}

	for _, opt := range opts {
		opt(p)
	}

	//nolint:gocritic // The chat processor is a system-level service.
	ctx = dbauthz.AsSystemRestricted(ctx)
	go p.start(ctx)

	return p
}

func (p *Processor) start(ctx context.Context) {
	defer close(p.closed)

	// First, recover any stale chats from crashed workers.
	p.recoverStaleChats(ctx)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processOnce(ctx)
		}
	}
}

func (p *Processor) processOnce(ctx context.Context) {
	// Try to acquire a pending chat.
	chat, err := p.db.AcquireChat(ctx, database.AcquireChatParams{
		StartedAt: time.Now(),
		WorkerID:  p.workerID,
	})
	if err != nil {
		if !xerrors.Is(err, sql.ErrNoRows) {
			p.logger.Error(ctx, "failed to acquire chat", slog.Error(err))
		}
		// No pending chats or error.
		return
	}

	// Process the chat (don't block the main loop).
	go p.processChat(ctx, chat)
}

func (p *Processor) processChat(ctx context.Context, chat database.Chat) {
	logger := p.logger.With(slog.F("chat_id", chat.ID))
	logger.Info(ctx, "processing chat")

	// Determine the final status to set when we're done.
	status := database.ChatStatusWaiting

	defer func() {
		// Handle panics gracefully.
		if r := recover(); r != nil {
			logger.Error(ctx, "panic during chat processing", slog.F("panic", r))
			status = database.ChatStatusError
		}

		// Release the chat when done.
		_, err := p.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:        chat.ID,
			Status:    status,
			WorkerID:  uuid.NullUUID{}, // Clear worker.
			StartedAt: sql.NullTime{},  // Clear started_at.
		})
		if err != nil {
			logger.Error(ctx, "failed to release chat", slog.Error(err))
		}
	}()

	// Get messages for this chat.
	messages, err := p.db.GetChatMessagesByChatID(ctx, chat.ID)
	if err != nil {
		logger.Error(ctx, "failed to get chat messages", slog.Error(err))
		status = database.ChatStatusError
		return
	}

	logger.Info(ctx, "chat has messages", slog.F("count", len(messages)))

	// TODO: Call AI Bridge, process tool calls, etc.
	// For now, just log and mark as waiting for more input.
}

func (p *Processor) recoverStaleChats(ctx context.Context) {
	staleThreshold := time.Now().Add(-p.staleThreshold)
	staleChats, err := p.db.GetStaleChats(ctx, staleThreshold)
	if err != nil {
		p.logger.Error(ctx, "failed to get stale chats", slog.Error(err))
		return
	}

	for _, chat := range staleChats {
		p.logger.Info(ctx, "recovering stale chat", slog.F("chat_id", chat.ID))

		// Reset to pending so any replica can pick it up.
		_, err := p.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:        chat.ID,
			Status:    database.ChatStatusPending,
			WorkerID:  uuid.NullUUID{},
			StartedAt: sql.NullTime{},
		})
		if err != nil {
			p.logger.Error(ctx, "failed to recover stale chat",
				slog.F("chat_id", chat.ID), slog.Error(err))
		}
	}

	if len(staleChats) > 0 {
		p.logger.Info(ctx, "recovered stale chats", slog.F("count", len(staleChats)))
	}
}

// Close stops the processor and waits for it to finish.
func (p *Processor) Close() error {
	p.cancel()
	<-p.closed
	return nil
}
