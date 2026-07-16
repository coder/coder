package coderd

import (
	"context"
	"net/http"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

const (
	workspaceSkillsAgentConnTimeout     = 30 * time.Second
	workspaceSkillsContextConfigTimeout = 5 * time.Second
)

// @Summary List workspace skills
// @ID list-workspace-skills
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {array} codersdk.WorkspaceSkillMetadata
// @Router /api/experimental/workspaces/{workspace}/skills [get]
// @x-apidocgen {"skip": true}
func (api *API) getWorkspaceSkills(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	logger := api.Logger.With(slog.F("workspace_id", workspace.ID))

	if !api.Authorize(r, policy.ActionSSH, workspace) {
		httpapi.Forbidden(rw)
		return
	}
	if workspace.Deleted {
		writeWorkspaceSkills(ctx, rw, nil)
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if build.Transition != database.WorkspaceTransitionStart {
		writeWorkspaceSkills(ctx, rw, nil)
		return
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if job.JobStatus != database.ProvisionerJobStatusSucceeded {
		writeWorkspaceSkills(ctx, rw, nil)
		return
	}

	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(agents) == 0 {
		writeWorkspaceSkills(ctx, rw, nil)
		return
	}

	agent, err := agentselect.FindChatAgent(agents)
	if err != nil {
		logger.Debug(ctx, "failed to select workspace skills agent", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to select workspace skills agent.",
			Detail:  err.Error(),
		})
		return
	}

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(),
		*api.TailnetCoordinator.Load(),
		agent,
		nil,
		nil,
		nil,
		api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		writeWorkspaceSkills(ctx, rw, nil)
		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, workspaceSkillsAgentConnTimeout)
	conn, release, err := api.agentProvider.AgentConn(dialCtx, agent.ID)
	cancel()
	if err != nil {
		logger.Debug(ctx, "failed to dial workspace skills agent", slog.F("agent_id", agent.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to connect to workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	configCtx, cancel := context.WithTimeout(ctx, workspaceSkillsContextConfigTimeout)
	cfg, err := conn.ContextConfig(configCtx)
	cancel()
	if err != nil {
		logger.Debug(ctx, "failed to fetch workspace skills context config", slog.F("agent_id", agent.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "Failed to fetch workspace skills from agent.",
			Detail:  err.Error(),
		})
		return
	}

	metas := chattool.SkillMetasFromContextParts(cfg.Parts)
	skills := make([]codersdk.WorkspaceSkillMetadata, 0, len(metas))
	for _, meta := range metas {
		skills = append(skills, codersdk.WorkspaceSkillMetadata{
			Name:        meta.Name,
			Description: meta.Description,
		})
	}
	writeWorkspaceSkills(ctx, rw, skills)
}

func writeWorkspaceSkills(ctx context.Context, rw http.ResponseWriter, skills []codersdk.WorkspaceSkillMetadata) {
	if skills == nil {
		skills = []codersdk.WorkspaceSkillMetadata{}
	}
	httpapi.Write(ctx, rw, http.StatusOK, skills)
}
