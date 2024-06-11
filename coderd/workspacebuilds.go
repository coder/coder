package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get workspace build
// @ID get-workspace-build
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID"
// @Success 200 {object} codersdk.WorkspaceBuild
// @Router /workspacebuilds/{workspacebuild} [get]
func (api *API) workspaceBuild(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace := httpmw.WorkspaceParam(r)

	data, err := api.workspaceBuildsData(ctx, []database.Workspace{workspace}, []database.WorkspaceBuild{workspaceBuild})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting workspace build data.",
			Detail:  err.Error(),
		})
		return
	}

	// Ensure we have the job and template version for the workspace build.
	// Otherwise we risk a panic in the api.convertWorkspaceBuild call below.
	if len(data.jobs) == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Internal error getting workspace build data.",
			Detail:  "No job found for workspace build.",
		})
		return
	}

	if len(data.templateVersions) == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Internal error getting workspace build data.",
			Detail:  "No template version found for workspace build.",
		})
		return
	}
	owner, ok := userByID(workspace.OwnerID, data.users)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  "owner not found for workspace",
		})
		return
	}

	apiBuild, err := api.convertWorkspaceBuild(
		workspaceBuild,
		workspace,
		data.jobs[0],
		owner.Username,
		owner.AvatarURL,
		data.resources,
		data.metadata,
		data.agents,
		data.apps,
		data.scripts,
		data.logSources,
		data.templateVersions[0],
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiBuild)
}

// @Summary Get workspace builds by workspace ID
// @ID get-workspace-builds-by-workspace-id
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param after_id query string false "After ID" format(uuid)
// @Param limit query int false "Page limit"
// @Param offset query int false "Page offset"
// @Param since query string false "Since timestamp" format(date-time)
// @Success 200 {array} codersdk.WorkspaceBuild
// @Router /workspaces/{workspace}/builds [get]
func (api *API) workspaceBuilds(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)

	paginationParams, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	var since time.Time

	sinceParam := r.URL.Query().Get("since")
	if sinceParam != "" {
		var err error
		since, err = time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "bad `since` format, must be RFC3339",
				Detail:  err.Error(),
			})
			return
		}
	}

	var workspaceBuilds []database.WorkspaceBuild
	// Ensure all db calls happen in the same tx
	err := api.Database.InTx(func(store database.Store) error {
		var err error
		if paginationParams.AfterID != uuid.Nil {
			// See if the record exists first. If the record does not exist, the pagination
			// query will not work.
			_, err := store.GetWorkspaceBuildByID(ctx, paginationParams.AfterID)
			if err != nil && xerrors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("Record at \"after_id\" (%q) does not exist.", paginationParams.AfterID.String()),
				})
				return err
			} else if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching workspace build at \"after_id\".",
					Detail:  err.Error(),
				})
				return err
			}
		}

		req := database.GetWorkspaceBuildsByWorkspaceIDParams{
			WorkspaceID: workspace.ID,
			AfterID:     paginationParams.AfterID,
			OffsetOpt:   int32(paginationParams.Offset),
			LimitOpt:    int32(paginationParams.Limit),
			Since:       dbtime.Time(since),
		}
		workspaceBuilds, err = store.GetWorkspaceBuildsByWorkspaceID(ctx, req)
		if xerrors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching workspace build.",
				Detail:  err.Error(),
			})
			return err
		}

		return nil
	}, nil)
	if err != nil {
		return
	}

	data, err := api.workspaceBuildsData(ctx, []database.Workspace{workspace}, workspaceBuilds)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting workspace build data.",
			Detail:  err.Error(),
		})
		return
	}

	apiBuilds, err := api.convertWorkspaceBuilds(
		workspaceBuilds,
		[]database.Workspace{workspace},
		data.jobs,
		data.users,
		data.resources,
		data.metadata,
		data.agents,
		data.apps,
		data.scripts,
		data.logSources,
		data.templateVersions,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiBuilds)
}

