package coderd

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/githubrunners"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) registerGitHubRunnerRoutes(r chi.Router) {
	r.Route("/github-runner-pools", func(r chi.Router) {
		r.Post("/", api.postGitHubRunnerPool)
		r.Get("/", api.listGitHubRunnerPools)
		r.Route("/{pool}", func(r chi.Router) {
			r.Get("/", api.githubRunnerPool)
			r.Patch("/", api.patchGitHubRunnerPool)
			r.Delete("/", api.deleteGitHubRunnerPool)
			r.Get("/runners", api.listGitHubRunners)
		})
	})
}

// @Summary Create GitHub runner pool
// @ID create-github-runner-pool
// @Security CoderSessionToken
// @Tags GitHub runners
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateGitHubRunnerPoolRequest true "Pool configuration"
// @Success 201 {object} codersdk.GitHubRunnerPool
// @Router /api/v2/organizations/{organization}/github-runner-pools [post]
func (api *API) postGitHubRunnerPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceGithubRunnerPool.InOrg(org.ID)) {
		httpapi.Forbidden(w)
		return
	}
	var req codersdk.CreateGitHubRunnerPoolRequest
	if !httpapi.Read(ctx, w, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{Message: "Invalid runner pool.", Validations: validations})
		return
	}
	if req.OwnerID == uuid.Nil {
		req.OwnerID = httpmw.APIKey(r).UserID
	}
	// Pool management does not grant permission to act as arbitrary owners.
	if req.OwnerID != httpmw.APIKey(r).UserID && !api.Authorize(r, policy.ActionCreate, rbac.ResourceWorkspace.InOrg(org.ID).WithOwner(req.OwnerID.String())) {
		httpapi.Forbidden(w)
		return
	}
	template, err := api.Database.GetTemplateByID(ctx, req.TemplateID)
	if err != nil || template.OrganizationID != org.ID {
		httpapi.ResourceNotFound(w)
		return
	}
	if members, err := api.Database.OrganizationMembers(ctx, database.OrganizationMembersParams{OrganizationID: org.ID, UserID: req.OwnerID, IncludeSystem: false, GithubUserID: 0}); err != nil || len(members) != 1 {
		httpapi.ResourceNotFound(w)
		return
	}
	actor, status, err := httpmw.UserRBACSubject(ctx, api.Database, req.OwnerID, rbac.ScopeAll)
	if err != nil || status != database.UserStatusActive {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{Message: "Runner owner must be active."})
		return
	}
	ownerCtx := dbauthz.As(ctx, actor)
	owner, err := api.Database.GetUserByID(ownerCtx, req.OwnerID)
	if err != nil {
		httpapi.ResourceNotFound(w)
		return
	}
	_, err = api.preflightWorkspaceCreate(ownerCtx, owner.ID, workspaceOwner{ID: owner.ID, Username: owner.Username, AvatarURL: owner.AvatarURL}, codersdk.CreateWorkspaceRequest{Name: "gha-preflight", TemplateID: template.ID})
	if err != nil {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{Message: "Owner cannot create runner workspaces with this template."})
		return
	}
	if req.RunnerGroupID == 0 {
		req.RunnerGroupID = 1
	}
	if req.StartupTimeoutSeconds == 0 {
		req.StartupTimeoutSeconds = 600
	}
	if req.JobTimeoutSeconds == 0 {
		req.JobTimeoutSeconds = 21600
	}
	aReq, commit := audit.InitRequest[database.GithubRunnerPool](w, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionCreate, OrganizationID: org.ID})
	defer commit()
	pool, err := api.Database.InsertGitHubRunnerPool(ctx, database.InsertGitHubRunnerPoolParams{ID: uuid.New(), OrganizationID: org.ID, OwnerID: req.OwnerID, TemplateID: req.TemplateID, Name: req.Name, GithubConfigUrl: strings.ToLower(strings.TrimRight(req.GitHubConfigURL, "/")), GithubToken: req.GitHubToken, GithubTokenKeyID: sql.NullString{}, RunnerGroupID: req.RunnerGroupID, MaxRunners: req.MaxRunners, StartupTimeoutSeconds: req.StartupTimeoutSeconds, JobTimeoutSeconds: req.JobTimeoutSeconds})
	if err != nil {
		status := http.StatusInternalServerError
		if database.IsUniqueViolation(err) {
			status = http.StatusConflict
		}
		httpapi.Write(ctx, w, status, codersdk.Response{Message: "Could not create runner pool."})
		return
	}
	aReq.New = pool
	aReq.New.GithubToken = ""
	httpapi.Write(ctx, w, http.StatusCreated, convertGitHubRunnerPool(pool))
}

