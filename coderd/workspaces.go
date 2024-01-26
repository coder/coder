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
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

var (
	ttlMin = time.Minute //nolint:revive // min here means 'minimum' not 'minutes'
	ttlMax = 30 * 24 * time.Hour

	errTTLMin              = xerrors.New("time until shutdown must be at least one minute")
	errTTLMax              = xerrors.New("time until shutdown must be less than 30 days")
	errDeadlineTooSoon     = xerrors.New("new deadline must be at least 30 minutes in the future")
	errDeadlineBeforeStart = xerrors.New("new deadline must be before workspace start time")
)

// @Summary Get workspace metadata by ID
// @ID get-workspace-metadata-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param include_deleted query bool false "Return data instead of HTTP 404 if the workspace is deleted"
// @Success 200 {object} codersdk.Workspace
// @Router /workspaces/{workspace} [get]
func (api *API) workspace(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	apiKey := httpmw.APIKey(r)

	var (
		deletedStr  = r.URL.Query().Get("include_deleted")
		showDeleted = false
	)
	if deletedStr != "" {
		var err error
		showDeleted, err = strconv.ParseBool(deletedStr)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid boolean value %q for \"include_deleted\" query param.", deletedStr),
				Validations: []codersdk.ValidationError{
					{Field: "deleted", Detail: "Must be a valid boolean"},
				},
			})
			return
		}
	}
	if workspace.Deleted && !showDeleted {
		httpapi.Write(ctx, rw, http.StatusGone, codersdk.Response{
			Message: fmt.Sprintf("Workspace %q was deleted, you can view this workspace by specifying '?deleted=true' and trying again.", workspace.ID.String()),
		})
		return
	}

	data, err := api.workspaceData(ctx, []database.Workspace{workspace})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}

	if len(data.templates) == 0 {
		httpapi.Forbidden(rw)
		return
	}
	ownerName, ok := usernameWithID(workspace.OwnerID, data.users)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  "unable to find workspace owner's username",
		})
		return
	}

	w, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		data.builds[0],
		data.templates[0],
		ownerName,
		api.Options.AllowWorkspaceRenames,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, w)
}

// workspaces returns all workspaces a user can read.
// Optional filters with query params
//
// @Summary List workspaces
// @ID list-workspaces
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param q query string false "Search query in the format `key:value`. Available keys are: owner, template, name, status, has-agent, dormant, last_used_after, last_used_before."
// @Param limit query int false "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.WorkspacesResponse
// @Router /workspaces [get]
func (api *API) workspaces(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	page, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.Workspaces(queryStr, page, api.AgentInactiveDisconnectTimeout)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid workspace search query.",
			Validations: errs,
		})
		return
	}

	if filter.OwnerUsername == "me" {
		filter.OwnerID = apiKey.UserID
		filter.OwnerUsername = ""
	}

	// Workspaces do not have ACL columns.
	prepared, err := api.HTTPAuth.AuthorizeSQLFilter(r, rbac.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error preparing sql filter.",
			Detail:  err.Error(),
		})
		return
	}

	// To show the requester's favorite workspaces first, we pass their userID and compare it to
	// the workspace owner_id when ordering the rows.
	filter.RequesterID = apiKey.UserID

	workspaceRows, err := api.Database.GetAuthorizedWorkspaces(ctx, filter, prepared)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  err.Error(),
		})
		return
	}
	if len(workspaceRows) == 0 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspacesResponse{
			Workspaces: []codersdk.Workspace{},
			Count:      0,
		})
		return
	}

	workspaces := database.ConvertWorkspaceRows(workspaceRows)

	data, err := api.workspaceData(ctx, workspaces)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}

	wss, err := convertWorkspaces(apiKey.UserID, workspaces, data)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspaces.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspacesResponse{
		Workspaces: wss,
		Count:      int(workspaceRows[0].Count),
	})
}

// @Summary Get workspace metadata by user and workspace name
// @ID get-workspace-metadata-by-user-and-workspace-name
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param user path string true "User ID, name, or me"
// @Param workspacename path string true "Workspace name"
// @Param include_deleted query bool false "Return data instead of HTTP 404 if the workspace is deleted"
// @Success 200 {object} codersdk.Workspace
// @Router /users/{user}/workspace/{workspacename} [get]
func (api *API) workspaceByOwnerAndName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	owner := httpmw.UserParam(r)
	workspaceName := chi.URLParam(r, "workspacename")
	apiKey := httpmw.APIKey(r)

	includeDeleted := false
	if s := r.URL.Query().Get("include_deleted"); s != "" {
		var err error
		includeDeleted, err = strconv.ParseBool(s)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid boolean value %q for \"include_deleted\" query param.", s),
				Validations: []codersdk.ValidationError{
					{Field: "include_deleted", Detail: "Must be a valid boolean"},
				},
			})
			return
		}
	}

	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: owner.ID,
		Name:    workspaceName,
	})
	if includeDeleted && errors.Is(err, sql.ErrNoRows) {
		workspace, err = api.Database.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: owner.ID,
			Name:    workspaceName,
			Deleted: includeDeleted,
		})
	}
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

	data, err := api.workspaceData(ctx, []database.Workspace{workspace})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}

	if len(data.builds) == 0 || len(data.templates) == 0 {
		httpapi.ResourceNotFound(rw)
		return
	}
	ownerName, ok := usernameWithID(workspace.OwnerID, data.users)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  "unable to find workspace owner's username",
		})
		return
	}
	w, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		data.builds[0],
		data.templates[0],
		ownerName,
		api.Options.AllowWorkspaceRenames,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, w)
}

