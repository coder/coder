package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/coderd/wspubsub"
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

	data, err := api.workspaceBuildsData(ctx, []database.WorkspaceBuild{workspaceBuild})
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

	apiBuild, err := api.convertWorkspaceBuild(
		workspaceBuild,
		workspace,
		data.jobs[0],
		data.resources,
		data.metadata,
		data.agents,
		data.apps,
		data.scripts,
		data.logSources,
		data.templateVersions[0],
		nil,
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
			if err != nil && errors.Is(err, sql.ErrNoRows) {
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
		if errors.Is(err, sql.ErrNoRows) {
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

	data, err := api.workspaceBuildsData(ctx, workspaceBuilds)
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
		data.resources,
		data.metadata,
		data.agents,
		data.apps,
		data.scripts,
		data.logSources,
		data.templateVersions,
		data.provisionerDaemons,
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

	data, err := api.workspaceBuildsData(ctx, []database.WorkspaceBuild{workspaceBuild})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting workspace build data.",
			Detail:  err.Error(),
		})
		return
	}

	apiBuild, err := api.convertWorkspaceBuild(
		workspaceBuild,
		workspace,
		data.jobs[0],
		data.resources,
		data.metadata,
		data.agents,
		data.apps,
		data.scripts,
		data.logSources,
		data.templateVersions[0],
		data.provisionerDaemons,
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

	var (
		previousWorkspaceBuild database.WorkspaceBuild
		workspaceBuild         *database.WorkspaceBuild
		provisionerJob         *database.ProvisionerJob
		provisionerDaemons     []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow
	)

	err := api.Database.InTx(func(tx database.Store) error {
		var err error

		previousWorkspaceBuild, err = tx.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			api.Logger.Error(ctx, "failed fetching previous workspace build", slog.F("workspace_id", workspace.ID), slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching previous workspace build",
				Detail:  err.Error(),
			})
			return nil
		}

		if createBuild.TemplateVersionID != uuid.Nil {
			builder = builder.VersionID(createBuild.TemplateVersionID)
		}

		if createBuild.Orphan {
			if createBuild.Transition != codersdk.WorkspaceTransitionDelete {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Orphan is only permitted when deleting a workspace.",
				})
				return nil
			}
			if len(createBuild.ProvisionerState) > 0 {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "ProvisionerState cannot be set alongside Orphan since state intent is unclear.",
				})
				return nil
			}
			builder = builder.Orphan()
		}
		if len(createBuild.ProvisionerState) > 0 {
			builder = builder.State(createBuild.ProvisionerState)
		}

		workspaceBuild, provisionerJob, provisionerDaemons, err = builder.Build(
			ctx,
			tx,
			func(action policy.Action, object rbac.Objecter) bool {
				return api.Authorize(r, action, object)
			},
			audit.WorkspaceBuildBaggageFromRequest(r),
		)
		return err
	}, nil)
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

	if provisionerJob != nil {
		if err := provisionerjobs.PostJob(api.Pubsub, *provisionerJob); err != nil {
			// Client probably doesn't care about this error, so just log it.
			api.Logger.Error(ctx, "failed to post provisioner job to pubsub", slog.Error(err))
		}
	}

	apiBuild, err := api.convertWorkspaceBuild(
		*workspaceBuild,
		workspace,
		database.GetProvisionerJobsByIDsWithQueuePositionRow{
			ProvisionerJob: *provisionerJob,
			QueuePosition:  0,
		},
		[]database.WorkspaceResource{},
		[]database.WorkspaceResourceMetadatum{},
		[]database.WorkspaceAgent{},
		[]database.WorkspaceApp{},
		[]database.WorkspaceAgentScript{},
		[]database.WorkspaceAgentLogSource{},
		database.TemplateVersion{},
		provisionerDaemons,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	// If this workspace build has a different template version ID to the previous build
	// we can assume it has just been updated.
	if createBuild.TemplateVersionID != uuid.Nil && createBuild.TemplateVersionID != previousWorkspaceBuild.TemplateVersionID {
		// nolint:gocritic // Need system context to fetch admins
		admins, err := findTemplateAdmins(dbauthz.AsSystemRestricted(ctx), api.Database)
		if err != nil {
			api.Logger.Error(ctx, "find template admins", slog.Error(err))
		} else {
			for _, admin := range admins {
				// Don't send notifications to user which initiated the event.
				if admin.ID == apiKey.UserID {
					continue
				}

				api.notifyWorkspaceUpdated(ctx, apiKey.UserID, admin.ID, workspace, createBuild.RichParameterValues)
			}
		}
	}

	api.publishWorkspaceUpdate(ctx, workspace.OwnerID, wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindStateChange,
		WorkspaceID: workspace.ID,
	})

	httpapi.Write(ctx, rw, http.StatusCreated, apiBuild)
}

