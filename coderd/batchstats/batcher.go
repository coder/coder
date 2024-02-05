package batchstats

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

const (
	defaultBufferSize    = 1024
	defaultFlushInterval = time.Second
)

// Batcher holds a buffer of agent stats and periodically flushes them to
// its configured store. It also updates the workspace's last used time.
type Batcher struct {
	store database.Store
	log   slog.Logger

	mu sync.Mutex
	// TODO: make this a buffered chan instead?
	buf *database.InsertWorkspaceAgentStatsParams
	// NOTE: we batch this separately as it's a jsonb field and
	// pq.Array + unnest doesn't play nicely with this.
	connectionsByProto []map[string]int64
	batchSize          int

	// tickCh is used to periodically flush the buffer.
	tickCh   <-chan time.Time
	ticker   *time.Ticker
	interval time.Duration
	// flushLever is used to signal the flusher to flush the buffer immediately.
	flushLever  chan struct{}
	flushForced atomic.Bool
	// flushed is used during testing to signal that a flush has completed.
	flushed chan<- int
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

// WithInterval sets the interval for flushes.
func WithInterval(d time.Duration) Option {
	return func(b *Batcher) {
		b.interval = d
	}
}

// WithLogger sets the logger to use for logging.
func WithLogger(log slog.Logger) Option {
	return func(b *Batcher) {
		b.log = log
	}
}

// New creates a new Batcher and starts it.
func New(ctx context.Context, opts ...Option) (*Batcher, func(), error) {
	b := &Batcher{}
	b.log = slog.Make(sloghuman.Sink(os.Stderr))
	b.flushLever = make(chan struct{}, 1) // Buffered so that it doesn't block.
	for _, opt := range opts {
		opt(b)
	}

	if b.store == nil {
		return nil, nil, xerrors.Errorf("no store configured for batcher")
	}

	if b.interval == 0 {
		b.interval = defaultFlushInterval
	}

	if b.batchSize == 0 {
		b.batchSize = defaultBufferSize
	}

	if b.tickCh == nil {
		b.ticker = time.NewTicker(b.interval)
		b.tickCh = b.ticker.C
	}

	b.initBuf(b.batchSize)

	cancelCtx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		b.run(cancelCtx)
		close(done)
	}()

	closer := func() {
		cancelFunc()
		if b.ticker != nil {
			b.ticker.Stop()
		}
		<-done
	}

	return b, closer, nil
}

// Add adds a stat to the batcher for the given workspace and agent.
func (b *Batcher) Add(
	now time.Time,
	agentID uuid.UUID,
	templateID uuid.UUID,
	userID uuid.UUID,
	workspaceID uuid.UUID,
	st *agentproto.Stats,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now = dbtime.Time(now)

	b.buf.ID = append(b.buf.ID, uuid.New())
	b.buf.CreatedAt = append(b.buf.CreatedAt, now)
	b.buf.AgentID = append(b.buf.AgentID, agentID)
	b.buf.UserID = append(b.buf.UserID, userID)
	b.buf.TemplateID = append(b.buf.TemplateID, templateID)
	b.buf.WorkspaceID = append(b.buf.WorkspaceID, workspaceID)

	// Store the connections by proto separately as it's a jsonb field. We marshal on flush.
	// b.buf.ConnectionsByProto = append(b.buf.ConnectionsByProto, st.ConnectionsByProto)
	b.connectionsByProto = append(b.connectionsByProto, st.ConnectionsByProto)

	b.buf.ConnectionCount = append(b.buf.ConnectionCount, st.ConnectionCount)
	b.buf.RxPackets = append(b.buf.RxPackets, st.RxPackets)
	b.buf.RxBytes = append(b.buf.RxBytes, st.RxBytes)
	b.buf.TxPackets = append(b.buf.TxPackets, st.TxPackets)
	b.buf.TxBytes = append(b.buf.TxBytes, st.TxBytes)
	b.buf.SessionCountVSCode = append(b.buf.SessionCountVSCode, st.SessionCountVscode)
	b.buf.SessionCountJetBrains = append(b.buf.SessionCountJetBrains, st.SessionCountJetbrains)
	b.buf.SessionCountReconnectingPTY = append(b.buf.SessionCountReconnectingPTY, st.SessionCountReconnectingPty)
	b.buf.SessionCountSSH = append(b.buf.SessionCountSSH, st.SessionCountSsh)
	b.buf.ConnectionMedianLatencyMS = append(b.buf.ConnectionMedianLatencyMS, st.ConnectionMedianLatencyMs)

	// If the buffer is over 80% full, signal the flusher to flush immediately.
	// We want to trigger flushes early to reduce the likelihood of
	// accidentally growing the buffer over batchSize.
	filled := float64(len(b.buf.ID)) / float64(b.batchSize)
	if filled >= 0.8 && !b.flushForced.Load() {
		b.flushLever <- struct{}{}
		b.flushForced.Store(true)
	}
	return nil
}

