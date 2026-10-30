package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// maxSpawnableProjectAgents bounds how many invokable agents are described
// to the model, which keeps the spawn_agent description small.
const maxSpawnableProjectAgents = 20

// spawnableProjectAgents returns the agents a root chat may invoke: the
// agents its own agent lists as invokable, in its organization, that the
// chat owner may use. It returns nil for delegated chats and chats outside
// an agent. Lookups fail open to an empty list.
func (p *Server) spawnableProjectAgents(ctx context.Context, parent database.Chat) []database.ChatProject {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return nil
	}
	if parent.ID == uuid.Nil || parent.ParentChatID.Valid || !parent.ProjectID.Valid {
		return nil
	}
	//nolint:gocritic // Chatd reads project config on behalf of the chat owner.
	chatdCtx := dbauthz.AsChatd(ctx)
	project, err := p.db.GetChatProjectByID(chatdCtx, parent.ProjectID.UUID)
	if err != nil {
		p.logger.Warn(ctx, "failed to load agent for chat",
			slog.F("chat_id", parent.ID),
			slog.F("project_id", parent.ProjectID.UUID),
			slog.Error(err),
		)
		return nil
	}
	if len(project.InvokableAgentIDs) == 0 {
		return nil
	}
	agents, err := p.db.GetChatProjectsByIDs(chatdCtx, project.InvokableAgentIDs)
	if err != nil {
		p.logger.Warn(ctx, "failed to list invokable agents for chat",
			slog.F("chat_id", parent.ID),
			slog.F("project_id", parent.ProjectID.UUID),
			slog.Error(err),
		)
		return nil
	}
	if !p.chatOwnerCanUseProject(chatdCtx, parent.ProjectID.UUID, parent.OwnerID) {
		return nil
	}
	// Invoking an agent lends its identity, so the inviting agent's owner
	// must still own the invoked agent or be able to edit it. This is
	// checked on every turn, so revoking that access stops invocation.
	// Chats owned by a person rather than the agent's user also need that
	// person to be able to use the invoked agent, because the child chat is
	// visible to them.
	ownedByAgentUser := project.AgentUserID == uuid.NullUUID{UUID: parent.OwnerID, Valid: true}
	agents = slices.DeleteFunc(agents, func(agent database.ChatProject) bool {
		if agent.ID == project.ID || agent.OrganizationID != parent.OrganizationID {
			return true
		}
		if agent.OwnerID != project.OwnerID && !p.userCanEditProject(chatdCtx, agent.ID, project.OwnerID) {
			return true
		}
		return !ownedByAgentUser && agent.OwnerID != parent.OwnerID && !p.chatOwnerCanUseProject(chatdCtx, agent.ID, parent.OwnerID)
	})
	if len(agents) > maxSpawnableProjectAgents {
		agents = agents[:maxSpawnableProjectAgents]
	}
	return agents
}

// userCanEditProject reports whether a user owns a project or may edit it.
// Lookup errors deny access.
func (p *Server) userCanEditProject(ctx context.Context, projectID, userID uuid.UUID) bool {
	//nolint:gocritic // Chatd checks project access on behalf of the agent's owner.
	ok, err := p.db.IsChatProjectEditableByUserID(dbauthz.AsChatd(ctx), database.IsChatProjectEditableByUserIDParams{
		ProjectID: projectID,
		UserID:    userID,
	})
	if err != nil {
		p.logger.Warn(ctx, "failed to check chat project edit access",
			slog.F("project_id", projectID),
			slog.F("user_id", userID),
			slog.Error(err),
		)
		return false
	}
	return ok
}

// chatOwnerCanUseProject reports whether a chat owner may still use a
// project: they own it or it is shared with them. Sharing can be revoked
// after a chat joins a project, so chatd checks before it uses project
// state such as memory and instructions. Lookup errors deny access.
func (p *Server) chatOwnerCanUseProject(ctx context.Context, projectID, ownerID uuid.UUID) bool {
	//nolint:gocritic // Chatd checks project access on behalf of the chat owner.
	ok, err := p.db.IsChatProjectAccessibleByUserID(dbauthz.AsChatd(ctx), database.IsChatProjectAccessibleByUserIDParams{
		ProjectID: projectID,
		UserID:    ownerID,
	})
	if err != nil {
		p.logger.Warn(ctx, "failed to check chat project access",
			slog.F("project_id", projectID),
			slog.F("owner_id", ownerID),
			slog.Error(err),
		)
		return false
	}
	return ok
}

