package prebuilds

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func NewPubsubWorkspaceClaimPublisher(ps pubsub.Pubsub) *PubsubWorkspaceClaimPublisher {
	return &PubsubWorkspaceClaimPublisher{ps: ps}
}

type PubsubWorkspaceClaimPublisher struct {
	ps pubsub.Pubsub
}

func (p PubsubWorkspaceClaimPublisher) PublishWorkspaceClaim(claim agentsdk.ReinitializationEvent) error {
	channel := agentsdk.PrebuildClaimedChannel(claim.WorkspaceID)
	if err := p.ps.Publish(channel, []byte(claim.Reason)); err != nil {
		return xerrors.Errorf("failed to trigger prebuilt workspace agent reinitialization: %w", err)
	}
	return nil
}

func NewPubsubWorkspaceClaimListener(ps pubsub.Pubsub, logger slog.Logger) *PubsubWorkspaceClaimListener {
	return &PubsubWorkspaceClaimListener{ps: ps, logger: logger}
}

type PubsubWorkspaceClaimListener struct {
	logger slog.Logger
	ps     pubsub.Pubsub
}

// ListenForWorkspaceClaims subscribes to a pubsub channel and sends any received events on the provided chan.
// The method returns a cancel function that should be called to unsubscribe from the channel.
// The cancel function will be automatically called if ctx expires or is canceled.
// The provided reinitEvents channel is never closed by this method.
func (p PubsubWorkspaceClaimListener) ListenForWorkspaceClaims(ctx context.Context, workspaceID uuid.UUID, reinitEvents chan<- agentsdk.ReinitializationEvent) (func(), error) {
	select {
	case <-ctx.Done():
		return func() {}, ctx.Err()
	default:
	}

	cancelSub, err := p.ps.Subscribe(agentsdk.PrebuildClaimedChannel(workspaceID), func(inner context.Context, reason []byte) {
		claim := agentsdk.ReinitializationEvent{
			WorkspaceID: workspaceID,
			Reason:      agentsdk.ReinitializationReason(reason),
		}

		select {
		case <-ctx.Done():
			return
		case <-inner.Done():
			return
		case reinitEvents <- claim:
		}
	})
	if err != nil {
		return func() {}, xerrors.Errorf("failed to subscribe to prebuild claimed channel: %w", err)
	}

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			cancelSub()
		})
	}

	go func() {
		<-ctx.Done()
		cancel()
	}()

	return cancel, nil
}