// Run runs the batcher.
func (b *Batcher) run(ctx context.Context) {
	// nolint:gocritic // This is only ever used for one thing - inserting agent stats.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	for {
		select {
		case <-b.tickCh:
			b.flush(authCtx, false, "scheduled")
		case <-b.flushLever:
			// If the flush lever is depressed, flush the buffer immediately.
			b.flush(authCtx, true, "reaching capacity")
		case <-ctx.Done():
			b.log.Debug(ctx, "context done, flushing before exit")

			// We must create a new context here as the parent context is done.
			ctxTimeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel() //nolint:revive // We're returning, defer is fine.

			// nolint:gocritic // This is only ever used for one thing - inserting agent stats.
			b.flush(dbauthz.AsSystemRestricted(ctxTimeout), true, "exit")
			return
		}
	}
}

// flush flushes the batcher's buffer.
func (b *Batcher) flush(ctx context.Context, forced bool, reason string) {
	b.mu.Lock()
	b.flushForced.Store(true)
	start := time.Now()
	count := len(b.buf.ID)
	defer func() {
		b.flushForced.Store(false)
		b.mu.Unlock()
		if count > 0 {
			elapsed := time.Since(start)
			b.log.Debug(ctx, "flush complete",
				slog.F("count", count),
				slog.F("elapsed", elapsed),
				slog.F("forced", forced),
				slog.F("reason", reason),
			)
		}
		// Notify that a flush has completed. This only happens in tests.
		if b.flushed != nil {
			select {
			case <-ctx.Done():
				close(b.flushed)
			default:
				b.flushed <- count
			}
		}
	}()

	if len(b.buf.ID) == 0 {
		return
	}

	// marshal connections by proto
	payload, err := json.Marshal(b.connectionsByProto)
	if err != nil {
		b.log.Error(ctx, "unable to marshal agent connections by proto, dropping data", slog.Error(err))
		b.buf.ConnectionsByProto = json.RawMessage(`[]`)
	} else {
		b.buf.ConnectionsByProto = payload
	}

	err = b.store.InsertWorkspaceAgentStats(ctx, *b.buf)
	elapsed := time.Since(start)
	if err != nil {
		if database.IsQueryCanceledError(err) {
			b.log.Debug(ctx, "query canceled, skipping insert of workspace agent stats", slog.F("elapsed", elapsed))
			return
		}
		b.log.Error(ctx, "error inserting workspace agent stats", slog.Error(err), slog.F("elapsed", elapsed))
		return
	}

	b.resetBuf()
}

// initBuf resets the buffer. b MUST be locked.
func (b *Batcher) initBuf(size int) {
	b.buf = &database.InsertWorkspaceAgentStatsParams{
		ID:                          make([]uuid.UUID, 0, b.batchSize),
		CreatedAt:                   make([]time.Time, 0, b.batchSize),
		UserID:                      make([]uuid.UUID, 0, b.batchSize),
		WorkspaceID:                 make([]uuid.UUID, 0, b.batchSize),
		TemplateID:                  make([]uuid.UUID, 0, b.batchSize),
		AgentID:                     make([]uuid.UUID, 0, b.batchSize),
		ConnectionsByProto:          json.RawMessage("[]"),
		ConnectionCount:             make([]int64, 0, b.batchSize),
		RxPackets:                   make([]int64, 0, b.batchSize),
		RxBytes:                     make([]int64, 0, b.batchSize),
		TxPackets:                   make([]int64, 0, b.batchSize),
		TxBytes:                     make([]int64, 0, b.batchSize),
		SessionCountVSCode:          make([]int64, 0, b.batchSize),
		SessionCountJetBrains:       make([]int64, 0, b.batchSize),
		SessionCountReconnectingPTY: make([]int64, 0, b.batchSize),
		SessionCountSSH:             make([]int64, 0, b.batchSize),
		ConnectionMedianLatencyMS:   make([]float64, 0, b.batchSize),
	}

	b.connectionsByProto = make([]map[string]int64, 0, size)
}

func (b *Batcher) resetBuf() {
	b.buf.ID = b.buf.ID[:0]
	b.buf.CreatedAt = b.buf.CreatedAt[:0]
	b.buf.UserID = b.buf.UserID[:0]
	b.buf.WorkspaceID = b.buf.WorkspaceID[:0]
	b.buf.TemplateID = b.buf.TemplateID[:0]
	b.buf.AgentID = b.buf.AgentID[:0]
	b.buf.ConnectionsByProto = json.RawMessage(`[]`)
	b.buf.ConnectionCount = b.buf.ConnectionCount[:0]
	b.buf.RxPackets = b.buf.RxPackets[:0]
	b.buf.RxBytes = b.buf.RxBytes[:0]
	b.buf.TxPackets = b.buf.TxPackets[:0]
	b.buf.TxBytes = b.buf.TxBytes[:0]
	b.buf.SessionCountVSCode = b.buf.SessionCountVSCode[:0]
	b.buf.SessionCountJetBrains = b.buf.SessionCountJetBrains[:0]
	b.buf.SessionCountReconnectingPTY = b.buf.SessionCountReconnectingPTY[:0]
	b.buf.SessionCountSSH = b.buf.SessionCountSSH[:0]
	b.buf.ConnectionMedianLatencyMS = b.buf.ConnectionMedianLatencyMS[:0]
	b.connectionsByProto = b.connectionsByProto[:0]
}
