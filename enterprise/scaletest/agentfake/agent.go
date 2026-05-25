package agentfake

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/quartz"
)

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

func NewAgent(coderURL *url.URL, token string, logger slog.Logger, opts ...Option) *Agent {
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

	client := agentsdk.New(a.coderURL, agentsdk.WithFixedToken(a.token))
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
func (a *Agent) connectAndServe(ctx context.Context, client *agentsdk.Client) error {
	rpc, _, err := client.ConnectRPC28WithRole(ctx, "agent")
	if err != nil {
		return xerrors.Errorf("connect dRPC: %w", err)
	}
	conn := rpc.DRPCConn()
	defer func() {
		_ = conn.Close()
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
		go a.runMetadata(ctx, rpc, workspaceID, descs)
	}

	select {
	case <-ctx.Done():
		return nil
	case <-conn.Closed():
		return xerrors.New("dRPC connection closed by remote")
	}
}

// runMetadata sends synthetic values for every metadata description in the
// agent manifest, batching per-tick into a single BatchUpdateMetadata call.
//
// One goroutine per agent (not per description): a 1s ticker pulses and we
// track per-description next-due timestamps so each key reports at its own
// declared interval. The goroutine is scoped to the connection's ctx; on
// disconnect or shutdown it exits cleanly.
//
// The payload is a single fixed value, computed once: the workspace ID
// prepended to a constant padding so each metadata row in scaletest logs
// and the database is traceable back to the agent that emitted it. We
// intentionally do not vary the value per key or per tick; if a future
// scenario requires per-key/per-tick variation we can extend this then.
//
// Errors from BatchUpdateMetadata are logged and ignored. Tearing the
// connection down over a metadata RPC blip would be wasteful; real agents
// behave the same way (see agent.reportMetadata).
func (a *Agent) runMetadata(ctx context.Context, rpc proto.DRPCAgentClient28, workspaceID uuid.UUID, descs []*proto.WorkspaceAgentMetadata_Description) {
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

	ticker := a.clock.NewTicker(metadataTickInterval, "agentfake", "runMetadata")
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
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
				continue
			}
			if _, err := rpc.BatchUpdateMetadata(ctx, &proto.BatchUpdateMetadataRequest{
				Metadata: batch,
			}); err != nil && ctx.Err() == nil {
				a.logger.Debug(ctx, "batch update metadata failed",
					slog.Error(err))
			}
		}
	}
}

// Close stops the agent. Safe to call multiple times.
func (a *Agent) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}
