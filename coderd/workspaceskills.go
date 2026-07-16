package coderd

import (
	"context"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/codersdk"
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

	// Match chat workspace binding: listing skills is part of attaching a
	// workspace to a chat, which requires SSH-level access, not just read.
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

	// The agent-pushed context snapshot is the same inventory chats pin and
	// read_skill resolves from, so the slash menu matches skill resolution.
	// An agent that has not pushed a snapshot yet simply has no rows.
	resources, err := api.Database.ListWorkspaceAgentContextResources(ctx, agent.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	skills := make([]codersdk.WorkspaceSkillMetadata, 0, len(resources))
	for _, resource := range resources {
		if resource.BodyKind != database.WorkspaceAgentContextBodyKindSkill ||
			resource.Status != database.WorkspaceAgentContextResourceStatusOk {
			continue
		}
		name, description, ok := chatd.SkillIdentityFromResourceBody(resource.Body)
		if !ok || name == "" {
			continue
		}
		skills = append(skills, codersdk.WorkspaceSkillMetadata{
			Name:        name,
			Description: description,
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