// Create a new workspace for the currently authenticated user.
//
// @Summary Create user workspace by organization
// @ID create-user-workspace-by-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Workspaces
// @Param organization path string true "Organization ID" format(uuid)
// @Param user path string true "Username, UUID, or me"
// @Param request body codersdk.CreateWorkspaceRequest true "Create workspace request"
// @Success 200 {object} codersdk.Workspace
// @Router /organizations/{organization}/members/{user}/workspaces [post]
func (api *API) postWorkspacesByOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx                   = r.Context()
		organization          = httpmw.OrganizationParam(r)
		apiKey                = httpmw.APIKey(r)
		auditor               = api.Auditor.Load()
		member                = httpmw.OrganizationMemberParam(r)
		workspaceResourceInfo = audit.AdditionalFields{
			WorkspaceOwner: member.Username,
		}
	)

	wriBytes, err := json.Marshal(workspaceResourceInfo)
	if err != nil {
		api.Logger.Warn(ctx, "marshal workspace owner name")
	}

	aReq, commitAudit := audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
		Audit:            *auditor,
		Log:              api.Logger,
		Request:          r,
		Action:           database.AuditActionCreate,
		AdditionalFields: wriBytes,
	})

	defer commitAudit()

	// Do this upfront to save work.
	if !api.Authorize(r, rbac.ActionCreate,
		rbac.ResourceWorkspace.InOrg(organization.ID).WithOwner(member.UserID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var createWorkspace codersdk.CreateWorkspaceRequest
	if !httpapi.Read(ctx, rw, r, &createWorkspace) {
		return
	}

	// If we were given a `TemplateVersionID`, we need to determine the `TemplateID` from it.
	templateID := createWorkspace.TemplateID
	if templateID == uuid.Nil {
		templateVersion, err := api.Database.GetTemplateVersionByID(ctx, createWorkspace.TemplateVersionID)
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Template version %q doesn't exist.", templateID.String()),
				Validations: []codersdk.ValidationError{{
					Field:  "template_version_id",
					Detail: "template not found",
				}},
			})
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching template version.",
				Detail:  err.Error(),
			})
			return
		}
		if templateVersion.Archived {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Archived template versions cannot be used to make a workspace.",
				Validations: []codersdk.ValidationError{
					{
						Field:  "template_version_id",
						Detail: "template version archived",
					},
				},
			})
			return
		}

		templateID = templateVersion.TemplateID.UUID
	}

	template, err := api.Database.GetTemplateByID(ctx, templateID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Template %q doesn't exist.", templateID.String()),
			Validations: []codersdk.ValidationError{{
				Field:  "template_id",
				Detail: "template not found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return
	}
	if template.Deleted {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Template %q has been deleted!", template.Name),
		})
		return
	}

	templateAccessControl := (*(api.AccessControlStore.Load())).GetTemplateAccessControl(template)
	if templateAccessControl.IsDeprecated() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Template %q has been deprecated, and cannot be used to create a new workspace.", template.Name),
			// Pass the deprecated message to the user.
			Detail:      templateAccessControl.Deprecated,
			Validations: nil,
		})
		return
	}

	if organization.ID != template.OrganizationID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Template is not in organization %q.", organization.Name),
		})
		return
	}

	dbAutostartSchedule, err := validWorkspaceSchedule(createWorkspace.AutostartSchedule)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid Autostart Schedule.",
			Validations: []codersdk.ValidationError{{Field: "schedule", Detail: err.Error()}},
		})
		return
	}

	templateSchedule, err := (*api.TemplateScheduleStore.Load()).Get(ctx, api.Database, template.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template schedule.",
			Detail:  err.Error(),
		})
		return
	}

	maxTTL := templateSchedule.MaxTTL
	if !templateSchedule.UseMaxTTL {
		// If we're using autostop requirements, there isn't a max TTL.
		maxTTL = 0
	}

	dbTTL, err := validWorkspaceTTLMillis(createWorkspace.TTLMillis, templateSchedule.DefaultTTL, maxTTL)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid Workspace Time to Shutdown.",
			Validations: []codersdk.ValidationError{{Field: "ttl_ms", Detail: err.Error()}},
		})
		return
	}

	// back-compatibility: default to "never" if not included.
	dbAU := database.AutomaticUpdatesNever
	if createWorkspace.AutomaticUpdates != "" {
		dbAU, err = validWorkspaceAutomaticUpdates(createWorkspace.AutomaticUpdates)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid Workspace Automatic Updates setting.",
				Validations: []codersdk.ValidationError{{Field: "automatic_updates", Detail: err.Error()}},
			})
			return
		}
	}

	// TODO: This should be a system call as the actor might not be able to
	// read other workspaces. Ideally we check the error on create and look for
	// a postgres conflict error.
	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: member.UserID,
		Name:    createWorkspace.Name,
	})
	if err == nil {
		// If the workspace already exists, don't allow creation.
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Workspace %q already exists.", createWorkspace.Name),
			Validations: []codersdk.ValidationError{{
				Field:  "name",
				Detail: "This value is already in use and should be unique.",
			}},
		})
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("Internal error fetching workspace by name %q.", createWorkspace.Name),
			Detail:  err.Error(),
		})
		return
	}

	var (
		provisionerJob *database.ProvisionerJob
		workspaceBuild *database.WorkspaceBuild
	)
	err = api.Database.InTx(func(db database.Store) error {
		now := dbtime.Now()
		// Workspaces are created without any versions.
		workspace, err = db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:                uuid.New(),
			CreatedAt:         now,
			UpdatedAt:         now,
			OwnerID:           member.UserID,
			OrganizationID:    template.OrganizationID,
			TemplateID:        template.ID,
			Name:              createWorkspace.Name,
			AutostartSchedule: dbAutostartSchedule,
			Ttl:               dbTTL,
			// The workspaces page will sort by last used at, and it's useful to
			// have the newly created workspace at the top of the list!
			LastUsedAt:       dbtime.Now(),
			AutomaticUpdates: dbAU,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace: %w", err)
		}

		builder := wsbuilder.New(workspace, database.WorkspaceTransitionStart).
			Reason(database.BuildReasonInitiator).
			Initiator(apiKey.UserID).
			ActiveVersion().
			RichParameterValues(createWorkspace.RichParameterValues)
		if createWorkspace.TemplateVersionID != uuid.Nil {
			builder = builder.VersionID(createWorkspace.TemplateVersionID)
		}

		workspaceBuild, provisionerJob, err = builder.Build(
			ctx,
			db,
			func(action rbac.Action, object rbac.Objecter) bool {
				return api.Authorize(r, action, object)
			},
			audit.WorkspaceBuildBaggageFromRequest(r),
		)
		return err
	}, nil)
	var bldErr wsbuilder.BuildError
	if xerrors.As(err, &bldErr) {
		httpapi.Write(ctx, rw, bldErr.Status, codersdk.Response{
			Message: bldErr.Message,
			Detail:  bldErr.Error(),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating workspace.",
			Detail:  err.Error(),
		})
		return
	}
	err = provisionerjobs.PostJob(api.Pubsub, *provisionerJob)
	if err != nil {
		// Client probably doesn't care about this error, so just log it.
		api.Logger.Error(ctx, "failed to post provisioner job to pubsub", slog.Error(err))
	}
	aReq.New = workspace

	api.Telemetry.Report(&telemetry.Snapshot{
		Workspaces:      []telemetry.Workspace{telemetry.ConvertWorkspace(workspace)},
		WorkspaceBuilds: []telemetry.WorkspaceBuild{telemetry.ConvertWorkspaceBuild(*workspaceBuild)},
	})

	apiBuild, err := api.convertWorkspaceBuild(
		*workspaceBuild,
		workspace,
		database.GetProvisionerJobsByIDsWithQueuePositionRow{
			ProvisionerJob: *provisionerJob,
			QueuePosition:  0,
		},
		member.Username,
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

	w, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		apiBuild,
		template,
		member.Username,
		api.Options.AllowWorkspaceRenames,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, w)
}

