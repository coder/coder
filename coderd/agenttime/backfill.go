package agenttime

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

const (
	// DefaultInterval is the default delay between agent time backfill attempts.
	DefaultInterval = 5 * time.Minute
	// DefaultBatchSize bounds each backfill transaction.
	DefaultBatchSize int32 = 1000

	maxStoredErrorRunes = 2048
)

// Event reports a bounded backfill attempt to test observers.
type Event struct {
	Init              bool      `json:"-"`
	Locked            bool      `json:"locked"`
	OrganizationID    uuid.UUID `json:"organization_id"`
	SelectedMessages  int64     `json:"selected_messages"`
	ProcessedMessages int64     `json:"processed_messages"`
	Completed         bool      `json:"completed"`
	ResetCursor       bool      `json:"reset_cursor"`
}

// Backfiller accounts surviving messages independently of chat retention.
type Backfiller struct {
	cancel    context.CancelFunc
	closed    chan struct{}
	db        database.Store
	logger    slog.Logger
	interval  time.Duration
	batchSize int32
	event     chan<- Event
}

// Option configures a backfill service.
type Option func(*Backfiller)

// WithInterval sets the interval between backfill attempts.
func WithInterval(interval time.Duration) Option {
	return func(b *Backfiller) {
		b.interval = interval
	}
}

// WithBatchSize sets the maximum messages handled by one backfill transaction.
func WithBatchSize(batchSize int32) Option {
	return func(b *Backfiller) {
		b.batchSize = batchSize
	}
}

// WithEventChannel sets the event channel used by tests.
func WithEventChannel(ch chan<- Event) Option {
	if flag.Lookup("test.v") == nil {
		panic("developer error: WithEventChannel is not to be used outside of tests")
	}
	return func(b *Backfiller) {
		b.event = ch
	}
}

// New creates a background agent time backfill service. The caller must close it.
func New(logger slog.Logger, db database.Store, opts ...Option) *Backfiller {
	ctx, cancel := context.WithCancel(context.Background())
	b := &Backfiller{
		cancel:    cancel,
		closed:    make(chan struct{}),
		db:        db,
		logger:    logger,
		interval:  DefaultInterval,
		batchSize: DefaultBatchSize,
	}
	for _, opt := range opts {
		opt(b)
	}

	//nolint:gocritic // The system backfills deployment storage without user input.
	ctx = dbauthz.AsSystemRestricted(ctx)
	go b.start(ctx)
	return b
}

func (b *Backfiller) start(ctx context.Context) {
	defer close(b.closed)

	b.sendEvent(ctx, Event{Init: true})

	t := time.NewTicker(time.Microsecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := time.Now()
			next := now.Add(b.interval).Truncate(b.interval)
			delay := next.Sub(now)
			if delay <= 0 {
				delay = b.interval
			}
			t.Reset(delay)

			event, err := RunOnce(ctx, b.db, b.batchSize)
			if err != nil {
				if database.IsQueryCanceledError(err) {
					continue
				}
				if ctx.Err() == nil {
					b.logger.Error(ctx, "failed to backfill agent time", slog.Error(err))
				}
				continue
			}
			if event.Locked {
				b.logger.Debug(ctx,
					"backfilled agent time",
					slog.F("organization_id", event.OrganizationID),
					slog.F("selected_messages", event.SelectedMessages),
					slog.F("processed_messages", event.ProcessedMessages),
					slog.F("completed", event.Completed),
					slog.F("reset_cursor", event.ResetCursor),
				)
			}
			b.sendEvent(ctx, event)
		}
	}
}

func (b *Backfiller) sendEvent(ctx context.Context, event Event) {
	if b.event == nil {
		return
	}
	select {
	case <-ctx.Done():
	case b.event <- event:
	}
}

// Close stops the background agent time backfill service.
func (b *Backfiller) Close() error {
	b.cancel()
	<-b.closed
	return nil
}

// RunOnce runs at most one bounded agent time backfill transaction.
func RunOnce(ctx context.Context, db database.Store, batchSize int32) (Event, error) {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	var event Event
	err := db.InTx(func(tx database.Store) error {
		ok, err := tx.TryAcquireLock(ctx, database.LockIDAgentTimeBackfill)
		if err != nil {
			return xerrors.Errorf("acquire agent time backfill lock: %w", err)
		}
		if !ok {
			return nil
		}
		event.Locked = true

		_, err = tx.EnsureAgentTimeBackfillStatuses(ctx)
		if err != nil {
			return xerrors.Errorf("ensure agent time backfill statuses: %w", err)
		}

		status, err := tx.AcquireAgentTimeBackfillOrganization(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("acquire agent time backfill organization: %w", err)
		}
		event.OrganizationID = status.OrganizationID

		batch, err := tx.BackfillAgentTimeBatch(ctx, database.BackfillAgentTimeBatchParams{
			CursorMessageID: status.CursorMessageID,
			OrganizationID:  status.OrganizationID,
			LimitCount:      batchSize,
		})
		if err != nil {
			return xerrors.Errorf("backfill agent time batch: %w", err)
		}
		event.SelectedMessages = batch.SelectedMessages
		event.ProcessedMessages = batch.ProcessedMessages

		if batch.SelectedMessages > 0 {
			return tx.UpdateAgentTimeBackfillProgress(ctx, database.UpdateAgentTimeBackfillProgressParams{
				CursorMessageID:   batch.CursorMessageID,
				ProcessedMessages: batch.ProcessedMessages,
				OrganizationID:    status.OrganizationID,
			})
		}

		hasUnaccounted, err := tx.HasUnaccountedAgentTimeMessages(ctx, status.OrganizationID)
		if err != nil {
			return xerrors.Errorf("check unaccounted agent time messages: %w", err)
		}
		if hasUnaccounted {
			event.ResetCursor = true
			return tx.ResetAgentTimeBackfillCursor(ctx, status.OrganizationID)
		}

		event.Completed = true
		return tx.CompleteAgentTimeBackfillOrganization(ctx, status.OrganizationID)
	}, database.DefaultTXOptions().WithID("agent_time_backfill"))
	if err == nil {
		return event, nil
	}

	if event.OrganizationID != uuid.Nil {
		markErr := db.MarkAgentTimeBackfillFailed(ctx, database.MarkAgentTimeBackfillFailedParams{
			OrganizationID: event.OrganizationID,
			LastError:      truncateError(err),
		})
		if markErr != nil && !database.IsQueryCanceledError(markErr) {
			return event, xerrors.Errorf("%w; mark agent time backfill failure: %v", err, markErr)
		}
	}
	return event, err
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if utf8.RuneCountInString(msg) <= maxStoredErrorRunes {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:maxStoredErrorRunes])
}
