package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// @Summary Regenerate user SSH key
// @ID regenerate-user-ssh-key
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.GitSSHKey
// @Router /users/{user}/gitsshkey [put]
func (api *API) regenerateGitSSHKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.GitSSHKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceUserData.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	oldKey, err := api.Database.GetGitSSHKey(ctx, user.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.Old = oldKey

	privateKey, publicKey, err := gitsshkey.Generate(api.SSHKeygenAlgorithm)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error generating a new SSH keypair.",
			Detail:  err.Error(),
		})
		return
	}

	newKey, err := api.Database.UpdateGitSSHKey(ctx, database.UpdateGitSSHKeyParams{
		UserID:     user.ID,
		UpdatedAt:  database.Now(),
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user's git SSH key.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = newKey

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.GitSSHKey{
		UserID:    newKey.UserID,
		CreatedAt: newKey.CreatedAt,
		UpdatedAt: newKey.UpdatedAt,
		// No need to return the private key to the user
		PublicKey: newKey.PublicKey,
	})
}

// @Summary Get user Git SSH key
// @ID get-user-git-ssh-key
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.GitSSHKey
// @Router /users/{user}/gitsshkey [get]
func (api *API) gitSSHKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUserData.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	gitSSHKey, err := api.Database.GetGitSSHKey(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's SSH key.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.GitSSHKey{
		UserID:    gitSSHKey.UserID,
		CreatedAt: gitSSHKey.CreatedAt,
		UpdatedAt: gitSSHKey.UpdatedAt,
		// No need to return the private key to the user
		PublicKey: gitSSHKey.PublicKey,
	})
}

// @Summary Get workspace agent Git SSH key
// @ID get-workspace-agent-git-ssh-key
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Success 200 {object} codersdk.AgentGitSSHKey
// @Router /workspaceagents/me/gitsshkey [get]
func (api *API) agentGitSSHKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := httpmw.WorkspaceAgent(r)
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, agent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resource.",
			Detail:  err.Error(),
		})
		return
	}

	job, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, job.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}

	gitSSHKey, err := api.Database.GetGitSSHKey(ctx, workspace.OwnerID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching git SSH key.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AgentGitSSHKey{
		PublicKey:  gitSSHKey.PublicKey,
		PrivateKey: gitSSHKey.PrivateKey,
	})
}
