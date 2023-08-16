package batchstats

import (
	"bytes"
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
	defaultBufferSize    = 1024
	defaultFlushInterval = time.Second
)

// Batcher holds a buffer of agent stats and periodically flushes them to
// its configured store. It also updates the workspace's last used time.
type Batcher struct {
	store database.Store
	log   slog.Logger

	buf       chan database.InsertWorkspaceAgentStatParams
	batchSize int

	// tickCh is used to periodically flush the buffer.
	tickCh   <-chan time.Time
	ticker   *time.Ticker
	interval time.Duration
	// notifyFlush is used to signal the flusher to flush its buffer.
	notifyFlush chan bool
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
	// intentionally not buffered so that the select in Add() behaves as desired.
	b.notifyFlush = make(chan bool)
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

	b.buf = make(chan database.InsertWorkspaceAgentStatParams, b.batchSize)

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
	st agentsdk.Stats,
) error {
	now = database.Time(now)

	cbp, err := json.Marshal(st.ConnectionsByProto)
	if err != nil {
		b.log.Warn(context.Background(), "unable to marshal connections by proto, dropping",
			slog.F("agent_id", agentID),
			slog.F("template_id", templateID),
			slog.F("workspace_id", workspaceID),
			slog.F("user_id", userID),
			slog.Error(err),
		)
		cbp = json.RawMessage(`{}`)
	}

	params := database.InsertWorkspaceAgentStatParams{
		AgentID:     agentID,
		CreatedAt:   now,
		ID:          uuid.New(),
		UserID:      userID,
		WorkspaceID: workspaceID,
		TemplateID:  templateID,

		ConnectionsByProto:          cbp,
		ConnectionMedianLatencyMS:   st.ConnectionMedianLatencyMS,
		ConnectionCount:             st.ConnectionCount,
		RxBytes:                     st.RxBytes,
		RxPackets:                   st.RxPackets,
		TxBytes:                     st.TxBytes,
		TxPackets:                   st.TxPackets,
		SessionCountJetBrains:       st.SessionCountJetBrains,
		SessionCountReconnectingPTY: st.SessionCountReconnectingPTY,
		SessionCountSSH:             st.SessionCountSSH,
		SessionCountVSCode:          st.SessionCountVSCode,
	}

	select {
	case b.buf <- params:
	default:
		return xerrors.Errorf("stats buffer full, please try again later")
	}

	// attempt to signal a flush if we're reaching capacity
	filled := float64(len(b.buf)) / float64(cap(b.buf))
	if filled >= 0.8 {
		select {
		case b.notifyFlush <- true: // force a flush
			b.log.Info(context.Background(), "forcing a flush", slog.F("len", len(b.buf)), slog.F("cap", cap(b.buf)))
		default:
			b.log.Warn(context.Background(), "flush already in progress")
		}
	}

	return nil
}

// Run runs the batcher.
func (b *Batcher) run(ctx context.Context) {
	// nolint:gocritic // This is only ever used for one thing - inserting agent stats.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-b.tickCh:
				b.notifyFlush <- false
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				b.log.Warn(authCtx, "context done, flushing before exit")
				b.flush(authCtx, true, "exit")
				return
			case forced := <-b.notifyFlush:
				reason := "scheduled"
				if forced {
					reason = "reaching capacity"
				}
				b.flush(authCtx, forced, reason)
			}
		}
	}()
	wg.Wait()
}

// flush flushes the batcher's buffer.
func (b *Batcher) flush(ctx context.Context, forced bool, reason string) {
	start := time.Now()
	// We're not going to try to flush the entire channel, just as many as exist at this point in time.
	count := len(b.buf)
	defer func() {
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

	if count == 0 {
		return // nothing to do
	}

	// We need to construct a JSON array of all the ConnectionsByProto.
	var batch database.InsertWorkspaceAgentStatsParams
	var connectionsByProtos []json.RawMessage
	for i := 0; i < count; i++ {
		params := <-b.buf
		// We massage this into a JSON array ourselves when it becomes time to flush.
		connectionsByProtos = append(connectionsByProtos, params.ConnectionsByProto)
		batch.ID = append(batch.ID, params.ID)
		batch.CreatedAt = append(batch.CreatedAt, params.CreatedAt)
		batch.UserID = append(batch.UserID, params.UserID)
		batch.WorkspaceID = append(batch.WorkspaceID, params.WorkspaceID)
		batch.TemplateID = append(batch.TemplateID, params.TemplateID)
		batch.AgentID = append(batch.AgentID, params.AgentID)
		batch.ConnectionCount = append(batch.ConnectionCount, params.ConnectionCount)
		batch.RxBytes = append(batch.RxBytes, params.RxBytes)
		batch.RxPackets = append(batch.RxPackets, params.RxPackets)
		batch.TxBytes = append(batch.TxBytes, params.TxBytes)
		batch.TxPackets = append(batch.TxPackets, params.TxPackets)
		batch.SessionCountReconnectingPTY = append(batch.SessionCountReconnectingPTY, params.SessionCountReconnectingPTY)
		batch.SessionCountSSH = append(batch.SessionCountSSH, params.SessionCountSSH)
		batch.SessionCountVSCode = append(batch.SessionCountVSCode, params.SessionCountVSCode)
		batch.SessionCountJetBrains = append(batch.SessionCountJetBrains, params.SessionCountJetBrains)
		batch.ConnectionMedianLatencyMS = append(batch.ConnectionMedianLatencyMS, params.ConnectionMedianLatencyMS)
	}

	// This field needs to be inserted as a single json.RawMessage as
	// JSONB, pq.Array, and unnest do not play nicely with each other.
	batch.ConnectionsByProto = jsonArray(connectionsByProtos)

	err := b.store.InsertWorkspaceAgentStats(ctx, batch)
	elapsed := time.Since(start)
	if err != nil {
		b.log.Error(ctx, "error inserting workspace agent stats", slog.Error(err), slog.F("elapsed", elapsed))
		return
	}
}

func jsonArray(elems []json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	_, _ = b.WriteRune('[')
	for idx, val := range elems {
		_, _ = b.Write(val)
		if idx < len(elems)-1 {
			_, _ = b.WriteRune(',')
		}
	}
	_, _ = b.WriteRune(']')
	return b.Bytes()
}
