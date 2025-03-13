package wspubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// WorkspaceEventChannel can be used to subscribe to events for
// workspaces owned by the provided user ID.
func WorkspaceEventChannel(ownerID uuid.UUID) string {
	return fmt.Sprintf("workspace_owner:%s", ownerID)
}

func HandleWorkspaceEvent(cb func(ctx context.Context, payload WorkspaceEvent, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, WorkspaceEvent{}, xerrors.Errorf("workspace event pubsub: %w", err))
			return
		}
		var payload WorkspaceEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, WorkspaceEvent{}, xerrors.Errorf("unmarshal workspace event"))
			return
		}
		if err := payload.Validate(); err != nil {
			cb(ctx, payload, xerrors.Errorf("validate workspace event"))
			return
		}
		cb(ctx, payload, err)
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
	WorkspaceEventKindAgentConnectionUpdate WorkspaceEventKind = "agt_connection_update"
	WorkspaceEventKindAgentFirstLogs        WorkspaceEventKind = "agt_first_logs"
	WorkspaceEventKindAgentLogsOverflow     WorkspaceEventKind = "agt_logs_overflow"
	WorkspaceEventKindAgentTimeout          WorkspaceEventKind = "agt_timeout"
	WorkspaceEventKindAgentTaskUpdate       WorkspaceEventKind = "agt_task_update"
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