// @Summary Update workspace metadata by ID
// @ID update-workspace-metadata-by-id
// @Security CoderSessionToken
// @Accept json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.UpdateWorkspaceRequest true "Metadata update request"
// @Success 204
// @Router /workspaces/{workspace} [patch]
func (api *API) patchWorkspace(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		workspace         = httpmw.WorkspaceParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = workspace

	var req codersdk.UpdateWorkspaceRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == "" || req.Name == workspace.Name {
		aReq.New = workspace
		// Nothing changed, optionally this could be an error.
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	// The reason we double check here is in case more fields can be
	// patched in the future, it's enough if one changes.
	name := workspace.Name
	if req.Name != "" || req.Name != workspace.Name {
		if !api.Options.AllowWorkspaceRenames {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Workspace renames are not allowed.",
			})
			return
		}
		name = req.Name
	}

	newWorkspace, err := api.Database.UpdateWorkspace(ctx, database.UpdateWorkspaceParams{
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
			httpapi.Write(ctx, rw, http.StatusMethodNotAllowed, codersdk.Response{
				Message: fmt.Sprintf("Workspace %q is deleted and cannot be updated.", workspace.Name),
			})
			return
		}
		// Check if the name was already in use.
		if database.IsUniqueViolation(err) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("Workspace %q already exists.", req.Name),
				Validations: []codersdk.ValidationError{{
					Field:  "name",
					Detail: "This value is already in use and should be unique.",
				}},
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace.",
			Detail:  err.Error(),
		})
		return
	}

	api.publishWorkspaceUpdate(ctx, workspace.ID)

	aReq.New = newWorkspace
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Update workspace autostart schedule by ID
// @ID update-workspace-autostart-schedule-by-id
// @Security CoderSessionToken
// @Accept json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.UpdateWorkspaceAutostartRequest true "Schedule update request"
// @Success 204
// @Router /workspaces/{workspace}/autostart [put]
func (api *API) putWorkspaceAutostart(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		workspace         = httpmw.WorkspaceParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = workspace

	var req codersdk.UpdateWorkspaceAutostartRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	dbSched, err := validWorkspaceSchedule(req.Schedule)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid autostart schedule.",
			Validations: []codersdk.ValidationError{{Field: "schedule", Detail: err.Error()}},
		})
		return
	}

	// Check if the template allows users to configure autostart.
	templateSchedule, err := (*api.TemplateScheduleStore.Load()).Get(ctx, api.Database, workspace.TemplateID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting template schedule options.",
			Detail:  err.Error(),
		})
		return
	}
	if !templateSchedule.UserAutostartEnabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Autostart is not allowed for workspaces using this template.",
			Validations: []codersdk.ValidationError{{Field: "schedule", Detail: "Autostart is not allowed for workspaces using this template."}},
		})
		return
	}

	err = api.Database.UpdateWorkspaceAutostart(ctx, database.UpdateWorkspaceAutostartParams{
		ID:                workspace.ID,
		AutostartSchedule: dbSched,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace autostart schedule.",
			Detail:  err.Error(),
		})
		return
	}

	newWorkspace := workspace
	newWorkspace.AutostartSchedule = dbSched
	aReq.New = newWorkspace

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Update workspace TTL by ID
// @ID update-workspace-ttl-by-id
// @Security CoderSessionToken
// @Accept json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.UpdateWorkspaceTTLRequest true "Workspace TTL update request"
// @Success 204
// @Router /workspaces/{workspace}/ttl [put]
func (api *API) putWorkspaceTTL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		workspace         = httpmw.WorkspaceParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = workspace

	var req codersdk.UpdateWorkspaceTTLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var dbTTL sql.NullInt64

	err := api.Database.InTx(func(s database.Store) error {
		templateSchedule, err := (*api.TemplateScheduleStore.Load()).Get(ctx, s, workspace.TemplateID)
		if err != nil {
			return xerrors.Errorf("get template schedule: %w", err)
		}
		if !templateSchedule.UserAutostopEnabled {
			return codersdk.ValidationError{Field: "ttl_ms", Detail: "Custom autostop TTL is not allowed for workspaces using this template."}
		}

		maxTTL := templateSchedule.MaxTTL
		if !templateSchedule.UseMaxTTL {
			// If we're using autostop requirements, there isn't a max TTL.
			maxTTL = 0
		}

		// don't override 0 ttl with template default here because it indicates
		// disabled autostop
		var validityErr error
		dbTTL, validityErr = validWorkspaceTTLMillis(req.TTLMillis, 0, maxTTL)
		if validityErr != nil {
			return codersdk.ValidationError{Field: "ttl_ms", Detail: validityErr.Error()}
		}
		if err := s.UpdateWorkspaceTTL(ctx, database.UpdateWorkspaceTTLParams{
			ID:  workspace.ID,
			Ttl: dbTTL,
		}); err != nil {
			return xerrors.Errorf("update workspace time until shutdown: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		resp := codersdk.Response{
			Message: "Error updating workspace time until shutdown.",
		}
		var validErr codersdk.ValidationError
		if errors.As(err, &validErr) {
			resp.Validations = []codersdk.ValidationError{validErr}
			httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
			return
		}

		resp.Detail = err.Error()
		httpapi.Write(ctx, rw, http.StatusInternalServerError, resp)
		return
	}

	newWorkspace := workspace
	newWorkspace.Ttl = dbTTL
	aReq.New = newWorkspace

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Update workspace dormancy status by id.
// @ID update-workspace-dormancy-status-by-id
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.UpdateWorkspaceDormancy true "Make a workspace dormant or active"
// @Success 200 {object} codersdk.Workspace
// @Router /workspaces/{workspace}/dormant [put]
func (api *API) putWorkspaceDormant(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		workspace         = httpmw.WorkspaceParam(r)
		apiKey            = httpmw.APIKey(r)
		oldWorkspace      = workspace
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	aReq.Old = oldWorkspace
	defer commitAudit()

	var req codersdk.UpdateWorkspaceDormancy
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// If the workspace is already in the desired state do nothing!
	if workspace.DormantAt.Valid == req.Dormant {
		httpapi.Write(ctx, rw, http.StatusNotModified, codersdk.Response{
			Message: "Nothing to do!",
		})
		return
	}

	dormantAt := sql.NullTime{
		Valid: req.Dormant,
	}
	if req.Dormant {
		dormantAt.Time = dbtime.Now()
	}

	workspace, err := api.Database.UpdateWorkspaceDormantDeletingAt(ctx, database.UpdateWorkspaceDormantDeletingAtParams{
		ID:        workspace.ID,
		DormantAt: dormantAt,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace locked status.",
			Detail:  err.Error(),
		})
		return
	}

	data, err := api.workspaceData(ctx, []database.Workspace{workspace})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}
	ownerName, ok := usernameWithID(workspace.OwnerID, data.users)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  "unable to find workspace owner's username",
		})
		return
	}

	if len(data.templates) == 0 {
		httpapi.Forbidden(rw)
		return
	}

	aReq.New = workspace

	w, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		data.builds[0],
		data.templates[0],
		ownerName,
		api.Options.AllowWorkspaceRenames,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, w)
}

