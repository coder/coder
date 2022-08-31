package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

var (
	ttlMin = time.Minute //nolint:revive // min here means 'minimum' not 'minutes'
	ttlMax = 7 * 24 * time.Hour

	errTTLMin                  = xerrors.New("time until shutdown must be at least one minute")
	errTTLMax                  = xerrors.New("time until shutdown must be less than 7 days")
	errDeadlineTooSoon         = xerrors.New("new deadline must be at least 30 minutes in the future")
	errDeadlineBeforeStart     = xerrors.New("new deadline must be before workspace start time")
	errDeadlineOverTemplateMax = xerrors.New("new deadline is greater than template allows")
)

func (api *API) workspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var (
		deletedStr  = r.URL.Query().Get("include_deleted")
		showDeleted = false
	)
	if deletedStr != "" {
		var err error
		showDeleted, err = strconv.ParseBool(deletedStr)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid boolean value %q for \"include_deleted\" query param.", deletedStr),
				Validations: []codersdk.ValidationError{
					{Field: "deleted", Detail: "Must be a valid boolean"},
				},
			})
			return
		}
	}
	if workspace.Deleted && !showDeleted {
		httpapi.Write(rw, http.StatusGone, codersdk.Response{
			Message: fmt.Sprintf("Workspace %q was deleted, you can view this workspace by specifying '?deleted=true' and trying again.", workspace.ID.String()),
		})
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	var (
		group    errgroup.Group
		job      database.ProvisionerJob
		template database.Template
		users    []database.User
	)
	group.Go(func() (err error) {
		job, err = api.Database.GetProvisionerJobByID(r.Context(), build.JobID)
		return err
	})
	group.Go(func() (err error) {
		template, err = api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
		return err
	})
	group.Go(func() (err error) {
		users, err = api.Database.GetUsersByIDs(r.Context(), []uuid.UUID{workspace.OwnerID, build.InitiatorID})
		return err
	})
	err = group.Wait()
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching resource.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspace(workspace, build, job, template,
		findUser(workspace.OwnerID, users), findUser(build.InitiatorID, users)))
}

// workspaces returns all workspaces a user can read.
// Optional filters with query params
func (api *API) workspaces(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)

	queryStr := r.URL.Query().Get("q")
	filter, errs := workspaceSearchQuery(queryStr)
	if len(errs) > 0 {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid workspace search query.",
			Validations: errs,
		})
		return
	}

	if filter.OwnerUsername == "me" {
		filter.OwnerID = apiKey.UserID
		filter.OwnerUsername = ""
	}

	workspaces, err := api.Database.GetWorkspaces(r.Context(), filter)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  err.Error(),
		})
		return
	}

	// Only return workspaces the user can read
	workspaces, err = AuthorizeFilter(api.httpAuth, r, rbac.ActionRead, workspaces)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  err.Error(),
		})
		return
	}

	apiWorkspaces, err := convertWorkspaces(r.Context(), api.Database, workspaces)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, apiWorkspaces)
}

func (api *API) workspaceByOwnerAndName(rw http.ResponseWriter, r *http.Request) {
	owner := httpmw.UserParam(r)
	workspaceName := chi.URLParam(r, "workspacename")

	includeDeleted := false
	if s := r.URL.Query().Get("include_deleted"); s != "" {
		var err error
		includeDeleted, err = strconv.ParseBool(s)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid boolean value %q for \"include_deleted\" query param.", s),
				Validations: []codersdk.ValidationError{
					{Field: "include_deleted", Detail: "Must be a valid boolean"},
				},
			})
			return
		}
	}

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: owner.ID,
		Name:    workspaceName,
	})
	if includeDeleted && errors.Is(err, sql.ErrNoRows) {
		workspace, err = api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: owner.ID,
			Name:    workspaceName,
			Deleted: includeDeleted,
		})
	}
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace by name.",
			Detail:  err.Error(),
		})
		return
	}
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), build.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	initiator, err := api.Database.GetUserByID(r.Context(), build.InitiatorID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspace(workspace, build, job, template, &owner, &initiator))
}