// resolveSpawnableProjectAgent looks up an agent by ID among the agents the
// parent may invoke.
func (p *Server) resolveSpawnableProjectAgent(ctx context.Context, parent database.Chat, rawID string) (database.ChatProject, error) {
	id, err := uuid.Parse(strings.TrimSpace(rawID))
	if err != nil {
		return database.ChatProject{}, xerrors.Errorf("invalid agent_id: %w", err)
	}
	for _, agent := range p.spawnableProjectAgents(ctx, parent) {
		if agent.ID == id {
			return agent, nil
		}
	}
	return database.ChatProject{}, xerrors.Errorf("agent_id %s is not an agent this chat may invoke", id)
}

// projectAgentMCPServerIDs returns the invoked agent's MCP servers that are
// still enabled and readable by the child chat's owner, the agent's user,
// plus any Force On servers. Anyone who can edit the invoked agent can change
// its list, so the owner's access is checked rather than trusted.
func (p *Server) projectAgentMCPServerIDs(ctx context.Context, organizationID, ownerID uuid.UUID, agent database.ChatProject) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	if len(agent.MCPServerIDs) > 0 {
		owner, _, err := httpmw.UserRBACSubject(ctx, p.db, ownerID, rbac.ScopeAll)
		if err != nil {
			return nil, xerrors.Errorf("load chat owner authorization: %w", err)
		}
		configs, err := enabledMCPServerConfigsForChatOrg(dbauthz.As(ctx, owner), p.db, organizationID, agent.MCPServerIDs)
		if err != nil {
			return nil, err
		}
		for _, cfg := range configs {
			ids = append(ids, cfg.ID)
		}
	}
	return enforceForcedMCPServerIDs(ctx, p.db, organizationID, ownerID, ids)
}

// projectAgentChildOwner returns the user a chat delegated to an invoked
// agent runs as: the agent's own user. That user must be able to use the
// chat's model, since the child runs on the parent's model.
func (p *Server) projectAgentChildOwner(ctx context.Context, agent database.ChatProject, modelConfigID uuid.UUID) (uuid.UUID, error) {
	if !agent.AgentUserID.Valid {
		return uuid.Nil, xerrors.Errorf("agent %q is not ready to run chats yet; try again shortly", agent.Name)
	}
	ownerCtx, err := p.callerModelConfigContext(ctx, agent.AgentUserID.UUID)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := p.db.GetChatModelConfigByID(ownerCtx, modelConfigID); err != nil {
		if dbauthz.IsNotAuthorizedError(err) || errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, xerrors.Errorf("agent %q cannot use this chat's model", agent.Name)
		}
		return uuid.Nil, xerrors.Errorf("check agent model access: %w", err)
	}
	return agent.AgentUserID.UUID, nil
}

// projectAgentModelConfigID returns the model an invoked agent is set to
// use when the agent's user can use it. A model that was deleted or
// disabled, or whose provider has no credentials for the agent, is skipped
// so the child falls back to the usual model.
func (p *Server) projectAgentModelConfigID(ctx context.Context, agent database.ChatProject) (uuid.UUID, bool) {
	if !agent.ModelConfigID.Valid || !agent.AgentUserID.Valid {
		return uuid.Nil, false
	}
	agentUserID := agent.AgentUserID.UUID
	modelConfig, providerName, err := p.resolveModelConfigForOrganization(ctx, agentUserID, agent.OrganizationID, agent.ModelConfigID.UUID)
	if err != nil {
		p.logger.Warn(ctx, "invoked agent's model is unavailable, using the default model",
			slog.F("project_id", agent.ID),
			slog.F("model_config_id", agent.ModelConfigID.UUID),
			slog.Error(err),
		)
		return uuid.Nil, false
	}
	//nolint:gocritic // Provider credentials remain privileged Chatd reads.
	providerKeys, err := p.resolveUserProviderAPIKeys(dbauthz.AsChatd(ctx), agentUserID, modelConfigAIProviderID(modelConfig))
	if err != nil {
		p.logger.Warn(ctx, "resolve provider API keys for invoked agent's model",
			slog.F("project_id", agent.ID),
			slog.Error(err),
		)
		return uuid.Nil, false
	}
	if !userCanUseProviderKeys(providerKeys, providerName) {
		p.logger.Warn(ctx, "invoked agent's model has no usable credentials, using the default model",
			slog.F("project_id", agent.ID),
			slog.F("model_config_id", modelConfig.ID),
			slog.F("provider", providerName),
		)
		return uuid.Nil, false
	}
	return modelConfig.ID, true
}

