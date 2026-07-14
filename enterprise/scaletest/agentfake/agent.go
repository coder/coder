package agentfake

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

// rpcDialer is the subset of agentsdk.Client agentfake uses. Defined
// locally so tests can plug in *agent/agenttest.Client (or any other
// test double) without depending on the rest of the agentsdk.Client
// surface.
type rpcDialer interface {
	ConnectRPC29WithRole(ctx context.Context, role string) (
		proto.DRPCAgentClient29, tailnetproto.DRPCTailnetClient28, error,
	)
}

const (
	reconnectBackoff = 1 * time.Second

	// metadataTickInterval is the scheduler pulse for the per-agent metadata
	// goroutine. Per-description cadence is enforced by tracking next-due
	// timestamps; the ticker just wakes us up often enough to honor the
	// shortest interval we expect (1s).
	metadataTickInterval = 1 * time.Second

	// metadataValueBytes matches the payload size produced by the real
	// scaletest template's metadata script (`dd if=/dev/urandom bs=3072
	// count=1 | base64`), so the synthetic load shape on the wire mirrors
	// what a real agent emits.
	metadataValueBytes = 3072

	// metadataMinInterval is a floor applied to manifest-declared intervals
	// to guard against a malformed manifest pinning the goroutine.
	metadataMinInterval = 1 * time.Second
)

// Agent is a single fake agent. It owns one workspace-agent auth token and one dRPC connection to coderd.
type Agent struct {
	coderURL *url.URL
	token    string
	logger   slog.Logger
	clock    quartz.Clock
	dialer   rpcDialer // nil → built from coderURL+token in Run
	metrics  *Metrics  // nil → no metrics

	// firstConnected guards firstConnect so reconnects don't re-report.
	firstConnect   chan<- time.Duration
	firstConnected atomic.Bool

	// A zero connReportInterval or connReportDuration disables synthetic SSH
	// connection reporting.
	connReportInterval time.Duration
	connReportDuration time.Duration

	start time.Time

	cancel context.CancelFunc
}

// Option configures an Agent.
type Option func(*Agent)

// WithClock injects a clock for time-based operations. Defaults to
// quartz.NewReal(). Tests pass a *quartz.Mock to drive the metadata
// loop deterministically. The clock is per-agent so a future caller
// can give different agents slightly different cadences.
func WithClock(c quartz.Clock) Option {
	return func(a *Agent) {
		a.clock = c
	}
}

// WithDialer injects a custom RPC dialer. Defaults to a real
// agentsdk.Client built from coderURL + token. Tests use this to
// substitute *agent/agenttest.Client and avoid standing up a real
// coderd.
func WithDialer(d rpcDialer) Option {
	return func(a *Agent) {
		a.dialer = d
	}
}

// WithMetrics injects Prometheus collectors. A nil *Metrics (the
// default when this option is not used) is a valid no-op; every
// collector helper method nil-guards on the receiver.
func WithMetrics(m *Metrics) Option {
	return func(a *Agent) {
		a.metrics = m
	}
}

// WithFirstConnect sets a shared channel used by the Manager to aggregate
// time-to-first-connect across all agents without one stalled agent blocking
// the others.
func WithFirstConnect(ch chan<- time.Duration) Option {
	return func(a *Agent) {
		a.firstConnect = ch
	}
}

// WithConnectionReports enables periodic synthetic SSH connection reporting.
// A zero interval or duration disables reporting.
func WithConnectionReports(interval, duration time.Duration) Option {
	return func(a *Agent) {
		a.connReportInterval = interval
		a.connReportDuration = duration
	}
}

