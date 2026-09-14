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

// ListenForWorkspaceClaims subscribes to a pubsub channel and returns a
// receive-only channel that emits claim events for the given workspace.
// The returned channel is owned by this function and is never closed,
// because pubsub.Pubsub does not guarantee that all in-flight callbacks
// have returned after unsubscribe. Call the returned cancel function to
// unsubscribe when events are no longer needed; cancel is also called
// automatically if ctx expires or is canceled.
func (p PubsubWorkspaceClaimListener) ListenForWorkspaceClaims(ctx context.Context, workspaceID uuid.UUID) (<-chan agentsdk.ReinitializationEvent, func(), error) {
	select {
	case <-ctx.Done():
		return nil, func() {}, ctx.Err()
	default:
	}

	reinitEvents := make(chan agentsdk.ReinitializationEvent, 1)

	cancelSub, err := p.ps.Subscribe(agentsdk.PrebuildClaimedChannel(workspaceID), func(inner context.Context, payload []byte) {
		var event agentsdk.ReinitializationEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			// Rolling upgrade: old publishers send the raw reason
			// string instead of JSON.
			event = agentsdk.ReinitializationEvent{
				WorkspaceID: workspaceID,
				Reason:      agentsdk.ReinitializationReason(payload),
			}
		}

		select {
		case <-ctx.Done():
		case <-inner.Done():
		case reinitEvents <- event:
		}
	})
	if err != nil {
		return nil, func() {}, xerrors.Errorf("failed to subscribe to prebuild claimed channel: %w", err)
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

	return reinitEvents, cancel, nil
}