// @Summary Get workspace build by user, workspace name, and build number
// @ID get-workspace-build-by-user-workspace-name-and-build-number
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param user path string true "User ID, name, or me"
// @Param workspacename path string true "Workspace name"
// @Param buildnumber path string true "Build number" format(number)
// @Success 200 {object} codersdk.WorkspaceBuild
// @Router /users/{user}/workspace/{workspacename}/builds/{buildnumber} [get]
func (api *API) workspaceBuildByBuildNumber(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	owner := httpmw.UserParam(r)
	workspaceName := chi.URLParam(r, "workspacename")
	buildNumber, err := strconv.ParseInt(chi.URLParam(r, "buildnumber"), 10, 32)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to parse build number as integer.",
			Detail:  err.Error(),
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: owner.ID,
		Name:    workspaceName,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace by name.",
			Detail:  err.Error(),
		})
		return
	}

	workspaceBuild, err := api.Database.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
		WorkspaceID: workspace.ID,
		BuildNumber: int32(buildNumber),
	})
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Workspace %q Build %d does not exist.", workspaceName, buildNumber),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	data, err := api.workspaceBuildsData(ctx, []database.Workspace{workspace}, []database.WorkspaceBuild{workspaceBuild})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting workspace build data.",
			Detail:  err.Error(),
		})
		return
	}
	owner, ok := userByID(workspace.OwnerID, data.users)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  "owner not found for workspace",
		})
		return
	}

	apiBuild, err := api.convertWorkspaceBuild(
		workspaceBuild,
		workspace,
		data.jobs[0],
		owner.Username,
		owner.AvatarURL,
		data.resources,
		data.metadata,
		data.agents,
		data.apps,
		data.scripts,
		data.logSources,
		data.templateVersions[0],
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiBuild)
}

// Azure supports instance identity verification:
// https://docs.microsoft.com/en-us/azure/virtual-machines/windows/instance-metadata-service?tabs=linux#tabgroup_14
//
// @Summary Create workspace build
// @ID create-workspace-build
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Builds
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.CreateWorkspaceBuildRequest true "Create workspace build request"
// @Success 200 {object} codersdk.WorkspaceBuild
// @Router /workspaces/{workspace}/builds [post]
func (api *API) postWorkspaceBuilds(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	workspace := httpmw.WorkspaceParam(r)
	var createBuild codersdk.CreateWorkspaceBuildRequest
	if !httpapi.Read(ctx, rw, r, &createBuild) {
		return
	}

	builder := wsbuilder.New(workspace, database.WorkspaceTransition(createBuild.Transition)).
		Initiator(apiKey.UserID).
		RichParameterValues(createBuild.RichParameterValues).
		LogLevel(string(createBuild.LogLevel)).
		DeploymentValues(api.Options.DeploymentValues)

	if createBuild.TemplateVersionID != uuid.Nil {
		builder = builder.VersionID(createBuild.TemplateVersionID)
	}

	if createBuild.Orphan {
		if createBuild.Transition != codersdk.WorkspaceTransitionDelete {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Orphan is only permitted when deleting a workspace.",
			})
			return
		}
		if len(createBuild.ProvisionerState) > 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "ProvisionerState cannot be set alongside Orphan since state intent is unclear.",
			})
			return
		}
		builder = builder.Orphan()
	}
	if len(createBuild.ProvisionerState) > 0 {
		builder = builder.State(createBuild.ProvisionerState)
	}

	workspaceBuild, provisionerJob, err := builder.Build(
		ctx,
		api.Database,
		func(action policy.Action, object rbac.Objecter) bool {
			return api.Authorize(r, action, object)
		},
		audit.WorkspaceBuildBaggageFromRequest(r),
	)
	var buildErr wsbuilder.BuildError
	if xerrors.As(err, &buildErr) {
		var authErr dbauthz.NotAuthorizedError
		if xerrors.As(err, &authErr) {
			buildErr.Status = http.StatusForbidden
		}

		if buildErr.Status == http.StatusInternalServerError {
			api.Logger.Error(ctx, "workspace build error", slog.Error(buildErr.Wrapped))
		}

		httpapi.Write(ctx, rw, buildErr.Status, codersdk.Response{
			Message: buildErr.Message,
			Detail:  buildErr.Error(),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error posting new build",
			Detail:  err.Error(),
		})
		return
	}
	err = provisionerjobs.PostJob(api.Pubsub, *provisionerJob)
	if err != nil {
		// Client probably doesn't care about this error, so just log it.
		api.Logger.Error(ctx, "failed to post provisioner job to pubsub", slog.Error(err))
	}

	users, err := api.Database.GetUsersByIDs(ctx, []uuid.UUID{
		workspace.OwnerID,
		workspaceBuild.InitiatorID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting user.",
			Detail:  err.Error(),
		})
		return
	}
	owner, exists := userByID(workspace.OwnerID, users)
	if !exists {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  "owner not found for workspace",
		})
		return
	}

	apiBuild, err := api.convertWorkspaceBuild(
		*workspaceBuild,
		workspace,
		database.GetProvisionerJobsByIDsWithQueuePositionRow{
			ProvisionerJob: *provisionerJob,
			QueuePosition:  0,
		},
		owner.Username,
		owner.AvatarURL,
		[]database.WorkspaceResource{},
		[]database.WorkspaceResourceMetadatum{},
		[]database.WorkspaceAgent{},
		[]database.WorkspaceApp{},
		[]database.WorkspaceAgentScript{},
		[]database.WorkspaceAgentLogSource{},
		database.TemplateVersion{},
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	api.publishWorkspaceUpdate(ctx, workspace.ID)

	httpapi.Write(ctx, rw, http.StatusCreated, apiBuild)
}