// @Summary Extend workspace deadline by ID
// @ID extend-workspace-deadline-by-id
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.PutExtendWorkspaceRequest true "Extend deadline update request"
// @Success 200 {object} codersdk.Response
// @Router /workspaces/{workspace}/extend [put]
func (api *API) putExtendWorkspace(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)

	var req codersdk.PutExtendWorkspaceRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	code := http.StatusOK
	resp := codersdk.Response{}

	err := api.Database.InTx(func(s database.Store) error {
		build, err := s.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
		if err != nil {
			code = http.StatusInternalServerError
			resp.Message = "Error fetching workspace build."
			return xerrors.Errorf("get latest workspace build: %w", err)
		}

		job, err := s.GetProvisionerJobByID(ctx, build.JobID)
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
		if err := validWorkspaceDeadline(job.CompletedAt.Time, newDeadline); err != nil {
			// NOTE(Cian): Putting the error in the Message field on request from the FE folks.
			// Normally, we would put the validation error in Validations, but this endpoint is
			// not tied to a form or specific named user input on the FE.
			code = http.StatusBadRequest
			resp.Message = "Cannot extend workspace: " + err.Error()
			return err
		}
		if !build.MaxDeadline.IsZero() && newDeadline.After(build.MaxDeadline) {
			code = http.StatusBadRequest
			resp.Message = "Cannot extend workspace beyond max deadline."
			return xerrors.New("Cannot extend workspace: deadline is beyond max deadline imposed by template")
		}

		if err := s.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
			ID:          build.ID,
			UpdatedAt:   dbtime.Now(),
			Deadline:    newDeadline,
			MaxDeadline: build.MaxDeadline,
		}); err != nil {
			code = http.StatusInternalServerError
			resp.Message = "Failed to extend workspace deadline."
			return xerrors.Errorf("update workspace build: %w", err)
		}
		resp.Message = "Deadline updated to " + newDeadline.Format(time.RFC3339) + "."

		return nil
	}, nil)
	if err != nil {
		api.Logger.Info(ctx, "extending workspace", slog.Error(err))
	}
	api.publishWorkspaceUpdate(ctx, workspace.ID)
	httpapi.Write(ctx, rw, code, resp)
}

