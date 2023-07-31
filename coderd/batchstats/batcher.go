package batchstats

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"os"
	"sync"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk/agentsdk"
)

const (
	// DefaultBatchSize is the default size of the batcher's buffer.
	DefaultBatchSize = 1024
)

// Batcher holds a buffer of agent stats and periodically flushes them to
// its configured store. It also updates the workspace's last used time.
type Batcher struct {
	store database.Store
	log   slog.Logger

	mu  sync.RWMutex
	buf []database.InsertWorkspaceAgentStatParams

	// ticker is used to periodically flush the buffer.
	ticker <-chan time.Time
	// flushLever is used to signal the flusher to flush the buffer immediately.
	flushLever chan struct{}
}

// Option is a functional option for configuring a Batcher.
type Option func(b *Batcher)

// WithStore sets the store to use for storing stats.
func WithStore(store database.Store) Option {
	return func(b *Batcher) {
		b.store = store
	}
}

// WithBatchSize sets the number of stats to store in a batch.
func WithBatchSize(size int) Option {
	return func(b *Batcher) {
		b.buf = make([]database.InsertWorkspaceAgentStatParams, 0, size)
	}
}

// WithTicker sets the flush interval.
func WithTicker(ch <-chan time.Time) Option {
	return func(b *Batcher) {
		b.ticker = ch
	}
}

// WithLogger sets the logger to use for logging.
func WithLogger(log slog.Logger) Option {
	return func(b *Batcher) {
		b.log = log
	}
}

// New creates a new Batcher.
func New(opts ...Option) (*Batcher, error) {
	b := &Batcher{}
	buf := make([]database.InsertWorkspaceAgentStatParams, 0, DefaultBatchSize)
	b.buf = buf
	b.log = slog.Make(sloghuman.Sink(os.Stderr))
	for _, opt := range opts {
		opt(b)
	}

	if b.store == nil {
		return nil, xerrors.Errorf("no store configured for batcher")
	}

	if b.ticker == nil {
		return nil, xerrors.Errorf("no ticker configured for batcher")
	}

	return b, nil
}

// Add adds a stat to the batcher for the given workspace and agent.
func (b *Batcher) Add(
	ctx context.Context,
	agentID uuid.UUID,
	st agentsdk.Stats,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := database.Now()
	ws, err := b.store.GetWorkspaceByAgentID(ctx, agentID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(st.ConnectionsByProto)
	if err != nil {
		b.log.Error(ctx, "marshal agent connections by proto",
			slog.F("workspace_agent_id", agentID),
			slog.Error(err),
		)
		payload = json.RawMessage("{}")
	}
	p := database.InsertWorkspaceAgentStatParams{
		ID:                          uuid.New(),
		CreatedAt:                   now,
		WorkspaceID:                 ws.ID,
		UserID:                      ws.OwnerID,
		TemplateID:                  ws.TemplateID,
		ConnectionsByProto:          payload,
		ConnectionCount:             st.ConnectionCount,
		RxPackets:                   st.RxPackets,
		RxBytes:                     st.RxBytes,
		TxPackets:                   st.TxPackets,
		TxBytes:                     st.TxBytes,
		SessionCountVSCode:          st.SessionCountVSCode,
		SessionCountJetBrains:       st.SessionCountJetBrains,
		SessionCountReconnectingPTY: st.SessionCountReconnectingPTY,
		SessionCountSSH:             st.SessionCountSSH,
		ConnectionMedianLatencyMS:   st.ConnectionMedianLatencyMS,
	}
	b.buf = append(b.buf, p)
	return nil
}

// Run runs the batcher.
func (b *Batcher) Run(ctx context.Context) {
	for {
		select {
		case tick := <-b.ticker:
			b.flush(ctx, tick)
		case <-b.flushLever:
			// If the flush lever is depressed, flush the buffer immediately.
			b.log.Warn(ctx, "flushing due to full buffer", slog.F("count", len(b.buf)))
			b.flush(ctx, database.Now())
		case <-ctx.Done():
			b.log.Warn(ctx, "context done, flushing before exit")
			b.flush(ctx, database.Now())
			return
		}
	}
}

// flush flushes the batcher's buffer.
func (b *Batcher) flush(ctx context.Context, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// TODO(Cian): After flushing, should we somehow reset the ticker?

	if len(b.buf) == 0 {
		b.log.Debug(ctx, "nothing to flush")
		return
	}

	b.log.Debug(context.Background(), "flushing buffer", slog.F("count", len(b.buf)))
	// TODO(cian): update the query to batch-insert multiple stats
	for range b.buf {
		if _, err := b.store.InsertWorkspaceAgentStat(ctx, database.InsertWorkspaceAgentStatParams{
			// TODO: fill
		}); err != nil {
			b.log.Error(context.Background(), "insert workspace agent stat", slog.Error(err))
		}
	}
}
