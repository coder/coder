package prebuilds

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func PublishWorkspaceClaim(ctx context.Context, ps pubsub.Pubsub, workspaceID, userID uuid.UUID) error {
	channel := agentsdk.PrebuildClaimedChannel(workspaceID)
	if err := ps.Publish(channel, []byte(userID.String())); err != nil {
		return xerrors.Errorf("failed to trigger prebuilt workspace agent reinitialization: %w", err)
	}
	return nil
}

func ListenForWorkspaceClaims(ctx context.Context, logger slog.Logger, ps pubsub.Pubsub, workspaceID uuid.UUID) (func(), <-chan agentsdk.ReinitializationEvent, error) {
	reinitEvents := make(chan agentsdk.ReinitializationEvent, 1)
	cancelSub, err := ps.Subscribe(agentsdk.PrebuildClaimedChannel(workspaceID), func(inner context.Context, id []byte) {
		select {
		case <-ctx.Done():
			return
		case <-inner.Done():
			return
		default:
		}

		claimantID, err := uuid.ParseBytes(id)
		if err != nil {
			logger.Error(ctx, "invalid prebuild claimed channel payload", slog.F("input", string(id)))
			return
		}
		// TODO: turn this into a <- uuid.UUID
		reinitEvents <- agentsdk.ReinitializationEvent{
			Message: fmt.Sprintf("prebuild claimed by user: %s", claimantID),
			Reason:  agentsdk.ReinitializeReasonPrebuildClaimed,
		}
	})
	if err != nil {
		return func() {}, nil, xerrors.Errorf("failed to subscribe to prebuild claimed channel: %w", err)
	}
	defer cancelSub()
	return func() { cancelSub() }, reinitEvents, nil
}

func StreamAgentReinitEvents(ctx context.Context, logger slog.Logger, rw http.ResponseWriter, r *http.Request, reinitEvents <-chan agentsdk.ReinitializationEvent) {
	sseSendEvent, sseSenderClosed, err := httpapi.ServerSentEventSender(rw, r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error setting up server-sent events.",
			Detail:  err.Error(),
		})
		return
	}
	// Prevent handler from returning until the sender is closed.
	defer func() {
		<-sseSenderClosed
	}()

	// An initial ping signals to the requester that the server is now ready
	// and the client can begin servicing a channel with data.
	_ = sseSendEvent(codersdk.ServerSentEvent{
		Type: codersdk.ServerSentEventTypePing,
	})

	for {
		select {
		case <-ctx.Done():
			return
		case reinitEvent := <-reinitEvents:
			err = sseSendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeData,
				Data: reinitEvent,
			})
			if err != nil {
				logger.Warn(ctx, "failed to send SSE response to trigger reinit", slog.Error(err))
			}
		}
	}
}

type MockClaimCoordinator interface{}

type ClaimListener interface{}
type PostgresClaimListener struct{}

type AgentReinitializer interface{}
type SSEAgentReinitializer struct{}

type ClaimCoordinator interface {
	ClaimListener
	AgentReinitializer
}