// @Summary Cancel workspace build
// @ID cancel-workspace-build
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID"
// @Success 200 {object} codersdk.Response
// @Router /workspacebuilds/{workspacebuild}/cancel [patch]
func (api *API) patchCancelWorkspaceBuild(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace, err := api.Database.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "No workspace exists for this job.",
		})
		return
	}

	valid, err := api.verifyUserCanCancelWorkspaceBuilds(ctx, httpmw.APIKey(r).UserID, workspace.TemplateID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error verifying permission to cancel workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	if !valid {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "User is not allowed to cancel workspace builds. Owner role is required.",
		})
		return
	}

	job, err := api.Database.GetProvisionerJobByID(ctx, workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already completed!",
		})
		return
	}
	if job.CanceledAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job has already been marked as canceled!",
		})
		return
	}
	err = api.Database.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{
		ID: job.ID,
		CanceledAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
		CompletedAt: sql.NullTime{
			Time: dbtime.Now(),
			// If the job is running, don't mark it completed!
			Valid: !job.WorkerID.Valid,
		},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating provisioner job.",
			Detail:  err.Error(),
		})
		return
	}

	api.publishWorkspaceUpdate(ctx, workspace.ID)

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Job has been marked as canceled...",
	})
}

func (api *API) verifyUserCanCancelWorkspaceBuilds(ctx context.Context, userID uuid.UUID, templateID uuid.UUID) (bool, error) {
	template, err := api.Database.GetTemplateByID(ctx, templateID)
	if err != nil {
		return false, xerrors.New("no template exists for this workspace")
	}

	if template.AllowUserCancelWorkspaceJobs {
		return true, nil // all users can cancel workspace builds
	}

	user, err := api.Database.GetUserByID(ctx, userID)
	if err != nil {
		return false, xerrors.New("user does not exist")
	}
	return slices.Contains(user.RBACRoles, rbac.RoleOwner().String()), nil // only user with "owner" role can cancel workspace builds
}

// @Summary Get build parameters for workspace build
// @ID get-build-parameters-for-workspace-build
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID"
// @Success 200 {array} codersdk.WorkspaceBuildParameter
// @Router /workspacebuilds/{workspacebuild}/parameters [get]
func (api *API) workspaceBuildParameters(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceBuild := httpmw.WorkspaceBuildParam(r)

	parameters, err := api.Database.GetWorkspaceBuildParameters(ctx, workspaceBuild.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build parameters.",
			Detail:  err.Error(),
		})
		return
	}
	apiParameters := db2sdk.WorkspaceBuildParameters(parameters)
	httpapi.Write(ctx, rw, http.StatusOK, apiParameters)
}