// Create a new workspace for the currently authenticated user.
func (api *API) postWorkspacesByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, rbac.ActionCreate,
		rbac.ResourceWorkspace.InOrg(organization.ID).WithOwner(apiKey.UserID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var createWorkspace codersdk.CreateWorkspaceRequest
	if !httpapi.Read(rw, r, &createWorkspace) {
		return
	}

	template, err := api.Database.GetTemplateByID(r.Context(), createWorkspace.TemplateID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Template %q doesn't exist.", createWorkspace.TemplateID.String()),
			Validations: []codersdk.ValidationError{{
				Field:  "template_id",
				Detail: "template not found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if organization.ID != template.OrganizationID {
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: fmt.Sprintf("Template is not in organization %q.", organization.Name),
		})
		return
	}

	dbAutostartSchedule, err := validWorkspaceSchedule(createWorkspace.AutostartSchedule, time.Duration(template.MinAutostartInterval))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid Autostart Schedule.",
			Validations: []codersdk.ValidationError{{Field: "schedule", Detail: err.Error()}},
		})
		return
	}

	dbTTL, err := validWorkspaceTTLMillis(createWorkspace.TTLMillis, time.Duration(template.MaxTtl))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid Workspace Time to Shutdown.",
			Validations: []codersdk.ValidationError{{Field: "ttl_ms", Detail: err.Error()}},
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: apiKey.UserID,
		Name:    createWorkspace.Name,
	})
	if err == nil {
		// If the workspace already exists, don't allow creation.
		httpapi.Write(rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Workspace %q already exists.", createWorkspace.Name),
			Validations: []codersdk.ValidationError{{
				Field:  "name",
				Detail: "This value is already in use and should be unique.",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("Internal error fetching workspace by name %q.", createWorkspace.Name),
			Detail:  err.Error(),
		})
		return
	}

	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), template.ActiveVersionID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}
	templateVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version job.",
			Detail:  err.Error(),
		})
		return
	}
	templateVersionJobStatus := convertProvisionerJob(templateVersionJob).Status
	switch templateVersionJobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		httpapi.Write(rw, http.StatusNotAcceptable, codersdk.Response{
			Message: fmt.Sprintf("The provided template version is %s. Wait for it to complete importing!", templateVersionJobStatus),
		})
		return
	case codersdk.ProvisionerJobFailed:
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: fmt.Sprintf("The provided template version %q has failed to import. You cannot create workspaces using it!", templateVersion.Name),
		})
		return
	case codersdk.ProvisionerJobCanceled:
		httpapi.Write(rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "The provided template version was canceled during import. You cannot create workspaces using it!",
		})
		return
	}

	var provisionerJob database.ProvisionerJob
	var workspaceBuild database.WorkspaceBuild
	err = api.Database.InTx(func(db database.Store) error {
		now := database.Now()
		workspaceBuildID := uuid.New()
		// Workspaces are created without any versions.
		workspace, err = db.InsertWorkspace(r.Context(), database.InsertWorkspaceParams{
			ID:                uuid.New(),
			CreatedAt:         now,
			UpdatedAt:         now,
			OwnerID:           apiKey.UserID,
			OrganizationID:    template.OrganizationID,
			TemplateID:        template.ID,
			Name:              createWorkspace.Name,
			AutostartSchedule: dbAutostartSchedule,
			Ttl:               dbTTL,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace: %w", err)
		}
		for _, parameterValue := range createWorkspace.ParameterValues {
			_, err = db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              parameterValue.Name,
				CreatedAt:         now,
				UpdatedAt:         now,
				Scope:             database.ParameterScopeWorkspace,
				ScopeID:           workspace.ID,
				SourceScheme:      database.ParameterSourceScheme(parameterValue.SourceScheme),
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: database.ParameterDestinationScheme(parameterValue.DestinationScheme),
			})
			if err != nil {
				return xerrors.Errorf("insert parameter value: %w", err)
			}
		}

		input, err := json.Marshal(workspaceProvisionJob{
			WorkspaceBuildID: workspaceBuildID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}
		provisionerJob, err = db.InsertProvisionerJob(r.Context(), database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      now,
			UpdatedAt:      now,
			InitiatorID:    apiKey.UserID,
			OrganizationID: template.OrganizationID,
			Provisioner:    template.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  templateVersionJob.StorageMethod,
			StorageSource:  templateVersionJob.StorageSource,
			Input:          input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}
		workspaceBuild, err = db.InsertWorkspaceBuild(r.Context(), database.InsertWorkspaceBuildParams{
			ID:                workspaceBuildID,
			CreatedAt:         now,
			UpdatedAt:         now,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersion.ID,
			Name:              namesgenerator.GetRandomName(1),
			InitiatorID:       apiKey.UserID,
			Transition:        database.WorkspaceTransitionStart,
			JobID:             provisionerJob.ID,
			BuildNumber:       1,           // First build!
			Deadline:          time.Time{}, // provisionerd will set this upon success
			Reason:            database.BuildReasonInitiator,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating workspace.",
			Detail:  err.Error(),
		})
		return
	}
	users, err := api.Database.GetUsersByIDs(r.Context(), []uuid.UUID{apiKey.UserID, workspaceBuild.InitiatorID})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		Workspaces:      []telemetry.Workspace{telemetry.ConvertWorkspace(workspace)},
		WorkspaceBuilds: []telemetry.WorkspaceBuild{telemetry.ConvertWorkspaceBuild(workspaceBuild)},
	})

	httpapi.Write(rw, http.StatusCreated, convertWorkspace(workspace, workspaceBuild, templateVersionJob, template,
		findUser(apiKey.UserID, users), findUser(workspaceBuild.InitiatorID, users)))
}