func (api *API) notifyWorkspaceUpdated(
	ctx context.Context,
	initiatorID uuid.UUID,
	receiverID uuid.UUID,
	workspace database.Workspace,
	parameters []codersdk.WorkspaceBuildParameter,
) {
	log := api.Logger.With(slog.F("workspace_id", workspace.ID))

	template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		log.Warn(ctx, "failed to fetch template for workspace creation notification", slog.F("template_id", workspace.TemplateID), slog.Error(err))
		return
	}

	version, err := api.Database.GetTemplateVersionByID(ctx, template.ActiveVersionID)
	if err != nil {
		log.Warn(ctx, "failed to fetch template version for workspace creation notification", slog.F("template_id", workspace.TemplateID), slog.Error(err))
		return
	}

	initiator, err := api.Database.GetUserByID(ctx, initiatorID)
	if err != nil {
		log.Warn(ctx, "failed to fetch user for workspace update notification", slog.F("initiator_id", initiatorID), slog.Error(err))
		return
	}

	owner, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		log.Warn(ctx, "failed to fetch user for workspace update notification", slog.F("owner_id", workspace.OwnerID), slog.Error(err))
		return
	}

	buildParameters := make([]map[string]any, len(parameters))
	for idx, parameter := range parameters {
		buildParameters[idx] = map[string]any{
			"name":  parameter.Name,
			"value": parameter.Value,
		}
	}

	if _, err := api.NotificationsEnqueuer.EnqueueWithData(
		// nolint:gocritic // Need notifier actor to enqueue notifications
		dbauthz.AsNotifier(ctx),
		receiverID,
		notifications.TemplateWorkspaceManuallyUpdated,
		map[string]string{
			"organization": template.OrganizationName,
			"initiator":    initiator.Name,
			"workspace":    workspace.Name,
			"template":     template.Name,
			"version":      version.Name,
		},
		map[string]any{
			"workspace":        map[string]any{"id": workspace.ID, "name": workspace.Name},
			"template":         map[string]any{"id": template.ID, "name": template.Name},
			"template_version": map[string]any{"id": version.ID, "name": version.Name},
			"owner":            map[string]any{"id": owner.ID, "name": owner.Name, "email": owner.Email},
			"parameters":       buildParameters,
		},
		"api-workspaces-updated",
		// Associate this notification with all the related entities
		workspace.ID, workspace.OwnerID, workspace.TemplateID, workspace.OrganizationID,
	); err != nil {
		log.Warn(ctx, "failed to notify of workspace update", slog.Error(err))
	}
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

	api.publishWorkspaceUpdate(ctx, workspace.OwnerID, wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindStateChange,
		WorkspaceID: workspace.ID,
	})

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
// @Param before query int false "Before log id"
// @Param after query int false "After log id"
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

// @Summary Get workspace build timings by ID
// @ID get-workspace-build-timings-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceBuildTimings
// @Router /workspacebuilds/{workspacebuild}/timings [get]
func (api *API) workspaceBuildTimings(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		build = httpmw.WorkspaceBuildParam(r)
	)

	timings, err := api.buildTimings(ctx, build)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching timings.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, timings)
}

