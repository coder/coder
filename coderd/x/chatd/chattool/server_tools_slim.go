//go:build slim

package chattool

import (
	"context"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// CreateWorkspaceFn creates a workspace for the given owner.
type CreateWorkspaceFn func(
	ctx context.Context,
	ownerID uuid.UUID,
	req codersdk.CreateWorkspaceRequest,
) (codersdk.Workspace, error)

// AgentConnFunc provides access to workspace agent connections.
type AgentConnFunc func(
	ctx context.Context,
	agentID uuid.UUID,
) (workspacesdk.AgentConn, func(), error)

// CreateWorkspaceOptions configures the create_workspace tool.
type CreateWorkspaceOptions struct {
	DB                             any
	OwnerID                        uuid.UUID
	OrganizationID                 uuid.UUID
	ChatID                         uuid.UUID
	CreateFn                       CreateWorkspaceFn
	AgentConnFn                    AgentConnFunc
	AgentChatRunnerEnabled         bool
	AgentInactiveDisconnectTimeout time.Duration
	WorkspaceMu                    *sync.Mutex
	OnChatUpdated                  any
	Logger                         slog.Logger
	AllowedTemplateIDs             func() map[uuid.UUID]bool
}

// ListTemplatesOptions configures the list_templates tool.
type ListTemplatesOptions struct {
	DB                 any
	OwnerID            uuid.UUID
	OrganizationID     uuid.UUID
	AllowedTemplateIDs func() map[uuid.UUID]bool
}

// ReadTemplateOptions configures the read_template tool.
type ReadTemplateOptions struct {
	DB                 any
	OwnerID            uuid.UUID
	AllowedTemplateIDs func() map[uuid.UUID]bool
}

// StartWorkspaceFn starts a workspace by creating a new build with the
// "start" transition.
type StartWorkspaceFn func(
	ctx context.Context,
	ownerID uuid.UUID,
	workspaceID uuid.UUID,
	req codersdk.CreateWorkspaceBuildRequest,
) (codersdk.WorkspaceBuild, error)

// StartWorkspaceOptions configures the start_workspace tool.
type StartWorkspaceOptions struct {
	DB                     any
	OwnerID                uuid.UUID
	ChatID                 uuid.UUID
	StartFn                StartWorkspaceFn
	AgentConnFn            AgentConnFunc
	AgentChatRunnerEnabled bool
	WorkspaceMu            *sync.Mutex
	OnChatUpdated          any
	Logger                 slog.Logger
}

type slimListTemplatesArgs struct {
	Query string `json:"query,omitempty"`
	Page  int    `json:"page,omitempty"`
}

type slimReadTemplateArgs struct {
	TemplateID string `json:"template_id"`
}

type slimCreateWorkspaceArgs struct {
	TemplateID string            `json:"template_id"`
	Name       string            `json:"name,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

func ListTemplates(ListTemplatesOptions) fantasy.AgentTool {
	return slimUnavailableTool[slimListTemplatesArgs](
		"list_templates",
		"List available workspace templates.",
	)
}

func ReadTemplate(ReadTemplateOptions) fantasy.AgentTool {
	return slimUnavailableTool[slimReadTemplateArgs](
		"read_template",
		"Get details about a workspace template, including its configurable parameters.",
	)
}

func CreateWorkspace(CreateWorkspaceOptions) fantasy.AgentTool {
	return slimUnavailableTool[slimCreateWorkspaceArgs](
		"create_workspace",
		"Create a new workspace from a template.",
	)
}

func StartWorkspace(StartWorkspaceOptions) fantasy.AgentTool {
	return slimUnavailableTool[struct{}](
		"start_workspace",
		"Start the chat's workspace if it is currently stopped.",
	)
}

func slimUnavailableTool[T any](name, description string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		name,
		description,
		func(context.Context, T, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextErrorResponse(name + " is unavailable in slim builds"), nil
		},
	)
}