func (api *API) patchWorkspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateWorkspaceRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	if req.Name == "" || req.Name == workspace.Name {
		// Nothing changed, optionally this could be an error.
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	// The reason we double check here is in case more fields can be
	// patched in the future, it's enough if one changes.
	name := workspace.Name
	if req.Name != "" || req.Name != workspace.Name {
		name = req.Name
	}

	_, err := api.Database.UpdateWorkspace(r.Context(), database.UpdateWorkspaceParams{
		ID:   workspace.ID,
		Name: name,
	})
	if err != nil {
		// The query protects against updating deleted workspaces and
		// the existence of the workspace is checked in the request,
		// if we get ErrNoRows it means the workspace was deleted.
		//
		// We could do this check earlier but we'd need to start a
		// transaction.
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(rw, http.StatusMethodNotAllowed, codersdk.Response{
				Message: fmt.Sprintf("Workspace %q is deleted and cannot be updated.", workspace.Name),
			})
			return
		}
		// Check if the name was already in use.
		if database.IsUniqueViolation(err) {
			httpapi.Write(rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("Workspace %q already exists.", req.Name),
				Validations: []codersdk.ValidationError{{
					Field:  "name",
					Detail: "This value is already in use and should be unique.",
				}},
			})
			return
		}
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) putWorkspaceAutostart(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateWorkspaceAutostartRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
	if err != nil {
		api.Logger.Error(r.Context(), "fetch workspace template", slog.F("workspace_id", workspace.ID), slog.F("template_id", workspace.TemplateID), slog.Error(err))
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error fetching workspace template.",
		})
		return
	}

	dbSched, err := validWorkspaceSchedule(req.Schedule, time.Duration(template.MinAutostartInterval))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid autostart schedule.",
			Validations: []codersdk.ValidationError{{Field: "schedule", Detail: err.Error()}},
		})
		return
	}

	err = api.Database.UpdateWorkspaceAutostart(r.Context(), database.UpdateWorkspaceAutostartParams{
		ID:                workspace.ID,
		AutostartSchedule: dbSched,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace autostart schedule.",
			Detail:  err.Error(),
		})
		return
	}
}

