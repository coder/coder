package agentfake_test

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/scaletest/agentfake"
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
// BatchUpdateMetadata. We assert against the dRPC call landing at the
// fake server rather than the full coderd → batcher → pubsub → SSE
// pipeline: this is a test of what agentfake emits, not what coderd
// does with what agentfake emits.
func TestAgent_SendsMetadata(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	workspaceID := uuid.New()
	manifest := &agentproto.Manifest{
		WorkspaceId: workspaceID[:],
		Metadata: []*agentproto.WorkspaceAgentMetadata_Description{
			{Key: "01_meta", DisplayName: "Meta 01", Script: "noop", Interval: durationpb.New(time.Second), Timeout: durationpb.New(10 * time.Second)},
			{Key: "02_meta", DisplayName: "Meta 02", Script: "noop", Interval: durationpb.New(time.Second), Timeout: durationpb.New(10 * time.Second)},
		},
	}

	dialer := &fakeDialer{manifest: manifest}
	a := agentfake.NewAgent(nil, "", logger,
		agentfake.WithDialer(dialer),
		agentfake.WithClock(mClock),
	)
	t.Cleanup(a.Close)

	// Trap the agent's runMetadata TickerFunc registration so we know
	// the goroutine is parked on the mock clock before we Advance.
	tickerTrap := mClock.Trap().TickerFunc("agentfake", "runMetadata")
	defer tickerTrap.Close()

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	runErr := make(chan error, 1)
	go func() { runErr <- a.Run(runCtx) }()

	tickerTrap.MustWait(ctx).Release(ctx)

	// One tick fires runMetadata's tick func, which calls
	// BatchUpdateMetadata synchronously against the fake dialer. No
	// flush, pubsub, or SSE involved.
	mClock.Advance(time.Second).MustWait(ctx)

	require.Eventually(t, func() bool {
		md := dialer.Metadata()
		for _, key := range []string{"01_meta", "02_meta"} {
			m, ok := md[key]
			if !ok || m.GetResult().GetValue() == "" {
				return false
			}
			if _, err := base64.StdEncoding.DecodeString(m.Result.Value); err != nil {
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