// @Summary List GitHub runner pools
// @ID list-github-runner-pools
// @Security CoderSessionToken
// @Tags GitHub runners
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.GitHubRunnerPool
// @Router /api/v2/organizations/{organization}/github-runner-pools [get]
func (api *API) listGitHubRunnerPools(w http.ResponseWriter, r *http.Request) {
	pools, err := api.Database.GetGitHubRunnerPools(r.Context(), httpmw.OrganizationParam(r).ID)
	if err != nil {
		httpapi.Forbidden(w)
		return
	}
	result := make([]codersdk.GitHubRunnerPool, 0, len(pools))
	for _, pool := range pools {
		result = append(result, convertGitHubRunnerPool(pool))
	}
	httpapi.Write(r.Context(), w, http.StatusOK, result)
}

func (api *API) githubRunnerPoolParam(w http.ResponseWriter, r *http.Request) (database.GithubRunnerPool, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "pool"))
	if err != nil {
		httpapi.ResourceNotFound(w)
		return database.GithubRunnerPool{}, false
	}
	pool, err := api.Database.GetGitHubRunnerPoolByID(r.Context(), id)
	if err != nil || pool.OrganizationID != httpmw.OrganizationParam(r).ID {
		httpapi.ResourceNotFound(w)
		return database.GithubRunnerPool{}, false
	}
	return pool, true
}

// @Summary Get GitHub runner pool
// @ID get-github-runner-pool
// @Security CoderSessionToken
// @Tags GitHub runners
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param pool path string true "Pool ID" format(uuid)
// @Success 200 {object} codersdk.GitHubRunnerPool
// @Router /api/v2/organizations/{organization}/github-runner-pools/{pool} [get]
func (api *API) githubRunnerPool(w http.ResponseWriter, r *http.Request) {
	pool, ok := api.githubRunnerPoolParam(w, r)
	if !ok {
		return
	}
	httpapi.Write(r.Context(), w, http.StatusOK, convertGitHubRunnerPool(pool))
}

// @Summary Update GitHub runner pool
// @ID update-github-runner-pool
// @Security CoderSessionToken
// @Tags GitHub runners
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param pool path string true "Pool ID" format(uuid)
// @Param request body codersdk.UpdateGitHubRunnerPoolRequest true "Pool changes"
// @Success 200 {object} codersdk.GitHubRunnerPool
// @Router /api/v2/organizations/{organization}/github-runner-pools/{pool} [patch]
func (api *API) patchGitHubRunnerPool(w http.ResponseWriter, r *http.Request) {
	pool, ok := api.githubRunnerPoolParam(w, r)
	if !ok {
		return
	}
	var req codersdk.UpdateGitHubRunnerPoolRequest
	if !httpapi.Read(r.Context(), w, r, &req) {
		return
	}
	if req.State == "" {
		req.State = pool.State
	}
	if req.MaxRunners == 0 {
		req.MaxRunners = pool.MaxRunners
	}
	if (req.State != "active" && req.State != "paused" && req.State != "draining") || req.MaxRunners < 1 || req.MaxRunners > 1000 || (pool.State == "draining" && req.State != "draining") || pool.State == "deleted" {
		httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{Message: "Invalid pool state or capacity."})
		return
	}
	api.updateGitHubRunnerPool(w, r, pool, req, database.AuditActionWrite, http.StatusOK)
}

