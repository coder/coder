package wspubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"cdr.dev/slog"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// WorkspaceEventChannel can be used to subscribe to events for
// workspaces owned by the provided user ID.
func WorkspaceEventChannel(ownerID uuid.UUID) string {
	return fmt.Sprintf("workspace_owner:%s", ownerID)
}

func HandleWorkspaceEvent(logger slog.Logger, cb func(ctx context.Context, payload WorkspaceEvent)) func(ctx context.Context, message []byte) {
	return func(ctx context.Context, message []byte) {
		var payload WorkspaceEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			logger.Warn(ctx, "failed to unmarshal workspace event", slog.Error(err))
			return
		}
		if err := payload.Validate(); err != nil {
			logger.Warn(ctx, "invalid workspace event", slog.Error(err))
			return
		}
		cb(ctx, payload)
	}
}

type WorkspaceEvent struct {
	Kind        WorkspaceEventKind `json:"kind"`
	WorkspaceID uuid.UUID          `json:"workspace_id" format:"uuid"`
	// AgentID is only set for WorkspaceEventKindAgent* events
	// (excluding AgentTimeout)
	AgentID *uuid.UUID `json:"agent_id,omitempty" format:"uuid"`
}

type WorkspaceEventKind string

const (
	WorkspaceEventKindStateChange     WorkspaceEventKind = "state_change"
	WorkspaceEventKindStatsUpdate     WorkspaceEventKind = "stats_update"
	WorkspaceEventKindMetadataUpdate  WorkspaceEventKind = "mtd_update"
	WorkspaceEventKindAppHealthUpdate WorkspaceEventKind = "app_health"

	WorkspaceEventKindAgentLifecycleUpdate  WorkspaceEventKind = "agt_lifecycle_update"
	WorkspaceEventKindAgentLogsUpdate       WorkspaceEventKind = "agt_logs_update"
	WorkspaceEventKindAgentConnectionUpdate WorkspaceEventKind = "agt_connection_update"
	WorkspaceEventKindAgentLogsOverflow     WorkspaceEventKind = "agt_logs_overflow"
	WorkspaceEventKindAgentTimeout          WorkspaceEventKind = "agt_timeout"
)

func (w *WorkspaceEvent) Validate() error {
	if w.WorkspaceID == uuid.Nil {
		return xerrors.New("workspaceID must be set")
	}
	if w.Kind == "" {
		return xerrors.New("kind must be set")
	}
	if w.Kind == WorkspaceEventKindAgentLifecycleUpdate && w.AgentID == nil {
		return xerrors.New("agentID must be set for Agent events")
	}
	return nil
}