func NewAgent(logger slog.Logger, coderURL *url.URL, token string, opts ...Option) *Agent {
	a := &Agent{
		coderURL: coderURL,
		token:    token,
		logger:   logger,
		clock:    quartz.NewReal(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
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

	client := a.dialer
	if client == nil {
		client = agentsdk.New(a.coderURL, agentsdk.WithFixedToken(a.token))
	}
	a.start = a.clock.Now()
	for {
		if err := runCtx.Err(); err != nil {
			return nil
		}
		err := a.connectAndServe(runCtx, client)
		if err != nil && runCtx.Err() == nil {
			a.logger.Warn(runCtx, "fake agent dRPC stream ended; reconnecting",
				slog.Error(err))
		}
		timer := a.clock.NewTimer(reconnectBackoff, "agentfake", "reconnect")
		select {
		case <-runCtx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// connectAndServe opens one dRPC websocket, announces lifecycle = READY, then blocks until ctx is canceled or the
// connection is closed by either side. Returns the underlying error, if any.
//
// A child ctx (connCtx) is derived from ctx and canceled when this function
// returns. Background goroutines started for the lifetime of this single dRPC
// connection (notably runMetadata) bind to connCtx rather than ctx so that
// they exit promptly on remote-close + reconnect, instead of leaking and
// continuing to issue RPCs against an already-closed rpc handle until the
// outer ctx (the whole Agent's lifetime) eventually cancels.
func (a *Agent) connectAndServe(ctx context.Context, client rpcDialer) error {
	rpc, tailnetClient, err := client.ConnectRPC29WithRole(ctx, "agent")
	if err != nil {
		return xerrors.Errorf("connect dRPC: %w", err)
	}
	connCtx, cancelConn := context.WithCancel(ctx)
	defer cancelConn()
	conn := rpc.DRPCConn()
	a.metrics.incConnected()
	// Non-blocking so a slow collector can never stall this agent's
	// reconnect loop.
	if a.firstConnect != nil && a.firstConnected.CompareAndSwap(false, true) {
		select {
		case a.firstConnect <- a.clock.Since(a.start):
		default:
		}
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

	// fatalErr collects errors from routines that must reconnect the dRPC
	// connection on failure, matching the real agent's managed subroutines
	// (metadata and the derp subscriber). runConnectionReports is excluded: the
	// real agent swallows connection-report failures (see
	// agent.reportConnectionsLoop), so a blip there must not reconnect.
	fatalErr := make(chan error, 2)

	// Fetch the agent manifest so we know which metadata descriptions the
	// template declared. We synthesize values for each declared key at the
	// declared interval. Failure here is non-fatal: a manifest fetch
	// hiccup shouldn't tear the connection down, we just skip metadata
	// for this session and let the next reconnect retry.
	manifest, err := rpc.GetManifest(ctx, &proto.GetManifestRequest{})
	if err != nil {
		if ctx.Err() == nil {
			a.logger.Warn(ctx, "get manifest for metadata", slog.Error(err))
		}
	} else if descs := manifest.GetMetadata(); len(descs) > 0 {
		// Parse the workspace ID out of the manifest so we can embed it
		// in the synthetic metadata payload below. If the manifest bytes
		// are malformed (shouldn't happen in practice), fall back to
		// uuid.Nil; the payload is still valid, just less identifiable.
		workspaceID, idErr := uuid.FromBytes(manifest.GetWorkspaceId())
		if idErr != nil && ctx.Err() == nil {
			a.logger.Warn(ctx, "parse workspace id from manifest; metadata payload will use uuid.Nil",
				slog.Error(idErr))
			workspaceID = uuid.Nil
		}
		go func() { fatalErr <- a.runMetadata(connCtx, rpc, workspaceID, descs) }()
	}

	// Connection reporting is non-fatal, so it isn't wired into fatalErr. Bound
	// to connCtx so it exits on reconnect, like the fatal routines.
	go a.runConnectionReports(connCtx, rpc)

	go func() { fatalErr <- a.runDERPMapSubscriber(connCtx, tailnetClient) }()

	select {
	case <-ctx.Done():
		return nil
	case <-conn.Closed():
		return xerrors.New("dRPC connection closed by remote")
	case err := <-fatalErr:
		// A non-nil error reconnects; nil is a clean shutdown. The routines
		// wrap their own errors, so return as-is.
		return err
	}
}

// runDERPMapSubscriber drains the DERP map stream, discarding each map since
// fake agents have no tailnet.Conn. It returns the stream error so
// connectAndServe can reconnect, re-creating coderd's send goroutine; ctx
// cancellation (shutdown/reconnect) returns nil to avoid churn.
func (a *Agent) runDERPMapSubscriber(ctx context.Context, tailnetClient tailnetproto.DRPCTailnetClient28) error {
	stream, err := tailnetClient.StreamDERPMaps(ctx, &tailnetproto.StreamDERPMapsRequest{})
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return xerrors.Errorf("open derp map stream: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()
	for {
		dmp, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return xerrors.Errorf("recv derp map: %w", err)
		}
		_ = tailnet.DERPMapFromProto(dmp)
		a.metrics.incDERPMapsReceived()
	}
}

// runMetadata batches synthetic values for every manifest metadata description
// into one BatchUpdateMetadata call per tick (a 1s ticker tracks per-key
// next-due timestamps so each reports at its declared interval). The payload is
// a fixed value computed once, the workspace ID plus padding, so rows are
// traceable to the emitting agent. It returns a BatchUpdateMetadata error so
// connectAndServe reconnects, matching the real agent (see
// agent.reportMetadata); ctx cancellation returns nil to avoid churn.
func (a *Agent) runMetadata(ctx context.Context, rpc proto.DRPCAgentClient29, workspaceID uuid.UUID, descs []*proto.WorkspaceAgentMetadata_Description) error {
	// Resolve declared intervals once, applying a floor so a malformed
	// manifest can't spin us. Initialize all keys as immediately due so
	// the first tick fires every description.
	intervals := make([]time.Duration, len(descs))
	nextDue := make([]time.Time, len(descs))
	now := a.clock.Now()
	for i, d := range descs {
		// The Interval field on the proto is a durationpb.Duration but
		// carries the raw int64 seconds value cast through time.Duration
		// (see coderd/agentapi/manifest.go and agent/agent.go). Mirror the
		// same recovery the real agent does so manifest-declared intervals
		// of e.g. 10s are honored as 10s, not 10ns.
		intervalSeconds := int64(d.GetInterval().AsDuration())
		interval := time.Duration(intervalSeconds) * time.Second
		if interval < metadataMinInterval {
			interval = metadataMinInterval
		}
		intervals[i] = interval
		nextDue[i] = now
	}

	// Build the metadata payload once: prepend the workspace ID so
	// scaletest log lines and DB rows are traceable back to the
	// emitting agent, then pad out to metadataValueBytes so the wire
	// shape (base64-encoded ~4096 chars) mirrors the real scaletest
	// template's `dd if=/dev/urandom bs=3072 count=1 | base64` output.
	// coderd truncates the stored value to 2048 chars (see
	// coderd/agentapi/metadata.go maxValueLen), and the workspace ID
	// lives in the first ~50 chars of the base64 output, so it
	// survives truncation.
	const tag = "fake-agent-metadata workspace="
	prefix := tag + workspaceID.String() + " "
	padLen := metadataValueBytes - len(prefix)
	if padLen < 0 {
		padLen = 0
	}
	value := base64.StdEncoding.EncodeToString([]byte(prefix + strings.Repeat("a", padLen)))

	// TickerFunc ticks until ctx is done or the func errors; Wait
	// returns that error. A BatchUpdateMetadata failure propagates so
	// the connection reconnects, while ctx cancellation returns nil.
	err := a.clock.TickerFunc(ctx, metadataTickInterval, func() error {
		now := a.clock.Now()
		var batch []*proto.Metadata
		for i, d := range descs {
			if now.Before(nextDue[i]) {
				continue
			}
			batch = append(batch, &proto.Metadata{
				Key: d.GetKey(),
				Result: &proto.WorkspaceAgentMetadata_Result{
					CollectedAt: timestamppb.New(now),
					Value:       value,
				},
			})
			nextDue[i] = now.Add(intervals[i])
		}
		if len(batch) == 0 {
			return nil
		}
		if _, err := rpc.BatchUpdateMetadata(ctx, &proto.BatchUpdateMetadataRequest{
			Metadata: batch,
		}); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return xerrors.Errorf("batch update metadata: %w", err)
		}
		return nil
	}, "agentfake", "runMetadata").Wait()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// runConnectionReports emits periodic synthetic SSH sessions (CONNECT then
// DISCONNECT) via ReportConnection. Each session reuses one connection_id so
// coderd pairs the two halves onto a single connection_log row.
func (a *Agent) runConnectionReports(ctx context.Context, rpc proto.DRPCAgentClient29) {
	// A zero-length session is meaningless, so a zero interval or duration
	// disables reporting entirely.
	if a.connReportInterval <= 0 || a.connReportDuration <= 0 {
		return
	}

	// Tick at the smaller of the two so neither boundary is overshot.
	tick := min(a.connReportInterval, a.connReportDuration)

	var (
		openID   uuid.UUID
		closeAt  time.Time
		nextOpen = a.clock.Now().Add(a.connReportInterval)
	)
	_ = a.clock.TickerFunc(ctx, tick, func() error {
		now := a.clock.Now()
		switch {
		case openID != uuid.Nil && !now.Before(closeAt):
			// A failed DISCONNECT send is non-fatal for scaletesting, so we
			// ignore the result and always reset the session.
			a.sendConnection(ctx, rpc, openID, proto.Connection_DISCONNECT, now)
			openID = uuid.Nil
			nextOpen = now.Add(a.connReportInterval)
		case openID == uuid.Nil && !now.Before(nextOpen):
			id := uuid.New()
			closeAt = now.Add(a.connReportDuration)
			if a.sendConnection(ctx, rpc, id, proto.Connection_CONNECT, now) {
				openID = id
			} else {
				// Leave openID nil so a failed CONNECT retries next interval
				// instead of desyncing the connect/disconnect pairing.
				nextOpen = now.Add(a.connReportInterval)
			}
		}
		return nil
	}, "agentfake", "connectionReports").Wait()
}

func (a *Agent) sendConnection(ctx context.Context, rpc proto.DRPCAgentClient29, id uuid.UUID, action proto.Connection_Action, now time.Time) bool {
	_, err := rpc.ReportConnection(ctx, &proto.ReportConnectionRequest{
		Connection: &proto.Connection{
			Id:        id[:],
			Action:    action,
			Type:      proto.Connection_SSH,
			Timestamp: timestamppb.New(now),
			Ip:        "127.0.0.1",
		},
	})
	if err != nil && ctx.Err() == nil {
		a.logger.Debug(ctx, "report connection failed",
			slog.F("action", action.String()),
			slog.Error(err))
		return false
	}
	return true
}

// Close stops the agent. Safe to call multiple times.
func (a *Agent) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}