// @Summary Favorite workspace by ID.
// @ID favorite-workspace-by-id
// @Security CoderSessionToken
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 204
// @Router /workspaces/{workspace}/favorite [put]
func (api *API) putFavoriteWorkspace(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
		workspace = httpmw.WorkspaceParam(r)
		apiKey    = httpmw.APIKey(r)
		auditor   = api.Auditor.Load()
	)

	if apiKey.UserID != workspace.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You can only favorite workspaces that you own.",
		})
		return
	}

	aReq, commitAudit := audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()
	aReq.Old = workspace

	err := api.Database.FavoriteWorkspace(ctx, workspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error setting workspace as favorite",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = workspace
	aReq.New.Favorite = true

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Unfavorite workspace by ID.
// @ID unfavorite-workspace-by-id
// @Security CoderSessionToken
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 204
// @Router /workspaces/{workspace}/favorite [delete]
func (api *API) deleteFavoriteWorkspace(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
		workspace = httpmw.WorkspaceParam(r)
		apiKey    = httpmw.APIKey(r)
		auditor   = api.Auditor.Load()
	)

	if apiKey.UserID != workspace.OwnerID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You can only un-favorite workspaces that you own.",
		})
		return
	}

	aReq, commitAudit := audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})

	defer commitAudit()
	aReq.Old = workspace

	err := api.Database.UnfavoriteWorkspace(ctx, workspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unsetting workspace as favorite",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = workspace
	aReq.New.Favorite = false

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Update workspace automatic updates by ID
// @ID update-workspace-automatic-updates-by-id
// @Security CoderSessionToken
// @Accept json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.UpdateWorkspaceAutomaticUpdatesRequest true "Automatic updates request"
// @Success 204
// @Router /workspaces/{workspace}/autoupdates [put]
func (api *API) putWorkspaceAutoupdates(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		workspace         = httpmw.WorkspaceParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Workspace](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = workspace

	var req codersdk.UpdateWorkspaceAutomaticUpdatesRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if !database.AutomaticUpdates(req.AutomaticUpdates).Valid() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request",
			Validations: []codersdk.ValidationError{{Field: "automatic_updates", Detail: "must be always or never"}},
		})
		return
	}

	err := api.Database.UpdateWorkspaceAutomaticUpdates(ctx, database.UpdateWorkspaceAutomaticUpdatesParams{
		ID:               workspace.ID,
		AutomaticUpdates: database.AutomaticUpdates(req.AutomaticUpdates),
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace automatic updates setting",
			Detail:  err.Error(),
		})
		return
	}

	newWorkspace := workspace
	newWorkspace.AutomaticUpdates = database.AutomaticUpdates(req.AutomaticUpdates)
	aReq.New = newWorkspace

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Resolve workspace autostart by id.
// @ID resolve-workspace-autostart-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {object} codersdk.ResolveAutostartResponse
// @Router /workspaces/{workspace}/resolve-autostart [get]
func (api *API) resolveAutostart(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
		workspace = httpmw.WorkspaceParam(r)
	)

	template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	templateAccessControl := (*(api.AccessControlStore.Load())).GetTemplateAccessControl(template)
	useActiveVersion := templateAccessControl.RequireActiveVersion || workspace.AutomaticUpdates == database.AutomaticUpdatesAlways
	if !useActiveVersion {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.ResolveAutostartResponse{})
		return
	}

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching latest workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	if build.TemplateVersionID == template.ActiveVersionID {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.ResolveAutostartResponse{})
		return
	}

	version, err := api.Database.GetTemplateVersionByID(ctx, template.ActiveVersionID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}

	dbVersionParams, err := api.Database.GetTemplateVersionParameters(ctx, version.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version parameters.",
			Detail:  err.Error(),
		})
		return
	}

	dbBuildParams, err := api.Database.GetWorkspaceBuildParameters(ctx, build.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching latest workspace build parameters.",
			Detail:  err.Error(),
		})
		return
	}

	versionParams, err := db2sdk.TemplateVersionParameters(dbVersionParams)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting template version parameters.",
			Detail:  err.Error(),
		})
		return
	}

	resolver := codersdk.ParameterResolver{
		Rich: db2sdk.WorkspaceBuildParameters(dbBuildParams),
	}

	var response codersdk.ResolveAutostartResponse
	for _, param := range versionParams {
		_, err := resolver.ValidateResolve(param, nil)
		// There's a parameter mismatch if we get an error back from the
		// resolver.
		response.ParameterMismatch = err != nil
		if response.ParameterMismatch {
			break
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Watch workspace by ID
// @ID watch-workspace-by-id
// @Security CoderSessionToken
// @Produce text/event-stream
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /workspaces/{workspace}/watch [get]
func (api *API) watchWorkspace(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	apiKey := httpmw.APIKey(r)

	sendEvent, senderClosed, err := httpapi.ServerSentEventSender(rw, r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error setting up server-sent events.",
			Detail:  err.Error(),
		})
		return
	}
	// Prevent handler from returning until the sender is closed.
	defer func() {
		<-senderClosed
	}()

	sendUpdate := func(_ context.Context, _ []byte) {
		workspace, err := api.Database.GetWorkspaceByID(ctx, workspace.ID)
		if err != nil {
			_ = sendEvent(ctx, codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Internal error fetching workspace.",
					Detail:  err.Error(),
				},
			})
			return
		}

		data, err := api.workspaceData(ctx, []database.Workspace{workspace})
		if err != nil {
			_ = sendEvent(ctx, codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Internal error fetching workspace data.",
					Detail:  err.Error(),
				},
			})
			return
		}
		if len(data.templates) == 0 {
			_ = sendEvent(ctx, codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Forbidden reading template of selected workspace.",
				},
			})
			return
		}

		ownerName, ok := usernameWithID(workspace.OwnerID, data.users)
		if !ok {
			_ = sendEvent(ctx, codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Internal error fetching workspace resources.",
					Detail:  "unable to find workspace owner's username",
				},
			})
			return
		}

		w, err := convertWorkspace(
			apiKey.UserID,
			workspace,
			data.builds[0],
			data.templates[0],
			ownerName,
			api.Options.AllowWorkspaceRenames,
		)
		if err != nil {
			_ = sendEvent(ctx, codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Internal error converting workspace.",
					Detail:  err.Error(),
				},
			})
		}
		_ = sendEvent(ctx, codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: w,
		})
	}

	cancelWorkspaceSubscribe, err := api.Pubsub.Subscribe(codersdk.WorkspaceNotifyChannel(workspace.ID), sendUpdate)
	if err != nil {
		_ = sendEvent(ctx, codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeError,
			Data: codersdk.Response{
				Message: "Internal error subscribing to workspace events.",
				Detail:  err.Error(),
			},
		})
		return
	}
	defer cancelWorkspaceSubscribe()

	// This is required to show whether the workspace is up-to-date.
	cancelTemplateSubscribe, err := api.Pubsub.Subscribe(watchTemplateChannel(workspace.TemplateID), sendUpdate)
	if err != nil {
		_ = sendEvent(ctx, codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeError,
			Data: codersdk.Response{
				Message: "Internal error subscribing to template events.",
				Detail:  err.Error(),
			},
		})
		return
	}
	defer cancelTemplateSubscribe()

	// An initial ping signals to the request that the server is now ready
	// and the client can begin servicing a channel with data.
	_ = sendEvent(ctx, codersdk.ServerSentEvent{
		Type: codersdk.ServerSentEventTypePing,
	})
	// Send updated workspace info after connection is established. This avoids
	// missing updates if the client connects after an update.
	sendUpdate(ctx, nil)

	for {
		select {
		case <-ctx.Done():
			return
		case <-senderClosed:
			return
		}
	}
}

