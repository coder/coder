package chattool

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// ExternalAgentResourceType is the Terraform resource type for externally
// managed agents.
const ExternalAgentResourceType = "coder_external_agent"

const createWorkspaceExternalAgentMessage = "create_workspace cannot create workspaces from templates with externally managed agents. " +
	"Use list_templates to choose a different template, or if the user wants " +
	"to use an external workspace, they should create it and start it up fully " +
	"themselves first, then attach it to this chat"

const externalAgentNotConnectedMessage = "workspace uses an externally managed agent that has not connected yet. " +
	"The user needs to start the workspace externally and make sure the " +
	"external agent is connected, then try again"

const externalAgentDisconnectedMessage = "workspace uses an externally managed agent that is currently offline. " +
	"The user needs to reconnect the external agent on its host, then try again"

// ExternalAgentUnavailableMessage explains how to make an externally managed
// agent usable based on its connection history.
func ExternalAgentUnavailableMessage(agent database.WorkspaceAgent) string {
	if agent.FirstConnectedAt.Valid {
		return externalAgentDisconnectedMessage
	}
	return externalAgentNotConnectedMessage
}

// IsExternalWorkspaceAgent reports whether agent belongs to an external
// resource.
func IsExternalWorkspaceAgent(ctx context.Context, db database.Store, agent database.WorkspaceAgent) (bool, error) {
	if db == nil || agent.ResourceID == uuid.Nil {
		return false, nil
	}
	resource, err := db.GetWorkspaceResourceByID(ctx, agent.ResourceID)
	if err != nil {
		return false, err
	}
	return resource.Type == ExternalAgentResourceType, nil
}