// chatTemplatePolicy returns the template restrictions of the agent a chat
// runs in. Delegated chats without their own agent use the root chat's.
// Lookups fail closed to a policy that allows no template, so a broken
// lookup cannot widen access.
func (p *Server) chatTemplatePolicy(ctx context.Context, chat database.Chat) chattool.TemplatePolicy {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return chattool.TemplatePolicy{}
	}
	//nolint:gocritic // Chatd reads project config on behalf of the chat owner.
	chatdCtx := dbauthz.AsChatd(ctx)
	projectID := chat.ProjectID
	if !projectID.Valid && chat.RootChatID.Valid && chat.RootChatID.UUID != chat.ID {
		root, err := p.db.GetChatByID(chatdCtx, chat.RootChatID.UUID)
		if err != nil {
			p.logger.Warn(ctx, "failed to load root chat for template policy",
				slog.F("chat_id", chat.ID), slog.Error(err))
			return denyAllTemplatePolicy
		}
		projectID = root.ProjectID
	}
	if !projectID.Valid {
		return chattool.TemplatePolicy{}
	}
	project, err := p.db.GetChatProjectByID(chatdCtx, projectID.UUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return chattool.TemplatePolicy{}
		}
		p.logger.Warn(ctx, "failed to load chat project for template policy",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return denyAllTemplatePolicy
	}
	return chattool.TemplatePolicy{
		AllowedTemplateIDs:             project.AllowedTemplateIDs,
		AllowedExternalAuthProviderIDs: project.ExternalAuthProviderIDs,
		ParameterRules:                 templateParameterRules(project.TemplateParameterRules),
	}
}

// workspaceToolsDisabledInstruction overrides prompt guidance about
// workspaces for chats in an agent that turned workspace tools off.
const workspaceToolsDisabledInstruction = "This chat's agent does not allow " +
	"workspace tools. You cannot create, start, or use a workspace, read or " +
	"write workspace files, or run commands. Ignore any instructions about " +
	"workspaces and complete the task with the tools you have."

// chatWorkspaceToolsEnabled reports whether the agent a chat runs in allows
// workspace tools. Delegated chats without their own agent follow the root
// chat's. Chats outside an agent always have them. Lookup errors disable
// them, so a broken lookup cannot widen access.
func (p *Server) chatWorkspaceToolsEnabled(ctx context.Context, chat database.Chat) bool {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return true
	}
	//nolint:gocritic // Chatd reads project config on behalf of the chat owner.
	chatdCtx := dbauthz.AsChatd(ctx)
	projectID := chat.ProjectID
	if !projectID.Valid && chat.RootChatID.Valid && chat.RootChatID.UUID != chat.ID {
		root, err := p.db.GetChatByID(chatdCtx, chat.RootChatID.UUID)
		if err != nil {
			p.logger.Warn(ctx, "failed to load root chat for workspace tools",
				slog.F("chat_id", chat.ID), slog.Error(err))
			return false
		}
		projectID = root.ProjectID
	}
	if !projectID.Valid {
		return true
	}
	project, err := p.db.GetChatProjectByID(chatdCtx, projectID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		p.logger.Warn(ctx, "failed to load chat project for workspace tools",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return false
	}
	return project.WorkspaceToolsEnabled
}

// denyAllTemplatePolicy matches no real template.
var denyAllTemplatePolicy = chattool.TemplatePolicy{AllowedTemplateIDs: []uuid.UUID{uuid.Nil}}

