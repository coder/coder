package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func (api *API) workspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(rw, r, rbac.ActionRead, workspace) {
		return
	}

	// The `deleted` query parameter (which defaults to `false`) MUST match the
	// `Deleted` field on the workspace otherwise you will get a 410 Gone.
	var (
		deletedStr  = r.URL.Query().Get("deleted")
		showDeleted = false
	)
	if deletedStr != "" {
		var err error
		showDeleted, err = strconv.ParseBool(deletedStr)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: fmt.Sprintf("Invalid boolean value %q for \"deleted\" query param", deletedStr),
				Validations: []httpapi.Error{
					{Field: "deleted", Detail: "Must be a valid boolean"},
				},
			})
			return
		}
	}
	if workspace.Deleted && !showDeleted {
		httpapi.Write(rw, http.StatusGone, httpapi.Response{
			Message: fmt.Sprintf("Workspace %q was deleted, you can view this workspace by specifying '?deleted=true' and trying again", workspace.ID.String()),
		})
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace build",
			Detail:  err.Error(),
		})
		return
	}
	var (
		group    errgroup.Group
		job      database.ProvisionerJob
		template database.Template
		owner    database.User
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
		owner, err = api.Database.GetUserByID(r.Context(), workspace.OwnerID)
		return err
	})
	err = group.Wait()
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching resource",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspace(workspace, build, job, template, owner))
}

// workspaces returns all workspaces a user can read.
// Optional filters with query params
func (api *API) workspaces(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)

	// Empty strings mean no filter
	orgFilter := r.URL.Query().Get("organization_id")
	ownerFilter := r.URL.Query().Get("owner")
	nameFilter := r.URL.Query().Get("name")

	filter := database.GetWorkspacesWithFilterParams{Deleted: false}
	if orgFilter != "" {
		orgID, err := uuid.Parse(orgFilter)
		if err == nil {
			filter.OrganizationID = orgID
		}
	}
	if ownerFilter == "me" {
		filter.OwnerID = apiKey.UserID
	} else if ownerFilter != "" {
		userID, err := uuid.Parse(ownerFilter)
		if err != nil {
			// Maybe it's a username
			user, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
				// Why not just accept 1 arg and use it for both in the sql?
				Username: ownerFilter,
				Email:    ownerFilter,
			})
			if err == nil {
				filter.OwnerID = user.ID
			}
		} else {
			filter.OwnerID = userID
		}
	}
	if nameFilter != "" {
		filter.Name = nameFilter
	}

	workspaces, err := api.Database.GetWorkspacesWithFilter(r.Context(), filter)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspaces",
			Detail:  err.Error(),
		})
		return
	}

	// Only return workspaces the user can read
	workspaces = AuthorizeFilter(api, r, rbac.ActionRead, workspaces)

	apiWorkspaces, err := convertWorkspaces(r.Context(), api.Database, workspaces)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("convert workspaces: %s", err),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, apiWorkspaces)
}

func (api *API) workspaceByOwnerAndName(rw http.ResponseWriter, r *http.Request) {
	owner := httpmw.UserParam(r)
	workspaceName := chi.URLParam(r, "workspacename")

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: owner.ID,
		Name:    workspaceName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		// Do not leak information if the workspace exists or not
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace by name",
			Detail:  err.Error(),
		})
		return
	}
	if !api.Authorize(rw, r, rbac.ActionRead, workspace) {
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace build",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), build.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching provisioner job",
			Detail:  err.Error(),
		})
		return
	}
	template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching template",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspace(workspace, build, job, template, owner))
}