func (api *API) putWorkspaceTTL(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateWorkspaceTTLRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	err := api.Database.InTx(func(s database.Store) error {
		template, err := s.GetTemplateByID(r.Context(), workspace.TemplateID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Error fetching workspace template!",
			})
			return xerrors.Errorf("fetch workspace template: %w", err)
		}

		dbTTL, err := validWorkspaceTTLMillis(req.TTLMillis, time.Duration(template.MaxTtl))
		if err != nil {
			return codersdk.ValidationError{Field: "ttl_ms", Detail: err.Error()}
		}
		if err := s.UpdateWorkspaceTTL(r.Context(), database.UpdateWorkspaceTTLParams{
			ID:  workspace.ID,
			Ttl: dbTTL,
		}); err != nil {
			return xerrors.Errorf("update workspace time until shutdown: %w", err)
		}

		return nil
	})
	if err != nil {
		resp := codersdk.Response{
			Message: "Error updating workspace time until shutdown.",
		}
		var validErr codersdk.ValidationError
		if errors.As(err, &validErr) {
			resp.Validations = []codersdk.ValidationError{validErr}
			httpapi.Write(rw, http.StatusBadRequest, resp)
			return
		}

		resp.Detail = err.Error()
		httpapi.Write(rw, http.StatusInternalServerError, resp)
		return
	}

	httpapi.Write(rw, http.StatusOK, nil)
}

func (api *API) putExtendWorkspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.PutExtendWorkspaceRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	code := http.StatusOK
	resp := codersdk.Response{}

	err := api.Database.InTx(func(s database.Store) error {
		template, err := s.GetTemplateByID(r.Context(), workspace.TemplateID)
		if err != nil {
			code = http.StatusInternalServerError
			resp.Message = "Error fetching workspace template!"
			return xerrors.Errorf("get workspace template: %w", err)
		}

		build, err := s.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
		if err != nil {
			code = http.StatusInternalServerError
			resp.Message = "Error fetching workspace build."
			return xerrors.Errorf("get latest workspace build: %w", err)
		}

		job, err := s.GetProvisionerJobByID(r.Context(), build.JobID)
		if err != nil {
			code = http.StatusInternalServerError
			resp.Message = "Error fetching workspace provisioner job."
			return xerrors.Errorf("get provisioner job: %w", err)
		}

		if build.Transition != database.WorkspaceTransitionStart {
			code = http.StatusConflict
			resp.Message = "Workspace must be started, current status: " + string(build.Transition)
			return xerrors.Errorf("workspace must be started, current status: %s", build.Transition)
		}

		if !job.CompletedAt.Valid {
			code = http.StatusConflict
			resp.Message = "Workspace is still building!"
			return xerrors.Errorf("workspace is still building")
		}

		if build.Deadline.IsZero() {
			code = http.StatusConflict
			resp.Message = "Workspace shutdown is manual."
			return xerrors.Errorf("workspace shutdown is manual")
		}

		newDeadline := req.Deadline.UTC()
		if err := validWorkspaceDeadline(job.CompletedAt.Time, newDeadline, time.Duration(template.MaxTtl)); err != nil {
			// NOTE(Cian): Putting the error in the Message field on request from the FE folks.
			// Normally, we would put the validation error in Validations, but this endpoint is
			// not tied to a form or specific named user input on the FE.
			code = http.StatusBadRequest
			resp.Message = "Cannot extend workspace: " + err.Error()
			return err
		}

		if err := s.UpdateWorkspaceBuildByID(r.Context(), database.UpdateWorkspaceBuildByIDParams{
			ID:               build.ID,
			UpdatedAt:        build.UpdatedAt,
			ProvisionerState: build.ProvisionerState,
			Deadline:         newDeadline,
		}); err != nil {
			code = http.StatusInternalServerError
			resp.Message = "Failed to extend workspace deadline."
			return xerrors.Errorf("update workspace build: %w", err)
		}
		resp.Message = "Deadline updated to " + newDeadline.Format(time.RFC3339) + "."

		return nil
	})
	if err != nil {
		api.Logger.Info(r.Context(), "extending workspace", slog.Error(err))
	}
	httpapi.Write(rw, code, resp)
}