// @Summary Get workspace build logs
// @ID get-workspace-build-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID"
// @Param before query int false "Before Unix timestamp"
// @Param after query int false "After Unix timestamp"
// @Param follow query bool false "Follow log stream"
// @Success 200 {array} codersdk.ProvisionerJobLog
// @Router /workspacebuilds/{workspacebuild}/logs [get]
func (api *API) workspaceBuildLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceBuild := httpmw.WorkspaceBuildParam(r)

	job, err := api.Database.GetProvisionerJobByID(ctx, workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

// @Summary Get provisioner state for workspace build
// @ID get-provisioner-state-for-workspace-build
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID"
// @Success 200 {object} codersdk.WorkspaceBuild
// @Router /workspacebuilds/{workspacebuild}/state [get]
func (api *API) workspaceBuildState(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspace, err := api.Database.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "No workspace exists for this job.",
		})
		return
	}
	template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get template",
			Detail:  err.Error(),
		})
		return
	}

	// You must have update permissions on the template to get the state.
	// This matches a push!
	if !api.Authorize(r, policy.ActionUpdate, template.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(workspaceBuild.ProvisionerState)
}

type workspaceBuildsData struct {
	users            []database.User
	jobs             []database.GetProvisionerJobsByIDsWithQueuePositionRow
	templateVersions []database.TemplateVersion
	resources        []database.WorkspaceResource
	metadata         []database.WorkspaceResourceMetadatum
	agents           []database.WorkspaceAgent
	apps             []database.WorkspaceApp
	scripts          []database.WorkspaceAgentScript
	logSources       []database.WorkspaceAgentLogSource
}

func (api *API) workspaceBuildsData(ctx context.Context, workspaces []database.Workspace, workspaceBuilds []database.WorkspaceBuild) (workspaceBuildsData, error) {
	userIDs := make([]uuid.UUID, 0, len(workspaceBuilds))
	for _, workspace := range workspaces {
		userIDs = append(userIDs, workspace.OwnerID)
	}
	users, err := api.Database.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return workspaceBuildsData{}, xerrors.Errorf("get users: %w", err)
	}

	jobIDs := make([]uuid.UUID, 0, len(workspaceBuilds))
	for _, build := range workspaceBuilds {
		jobIDs = append(jobIDs, build.JobID)
	}
	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, jobIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceBuildsData{}, xerrors.Errorf("get provisioner jobs: %w", err)
	}

	templateVersionIDs := make([]uuid.UUID, 0, len(workspaceBuilds))
	for _, build := range workspaceBuilds {
		templateVersionIDs = append(templateVersionIDs, build.TemplateVersionID)
	}

	// nolint:gocritic // Getting template versions by ID is a system function.
	templateVersions, err := api.Database.GetTemplateVersionsByIDs(dbauthz.AsSystemRestricted(ctx), templateVersionIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceBuildsData{}, xerrors.Errorf("get template versions: %w", err)
	}

	// nolint:gocritic // Getting workspace resources by job ID is a system function.
	resources, err := api.Database.GetWorkspaceResourcesByJobIDs(dbauthz.AsSystemRestricted(ctx), jobIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceBuildsData{}, xerrors.Errorf("get workspace resources by job: %w", err)
	}

	if len(resources) == 0 {
		return workspaceBuildsData{
			users:            users,
			jobs:             jobs,
			templateVersions: templateVersions,
		}, nil
	}

	resourceIDs := make([]uuid.UUID, 0)
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
	}

	// nolint:gocritic // Getting workspace resource metadata by resource ID is a system function.
	metadata, err := api.Database.GetWorkspaceResourceMetadataByResourceIDs(dbauthz.AsSystemRestricted(ctx), resourceIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceBuildsData{}, xerrors.Errorf("fetching resource metadata: %w", err)
	}

	// nolint:gocritic // Getting workspace agents by resource IDs is a system function.
	agents, err := api.Database.GetWorkspaceAgentsByResourceIDs(dbauthz.AsSystemRestricted(ctx), resourceIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceBuildsData{}, xerrors.Errorf("get workspace agents: %w", err)
	}

	if len(resources) == 0 {
		return workspaceBuildsData{
			users:            users,
			jobs:             jobs,
			templateVersions: templateVersions,
			resources:        resources,
			metadata:         metadata,
		}, nil
	}

	agentIDs := make([]uuid.UUID, 0)
	for _, agent := range agents {
		agentIDs = append(agentIDs, agent.ID)
	}

	var (
		apps       []database.WorkspaceApp
		scripts    []database.WorkspaceAgentScript
		logSources []database.WorkspaceAgentLogSource
	)

	var eg errgroup.Group
	eg.Go(func() (err error) {
		// nolint:gocritic // Getting workspace apps by agent IDs is a system function.
		apps, err = api.Database.GetWorkspaceAppsByAgentIDs(dbauthz.AsSystemRestricted(ctx), agentIDs)
		return err
	})
	eg.Go(func() (err error) {
		// nolint:gocritic // Getting workspace scripts by agent IDs is a system function.
		scripts, err = api.Database.GetWorkspaceAgentScriptsByAgentIDs(dbauthz.AsSystemRestricted(ctx), agentIDs)
		return err
	})
	eg.Go(func() error {
		// nolint:gocritic // Getting workspace agent log sources by agent IDs is a system function.
		logSources, err = api.Database.GetWorkspaceAgentLogSourcesByAgentIDs(dbauthz.AsSystemRestricted(ctx), agentIDs)
		return err
	})
	err = eg.Wait()
	if err != nil {
		return workspaceBuildsData{}, err
	}

	return workspaceBuildsData{
		users:            users,
		jobs:             jobs,
		templateVersions: templateVersions,
		resources:        resources,
		metadata:         metadata,
		agents:           agents,
		apps:             apps,
		scripts:          scripts,
		logSources:       logSources,
	}, nil
}