// Create a new workspace for the currently authenticated user.
func (api *API) postWorkspacesByOrganization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(rw, r, rbac.ActionCreate,
		rbac.ResourceWorkspace.InOrg(organization.ID).WithOwner(apiKey.UserID.String())) {
		return
	}

	var createWorkspace codersdk.CreateWorkspaceRequest
	if !httpapi.Read(rw, r, &createWorkspace) {
		return
	}

	template, err := api.Database.GetTemplateByID(r.Context(), createWorkspace.TemplateID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("Template %q doesn't exist", createWorkspace.TemplateID.String()),
			Validations: []httpapi.Error{{
				Field:  "template_id",
				Detail: "template not found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching template",
			Detail:  err.Error(),
		})
		return
	}

	if organization.ID != template.OrganizationID {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("Template is not in organization %q", organization.Name),
		})
		return
	}
	_, err = api.Database.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: template.OrganizationID,
		UserID:         apiKey.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "You aren't allowed to access templates in that organization",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching organization member",
			Detail:  err.Error(),
		})
		return
	}

	dbAutostartSchedule, err := validWorkspaceSchedule(createWorkspace.AutostartSchedule, time.Duration(template.MinAutostartInterval))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message:     "Invalid Autostart Schedule",
			Validations: []httpapi.Error{{Field: "schedule", Detail: err.Error()}},
		})
		return
	}

	dbTTL, err := validWorkspaceTTLMillis(createWorkspace.TTLMillis, time.Duration(template.MaxTtl))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message:     "Invalid Workspace TTL",
			Validations: []httpapi.Error{{Field: "ttl_ms", Detail: err.Error()}},
		})
		return
	}

	if !dbTTL.Valid {
		// Default to template maximum when creating a new workspace
		dbTTL = sql.NullInt64{Valid: true, Int64: template.MaxTtl}
	}

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: apiKey.UserID,
		Name:    createWorkspace.Name,
	})
	if err == nil {
		// If the workspace already exists, don't allow creation.
		template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("Find template for conflicting workspace name %q", createWorkspace.Name),
				Detail:  err.Error(),
			})
			return
		}
		// The template is fetched for clarity to the user on where the conflicting name may be.
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("Workspace %q already exists in the %q template", createWorkspace.Name, template.Name),
			Validations: []httpapi.Error{{
				Field:  "name",
				Detail: "this value is already in use and should be unique",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("Internal error fetching workspace by name %q", createWorkspace.Name),
			Detail:  err.Error(),
		})
		return
	}

	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), template.ActiveVersionID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching template version",
			Detail:  err.Error(),
		})
		return
	}
	templateVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching template version job",
			Detail:  err.Error(),
		})
		return
	}
	templateVersionJobStatus := convertProvisionerJob(templateVersionJob).Status
	switch templateVersionJobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		httpapi.Write(rw, http.StatusNotAcceptable, httpapi.Response{
			Message: fmt.Sprintf("The provided template version is %s. Wait for it to complete importing!", templateVersionJobStatus),
		})
		return
	case codersdk.ProvisionerJobFailed:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: fmt.Sprintf("The provided template version %q has failed to import. You cannot create workspaces using it!", templateVersion.Name),
		})
		return
	case codersdk.ProvisionerJobCanceled:
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
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
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error creating workspace",
			Detail:  err.Error(),
		})
		return
	}
	user, err := api.Database.GetUserByID(r.Context(), apiKey.UserID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching user",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertWorkspace(workspace, workspaceBuild, templateVersionJob, template, user))
}

func (api *API) putWorkspaceAutostart(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(rw, r, rbac.ActionUpdate, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	var req codersdk.UpdateWorkspaceAutostartRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
	if err != nil {
		api.Logger.Error(r.Context(), "fetch workspace template", slog.F("workspace_id", workspace.ID), slog.F("template_id", workspace.TemplateID), slog.Error(err))
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Error fetching workspace template",
		})
	}

	dbSched, err := validWorkspaceSchedule(req.Schedule, time.Duration(template.MinAutostartInterval))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message:     "Invalid autostart schedule",
			Validations: []httpapi.Error{{Field: "schedule", Detail: err.Error()}},
		})
		return
	}

	err = api.Database.UpdateWorkspaceAutostart(r.Context(), database.UpdateWorkspaceAutostartParams{
		ID:                workspace.ID,
		AutostartSchedule: dbSched,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error updating workspace autostart schedule",
			Detail:  err.Error(),
		})
		return
	}
}

func (api *API) putWorkspaceTTL(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(rw, r, rbac.ActionUpdate, rbac.ResourceWorkspace.
		InOrg(workspace.OrganizationID).WithOwner(workspace.OwnerID.String()).WithID(workspace.ID.String())) {
		return
	}

	var req codersdk.UpdateWorkspaceTTLRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Error fetching workspace template!",
		})
		return
	}

	dbTTL, err := validWorkspaceTTLMillis(req.TTLMillis, time.Duration(template.MaxTtl))
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Invalid workspace TTL",
			Detail:  err.Error(),
			Validations: []httpapi.Error{
				{
					Field:  "ttl_ms",
					Detail: err.Error(),
				},
			},
		})
		return
	}

	err = api.Database.UpdateWorkspaceTTL(r.Context(), database.UpdateWorkspaceTTLParams{
		ID:  workspace.ID,
		Ttl: dbTTL,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error updating workspace TTL",
			Detail:  err.Error(),
		})
		return
	}
}

