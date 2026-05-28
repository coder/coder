package agentfake_test

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// Assert that our fake agent routine establishes the drpc connection and sets its lifecycle status to Ready.
func TestAgent_ConnectsAndReachesReady(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	agentID := uuid.New()
	manifest := agentsdk.Manifest{
		AgentID:     agentID,
		WorkspaceID: uuid.New(),
	}
	statsCh := make(chan *agentproto.Stats, 1)
	coord := tailnet.NewCoordinator(logger)
	t.Cleanup(func() { _ = coord.Close() })
	dialer := agenttest.NewClient(t, logger, agentID, manifest, statsCh, coord)
	t.Cleanup(dialer.Close)

	a := agentfake.NewAgent(nil, "", logger, agentfake.WithDialer(dialer))
	t.Cleanup(a.Close)

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	runErr := make(chan error, 1)
	go func() { runErr <- a.Run(runCtx) }()

	// The fake agent sends UpdateLifecycle(READY) once per dRPC
	// connect; agenttest records every lifecycle update.
	require.Eventually(t, func() bool {
		for _, state := range dialer.GetLifecycleStates() {
			if state == codersdk.WorkspaceAgentLifecycleReady {
				return true
			}
		}
		return false
	}, testutil.WaitShort, testutil.IntervalFast,
		"agent never reported Lifecycle=ready")

	// Cancel Run and confirm a clean exit (nil error, not ctx error).
	cancel()
	select {
	case err := <-runErr:
		require.NoError(t, err, "Agent.Run returned unexpected error")
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Agent.Run to return: %v", ctx.Err())
	}

	// Close is idempotent and safe to call after Run returns.
	a.Close()
	a.Close()
}

// Assert that, when the workspace agent manifest declares metadata
// descriptions, the fake agent sends synthetic values for each key via
// BatchUpdateMetadata. The test drives the agent against
// agent/agenttest.Client (an in-process fake of the agent-side coderd
// API) rather than a real coderd, so the only quartz mock involved is
// the agentfake clock that drives the metadata ticker.
func TestAgent_SendsMetadata(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	agentID := uuid.New()
	manifest := agentsdk.Manifest{
		AgentID:     agentID,
		WorkspaceID: uuid.New(),
		Metadata: []codersdk.WorkspaceAgentMetadataDescription{
			{Key: "01_meta", DisplayName: "Meta 01", Script: "noop", Interval: 1, Timeout: 10},
			{Key: "02_meta", DisplayName: "Meta 02", Script: "noop", Interval: 1, Timeout: 10},
		},
	}

	// statsCh and coord are required by agenttest.NewClient but
	// unused by agentfake. The dialer is the standin for the real
	// agentsdk.Client; it records every RPC the agent makes so we
	// can assert against the metadata batch directly.
	statsCh := make(chan *agentproto.Stats, 1)
	coord := tailnet.NewCoordinator(logger)
	t.Cleanup(func() { _ = coord.Close() })
	dialer := agenttest.NewClient(t, logger, agentID, manifest, statsCh, coord)
	t.Cleanup(dialer.Close)

	a := agentfake.NewAgent(nil, "", logger,
		agentfake.WithDialer(dialer),
		agentfake.WithClock(mClock),
	)
	t.Cleanup(a.Close)

	// Trap the agent's runMetadata TickerFunc registration so we know
	// the goroutine is parked on the mock clock before we Advance.
	// Otherwise Advance could race the goroutine startup and the
	// first tick would be missed.
	tickerTrap := mClock.Trap().TickerFunc("agentfake", "runMetadata")
	defer tickerTrap.Close()

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	runErr := make(chan error, 1)
	go func() { runErr <- a.Run(runCtx) }()

	tickerTrap.MustWait(ctx).Release(ctx)

	// One tick fires runMetadata's tick func, which calls
	// BatchUpdateMetadata against agenttest.Client. The fake records
	// it synchronously in-process; no pubsub, batcher, or SSE involved.
	mClock.Advance(time.Second).MustWait(ctx)

	require.Eventually(t, func() bool {
		md := dialer.GetMetadata()
		for _, key := range []string{"01_meta", "02_meta"} {
			m, ok := md[key]
			if !ok || m.Value == "" {
				return false
			}
			if _, err := base64.StdEncoding.DecodeString(m.Value); err != nil {
				return false
			}
		}
		return true
	}, testutil.WaitShort, testutil.IntervalFast)

	cancel()
	select {
	case err := <-runErr:
		require.NoError(t, err, "Agent.Run returned unexpected error")
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Agent.Run to return: %v", ctx.Err())
	}
}
