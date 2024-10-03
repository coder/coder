package wspubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// WorkspaceEventChannel can be used to subscribe to events for
// workspaces owned by the provided user ID.
func WorkspaceEventChannel(ownerID uuid.UUID) string {
	return fmt.Sprintf("workspace_owner:%s", ownerID)
}

func HandleWorkspaceEvent(cb func(ctx context.Context, payload WorkspaceEvent)) func(ctx context.Context, message []byte) {
	return func(ctx context.Context, message []byte) {
		var payload WorkspaceEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			return
		}
		cb(ctx, payload)
	}
}

type WorkspaceEvent struct {
	Kind        WorkspaceEventKind `json:"kind"`
	WorkspaceID uuid.UUID          `json:"workspace_id" format:"uuid"`

	// WorkspaceName is only set for WorkspaceEventTypeStateChange
	WorkspaceName *string `json:"workspace_name"`
	// Transition is only set for WorkspaceEventTypeStateChange
	Transition *database.WorkspaceTransition `json:"transition,omitempty"`
	// JobStatus is only set for WorkspaceEventTypeStateChange
	JobStatus *database.ProvisionerJobStatus `json:"job_status,omitempty"`
	// AgentID is only set for WorkspaceEventKindAgentUpdate
	AgentID *uuid.UUID `json:"agent_id,omitempty" format:"uuid"`
	// AgentName is only set for WorkspaceEventKindAgentUpdate
	AgentName *string `json:"agent_name,omitempty"`
}

type WorkspaceEventKind string

const (
	WorkspaceEventKindStateChange    WorkspaceEventKind = "upd_workspace"
	WorkspaceEventKindUpdatedStats   WorkspaceEventKind = "upd_stats"
	WorkspaceEventKindLogs           WorkspaceEventKind = "new_logs"
	WorkspaceEventKindMetadataUpdate WorkspaceEventKind = "mtd_update"
	WorkspaceEventKindAgentUpdate    WorkspaceEventKind = "agt_update"
	WorkspaceEventKindAgentTimeout   WorkspaceEventKind = "agt_timeout"
)

func (w *WorkspaceEvent) UnmarshalJSON(data []byte) error {
	type AliasedEvent WorkspaceEvent
	var w2 AliasedEvent
	err := json.Unmarshal(data, &w2)
	if err != nil {
		return err
	}
	if w2.WorkspaceID == uuid.Nil {
		return xerrors.New("workspaceID must be set")
	}
	if w2.Kind == "" {
		return xerrors.New("kind must be set")
	}
	if w2.Kind == WorkspaceEventKindStateChange {
		if w2.WorkspaceName == nil {
			return xerrors.New("workspaceName must be set for WorkspaceEventTypeStateChange")
		}
		if w2.Transition == nil {
			return xerrors.New("transition must be set for WorkspaceEventTypeStateChange")
		}
		if w2.JobStatus == nil {
			return xerrors.New("jobStatus must be set for WorkspaceEventTypeStateChange")
		}
	}
	if w2.Kind == WorkspaceEventKindAgentUpdate {
		if w2.AgentID == nil {
			return xerrors.New("agentID must be set for Agent events")
		}
		if w2.AgentName == nil {
			return xerrors.New("agentName must be set for Agent events")
		}
	}
	*w = WorkspaceEvent(w2)
	return nil
}