func (api *API) putExtendWorkspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)

	if !api.Authorize(rw, r, rbac.ActionUpdate, workspace) {
		return
	}

	var req codersdk.PutExtendWorkspaceRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	code := http.StatusOK
	resp := httpapi.Response{}

	err := api.Database.InTx(func(s database.Store) error {
		build, err := s.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
		if err != nil {
			code = http.StatusInternalServerError
			resp.Message = "workspace not found"
			return xerrors.Errorf("get latest workspace build: %w", err)
		}

		if build.Transition != database.WorkspaceTransitionStart {
			code = http.StatusConflict
			resp.Message = "workspace must be started, current status: " + string(build.Transition)
			return xerrors.Errorf("workspace must be started, current status: %s", build.Transition)
		}

		newDeadline := req.Deadline.UTC()
		if err := validWorkspaceDeadline(build.Deadline, newDeadline); err != nil {
			code = http.StatusBadRequest
			resp.Message = "bad extend workspace request"
			resp.Validations = append(resp.Validations, httpapi.Error{Field: "deadline", Detail: err.Error()})
			return err
		}

		if err := s.UpdateWorkspaceBuildByID(r.Context(), database.UpdateWorkspaceBuildByIDParams{
			ID:               build.ID,
			UpdatedAt:        build.UpdatedAt,
			ProvisionerState: build.ProvisionerState,
			Deadline:         newDeadline,
		}); err != nil {
			code = http.StatusInternalServerError
			resp.Message = "failed to extend workspace deadline"
			return xerrors.Errorf("update workspace build: %w", err)
		}
		resp.Message = "deadline updated to " + newDeadline.Format(time.RFC3339)

		return nil
	})

	if err != nil {
		api.Logger.Info(r.Context(), "extending workspace", slog.Error(err))
	}
	httpapi.Write(rw, code, resp)
}

func (api *API) watchWorkspace(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(rw, r, rbac.ActionRead, workspace) {
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
				_ = wsjson.Write(ctx, c, httpapi.Response{
					Message: "Internal error fetching workspace",
					Detail:  err.Error(),
				})
				return
			}
			build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
			if err != nil {
				_ = wsjson.Write(ctx, c, httpapi.Response{
					Message: "Internal error fetching workspace build",
					Detail:  err.Error(),
				})
				return
			}
			var (
				group    errgroup.Group
				job      database.ProvisionerJob
				template database.Template
				owner    database.User
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
				owner, err = api.Database.GetUserByID(r.Context(), workspace.OwnerID)
				return err
			})
			err = group.Wait()
			if err != nil {
				_ = wsjson.Write(ctx, c, httpapi.Response{
					Message: "Internal error fetching resource",
					Detail:  err.Error(),
				})
				return
			}

			_ = wsjson.Write(ctx, c, convertWorkspace(workspace, build, job, template, owner))
		case <-ctx.Done():
			return
		}
	}
}

func convertWorkspaces(ctx context.Context, db database.Store, workspaces []database.Workspace) ([]codersdk.Workspace, error) {
	workspaceIDs := make([]uuid.UUID, 0, len(workspaces))
	templateIDs := make([]uuid.UUID, 0, len(workspaces))
	ownerIDs := make([]uuid.UUID, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
		templateIDs = append(templateIDs, workspace.TemplateID)
		ownerIDs = append(ownerIDs, workspace.OwnerID)
	}
	workspaceBuilds, err := db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, workspaceIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get workspace builds: %w", err)
	}
	templates, err := db.GetTemplatesByIDs(ctx, templateIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get templates: %w", err)
	}
	users, err := db.GetUsersByIDs(ctx, ownerIDs)
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
		user, exists := userByID[workspace.OwnerID]
		if !exists {
			return nil, xerrors.Errorf("owner not found for workspace: %q", workspace.Name)
		}
		apiWorkspaces = append(apiWorkspaces, convertWorkspace(workspace, build, job, template, user))
	}
	return apiWorkspaces, nil
}
func convertWorkspace(
	workspace database.Workspace,
	workspaceBuild database.WorkspaceBuild,
	job database.ProvisionerJob,
	template database.Template,
	owner database.User) codersdk.Workspace {
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
		LatestBuild:       convertWorkspaceBuild(workspace, workspaceBuild, job),
		TemplateName:      template.Name,
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
	if truncated < time.Minute {
		return sql.NullInt64{}, xerrors.New("ttl must be at least one minute")
	}

	if truncated > 24*7*time.Hour {
		return sql.NullInt64{}, xerrors.New("ttl must be less than 7 days")
	}

	if truncated > max {
		return sql.NullInt64{}, xerrors.Errorf("ttl must be below template maximum %s", max.String())
	}

	return sql.NullInt64{
		Valid: true,
		Int64: int64(truncated),
	}, nil
}

func validWorkspaceDeadline(old, new time.Time) error {
	if old.IsZero() {
		return xerrors.New("nothing to do: no existing deadline set")
	}

	now := time.Now()
	if new.Before(now) {
		return xerrors.New("new deadline must be in the future")
	}

	delta := new.Sub(old)
	if delta < time.Minute {
		return xerrors.New("minimum extension is one minute")
	}

	if delta > 24*time.Hour {
		return xerrors.New("maximum extension is 24 hours")
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