type workspaceData struct {
	templates    []database.Template
	builds       []codersdk.WorkspaceBuild
	users        []database.User
	allowRenames bool
}

// workspacesData only returns the data the caller can access. If the caller
// does not have the correct perms to read a given template, the template will
// not be returned.
// So the caller must check the templates & users exist before using them.
func (api *API) workspaceData(ctx context.Context, workspaces []database.Workspace) (workspaceData, error) {
	workspaceIDs := make([]uuid.UUID, 0, len(workspaces))
	templateIDs := make([]uuid.UUID, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
		templateIDs = append(templateIDs, workspace.TemplateID)
	}

	templates, err := api.Database.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
		IDs: templateIDs,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceData{}, xerrors.Errorf("get templates: %w", err)
	}

	// This query must be run as system restricted to be efficient.
	// nolint:gocritic
	builds, err := api.Database.GetLatestWorkspaceBuildsByWorkspaceIDs(dbauthz.AsSystemRestricted(ctx), workspaceIDs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return workspaceData{}, xerrors.Errorf("get workspace builds: %w", err)
	}

	data, err := api.workspaceBuildsData(ctx, workspaces, builds)
	if err != nil {
		return workspaceData{}, xerrors.Errorf("get workspace builds data: %w", err)
	}

	apiBuilds, err := api.convertWorkspaceBuilds(
		builds,
		workspaces,
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
		return workspaceData{}, xerrors.Errorf("convert workspace builds: %w", err)
	}

	return workspaceData{
		templates:    templates,
		builds:       apiBuilds,
		users:        data.users,
		allowRenames: api.Options.AllowWorkspaceRenames,
	}, nil
}