func (api *API) watchWorkspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	c, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Fix for Safari 15.1:
		// There is a bug in latest Safari in which compressed web socket traffic
		// isn't handled correctly. Turning off compression is a workaround:
		// https://github.com/nhooyr/websocket/issues/218
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		api.Logger.Warn(r.Context(), "accept websocket connection", slog.Error(err))
		return
	}
	defer c.Close(websocket.StatusInternalError, "internal error")

	// Makes the websocket connection write-only
	ctx := c.CloseRead(r.Context())

	// Send a heartbeat every 15 seconds to avoid the websocket being killed.
	go func() {
		ticker := time.NewTicker(time.Second * 15)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := c.Ping(ctx)
				if err != nil {
					return
				}
			}
		}
	}()

	t := time.NewTicker(time.Second * 1)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			workspace, err := api.Database.GetWorkspaceByID(r.Context(), workspace.ID)
			if err != nil {
				_ = wsjson.Write(ctx, c, codersdk.Response{
					Message: "Internal error fetching workspace.",
					Detail:  err.Error(),
				})
				return
			}
			build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
			if err != nil {
				_ = wsjson.Write(ctx, c, codersdk.Response{
					Message: "Internal error fetching workspace build.",
					Detail:  err.Error(),
				})
				return
			}
			var (
				group    errgroup.Group
				job      database.ProvisionerJob
				template database.Template
				users    []database.User
			)
			group.Go(func() (err error) {
				job, err = api.Database.GetProvisionerJobByID(r.Context(), build.JobID)
				return err
			})
			group.Go(func() (err error) {
				template, err = api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
				return err
			})
			group.Go(func() (err error) {
				users, err = api.Database.GetUsersByIDs(r.Context(), []uuid.UUID{workspace.OwnerID, build.InitiatorID})
				return err
			})
			err = group.Wait()
			if err != nil {
				_ = wsjson.Write(ctx, c, codersdk.Response{
					Message: "Internal error fetching resource.",
					Detail:  err.Error(),
				})
				return
			}

			_ = wsjson.Write(ctx, c, convertWorkspace(workspace, build, job, template,
				findUser(workspace.OwnerID, users), findUser(build.InitiatorID, users)))
		case <-ctx.Done():
			return
		}
	}
}