func (api *API) convertWorkspaceBuilds(
	workspaceBuilds []database.WorkspaceBuild,
	workspaces []database.Workspace,
	jobs []database.GetProvisionerJobsByIDsWithQueuePositionRow,
	users []database.User,
	workspaceResources []database.WorkspaceResource,
	resourceMetadata []database.WorkspaceResourceMetadatum,
	resourceAgents []database.WorkspaceAgent,
	agentApps []database.WorkspaceApp,
	agentScripts []database.WorkspaceAgentScript,
	agentLogSources []database.WorkspaceAgentLogSource,
	templateVersions []database.TemplateVersion,
) ([]codersdk.WorkspaceBuild, error) {
	workspaceByID := map[uuid.UUID]database.Workspace{}
	for _, workspace := range workspaces {
		workspaceByID[workspace.ID] = workspace
	}
	jobByID := map[uuid.UUID]database.GetProvisionerJobsByIDsWithQueuePositionRow{}
	for _, job := range jobs {
		jobByID[job.ProvisionerJob.ID] = job
	}
	templateVersionByID := map[uuid.UUID]database.TemplateVersion{}
	for _, templateVersion := range templateVersions {
		templateVersionByID[templateVersion.ID] = templateVersion
	}

	// Should never be nil for API consistency
	apiBuilds := []codersdk.WorkspaceBuild{}
	for _, build := range workspaceBuilds {
		job, exists := jobByID[build.JobID]
		if !exists {
			return nil, xerrors.New("build job not found")
		}
		workspace, exists := workspaceByID[build.WorkspaceID]
		if !exists {
			return nil, xerrors.New("workspace not found")
		}
		templateVersion, exists := templateVersionByID[build.TemplateVersionID]
		if !exists {
			return nil, xerrors.New("template version not found")
		}
		owner, exists := userByID(workspace.OwnerID, users)
		if !exists {
			return nil, xerrors.Errorf("owner not found for workspace: %q", workspace.Name)
		}

		apiBuild, err := api.convertWorkspaceBuild(
			build,
			workspace,
			job,
			owner.Username,
			owner.AvatarURL,
			workspaceResources,
			resourceMetadata,
			resourceAgents,
			agentApps,
			agentScripts,
			agentLogSources,
			templateVersion,
		)
		if err != nil {
			return nil, xerrors.Errorf("converting workspace build: %w", err)
		}

		apiBuilds = append(apiBuilds, apiBuild)
	}

	return apiBuilds, nil
}

