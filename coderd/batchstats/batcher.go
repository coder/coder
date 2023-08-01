package batchstats

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/codersdk/agentsdk"
)

const (
	// DefaultBatchSize is the default size of the batcher's buffer.
	DefaultBatchSize = 1024
)

// batchInsertParams is a struct used to batch-insert stats.
// It looks weird because of how we insert the data into the database
// (using unnest).
// Ideally we would just use COPY FROM but that isn't supported by
// sqlc using lib/pq. At a later date we should consider switching to
// pgx. See: https://docs.sqlc.dev/en/stable/guides/using-go-and-pgx.html
type batchInsertParams struct {
	IDs                          []uuid.UUID
	CreatedAts                   []time.Time
	UserIDs                      []uuid.UUID
	WorkspaceIDs                 []uuid.UUID
	TemplateIDs                  []uuid.UUID
	AgentIDs                     []uuid.UUID
	ConnectionsByProtos          []json.RawMessage
	ConnectionCounts             []int64
	RxPacketses                  []int64
	RxByteses                    []int64
	TxPacketses                  []int64
	TxByteses                    []int64
	SessionCountVSCodes          []int64
	SessionCountJetBrainses      []int64
	SessionCountReconnectingPTYs []int64
	SessionCountSSHs             []int64
	ConnectionMedianLatencyMSes  []float64
}