// @Summary Drain and delete GitHub runner pool
// @ID delete-github-runner-pool
// @Security CoderSessionToken
// @Tags GitHub runners
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param pool path string true "Pool ID" format(uuid)
// @Success 202 {object} codersdk.GitHubRunnerPool
// @Router /api/v2/organizations/{organization}/github-runner-pools/{pool} [delete]
func (api *API) deleteGitHubRunnerPool(w http.ResponseWriter, r *http.Request) {
	pool, ok := api.githubRunnerPoolParam(w, r)
	if !ok {
		return
	}
	if !api.Authorize(r, policy.ActionDelete, rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID)) {
		httpapi.Forbidden(w)
		return
	}
	if pool.State == "deleted" {
		httpapi.Write(r.Context(), w, http.StatusAccepted, convertGitHubRunnerPool(pool))
		return
	}
	api.updateGitHubRunnerPool(w, r, pool, codersdk.UpdateGitHubRunnerPoolRequest{State: "draining", MaxRunners: pool.MaxRunners}, database.AuditActionDelete, http.StatusAccepted)
}

func (api *API) updateGitHubRunnerPool(w http.ResponseWriter, r *http.Request, pool database.GithubRunnerPool, req codersdk.UpdateGitHubRunnerPoolRequest, action database.AuditAction, status int) {
	aReq, commit := audit.InitRequest[database.GithubRunnerPool](w, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: action, OrganizationID: pool.OrganizationID})
	defer commit()
	aReq.Old = pool
	aReq.Old.GithubToken = ""
	updated, err := api.Database.UpdateGitHubRunnerPool(r.Context(), database.UpdateGitHubRunnerPoolParams{ID: pool.ID, State: req.State, MaxRunners: req.MaxRunners})
	if err != nil {
		httpapi.Forbidden(w)
		return
	}
	aReq.New = updated
	aReq.New.GithubToken = ""
	httpapi.Write(r.Context(), w, status, convertGitHubRunnerPool(updated))
}

// @Summary List GitHub runner allocations
// @ID list-github-runners
// @Security CoderSessionToken
// @Tags GitHub runners
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param pool path string true "Pool ID" format(uuid)
// @Success 200 {array} codersdk.GitHubRunner
// @Router /api/v2/organizations/{organization}/github-runner-pools/{pool}/runners [get]
func (api *API) listGitHubRunners(w http.ResponseWriter, r *http.Request) {
	pool, ok := api.githubRunnerPoolParam(w, r)
	if !ok {
		return
	}
	runners, err := api.Database.GetGitHubRunnersByPoolID(r.Context(), pool.ID)
	if err != nil {
		httpapi.InternalServerError(w, err)
		return
	}
	result := make([]codersdk.GitHubRunner, 0, len(runners))
	for _, v := range runners {
		result = append(result, codersdk.GitHubRunner{ID: v.ID, OrganizationID: v.OrganizationID, PoolID: v.PoolID, WorkspaceID: v.WorkspaceID.UUID, Name: v.Name, GitHubRunnerID: v.GithubRunnerID, State: v.State, JobRequestID: v.JobRequestID, JobResult: v.JobResult, LastError: v.LastError, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt})
	}
	httpapi.Write(r.Context(), w, http.StatusOK, result)
}