func convertWorkspaces(ctx context.Context, db database.Store, workspaces []database.Workspace) ([]codersdk.Workspace, error) {
	workspaceIDs := make([]uuid.UUID, 0, len(workspaces))
	templateIDs := make([]uuid.UUID, 0, len(workspaces))
	userIDs := make([]uuid.UUID, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
		templateIDs = append(templateIDs, workspace.TemplateID)
		userIDs = append(userIDs, workspace.OwnerID)
	}
	workspaceBuilds, err := db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, workspaceIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	for _, build := range workspaceBuilds {
		userIDs = append(userIDs, build.InitiatorID)
	}

	if err != nil {
		return nil, xerrors.Errorf("get workspace builds: %w", err)
	}
	templates, err := db.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
		Ids: templateIDs,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get templates: %w", err)
	}
	users, err := db.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, xerrors.Errorf("get users: %w", err)
	}
	jobIDs := make([]uuid.UUID, 0, len(workspaceBuilds))
	for _, build := range workspaceBuilds {
		jobIDs = append(jobIDs, build.JobID)
	}
	jobs, err := db.GetProvisionerJobsByIDs(ctx, jobIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get provisioner jobs: %w", err)
	}

	buildByWorkspaceID := map[uuid.UUID]database.WorkspaceBuild{}
	for _, workspaceBuild := range workspaceBuilds {
		buildByWorkspaceID[workspaceBuild.WorkspaceID] = database.WorkspaceBuild{
			ID:                workspaceBuild.ID,
			CreatedAt:         workspaceBuild.CreatedAt,
			UpdatedAt:         workspaceBuild.UpdatedAt,
			WorkspaceID:       workspaceBuild.WorkspaceID,
			TemplateVersionID: workspaceBuild.TemplateVersionID,
			Name:              workspaceBuild.Name,
			BuildNumber:       workspaceBuild.BuildNumber,
			Transition:        workspaceBuild.Transition,
			InitiatorID:       workspaceBuild.InitiatorID,
			ProvisionerState:  workspaceBuild.ProvisionerState,
			JobID:             workspaceBuild.JobID,
			Deadline:          workspaceBuild.Deadline,
			Reason:            workspaceBuild.Reason,
		}
	}
	templateByID := map[uuid.UUID]database.Template{}
	for _, template := range templates {
		templateByID[template.ID] = template
	}
	userByID := map[uuid.UUID]database.User{}
	for _, user := range users {
		userByID[user.ID] = user
	}
	jobByID := map[uuid.UUID]database.ProvisionerJob{}
	for _, job := range jobs {
		jobByID[job.ID] = job
	}
	apiWorkspaces := make([]codersdk.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		build, exists := buildByWorkspaceID[workspace.ID]
		if !exists {
			return nil, xerrors.Errorf("build not found for workspace %q", workspace.Name)
		}
		template, exists := templateByID[workspace.TemplateID]
		if !exists {
			return nil, xerrors.Errorf("template not found for workspace %q", workspace.Name)
		}
		job, exists := jobByID[build.JobID]
		if !exists {
			return nil, xerrors.Errorf("build job not found for workspace: %w", err)
		}
		owner, exists := userByID[workspace.OwnerID]
		if !exists {
			return nil, xerrors.Errorf("owner not found for workspace: %q", workspace.Name)
		}
		initiator, exists := userByID[build.InitiatorID]
		if !exists {
			return nil, xerrors.Errorf("build initiator not found for workspace: %q", workspace.Name)
		}
		apiWorkspaces = append(apiWorkspaces, convertWorkspace(workspace, build, job, template, &owner, &initiator))
	}
	return apiWorkspaces, nil
}

func convertWorkspace(
	workspace database.Workspace,
	workspaceBuild database.WorkspaceBuild,
	job database.ProvisionerJob,
	template database.Template,
	owner *database.User,
	initiator *database.User,
) codersdk.Workspace {
	var autostartSchedule *string
	if workspace.AutostartSchedule.Valid {
		autostartSchedule = &workspace.AutostartSchedule.String
	}

	ttlMillis := convertWorkspaceTTLMillis(workspace.Ttl)
	return codersdk.Workspace{
		ID:                workspace.ID,
		CreatedAt:         workspace.CreatedAt,
		UpdatedAt:         workspace.UpdatedAt,
		OwnerID:           workspace.OwnerID,
		OwnerName:         owner.Username,
		TemplateID:        workspace.TemplateID,
		LatestBuild:       convertWorkspaceBuild(owner, initiator, workspace, workspaceBuild, job),
		TemplateName:      template.Name,
		TemplateIcon:      template.Icon,
		Outdated:          workspaceBuild.TemplateVersionID.String() != template.ActiveVersionID.String(),
		Name:              workspace.Name,
		AutostartSchedule: autostartSchedule,
		TTLMillis:         ttlMillis,
	}
}

func convertWorkspaceTTLMillis(i sql.NullInt64) *int64 {
	if !i.Valid {
		return nil
	}

	millis := time.Duration(i.Int64).Milliseconds()
	return &millis
}

func validWorkspaceTTLMillis(millis *int64, max time.Duration) (sql.NullInt64, error) {
	if ptr.NilOrZero(millis) {
		return sql.NullInt64{}, nil
	}

	dur := time.Duration(*millis) * time.Millisecond
	truncated := dur.Truncate(time.Minute)
	if truncated < ttlMin {
		return sql.NullInt64{}, errTTLMin
	}

	if truncated > ttlMax {
		return sql.NullInt64{}, errTTLMax
	}

	// template level
	if max > 0 && truncated > max {
		return sql.NullInt64{}, xerrors.Errorf("time until shutdown must be below template maximum %s", max.String())
	}

	return sql.NullInt64{
		Valid: true,
		Int64: int64(truncated),
	}, nil
}

