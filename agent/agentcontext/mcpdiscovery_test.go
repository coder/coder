package agentcontext_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

// TestManager_MCPDiscoveryPhaseSampledBeforeResolve verifies both resolve
// paths take exactly one report sample per pass and carry that sample's
// phase, so a settlement that lands during the filesystem walk cannot
// label the pre-settlement catalog complete. The report function flips
// itself to complete after it is sampled, mirroring an engine that settles
// mid-resolve; the published snapshot must still be pending and the next
// pass carries complete together with the servers.
func TestManager_MCPDiscoveryPhaseSampledBeforeResolve(t *testing.T) {
	t.Parallel()
	for _, entrypoint := range []string{"SetReady", "Resync"} {
		t.Run(entrypoint, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			dir := t.TempDir()
			var (
				settled atomic.Bool
				samples atomic.Int32
			)
			m := newPendingTestManager(t, agentcontext.ManagerOptions{
				WorkingDir: func() string { return dir },
				AgentRunID: "run-1",
				MCPReport: func() agentcontext.MCPReport {
					samples.Add(1)
					if settled.Swap(true) {
						return agentcontext.MCPReport{
							Phase: agentcontext.MCPDiscoveryComplete,
							Servers: []agentcontext.MCPServerStatus{{
								Name: "srv", Connected: true,
								Tools: []agentcontext.MCPTool{{Name: "echo"}},
							}},
						}
					}
					return agentcontext.MCPReport{Phase: agentcontext.MCPDiscoveryPending}
				},
			})
			if entrypoint == "Resync" {
				m.SetReady()
				settled.Store(false)
				samples.Store(0)
				_, err := m.Resync(ctx)
				require.NoError(t, err)
			} else {
				m.SetReady()
			}
			require.EqualValues(t, 1, samples.Load(), "one sample per resolve")
			snap := m.Snapshot()
			require.Equal(t, agentcontext.MCPDiscoveryPending, snap.MCPDiscovery,
				"a pre-settlement catalog must not be labeled complete")
			require.Equal(t, "run-1", snap.AgentRunID)
			require.Nil(t, findMCPServerResource(snap, "srv"))

			snap, err := m.Resync(ctx)
			require.NoError(t, err)
			require.Equal(t, agentcontext.MCPDiscoveryComplete, snap.MCPDiscovery)
			srv := findMCPServerResource(snap, "srv")
			require.NotNil(t, srv)
			require.Equal(t, "echo", srv.Tools[0].Name)
		})
	}
}

// TestManager_MCPDiscoverySettlementPublishesUnchangedResources verifies a
// zero-server settlement still produces a new, complete snapshot: the
// resources and aggregate hash are unchanged, only the phase moves, and
// the push loop delivers it.
func TestManager_MCPDiscoverySettlementPublishesUnchangedResources(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	dir := t.TempDir()
	mustWriteFile(t, dir+"/AGENTS.md", "rules")

	var mu sync.Mutex
	report := agentcontext.MCPReport{Phase: agentcontext.MCPDiscoveryPending}
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
		AgentRunID: "run-1",
		MCPReport: func() agentcontext.MCPReport {
			mu.Lock()
			defer mu.Unlock()
			return report
		},
	})
	initial := m.Snapshot()
	require.Equal(t, agentcontext.MCPDiscoveryPending, initial.MCPDiscovery)

	pusher := newFakePusher()
	pushDone := make(chan error, 1)
	go func() {
		pushDone <- m.RunPush(ctx, pusher, agentcontext.PushOptions{Logger: testutil.Logger(t)})
	}()
	t.Cleanup(func() {
		require.NoError(t, m.Close())
		require.NoError(t, testutil.RequireReceive(ctx, t, pushDone))
	})
	testutil.RequireReceive(ctx, t, pusher.signal)
	first := pusher.snapshot()[0]
	require.Equal(t, agentcontext.MCPDiscoveryPending, first.MCPDiscovery)
	require.Equal(t, "run-1", first.AgentRunID)

	// The engine settles with zero servers and notifies (in production
	// through Trigger; Resync here so the test resolves inline).
	mu.Lock()
	report = agentcontext.MCPReport{Phase: agentcontext.MCPDiscoveryComplete}
	mu.Unlock()
	settledSnap, err := m.Resync(ctx)
	require.NoError(t, err)
	assert.Equal(t, agentcontext.MCPDiscoveryComplete, settledSnap.MCPDiscovery)
	assert.Equal(t, initial.AggregateHash, settledSnap.AggregateHash)
	assert.Equal(t, initial.Resources, settledSnap.Resources)
	assert.Greater(t, settledSnap.Version, initial.Version)

	require.Eventually(t, func() bool {
		requests := pusher.snapshot()
		return requests[len(requests)-1].MCPDiscovery == agentcontext.MCPDiscoveryComplete
	}, testutil.WaitShort, testutil.IntervalFast, "settlement must be pushed even with unchanged resources")
	requests := pusher.snapshot()
	last := requests[len(requests)-1]
	assert.Equal(t, first.AggregateHash, last.AggregateHash)
	assert.False(t, last.Initial)
	assert.Equal(t, "run-1", last.AgentRunID)
}

// TestManager_NoMCPReportPublishesUnspecified verifies a Manager without an
// engine publishes no completeness guarantee.
func TestManager_NoMCPReportPublishesUnspecified(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, agentcontext.ManagerOptions{WorkingDir: t.TempDir})
	require.Equal(t, agentcontext.MCPDiscoveryUnspecified, m.Snapshot().MCPDiscovery)
	require.Empty(t, m.Snapshot().AgentRunID)
}
