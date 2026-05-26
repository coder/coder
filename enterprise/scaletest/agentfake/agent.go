package agentfake

import (
	"context"
	"net/url"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const reconnectBackoff = 1 * time.Second

// Agent is a single fake agent. It owns one workspace-agent auth token and one dRPC connection to coderd.
type Agent struct {
	coderURL *url.URL
	token    string
	logger   slog.Logger
	metrics  *Metrics

	// firstConnectDuration receives the time-to-first-connect once.
	// Buffered so the send never blocks.
	firstConnectDuration chan time.Duration
	firstConnected       atomic.Bool

	cancel context.CancelFunc
}

// NewAgent constructs a fake agent. metrics may be nil.
func NewAgent(coderURL *url.URL, token string, logger slog.Logger, metrics *Metrics) *Agent {
	return &Agent{
		coderURL:             coderURL,
		token:                token,
		logger:               logger,
		metrics:              metrics,
		firstConnectDuration: make(chan time.Duration, 1),
	}
}

// Run opens a dRPC websocket to coderd as the "agent" role and keeps it open until ctx is canceled or Close is called.
// On transient failures (e.g., coderd restart, brief auth churn while the workspace build is finalizing) Run reconnects
// with a small backoff.
// Returns nil when ctx is canceled or Close is called, and a non-nil error only if ctx returns a non-context error.
func (a *Agent) Run(ctx context.Context) error {
	// Tie a.closed into ctx so a single select can wait on either.
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	defer a.cancel()

	client := agentsdk.New(a.coderURL, agentsdk.WithFixedToken(a.token))
	start := time.Now()
	for {
		if err := runCtx.Err(); err != nil {
			return nil
		}
		err := a.connectAndServe(runCtx, client, start)
		if err != nil && runCtx.Err() == nil {
			a.logger.Warn(runCtx, "fake agent dRPC stream ended; reconnecting",
				slog.Error(err))
		}
		select {
		case <-runCtx.Done():
			return nil
		case <-time.After(reconnectBackoff):
		}
	}
}

// connectAndServe opens one dRPC websocket, announces lifecycle = READY, then blocks until ctx is canceled or the
// connection is closed by either side. start is the time Run began, used to record time-to-first-connect.
func (a *Agent) connectAndServe(ctx context.Context, client *agentsdk.Client, start time.Time) error {
	rpc, _, err := client.ConnectRPC28WithRole(ctx, "agent")
	if err != nil {
		return xerrors.Errorf("connect dRPC: %w", err)
	}
	conn := rpc.DRPCConn()
	a.metrics.incConnected()
	// Only record the first connect; reconnects reflect failures, not startup time.
	if a.firstConnected.CompareAndSwap(false, true) {
		a.firstConnectDuration <- time.Since(start)
	}
	defer func() {
		_ = conn.Close()
		a.metrics.decConnected()
	}()

	// Real agents transition to READY once their startup script finishes. Fakes have no startup script, so they're
	// "ready" the moment the dRPC stream is open. We send this once per (re)connect because coderd's per-connection
	// lifecycle state is reset each time.
	// Failure here is logged but not treated as fatal: the connection itself is what flips Connected, and a transient
	// failure to update lifecycle shouldn't tear the whole agent down.
	if _, err := rpc.UpdateLifecycle(ctx, &proto.UpdateLifecycleRequest{
		Lifecycle: &proto.Lifecycle{
			State:     proto.Lifecycle_READY,
			ChangedAt: timestamppb.Now(),
		},
	}); err != nil && ctx.Err() == nil {
		a.logger.Warn(ctx, "failed to send lifecycle=READY",
			slog.Error(err))
	}

	select {
	case <-ctx.Done():
		return nil
	case <-conn.Closed():
		return xerrors.New("dRPC connection closed by remote")
	}
}

// Close stops the agent. Safe to call multiple times.
func (a *Agent) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}
