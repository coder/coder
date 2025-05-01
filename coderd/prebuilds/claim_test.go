package prebuilds_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPubsubWorkspaceClaimPublisher(t *testing.T) {
	t.Parallel()
	t.Run("publish claim", func(t *testing.T) {
		t.Parallel()

		ps := pubsub.NewInMemory()
		publisher := prebuilds.NewPubsubWorkspaceClaimPublisher(ps)

		workspaceID := uuid.New()
		userID := uuid.New()

		userIDCh := make(chan uuid.UUID, 1)
		channel := agentsdk.PrebuildClaimedChannel(workspaceID)
		cancel, err := ps.Subscribe(channel, func(ctx context.Context, message []byte) {
			userIDCh <- uuid.MustParse(string(message))
		})
		require.NoError(t, err)
		defer cancel()

		claim := agentsdk.ReinitializationEvent{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Reason:      agentsdk.ReinitializeReasonPrebuildClaimed,
		}
		err = publisher.PublishWorkspaceClaim(claim)
		require.NoError(t, err)

		gotUserID := testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, userIDCh)
		require.Equal(t, userID, gotUserID)
	})

	t.Run("fail to publish claim", func(t *testing.T) {
		t.Parallel()

		ps := &brokenPubsub{}

		publisher := prebuilds.NewPubsubWorkspaceClaimPublisher(ps)
		claim := agentsdk.ReinitializationEvent{
			UserID:      uuid.New(),
			WorkspaceID: uuid.New(),
			Reason:      agentsdk.ReinitializeReasonPrebuildClaimed,
		}

		err := publisher.PublishWorkspaceClaim(claim)
		require.ErrorContains(t, err, "failed to trigger prebuilt workspace reinitialization")
	})
}

func TestPubsubWorkspaceClaimListener(t *testing.T) {
	t.Parallel()
	t.Run("stops listening if context canceled", func(t *testing.T) {
		t.Parallel()

		ps := pubsub.NewInMemory()
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		ctx, cancel := context.WithCancel(context.Background())

		cancelFunc, claims, err := listener.ListenForWorkspaceClaims(ctx, uuid.New())
		require.NoError(t, err)
		defer cancelFunc()

		cancel()
		// Channel should be closed immediately due to context cancellation
		select {
		case _, ok := <-claims:
			require.False(t, ok)
		case <-time.After(testutil.WaitShort):
			t.Fatal("timeout waiting for closed channel")
		}
	})

	t.Run("stops listening if cancel func is called", func(t *testing.T) {
		t.Parallel()

		ps := pubsub.NewInMemory()
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		cancelFunc, claims, err := listener.ListenForWorkspaceClaims(context.Background(), uuid.New())
		require.NoError(t, err)

		cancelFunc()
		select {
		case _, ok := <-claims:
			require.False(t, ok)
		case <-time.After(testutil.WaitShort):
			t.Fatal("timeout waiting for closed channel")
		}
	})

	t.Run("finds claim events for its workspace", func(t *testing.T) {
		t.Parallel()

		ps := pubsub.NewInMemory()
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		workspaceID := uuid.New()
		userID := uuid.New()
		cancelFunc, claims, err := listener.ListenForWorkspaceClaims(context.Background(), workspaceID)
		require.NoError(t, err)
		defer cancelFunc()

		// Publish a claim
		channel := agentsdk.PrebuildClaimedChannel(workspaceID)
		err = ps.Publish(channel, []byte(userID.String()))
		require.NoError(t, err)

		// Verify we receive the claim
		select {
		case claim := <-claims:
			require.Equal(t, userID, claim.UserID)
			require.Equal(t, workspaceID, claim.WorkspaceID)
			require.Equal(t, agentsdk.ReinitializeReasonPrebuildClaimed, claim.Reason)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for claim")
		}
	})

	t.Run("ignores claim events for other workspaces", func(t *testing.T) {
		t.Parallel()

		ps := pubsub.NewInMemory()
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		workspaceID := uuid.New()
		otherWorkspaceID := uuid.New()
		cancelFunc, claims, err := listener.ListenForWorkspaceClaims(context.Background(), workspaceID)
		require.NoError(t, err)
		defer cancelFunc()

		// Publish a claim for a different workspace
		channel := agentsdk.PrebuildClaimedChannel(otherWorkspaceID)
		err = ps.Publish(channel, []byte(uuid.New().String()))
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

		ps := &brokenPubsub{}
		listener := prebuilds.NewPubsubWorkspaceClaimListener(ps, slogtest.Make(t, nil))

		_, _, err := listener.ListenForWorkspaceClaims(context.Background(), uuid.New())
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