func convertGitHubRunnerPool(p database.GithubRunnerPool) codersdk.GitHubRunnerPool {
	return codersdk.GitHubRunnerPool{ID: p.ID, OrganizationID: p.OrganizationID, OwnerID: p.OwnerID, TemplateID: p.TemplateID, Name: p.Name, GitHubConfigURL: p.GithubConfigUrl, RunnerGroupID: p.RunnerGroupID, ScaleSetID: p.ScaleSetID, MaxRunners: p.MaxRunners, State: p.State, StartupTimeoutSeconds: p.StartupTimeoutSeconds, JobTimeoutSeconds: p.JobTimeoutSeconds, AssignedJobs: p.AssignedJobs, LastError: p.LastError, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

func (api *API) createGitHubRunnerWorkspace(ctx context.Context, pool database.GithubRunnerPool, runner database.GithubRunner, jit string) (uuid.UUID, error) {
	actor, status, err := httpmw.UserRBACSubject(ctx, api.Database, pool.OwnerID, rbac.ScopeAll)
	if err != nil {
		return uuid.Nil, err
	}
	if status != database.UserStatusActive {
		return uuid.Nil, xerrors.New("runner owner is not active")
	}
	ctx = dbauthz.As(ctx, actor)
	owner, err := api.Database.GetUserByID(ctx, pool.OwnerID)
	if err != nil {
		return uuid.Nil, err
	}
	sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
	r, err := http.NewRequestWithContext(httpmw.WithRequestID(ctx, uuid.New()), http.MethodPost, "http://localhost/internal/github-runners/workspace", nil)
	if err != nil {
		return uuid.Nil, err
	}
	aReq, commit := audit.InitRequest[database.WorkspaceTable](sw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, Action: database.AuditActionCreate, OrganizationID: pool.OrganizationID})
	aReq.UserID = owner.ID
	defer commit()
	ttl := int64(0)
	workspace, err := createWorkspace(ctx, aReq, owner.ID, api, workspaceOwner{ID: owner.ID, Username: owner.Username, AvatarURL: owner.AvatarURL}, codersdk.CreateWorkspaceRequest{Name: runner.Name, TemplateID: pool.TemplateID, TTLMillis: &ttl, Secrets: []codersdk.WorkspaceSecretInput{{Name: "github-runner-jit", Value: &jit, EnvName: "ACTIONS_RUNNER_INPUT_JITCONFIG", Ephemeral: true}}}, &createWorkspaceOptions{
		postBuildInTX: func(ctx context.Context, tx database.Store, workspace database.Workspace, _ database.WorkspaceBuild) error {
			runner.WorkspaceID = uuid.NullUUID{UUID: workspace.ID, Valid: true}
			runner.State = "provisioning"
			// Only the leased controller may bind its allocation to this workspace.
			_, err := githubrunners.SaveRunner(dbauthz.AsGitHubRunnerController(ctx), tx, runner, pool.LeaseOwner.UUID) //nolint:gocritic // The workspace owner cannot update controller-owned allocation records.
			return err
		},
	})
	if err != nil {
		sw.WriteHeader(http.StatusInternalServerError)
		return uuid.Nil, err
	}
	sw.WriteHeader(http.StatusCreated)
	return workspace.ID, nil
}

func (api *API) githubRunnerWorkspaceStatus(ctx context.Context, id uuid.UUID) (string, error) {
	workspace, err := api.Database.GetWorkspaceByID(ctx, id)
	if err != nil {
		return "", err
	}
	if workspace.Deleted {
		return "deleted", nil
	}
	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, id)
	if err != nil {
		return "", err
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return "", err
	}
	return string(codersdk.ConvertWorkspaceStatus(codersdk.ProvisionerJobStatus(job.JobStatus), codersdk.WorkspaceTransition(build.Transition))), nil
}

func (api *API) deleteGitHubRunnerWorkspace(ctx context.Context, runner database.GithubRunner) error {
	id := runner.WorkspaceID.UUID
	workspace, err := api.Database.GetWorkspaceByID(ctx, id)
	if err != nil {
		return err
	}
	if workspace.Deleted {
		return nil
	}
	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, id)
	if err != nil {
		return err
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return err
	}
	if !job.CompletedAt.Valid {
		// The query scopes cancellation to this allocation's workspace, allowing
		// disposal even when developer-initiated cancellation is disabled.
		return api.Database.CancelGitHubRunnerWorkspaceBuild(ctx, database.CancelGitHubRunnerWorkspaceBuildParams{RunnerID: runner.ID, JobID: job.ID})
	}
	actor, ok := dbauthz.ActorFromContext(ctx)
	if !ok {
		return xerrors.New("runner controller has no actor")
	}
	_, err = api.postWorkspaceBuildsInternal(ctx, database.APIKey{UserID: workspace.OwnerID}, workspace, codersdk.CreateWorkspaceBuildRequest{Transition: codersdk.WorkspaceTransitionDelete}, func(action policy.Action, object rbac.Objecter) bool {
		return api.HTTPAuth.Authorizer.Authorize(ctx, actor, action, object.RBACObject()) == nil
	}, audit.WorkspaceBuildBaggage{})
	return err
}
