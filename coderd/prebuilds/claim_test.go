package prebuilds_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPubsubWorkspaceClaimPublisher(t *testing.T) {
	t.Parallel()
	t.Run("published claim is received by a listener for the same workspace", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := testutil.Logger(t)
		ps := pubsub.NewInMemory()
		workspaceID := uuid.New()
		reinitEvents := make(chan agentsdk.ReinitializationEvent, 1)
		publisher := prebuilds.NewPubsubWorkspaceClaimPublisher(ps)
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, logger)

		cancel, err := listener.ListenForWorkspaceClaims(ctx, workspaceID, reinitEvents)
		require.NoError(t, err)
		defer cancel()

		claim := agentsdk.ReinitializationEvent{
			WorkspaceID: workspaceID,
			Reason:      agentsdk.ReinitializeReasonPrebuildClaimed,
		}
		err = publisher.PublishWorkspaceClaim(claim)
		require.NoError(t, err)

		gotEvent := testutil.RequireReceive(ctx, t, reinitEvents)
		require.Equal(t, workspaceID, gotEvent.WorkspaceID)
		require.Equal(t, claim.Reason, gotEvent.Reason)
	})

	t.Run("fail to publish claim", func(t *testing.T) {
		t.Parallel()

		ps := &brokenPubsub{}

		publisher := prebuilds.NewPubsubWorkspaceClaimPublisher(ps)
		claim := agentsdk.ReinitializationEvent{
			WorkspaceID: uuid.New(),
			Reason:      agentsdk.ReinitializeReasonPrebuildClaimed,
		}

		err := publisher.PublishWorkspaceClaim(claim)
		require.ErrorContains(t, err, "failed to trigger prebuilt workspace agent reinitialization")
	})
}

func TestPubsubWorkspaceClaimListener(t *testing.T) {
	t.Parallel()
	t.Run("finds claim events for its workspace", func(t *testing.T) {
		t.Parallel()

		ps := pubsub.NewInMemory()
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		claims := make(chan agentsdk.ReinitializationEvent, 1) // Buffer to avoid messing with goroutines in the rest of the test

		workspaceID := uuid.New()
		cancelFunc, err := listener.ListenForWorkspaceClaims(context.Background(), workspaceID, claims)
		require.NoError(t, err)
		defer cancelFunc()

		// Publish a claim
		channel := agentsdk.PrebuildClaimedChannel(workspaceID)
		reason := agentsdk.ReinitializeReasonPrebuildClaimed
		err = ps.Publish(channel, []byte(reason))
		require.NoError(t, err)

		// Verify we receive the claim
		ctx := testutil.Context(t, testutil.WaitShort)
		claim := testutil.RequireReceive(ctx, t, claims)
		require.Equal(t, workspaceID, claim.WorkspaceID)
		require.Equal(t, reason, claim.Reason)
	})

	t.Run("ignores claim events for other workspaces", func(t *testing.T) {
		t.Parallel()

		ps := pubsub.NewInMemory()
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		claims := make(chan agentsdk.ReinitializationEvent)
		workspaceID := uuid.New()
		otherWorkspaceID := uuid.New()
		cancelFunc, err := listener.ListenForWorkspaceClaims(context.Background(), workspaceID, claims)
		require.NoError(t, err)
		defer cancelFunc()

		// Publish a claim for a different workspace
		channel := agentsdk.PrebuildClaimedChannel(otherWorkspaceID)
		err = ps.Publish(channel, []byte(agentsdk.ReinitializeReasonPrebuildClaimed))
		require.NoError(t, err)

		// Verify we don't receive the claim
		select {
		case <-claims:
			t.Fatal("received claim for wrong workspace")
		case <-time.After(100 * time.Millisecond):
			// Expected - no claim received
		}
	})

	t.Run("communicates the error if it can't subscribe", func(t *testing.T) {
		t.Parallel()

		claims := make(chan agentsdk.ReinitializationEvent)
		ps := &brokenPubsub{}
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		_, err := listener.ListenForWorkspaceClaims(context.Background(), uuid.New(), claims)
		require.ErrorContains(t, err, "failed to subscribe to prebuild claimed channel")
	})
}

type brokenPubsub struct {
	pubsub.Pubsub
}

func (brokenPubsub) Subscribe(_ string, _ pubsub.Listener) (func(), error) {
	return nil, xerrors.New("broken")
}

func (brokenPubsub) Publish(_ string, _ []byte) error {
	return xerrors.New("broken")
}