func convertWorkspaces(requesterID uuid.UUID, workspaces []database.Workspace, data workspaceData) ([]codersdk.Workspace, error) {
	buildByWorkspaceID := map[uuid.UUID]codersdk.WorkspaceBuild{}
	for _, workspaceBuild := range data.builds {
		buildByWorkspaceID[workspaceBuild.WorkspaceID] = workspaceBuild
	}
	templateByID := map[uuid.UUID]database.Template{}
	for _, template := range data.templates {
		templateByID[template.ID] = template
	}
	userByID := map[uuid.UUID]database.User{}
	for _, user := range data.users {
		userByID[user.ID] = user
	}

	apiWorkspaces := make([]codersdk.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		// If any data is missing from the workspace, just skip returning
		// this workspace. This is not ideal, but the user cannot read
		// all the workspace's data, so do not show them.
		// Ideally we could just return some sort of "unknown" for the missing
		// fields?
		build, exists := buildByWorkspaceID[workspace.ID]
		if !exists {
			continue
		}
		template, exists := templateByID[workspace.TemplateID]
		if !exists {
			continue
		}
		owner, exists := userByID[workspace.OwnerID]
		if !exists {
			continue
		}

		w, err := convertWorkspace(
			requesterID,
			workspace,
			build,
			template,
			owner.Username,
			data.allowRenames,
		)
		if err != nil {
			return nil, xerrors.Errorf("convert workspace: %w", err)
		}

		apiWorkspaces = append(apiWorkspaces, w)
	}
	return apiWorkspaces, nil
}

