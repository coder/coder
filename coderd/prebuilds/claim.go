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
	if err := p.ps.Publish(channel, []byte(claim.UserID.String())); err != nil {
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

func (p PubsubWorkspaceClaimListener) ListenForWorkspaceClaims(ctx context.Context, workspaceID uuid.UUID) (func(), <-chan agentsdk.ReinitializationEvent, error) {
	select {
	case <-ctx.Done():
		return func() {}, nil, ctx.Err()
	default:
	}

	workspaceClaims := make(chan agentsdk.ReinitializationEvent, 1)
	cancelSub, err := p.ps.Subscribe(agentsdk.PrebuildClaimedChannel(workspaceID), func(inner context.Context, id []byte) {
		claimantID, err := uuid.ParseBytes(id)
		if err != nil {
			p.logger.Error(ctx, "invalid prebuild claimed channel payload", slog.F("input", string(id)))
			return
		}
		claim := agentsdk.ReinitializationEvent{
			UserID:      claimantID,
			WorkspaceID: workspaceID,
			Reason:      agentsdk.ReinitializeReasonPrebuildClaimed,
		}
		select {
		case <-ctx.Done():
			return
		case <-inner.Done():
			return
		case workspaceClaims <- claim:
		default:
		}
	})

	if err != nil {
		close(workspaceClaims)
		return func() {}, nil, xerrors.Errorf("failed to subscribe to prebuild claimed channel: %w", err)
	}

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			cancelSub()
			close(workspaceClaims)
		})
	}

	go func() {
		<-ctx.Done()
		cancel()
	}()

	return cancel, workspaceClaims, nil
}
