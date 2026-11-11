package coderd

import (
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"net/http"

	"golang.org/x/crypto/ssh"

	"cdr.dev/slog/v3"
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
// @ID regenerate-user-ssh-key
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
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
// @ID get-user-git-ssh-key
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
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

// agentGitSign signs a message digest with the workspace owner's git SSH
// key on the server, so signing needs no key file in the workspace. It backs
// `coder gitsign-ssh`, which git runs as gpg.ssh.program, and accepts only
// the workspace agent token. It does not check the owner's signing
// preference: the agent reads that when it connects, so turning signing off
// takes effect at the same point turning it on does, instead of failing
// commits until the workspace restarts. The same token can already fetch the
// key itself, so the check would add no protection.
//
// @x-apidocgen {"skip": true}
func (api *API) agentGitSign(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := httpmw.WorkspaceAgent(r)

	var req agentsdk.GitSignRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	// Git is the only supported caller. Restricting the namespace keeps the
	// endpoint from signing for other SSHSIG uses, such as file signatures.
	if req.Namespace != "git" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: `Only the "git" signature namespace is supported.`,
		})
		return
	}
	hashSizes := map[string]int{"sha256": sha256.Size, "sha512": sha512.Size}
	if size, ok := hashSizes[req.HashAlgorithm]; !ok || len(req.Hash) != size {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Hash must be a sha256 or sha512 digest matching hash_algorithm.",
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, agent.ID)
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
	signer, err := ssh.ParsePrivateKey([]byte(gitSSHKey.PrivateKey))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error parsing git SSH key.",
			Detail:  err.Error(),
		})
		return
	}
	signature, err := gitsshkey.SignSSHSig(signer, req.Namespace, req.HashAlgorithm, req.Hash)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error signing.",
			Detail:  err.Error(),
		})
		return
	}

	api.Logger.Info(ctx, "signed git object for workspace agent",
		slog.F("workspace_id", workspace.ID),
		slog.F("agent_id", agent.ID),
		slog.F("owner_id", workspace.OwnerID),
	)
	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.GitSignResponse{Signature: signature})
}
