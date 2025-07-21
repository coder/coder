package agentapi

import (
	"context"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type PrebuildClaimAPI struct {
	AgentFn  func(context.Context) (database.WorkspaceAgent, error)
	Database database.Store
	Log      slog.Logger
	Pubsub   pubsub.Pubsub
}

func (a *PrebuildClaimAPI) StreamPrebuildStatus(req *agentproto.StreamPrebuildStatusRequest, stream agentproto.DRPCAgent_StreamPrebuildStatusStream) error {
	workspaceAgent, err := a.AgentFn(stream.Context())
	if err != nil {
		return xerrors.Errorf("get workspace agent: %w", err)
	}

	workspace, err := a.Database.GetWorkspaceByAgentID(stream.Context(), workspaceAgent.ID)
	if err != nil {
		return xerrors.Errorf("get workspace by agent ID: %w", err)
	}

	a.Log.Info(stream.Context(), "agent streaming prebuild status",
		slog.F("workspace_id", workspace.ID),
		slog.F("agent_id", workspaceAgent.ID),
	)

	// Determine initial prebuild status
	initialStatus := a.determinePrebuildStatus(workspace)

	// Send initial status
	err = stream.Send(&agentproto.StreamPrebuildStatusResponse{
		Status:    initialStatus,
		UpdatedAt: timestamppb.New(workspace.UpdatedAt),
	})
	if err != nil {
		return xerrors.Errorf("send initial prebuild status: %w", err)
	}

	// Create a channel to receive workspace claim events
	reinitEvents := make(chan agentsdk.ReinitializationEvent)

	// Subscribe to workspace claim events
	cancel, err := a.subscribeToWorkspaceUpdates(stream.Context(), workspace.ID, reinitEvents)
	if err != nil {
		return xerrors.Errorf("subscribe to workspace updates: %w", err)
	}
	defer cancel()

	// Stream prebuild status updates
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-reinitEvents:
			// Re-fetch the workspace to get the latest state
			updatedWorkspace, err := a.Database.GetWorkspaceByAgentID(stream.Context(), workspaceAgent.ID)
			if err != nil {
				return xerrors.Errorf("get updated workspace: %w", err)
			}

			status := a.determinePrebuildStatus(updatedWorkspace)

			err = stream.Send(&agentproto.StreamPrebuildStatusResponse{
				Status:    status,
				UpdatedAt: timestamppb.New(updatedWorkspace.UpdatedAt),
			})
			if err != nil {
				return xerrors.Errorf("send prebuild status update: %w", err)
			}
		}
	}
}

// subscribeToWorkspaceUpdates subscribes to workspace claim events using the pubsub system
func (a *PrebuildClaimAPI) subscribeToWorkspaceUpdates(ctx context.Context, workspaceID uuid.UUID, reinitEvents chan<- agentsdk.ReinitializationEvent) (func(), error) {
	cancelSub, err := a.Pubsub.Subscribe(agentsdk.PrebuildClaimedChannel(workspaceID), func(inner context.Context, reason []byte) {
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

	return cancelSub, nil
}

// determinePrebuildStatus determines the current prebuild status of a workspace based on its properties
func (a *PrebuildClaimAPI) determinePrebuildStatus(workspace database.Workspace) agentproto.PrebuildStatus {
	// Check if this is a prebuilt workspace (owned by the prebuild system user)
	if workspace.IsPrebuild() {
		// This is an unclaimed prebuilt workspace
		return agentproto.PrebuildStatus_PREBUILD_CLAIM_STATUS_UNCLAIMED
	}

	// Check if this workspace was originally a prebuild but has been claimed
	// We can determine this by checking if the workspace has a template version
	// and is not owned by the prebuild system user
	if workspace.TemplateID != uuid.Nil {
		// This could be a claimed prebuild workspace or a normal workspace
		// For now, we'll treat it as a normal workspace since we can't easily
		// distinguish between claimed prebuilds and normal workspaces
		return agentproto.PrebuildStatus_PREBUILD_CLAIM_STATUS_NORMAL
	}

	// Fallback to normal workspace
	return agentproto.PrebuildStatus_PREBUILD_CLAIM_STATUS_NORMAL
}