func (api *API) convertWorkspaceBuild(
	build database.WorkspaceBuild,
	workspace database.Workspace,
	job database.GetProvisionerJobsByIDsWithQueuePositionRow,
	username, avatarURL string,
	workspaceResources []database.WorkspaceResource,
	resourceMetadata []database.WorkspaceResourceMetadatum,
	resourceAgents []database.WorkspaceAgent,
	agentApps []database.WorkspaceApp,
	agentScripts []database.WorkspaceAgentScript,
	agentLogSources []database.WorkspaceAgentLogSource,
	templateVersion database.TemplateVersion,
) (codersdk.WorkspaceBuild, error) {
	resourcesByJobID := map[uuid.UUID][]database.WorkspaceResource{}
	for _, resource := range workspaceResources {
		resourcesByJobID[resource.JobID] = append(resourcesByJobID[resource.JobID], resource)
	}
	metadataByResourceID := map[uuid.UUID][]database.WorkspaceResourceMetadatum{}
	for _, metadata := range resourceMetadata {
		metadataByResourceID[metadata.WorkspaceResourceID] = append(metadataByResourceID[metadata.WorkspaceResourceID], metadata)
	}
	agentsByResourceID := map[uuid.UUID][]database.WorkspaceAgent{}
	for _, agent := range resourceAgents {
		agentsByResourceID[agent.ResourceID] = append(agentsByResourceID[agent.ResourceID], agent)
	}
	appsByAgentID := map[uuid.UUID][]database.WorkspaceApp{}
	for _, app := range agentApps {
		appsByAgentID[app.AgentID] = append(appsByAgentID[app.AgentID], app)
	}
	scriptsByAgentID := map[uuid.UUID][]database.WorkspaceAgentScript{}
	for _, script := range agentScripts {
		scriptsByAgentID[script.WorkspaceAgentID] = append(scriptsByAgentID[script.WorkspaceAgentID], script)
	}
	logSourcesByAgentID := map[uuid.UUID][]database.WorkspaceAgentLogSource{}
	for _, logSource := range agentLogSources {
		logSourcesByAgentID[logSource.WorkspaceAgentID] = append(logSourcesByAgentID[logSource.WorkspaceAgentID], logSource)
	}

	resources := resourcesByJobID[job.ProvisionerJob.ID]
	apiResources := make([]codersdk.WorkspaceResource, 0)
	resourceAgentsMinOrder := map[uuid.UUID]int32{} // map[resource.ID]minOrder
	for _, resource := range resources {
		agents := agentsByResourceID[resource.ID]
		sort.Slice(agents, func(i, j int) bool {
			if agents[i].DisplayOrder != agents[j].DisplayOrder {
				return agents[i].DisplayOrder < agents[j].DisplayOrder
			}
			return agents[i].Name < agents[j].Name
		})

		apiAgents := make([]codersdk.WorkspaceAgent, 0)
		resourceAgentsMinOrder[resource.ID] = math.MaxInt32

		for _, agent := range agents {
			resourceAgentsMinOrder[resource.ID] = min(resourceAgentsMinOrder[resource.ID], agent.DisplayOrder)

			apps := appsByAgentID[agent.ID]
			scripts := scriptsByAgentID[agent.ID]
			logSources := logSourcesByAgentID[agent.ID]
			apiAgent, err := db2sdk.WorkspaceAgent(
				api.DERPMap(), *api.TailnetCoordinator.Load(), agent, db2sdk.Apps(apps, agent, username, workspace), convertScripts(scripts), convertLogSources(logSources), api.AgentInactiveDisconnectTimeout,
				api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
			)
			if err != nil {
				return codersdk.WorkspaceBuild{}, xerrors.Errorf("converting workspace agent: %w", err)
			}
			apiAgents = append(apiAgents, apiAgent)
		}
		metadata := append(make([]database.WorkspaceResourceMetadatum, 0), metadataByResourceID[resource.ID]...)
		apiResources = append(apiResources, convertWorkspaceResource(resource, apiAgents, metadata))
	}
	sort.Slice(apiResources, func(i, j int) bool {
		orderI := resourceAgentsMinOrder[apiResources[i].ID]
		orderJ := resourceAgentsMinOrder[apiResources[j].ID]
		if orderI != orderJ {
			return orderI < orderJ
		}
		return apiResources[i].Name < apiResources[j].Name
	})

	apiJob := convertProvisionerJob(job)
	transition := codersdk.WorkspaceTransition(build.Transition)
	return codersdk.WorkspaceBuild{
		ID:                      build.ID,
		CreatedAt:               build.CreatedAt,
		UpdatedAt:               build.UpdatedAt,
		WorkspaceOwnerID:        workspace.OwnerID,
		WorkspaceOwnerName:      username,
		WorkspaceOwnerAvatarURL: avatarURL,
		WorkspaceID:             build.WorkspaceID,
		WorkspaceName:           workspace.Name,
		TemplateVersionID:       build.TemplateVersionID,
		TemplateVersionName:     templateVersion.Name,
		BuildNumber:             build.BuildNumber,
		Transition:              transition,
		InitiatorID:             build.InitiatorID,
		InitiatorUsername:       build.InitiatorByUsername,
		Job:                     apiJob,
		Deadline:                codersdk.NewNullTime(build.Deadline, !build.Deadline.IsZero()),
		MaxDeadline:             codersdk.NewNullTime(build.MaxDeadline, !build.MaxDeadline.IsZero()),
		Reason:                  codersdk.BuildReason(build.Reason),
		Resources:               apiResources,
		Status:                  convertWorkspaceStatus(apiJob.Status, transition),
		DailyCost:               build.DailyCost,
	}, nil
}

