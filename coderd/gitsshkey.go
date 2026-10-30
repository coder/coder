package coderd

import (
	"database/sql"
	"net/http"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// @Summary Regenerate user SSH key
// @Description Replaces the user's Git SSH key with a newly generated key pair and returns the new public key.
// @Description Update your Git provider with the new public key, because Coder no longer has the old private key.
// @Description You can regenerate your own key, and regenerating another user's key requires permission to update personal user data, which the Owner and User Admin roles have.
// @ID regenerate-user-ssh-key
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or `me` for the authenticated user."
// @Success 200 {object} codersdk.GitSSHKey
// @Router /api/v2/users/{user}/gitsshkey [put]
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
		UserID:          user.ID,
		UpdatedAt:       dbtime.Now(),
		PrivateKey:      privateKey,
		PrivateKeyKeyID: sql.NullString{}, // dbcrypt will update as required
		PublicKey:       publicKey,
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
// @Description Returns the user's public Git SSH key and never the private key.
// @Description You can read your own key, and reading another user's key requires permission to read personal user data, which the Owner and User Admin roles have.
// @ID get-user-git-ssh-key
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or `me` for the authenticated user."
// @Success 200 {object} codersdk.GitSSHKey
// @Router /api/v2/users/{user}/gitsshkey [get]
func (api *API) gitSSHKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

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
// @Description Returns the workspace owner's Git SSH key pair, including the private key, so the workspace agent can authenticate Git over SSH.
// @Description Authenticates with the workspace agent token instead of a session token.
// @ID get-workspace-agent-git-ssh-key
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Success 200 {object} agentsdk.GitSSHKey
// @Router /api/v2/workspaceagents/me/gitsshkey [get]
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
	if httpapi.IsUnauthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching git SSH key.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.GitSSHKey{
		PublicKey:  gitSSHKey.PublicKey,
		PrivateKey: gitSSHKey.PrivateKey,
	})
}