type workspaceBuildsData struct {
	jobs               []database.GetProvisionerJobsByIDsWithQueuePositionRow
	templateVersions   []database.TemplateVersion
	resources          []database.WorkspaceResource
	metadata           []database.WorkspaceResourceMetadatum
	agents             []database.WorkspaceAgent
	apps               []database.WorkspaceApp
	scripts            []database.WorkspaceAgentScript
	logSources         []database.WorkspaceAgentLogSource
	provisionerDaemons []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow
}

func (api *API) workspaceBuildsData(ctx context.Context, workspaceBuilds []database.WorkspaceBuild) (workspaceBuildsData, error) {
	jobIDs := make([]uuid.UUID, 0, len(workspaceBuilds))
	for _, build := range workspaceBuilds {
		jobIDs = append(jobIDs, build.JobID)
	}
	jobs, err := api.Database.GetProvisionerJobsByIDsWithQueuePosition(ctx, jobIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceBuildsData{}, xerrors.Errorf("get provisioner jobs: %w", err)
	}
	pendingJobIDs := []uuid.UUID{}
	for _, job := range jobs {
		if job.ProvisionerJob.JobStatus == database.ProvisionerJobStatusPending {
			pendingJobIDs = append(pendingJobIDs, job.ProvisionerJob.ID)
		}
	}

	pendingJobProvisioners, err := api.Database.GetEligibleProvisionerDaemonsByProvisionerJobIDs(ctx, pendingJobIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceBuildsData{}, xerrors.Errorf("get provisioner daemons: %w", err)
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
			jobs:               jobs,
			templateVersions:   templateVersions,
			provisionerDaemons: pendingJobProvisioners,
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
			jobs:               jobs,
			templateVersions:   templateVersions,
			resources:          resources,
			metadata:           metadata,
			provisionerDaemons: pendingJobProvisioners,
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
		jobs:               jobs,
		templateVersions:   templateVersions,
		resources:          resources,
		metadata:           metadata,
		agents:             agents,
		apps:               apps,
		scripts:            scripts,
		logSources:         logSources,
		provisionerDaemons: pendingJobProvisioners,
	}, nil
}

