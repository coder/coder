package prebuilds

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
	payload, err := json.Marshal(claim)
	if err != nil {
		return xerrors.Errorf("marshal claim event: %w", err)
	}
	if err := p.ps.Publish(channel, payload); err != nil {
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

// ListenForWorkspaceClaims subscribes to a pubsub channel and sends any received events on the chan that it returns.
// pubsub.Pubsub does not communicate when its last callback has been called after it has been closed. As such the chan
// returned by this method is never closed. Call the returned cancel() function to close the subscription when it is no longer needed.
// cancel() will be called if ctx expires or is canceled.
func (p PubsubWorkspaceClaimListener) ListenForWorkspaceClaims(ctx context.Context, workspaceID uuid.UUID, reinitEvents chan<- agentsdk.ReinitializationEvent) (func(), error) {
	select {
	case <-ctx.Done():
		return func() {}, ctx.Err()
	default:
	}

	cancelSub, err := p.ps.Subscribe(agentsdk.PrebuildClaimedChannel(workspaceID), func(inner context.Context, payload []byte) {
		var event agentsdk.ReinitializationEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			// Backward compatibility: treat payload as raw reason
			// string from publishers that predate JSON encoding.
			event = agentsdk.ReinitializationEvent{
				WorkspaceID: workspaceID,
				Reason:      agentsdk.ReinitializationReason(payload),
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-inner.Done():
			return
		case reinitEvents <- event:
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