func validWorkspaceDeadline(startedAt, newDeadline time.Time, max time.Duration) error {
	soon := time.Now().Add(29 * time.Minute)
	if newDeadline.Before(soon) {
		return errDeadlineTooSoon
	}

	// No idea how this could happen.
	if newDeadline.Before(startedAt) {
		return errDeadlineBeforeStart
	}

	delta := newDeadline.Sub(startedAt)
	if delta > max {
		return errDeadlineOverTemplateMax
	}

	return nil
}

func validWorkspaceSchedule(s *string, min time.Duration) (sql.NullString, error) {
	if ptr.NilOrEmpty(s) {
		return sql.NullString{}, nil
	}

	sched, err := schedule.Weekly(*s)
	if err != nil {
		return sql.NullString{}, err
	}

	if schedMin := sched.Min(); schedMin < min {
		return sql.NullString{}, xerrors.Errorf("Minimum autostart interval %s below template minimum %s", schedMin, min)
	}

	return sql.NullString{
		Valid:  true,
		String: *s,
	}, nil
}

// workspaceSearchQuery takes a query string and returns the workspace filter.
// It also can return the list of validation errors to return to the api.
func workspaceSearchQuery(query string) (database.GetWorkspacesParams, []codersdk.ValidationError) {
	searchParams := make(url.Values)
	if query == "" {
		// No filter
		return database.GetWorkspacesParams{}, nil
	}
	query = strings.ToLower(query)
	// Because we do this in 2 passes, we want to maintain quotes on the first
	// pass.Further splitting occurs on the second pass and quotes will be
	// dropped.
	elements := splitQueryParameterByDelimiter(query, ' ', true)
	for _, element := range elements {
		parts := splitQueryParameterByDelimiter(element, ':', false)
		switch len(parts) {
		case 1:
			// No key:value pair. It is a workspace name, and maybe includes an owner
			parts = splitQueryParameterByDelimiter(element, '/', false)
			switch len(parts) {
			case 1:
				searchParams.Set("name", parts[0])
			case 2:
				searchParams.Set("owner", parts[0])
				searchParams.Set("name", parts[1])
			default:
				return database.GetWorkspacesParams{}, []codersdk.ValidationError{
					{Field: "q", Detail: fmt.Sprintf("Query element %q can only contain 1 '/'", element)},
				}
			}
		case 2:
			searchParams.Set(parts[0], parts[1])
		default:
			return database.GetWorkspacesParams{}, []codersdk.ValidationError{
				{Field: "q", Detail: fmt.Sprintf("Query element %q can only contain 1 ':'", element)},
			}
		}
	}

	// Using the query param parser here just returns consistent errors with
	// other parsing.
	parser := httpapi.NewQueryParamParser()
	filter := database.GetWorkspacesParams{
		Deleted:       false,
		OwnerUsername: parser.String(searchParams, "", "owner"),
		TemplateName:  parser.String(searchParams, "", "template"),
		Name:          parser.String(searchParams, "", "name"),
	}

	return filter, parser.Errors
}

// splitQueryParameterByDelimiter takes a query string and splits it into the individual elements
// of the query. Each element is separated by a delimiter. All quoted strings are
// kept as a single element.
//
// Although all our names cannot have spaces, that is a validation error.
// We should still parse the quoted string as a single value so that validation
// can properly fail on the space. If we do not, a value of `template:"my name"`
// will search `template:"my name:name"`, which produces an empty list instead of
// an error.
// nolint:revive
func splitQueryParameterByDelimiter(query string, delimiter rune, maintainQuotes bool) []string {
	quoted := false
	parts := strings.FieldsFunc(query, func(r rune) bool {
		if r == '"' {
			quoted = !quoted
		}
		return !quoted && r == delimiter
	})
	if !maintainQuotes {
		for i, part := range parts {
			parts[i] = strings.Trim(part, "\"")
		}
	}

	return parts
}