func (api *API) convertWorkspaceBuilds(
	workspaceBuilds []database.WorkspaceBuild,
	workspaces []database.Workspace,
	jobs []database.GetProvisionerJobsByIDsWithQueuePositionRow,
	workspaceResources []database.WorkspaceResource,
	resourceMetadata []database.WorkspaceResourceMetadatum,
	resourceAgents []database.WorkspaceAgent,
	agentApps []database.WorkspaceApp,
	agentScripts []database.WorkspaceAgentScript,
	agentLogSources []database.WorkspaceAgentLogSource,
	templateVersions []database.TemplateVersion,
	provisionerDaemons []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow,
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

		apiBuild, err := api.convertWorkspaceBuild(
			build,
			workspace,
			job,
			workspaceResources,
			resourceMetadata,
			resourceAgents,
			agentApps,
			agentScripts,
			agentLogSources,
			templateVersion,
			provisionerDaemons,
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
	workspaceResources []database.WorkspaceResource,
	resourceMetadata []database.WorkspaceResourceMetadatum,
	resourceAgents []database.WorkspaceAgent,
	agentApps []database.WorkspaceApp,
	agentScripts []database.WorkspaceAgentScript,
	agentLogSources []database.WorkspaceAgentLogSource,
	templateVersion database.TemplateVersion,
	provisionerDaemons []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow,
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
	provisionerDaemonsForThisWorkspaceBuild := []database.ProvisionerDaemon{}
	for _, provisionerDaemon := range provisionerDaemons {
		if provisionerDaemon.JobID != job.ProvisionerJob.ID {
			continue
		}
		provisionerDaemonsForThisWorkspaceBuild = append(provisionerDaemonsForThisWorkspaceBuild, provisionerDaemon.ProvisionerDaemon)
	}
	matchedProvisioners := db2sdk.MatchedProvisioners(provisionerDaemonsForThisWorkspaceBuild, job.ProvisionerJob.CreatedAt, provisionerdserver.StaleInterval)

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
				api.DERPMap(), *api.TailnetCoordinator.Load(), agent, db2sdk.Apps(apps, agent, workspace.OwnerUsername, workspace), convertScripts(scripts), convertLogSources(logSources), api.AgentInactiveDisconnectTimeout,
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
		WorkspaceOwnerName:      workspace.OwnerUsername,
		WorkspaceOwnerAvatarURL: workspace.OwnerAvatarUrl,
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
		Status:                  codersdk.ConvertWorkspaceStatus(apiJob.Status, transition),
		DailyCost:               build.DailyCost,
		MatchedProvisioners:     &matchedProvisioners,
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

func (api *API) buildTimings(ctx context.Context, build database.WorkspaceBuild) (codersdk.WorkspaceBuildTimings, error) {
	provisionerTimings, err := api.Database.GetProvisionerJobTimingsByJobID(ctx, build.JobID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return codersdk.WorkspaceBuildTimings{}, xerrors.Errorf("fetching provisioner job timings: %w", err)
	}

	//nolint:gocritic // Already checked if the build can be fetched.
	agentScriptTimings, err := api.Database.GetWorkspaceAgentScriptTimingsByBuildID(dbauthz.AsSystemRestricted(ctx), build.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return codersdk.WorkspaceBuildTimings{}, xerrors.Errorf("fetching workspace agent script timings: %w", err)
	}

	resources, err := api.Database.GetWorkspaceResourcesByJobID(ctx, build.JobID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return codersdk.WorkspaceBuildTimings{}, xerrors.Errorf("fetching workspace resources: %w", err)
	}
	resourceIDs := make([]uuid.UUID, 0, len(resources))
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
	}
	//nolint:gocritic // Already checked if the build can be fetched.
	agents, err := api.Database.GetWorkspaceAgentsByResourceIDs(dbauthz.AsSystemRestricted(ctx), resourceIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return codersdk.WorkspaceBuildTimings{}, xerrors.Errorf("fetching workspace agents: %w", err)
	}

	res := codersdk.WorkspaceBuildTimings{
		ProvisionerTimings:     make([]codersdk.ProvisionerTiming, 0, len(provisionerTimings)),
		AgentScriptTimings:     make([]codersdk.AgentScriptTiming, 0, len(agentScriptTimings)),
		AgentConnectionTimings: make([]codersdk.AgentConnectionTiming, 0, len(agents)),
	}

	for _, t := range provisionerTimings {
		res.ProvisionerTimings = append(res.ProvisionerTimings, codersdk.ProvisionerTiming{
			JobID:     t.JobID,
			Stage:     codersdk.TimingStage(t.Stage),
			Source:    t.Source,
			Action:    t.Action,
			Resource:  t.Resource,
			StartedAt: t.StartedAt,
			EndedAt:   t.EndedAt,
		})
	}
	for _, t := range agentScriptTimings {
		res.AgentScriptTimings = append(res.AgentScriptTimings, codersdk.AgentScriptTiming{
			StartedAt:          t.StartedAt,
			EndedAt:            t.EndedAt,
			ExitCode:           t.ExitCode,
			Stage:              codersdk.TimingStage(t.Stage),
			Status:             string(t.Status),
			DisplayName:        t.DisplayName,
			WorkspaceAgentID:   t.WorkspaceAgentID.String(),
			WorkspaceAgentName: t.WorkspaceAgentName,
		})
	}
	for _, agent := range agents {
		res.AgentConnectionTimings = append(res.AgentConnectionTimings, codersdk.AgentConnectionTiming{
			WorkspaceAgentID:   agent.ID.String(),
			WorkspaceAgentName: agent.Name,
			StartedAt:          agent.CreatedAt,
			Stage:              codersdk.TimingStageConnect,
			EndedAt:            agent.FirstConnectedAt.Time,
		})
	}

	return res, nil
}
