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
	"github.com/coder/coder/v2/coderd/agentapi/metadatabatcher"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// Assert that our fake agent routine establishes the drpc connection and sets its lifecycle status to Ready.
func TestAgent_ConnectsAndReachesReady(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	a := agentfake.NewAgent(client.URL, r.AgentToken, logger)
	t.Cleanup(func() { a.Close() })

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	runErr := make(chan error, 1)
	go func() {
		runErr <- a.Run(runCtx)
	}()

	coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
		WithContext(ctx).
		Wait()

	require.Eventually(t, func() bool {
		ws, err := client.Workspace(ctx, r.Workspace.ID)
		if err != nil {
			return false
		}
		for _, res := range ws.LatestBuild.Resources {
			for _, agent := range res.Agents {
				if agent.LifecycleState != codersdk.WorkspaceAgentLifecycleReady {
					return false
				}
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast,
		"agent never reached Lifecycle=ready in workspace %s", r.Workspace.ID)

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
// BatchUpdateMetadata. We verify end-to-end by subscribing to the same SSE
// watch-metadata endpoint coderd uses to surface metadata in the UI.
func TestAgent_SendsMetadata(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	// Drive both the fake agent's metadata ticker AND coderd's
	// metadatabatcher off the same mock clock so we don't wait on
	// wall-clock time for either. Without this the test takes ~5s
	// (the batcher's default scheduled flush interval); with it the
	// test bottoms out at the SSE handler's stdlib 1s ticker.
	mClock := quartz.NewMock(t)

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		Clock: mClock,
		MetadataBatcherOptions: []metadatabatcher.Option{
			metadatabatcher.WithClock(mClock),
			// Shorten the scheduled flush so we don't have to advance
			// the mock by 5s every time; 100ms is small enough to feel
			// instant but large enough that we're not racing the agent
			// tick when we advance below.
			metadatabatcher.WithInterval(100 * time.Millisecond),
		},
	})
	user := coderdtest.CreateFirstUser(t, client)

	// Declare two metadata descriptions on the workspace agent. Both at
	// interval=1 so the test budget stays small. The script value is
	// irrelevant; agentfake never runs it, it synthesizes a value
	// directly.
	descs := []*sdkproto.Agent_Metadata{
		{Key: "01_meta", DisplayName: "Meta 01", Script: "noop", Interval: 1, Timeout: 10},
		{Key: "02_meta", DisplayName: "Meta 02", Script: "noop", Interval: 1, Timeout: 10},
	}

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].Metadata = descs
		return agents
	}).Do()

	// dbfake.WorkspaceBuild drives provisionerdserver.InsertWorkspaceResource
	// under the hood, which inserts the metadata description rows for each
	// metadata block on the agent. BatchUpdateMetadata's UPDATE will match
	// the rows that path created. No manual seeding needed.
	agentID := workspaceAgentID(ctx, t, client, r.Workspace.ID)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	a := agentfake.NewAgent(client.URL, r.AgentToken, logger, agentfake.WithClock(mClock))
	t.Cleanup(func() { a.Close() })

	// Trap the agent's runMetadata TickerFunc registration so we know
	// the goroutine is parked on the mock clock before we Advance.
	// Otherwise Advance could race the goroutine startup and the first
	// tick would be missed.
	tickerTrap := mClock.Trap().TickerFunc("agentfake", "runMetadata")
	defer tickerTrap.Close()

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	runErr := make(chan error, 1)
	go func() {
		runErr <- a.Run(runCtx)
	}()

	coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).
		WithContext(ctx).
		Wait()

	// Wait for the agent to register its metadata ticker on the mock,
	// then release the trapped call so the TickerFunc proceeds.
	tickerTrap.MustWait(ctx).Release(ctx)

	// Watch metadata via SSE. This exercises the same path coderd uses to
	// surface metadata in the UI: BatchUpdate -> pubsub flush -> watcher.
	// We wait for both declared keys to receive a non-empty, validly-encoded
	// base64 value.
	watchCtx, watchCancel := context.WithCancel(ctx)
	t.Cleanup(watchCancel)
	mdChan, mdErrChan := client.WatchWorkspaceAgentMetadata(watchCtx, agentID)

	// Advance the mock clock past the agent's 1s metadata tick and
	// the batcher's 100ms scheduled flush. coderd registers other
	// tickers on this clock too (cache refresh, etc.), so we use
	// AdvanceNext in a loop to fire the soonest pending event each
	// iteration until we've covered our 2-second window. This is
	// robust regardless of which internal timers are registered.
	target := mClock.Now().Add(2 * time.Second)
	for mClock.Now().Before(target) {
		d, w := mClock.AdvanceNext()
		w.MustWait(ctx)
		if d == 0 {
			break
		}
	}

	wantKeys := map[string]bool{"01_meta": false, "02_meta": false}
waitLoop:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for metadata; remaining keys: %v (%v)", wantKeys, ctx.Err())
		case err := <-mdErrChan:
			require.NoError(t, err, "metadata watcher errored")
		case md := <-mdChan:
			for _, m := range md {
				if _, want := wantKeys[m.Description.Key]; !want {
					continue
				}
				// coderd truncates the agent-reported value to 2048 chars
				// (see coderd/agentapi/metadata.go maxValueLen). Our
				// synthetic payload is larger than that on purpose, so we
				// only check that we received a non-empty value and that
				// the surviving chars are valid base64.
				if m.Result.Value == "" {
					continue
				}
				if _, err := base64.StdEncoding.DecodeString(m.Result.Value); err != nil {
					continue
				}
				wantKeys[m.Description.Key] = true
			}
			allSeen := true
			for _, ok := range wantKeys {
				if !ok {
					allSeen = false
					break
				}
			}
			if allSeen {
				break waitLoop
			}
		}
	}
	watchCancel()

	cancel()
	select {
	case err := <-runErr:
		require.NoError(t, err, "Agent.Run returned unexpected error")
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Agent.Run to return: %v", ctx.Err())
	}
}

func workspaceAgentID(ctx context.Context, t *testing.T, client *codersdk.Client, workspaceID uuid.UUID) uuid.UUID {
	t.Helper()
	ws, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err)
	for _, res := range ws.LatestBuild.Resources {
		for _, agent := range res.Agents {
			return agent.ID
		}
	}
	t.Fatalf("no agent on workspace %s", workspaceID)
	return uuid.Nil
}