// Batcher holds a buffer of agent stats and periodically flushes them to
// its configured store. It also updates the workspace's last used time.
type Batcher struct {
	store database.Store
	log   slog.Logger

	mu        sync.RWMutex
	buf       batchInsertParams
	batchSize int

	// ticker is used to periodically flush the buffer.
	ticker <-chan time.Time
	// flushLever is used to signal the flusher to flush the buffer immediately.
	flushLever chan struct{}
	// flushed is used during testing to signal that a flush has completed.
	flushed chan<- bool
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
		b.batchSize = size
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

// With Flushed sets the channel to use for signaling that a flush has completed.
// This is only used for testing.
// True signifies that a flush was forced.
func WithFlushed(ch chan bool) Option {
	return func(b *Batcher) {
		b.flushed = ch
	}
}

// New creates a new Batcher.
func New(opts ...Option) (*Batcher, error) {
	b := &Batcher{}
	b.log = slog.Make(sloghuman.Sink(os.Stderr))
	b.flushLever = make(chan struct{}, 1) // Buffered so that it doesn't block.
	for _, opt := range opts {
		opt(b)
	}

	if b.store == nil {
		return nil, xerrors.Errorf("no store configured for batcher")
	}

	if b.ticker == nil {
		return nil, xerrors.Errorf("no ticker configured for batcher")
	}

	if b.batchSize == 0 {
		b.batchSize = DefaultBatchSize
	}

	b.buf = batchInsertParams{
		IDs:                          make([]uuid.UUID, 0, b.batchSize),
		CreatedAts:                   make([]time.Time, 0, b.batchSize),
		UserIDs:                      make([]uuid.UUID, 0, b.batchSize),
		WorkspaceIDs:                 make([]uuid.UUID, 0, b.batchSize),
		TemplateIDs:                  make([]uuid.UUID, 0, b.batchSize),
		AgentIDs:                     make([]uuid.UUID, 0, b.batchSize),
		ConnectionsByProtos:          make([]json.RawMessage, 0, b.batchSize),
		ConnectionCounts:             make([]int64, 0, b.batchSize),
		RxPacketses:                  make([]int64, 0, b.batchSize),
		RxByteses:                    make([]int64, 0, b.batchSize),
		TxPacketses:                  make([]int64, 0, b.batchSize),
		TxByteses:                    make([]int64, 0, b.batchSize),
		SessionCountVSCodes:          make([]int64, 0, b.batchSize),
		SessionCountJetBrainses:      make([]int64, 0, b.batchSize),
		SessionCountReconnectingPTYs: make([]int64, 0, b.batchSize),
		SessionCountSSHs:             make([]int64, 0, b.batchSize),
		ConnectionMedianLatencyMSes:  make([]float64, 0, b.batchSize),
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

	// TODO(Cian): add a specific dbauthz context for this.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	now := database.Now()
	// TODO(Cian): cache agentID -> workspaceID?
	ws, err := b.store.GetWorkspaceByAgentID(authCtx, agentID)
	if err != nil {
		return xerrors.Errorf("get workspace by agent id: %w", err)
	}
	payload, err := json.Marshal(st.ConnectionsByProto)
	if err != nil {
		b.log.Error(ctx, "marshal agent connections by proto",
			slog.F("workspace_agent_id", agentID),
			slog.Error(err),
		)
		payload = json.RawMessage("{}")
	}

	b.buf.IDs = append(b.buf.IDs, uuid.New())
	b.buf.AgentIDs = append(b.buf.AgentIDs, agentID)
	b.buf.CreatedAts = append(b.buf.CreatedAts, now)
	b.buf.UserIDs = append(b.buf.UserIDs, ws.OwnerID)
	b.buf.WorkspaceIDs = append(b.buf.WorkspaceIDs, ws.ID)
	b.buf.TemplateIDs = append(b.buf.TemplateIDs, ws.TemplateID)
	b.buf.ConnectionsByProtos = append(b.buf.ConnectionsByProtos, payload)
	b.buf.ConnectionCounts = append(b.buf.ConnectionCounts, st.ConnectionCount)
	b.buf.RxPacketses = append(b.buf.RxPacketses, st.RxPackets)
	b.buf.RxByteses = append(b.buf.RxByteses, st.RxBytes)
	b.buf.TxPacketses = append(b.buf.TxPacketses, st.TxPackets)
	b.buf.TxByteses = append(b.buf.TxByteses, st.TxBytes)
	b.buf.SessionCountVSCodes = append(b.buf.SessionCountVSCodes, st.SessionCountVSCode)
	b.buf.SessionCountJetBrainses = append(b.buf.SessionCountJetBrainses, st.SessionCountJetBrains)
	b.buf.SessionCountReconnectingPTYs = append(b.buf.SessionCountReconnectingPTYs, st.SessionCountReconnectingPTY)
	b.buf.SessionCountSSHs = append(b.buf.SessionCountSSHs, st.SessionCountSSH)
	b.buf.ConnectionMedianLatencyMSes = append(b.buf.ConnectionMedianLatencyMSes, st.ConnectionMedianLatencyMS)

	// If the buffer is full, signal the flusher to flush immediately.
	if len(b.buf.IDs) == cap(b.buf.IDs) {
		b.flushLever <- struct{}{}
	}
	return nil
}

// Run runs the batcher.
func (b *Batcher) Run(ctx context.Context) {
	authCtx := dbauthz.AsSystemRestricted(ctx)
	for {
		select {
		case <-b.ticker:
			b.flush(authCtx, false, "scheduled")
		case <-b.flushLever:
			// If the flush lever is depressed, flush the buffer immediately.
			b.flush(authCtx, true, "full buffer")
		case <-ctx.Done():
			b.log.Warn(ctx, "context done, flushing before exit")
			b.flush(authCtx, true, "exit")
			return
		}
	}
}

// flush flushes the batcher's buffer.
func (b *Batcher) flush(ctx context.Context, forced bool, reason string) {
	// TODO(Cian): After flushing, should we somehow reset the ticker?
	b.mu.Lock()
	defer b.mu.Unlock()
	defer func() {
		// Notify that a flush has completed.
		if b.flushed != nil {
			b.flushed <- forced
			b.log.Debug(ctx, "notify flush")
		}
	}()

	b.log.Debug(ctx, "flushing buffer", slog.F("count", len(b.buf.IDs)), slog.F("forced", forced))
	if len(b.buf.IDs) == 0 {
		b.log.Debug(ctx, "nothing to flush")
		return
	}

	if err := b.store.InsertWorkspaceAgentStats(ctx, database.InsertWorkspaceAgentStatsParams{
		ID:                          b.buf.IDs,
		CreatedAt:                   b.buf.CreatedAts,
		UserID:                      b.buf.UserIDs,
		WorkspaceID:                 b.buf.WorkspaceIDs,
		TemplateID:                  b.buf.TemplateIDs,
		AgentID:                     b.buf.AgentIDs,
		ConnectionsByProto:          b.buf.ConnectionsByProtos,
		ConnectionCount:             b.buf.ConnectionCounts,
		RxPackets:                   b.buf.RxPacketses,
		RxBytes:                     b.buf.RxByteses,
		TxPackets:                   b.buf.TxPacketses,
		TxBytes:                     b.buf.TxByteses,
		SessionCountVSCode:          b.buf.SessionCountVSCodes,
		SessionCountJetBrains:       b.buf.SessionCountJetBrainses,
		SessionCountReconnectingPTY: b.buf.SessionCountReconnectingPTYs,
		SessionCountSSH:             b.buf.SessionCountSSHs,
		ConnectionMedianLatencyMS:   b.buf.ConnectionMedianLatencyMSes,
	}); err != nil {
		b.log.Error(ctx, "insert workspace agent stats", slog.Error(err))
	}

	// Reset the buffer.
	// b.buf = b.buf[:0]
	b.buf.IDs = b.buf.IDs[:0]
	b.buf.CreatedAts = b.buf.CreatedAts[:0]
	b.buf.UserIDs = b.buf.UserIDs[:0]
	b.buf.WorkspaceIDs = b.buf.WorkspaceIDs[:0]
	b.buf.TemplateIDs = b.buf.TemplateIDs[:0]
	b.buf.AgentIDs = b.buf.AgentIDs[:0]
	b.buf.ConnectionsByProtos = b.buf.ConnectionsByProtos[:0]
	b.buf.ConnectionCounts = b.buf.ConnectionCounts[:0]
	b.buf.RxPacketses = b.buf.RxPacketses[:0]
	b.buf.RxByteses = b.buf.RxByteses[:0]
	b.buf.TxPacketses = b.buf.TxPacketses[:0]
	b.buf.TxByteses = b.buf.TxByteses[:0]
	b.buf.SessionCountVSCodes = b.buf.SessionCountVSCodes[:0]
	b.buf.SessionCountJetBrainses = b.buf.SessionCountJetBrainses[:0]
	b.buf.SessionCountReconnectingPTYs = b.buf.SessionCountReconnectingPTYs[:0]
	b.buf.SessionCountSSHs = b.buf.SessionCountSSHs[:0]
	b.buf.ConnectionMedianLatencyMSes = b.buf.ConnectionMedianLatencyMSes[:0]
}