func convertWorkspaceResource(resource database.WorkspaceResource, agents []codersdk.WorkspaceAgent, metadata []database.WorkspaceResourceMetadatum) codersdk.WorkspaceResource {
	var convertedMetadata []codersdk.WorkspaceResourceMetadata
	for _, field := range metadata {
		convertedMetadata = append(convertedMetadata, codersdk.WorkspaceResourceMetadata{
			Key:       field.Key,
			Value:     field.Value.String,
			Sensitive: field.Sensitive,
		})
	}

	return codersdk.WorkspaceResource{
		ID:         resource.ID,
		CreatedAt:  resource.CreatedAt,
		JobID:      resource.JobID,
		Transition: codersdk.WorkspaceTransition(resource.Transition),
		Type:       resource.Type,
		Name:       resource.Name,
		Hide:       resource.Hide,
		Icon:       resource.Icon,
		Agents:     agents,
		Metadata:   convertedMetadata,
		DailyCost:  resource.DailyCost,
	}
}

func convertWorkspaceStatus(jobStatus codersdk.ProvisionerJobStatus, transition codersdk.WorkspaceTransition) codersdk.WorkspaceStatus {
	switch jobStatus {
	case codersdk.ProvisionerJobPending:
		return codersdk.WorkspaceStatusPending
	case codersdk.ProvisionerJobRunning:
		switch transition {
		case codersdk.WorkspaceTransitionStart:
			return codersdk.WorkspaceStatusStarting
		case codersdk.WorkspaceTransitionStop:
			return codersdk.WorkspaceStatusStopping
		case codersdk.WorkspaceTransitionDelete:
			return codersdk.WorkspaceStatusDeleting
		}
	case codersdk.ProvisionerJobSucceeded:
		switch transition {
		case codersdk.WorkspaceTransitionStart:
			return codersdk.WorkspaceStatusRunning
		case codersdk.WorkspaceTransitionStop:
			return codersdk.WorkspaceStatusStopped
		case codersdk.WorkspaceTransitionDelete:
			return codersdk.WorkspaceStatusDeleted
		}
	case codersdk.ProvisionerJobCanceling:
		return codersdk.WorkspaceStatusCanceling
	case codersdk.ProvisionerJobCanceled:
		return codersdk.WorkspaceStatusCanceled
	case codersdk.ProvisionerJobFailed:
		return codersdk.WorkspaceStatusFailed
	}

	// return error status since we should never get here
	return codersdk.WorkspaceStatusFailed
}