func convertWorkspace(
	requesterID uuid.UUID,
	workspace database.Workspace,
	workspaceBuild codersdk.WorkspaceBuild,
	template database.Template,
	ownerName string,
	allowRenames bool,
) (codersdk.Workspace, error) {
	if requesterID == uuid.Nil {
		return codersdk.Workspace{}, xerrors.Errorf("developer error: requesterID cannot be uuid.Nil!")
	}
	var autostartSchedule *string
	if workspace.AutostartSchedule.Valid {
		autostartSchedule = &workspace.AutostartSchedule.String
	}

	var dormantAt *time.Time
	if workspace.DormantAt.Valid {
		dormantAt = &workspace.DormantAt.Time
	}

	var deletingAt *time.Time
	if workspace.DeletingAt.Valid {
		deletingAt = &workspace.DeletingAt.Time
	}

	failingAgents := []uuid.UUID{}
	for _, resource := range workspaceBuild.Resources {
		for _, agent := range resource.Agents {
			if !agent.Health.Healthy {
				failingAgents = append(failingAgents, agent.ID)
			}
		}
	}

	ttlMillis := convertWorkspaceTTLMillis(workspace.Ttl)

	// Only show favorite status if you own the workspace.
	requesterFavorite := workspace.OwnerID == requesterID && workspace.Favorite

	return codersdk.Workspace{
		ID:                                   workspace.ID,
		CreatedAt:                            workspace.CreatedAt,
		UpdatedAt:                            workspace.UpdatedAt,
		OwnerID:                              workspace.OwnerID,
		OwnerName:                            ownerName,
		OrganizationID:                       workspace.OrganizationID,
		TemplateID:                           workspace.TemplateID,
		LatestBuild:                          workspaceBuild,
		TemplateName:                         template.Name,
		TemplateIcon:                         template.Icon,
		TemplateDisplayName:                  template.DisplayName,
		TemplateAllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
		TemplateActiveVersionID:              template.ActiveVersionID,
		TemplateRequireActiveVersion:         template.RequireActiveVersion,
		Outdated:                             workspaceBuild.TemplateVersionID.String() != template.ActiveVersionID.String(),
		Name:                                 workspace.Name,
		AutostartSchedule:                    autostartSchedule,
		TTLMillis:                            ttlMillis,
		LastUsedAt:                           workspace.LastUsedAt,
		DeletingAt:                           deletingAt,
		DormantAt:                            dormantAt,
		Health: codersdk.WorkspaceHealth{
			Healthy:       len(failingAgents) == 0,
			FailingAgents: failingAgents,
		},
		AutomaticUpdates: codersdk.AutomaticUpdates(workspace.AutomaticUpdates),
		AllowRenames:     allowRenames,
		Favorite:         requesterFavorite,
	}, nil
}

func convertWorkspaceTTLMillis(i sql.NullInt64) *int64 {
	if !i.Valid {
		return nil
	}

	millis := time.Duration(i.Int64).Milliseconds()
	return &millis
}

func validWorkspaceTTLMillis(millis *int64, templateDefault, templateMax time.Duration) (sql.NullInt64, error) {
	if templateDefault == 0 && templateMax != 0 || (templateMax > 0 && templateDefault > templateMax) {
		templateDefault = templateMax
	}

	if ptr.NilOrZero(millis) {
		if templateDefault == 0 {
			if templateMax > 0 {
				return sql.NullInt64{
					Int64: int64(templateMax),
					Valid: true,
				}, nil
			}

			return sql.NullInt64{}, nil
		}

		return sql.NullInt64{
			Int64: int64(templateDefault),
			Valid: true,
		}, nil
	}

	dur := time.Duration(*millis) * time.Millisecond
	truncated := dur.Truncate(time.Minute)
	if truncated < ttlMin {
		return sql.NullInt64{}, errTTLMin
	}

	if truncated > ttlMax {
		return sql.NullInt64{}, errTTLMax
	}

	if templateMax > 0 && truncated > templateMax {
		return sql.NullInt64{}, xerrors.Errorf("time until shutdown must be less than or equal to the template's maximum TTL %q", templateMax.String())
	}

	return sql.NullInt64{
		Valid: true,
		Int64: int64(truncated),
	}, nil
}

func validWorkspaceAutomaticUpdates(updates codersdk.AutomaticUpdates) (database.AutomaticUpdates, error) {
	if updates == "" {
		return database.AutomaticUpdatesNever, nil
	}
	dbAU := database.AutomaticUpdates(updates)
	if !dbAU.Valid() {
		return "", xerrors.New("Automatic updates must be always or never")
	}
	return dbAU, nil
}

func validWorkspaceDeadline(startedAt, newDeadline time.Time) error {
	soon := time.Now().Add(29 * time.Minute)
	if newDeadline.Before(soon) {
		return errDeadlineTooSoon
	}

	// No idea how this could happen.
	if newDeadline.Before(startedAt) {
		return errDeadlineBeforeStart
	}

	return nil
}

func validWorkspaceSchedule(s *string) (sql.NullString, error) {
	if ptr.NilOrEmpty(s) {
		return sql.NullString{}, nil
	}

	_, err := cron.Weekly(*s)
	if err != nil {
		return sql.NullString{}, err
	}

	return sql.NullString{
		Valid:  true,
		String: *s,
	}, nil
}

func (api *API) publishWorkspaceUpdate(ctx context.Context, workspaceID uuid.UUID) {
	err := api.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspaceID), []byte{})
	if err != nil {
		api.Logger.Warn(ctx, "failed to publish workspace update",
			slog.F("workspace_id", workspaceID), slog.Error(err))
	}
}

func (api *API) publishWorkspaceAgentLogsUpdate(ctx context.Context, workspaceAgentID uuid.UUID, m agentsdk.LogsNotifyMessage) {
	b, err := json.Marshal(m)
	if err != nil {
		api.Logger.Warn(ctx, "failed to marshal logs notify message", slog.F("workspace_agent_id", workspaceAgentID), slog.Error(err))
	}
	err = api.Pubsub.Publish(agentsdk.LogsNotifyChannel(workspaceAgentID), b)
	if err != nil {
		api.Logger.Warn(ctx, "failed to publish workspace agent logs update", slog.F("workspace_agent_id", workspaceAgentID), slog.Error(err))
	}
}