// inheritedWorkspaceBinding returns the workspace, build, and agent a chat
// delegated to an invoked agent shares with spawn_agent's inherit_workspace.
// Spawn parents are always root chats, so this is the root chat's current
// binding. The root chat's owner grants the child that one workspace by
// asking for it. The child gets no workspace lifecycle tools while it uses a
// workspace its own user does not own (see chatBorrowsWorkspace), so it
// cannot start, stop, or switch the root owner's workspaces.
func (p *Server) inheritedWorkspaceBinding(ctx context.Context, parent database.Chat, agent database.ChatProject) (workspaceID, buildID, agentID uuid.NullUUID, err error) {
	if !agent.WorkspaceToolsEnabled {
		return uuid.NullUUID{}, uuid.NullUUID{}, uuid.NullUUID{}, xerrors.Errorf(
			"inherit_workspace is not allowed: agent %q does not allow workspace tools", agent.Name)
	}
	if parent.ParentChatID.Valid {
		return uuid.NullUUID{}, uuid.NullUUID{}, uuid.NullUUID{}, xerrors.New("delegated chats cannot create child subagents")
	}
	if !parent.WorkspaceID.Valid {
		return uuid.NullUUID{}, uuid.NullUUID{}, uuid.NullUUID{}, xerrors.New(
			"inherit_workspace failed: this chat has no workspace to share. Create one first, or spawn the agent without inherit_workspace so it creates its own")
	}
	//nolint:gocritic // Chatd checks the workspace it is about to share with the child.
	ws, err := p.db.GetWorkspaceByID(dbauthz.AsChatd(ctx), parent.WorkspaceID.UUID)
	if err != nil {
		return uuid.NullUUID{}, uuid.NullUUID{}, uuid.NullUUID{}, xerrors.Errorf("inherit_workspace failed: load this chat's workspace: %w", err)
	}
	if ws.Deleted {
		return uuid.NullUUID{}, uuid.NullUUID{}, uuid.NullUUID{}, xerrors.New(
			"inherit_workspace failed: this chat's workspace was deleted")
	}
	return parent.WorkspaceID, parent.BuildID, parent.AgentID, nil
}

// chatBorrowsWorkspace reports whether a delegated chat is bound to a
// workspace its owner does not own, which happens when an invoked agent
// inherits the root chat's workspace. Such a chat may use the workspace but
// gets no tools that create, start, stop, or rebind workspaces. Lookup
// errors count as borrowed, so a broken lookup cannot widen access.
func (p *Server) chatBorrowsWorkspace(ctx context.Context, chat database.Chat) bool {
	if !chat.ParentChatID.Valid || !chat.WorkspaceID.Valid {
		return false
	}
	//nolint:gocritic // Chatd compares the bound workspace's owner with the chat's.
	ws, err := p.db.GetWorkspaceByID(dbauthz.AsChatd(ctx), chat.WorkspaceID.UUID)
	if err != nil {
		p.logger.Warn(ctx, "failed to load chat workspace owner",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return true
	}
	return ws.OwnerID != chat.OwnerID
}

// formatSpawnableProjectAgents describes invokable agents for the spawn_agent
// tool description, or returns "" when there are none.
func formatSpawnableProjectAgents(agents []database.ChatProject) string {
	if len(agents) == 0 {
		return ""
	}
	parts := make([]string, 0, len(agents))
	for _, agent := range agents {
		entry := fmt.Sprintf("%s (%q", agent.ID, agent.Name)
		if description := strings.TrimSpace(agent.Description); description != "" {
			entry += ": " + subagentTruncateRunes(description, 200)
		}
		if !agent.WorkspaceToolsEnabled {
			entry += "; no workspace tools"
		}
		parts = append(parts, entry+")")
	}
	return " This chat's agent may invoke other agents. Set agent_id to " +
		"delegate to one; it runs with its own instructions, MCP servers, and " +
		"allowed templates. By default it starts without this chat's " +
		"workspace and creates its own when needed. Set inherit_workspace to " +
		"true to work in this chat's workspace, e.g. to review or edit files " +
		"already there; it then cannot create, start, stop, or switch " +
		"workspaces, and agents marked no workspace tools cannot inherit one. " +
		"Prefer an agent whose purpose matches the " +
		"task. Available agent_id values: " + strings.Join(parts, ", ") + "."
}

// templateParameterRules indexes a project's parameter rules by template ID
// and parameter name.
func templateParameterRules(rules database.ChatProjectTemplateParameterRules) map[uuid.UUID]map[string][]string {
	if len(rules) == 0 {
		return nil
	}
	out := make(map[uuid.UUID]map[string][]string, len(rules))
	for _, rule := range rules {
		if out[rule.TemplateID] == nil {
			out[rule.TemplateID] = map[string][]string{}
		}
		out[rule.TemplateID][rule.ParameterName] = rule.AllowedValues
	}
	return out
}
