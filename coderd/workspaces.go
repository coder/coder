package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/searchquery"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

var (
	ttlMin = time.Minute //nolint:revive // min here means 'minimum' not 'minutes'
	ttlMax = 7 * 24 * time.Hour

	errTTLMin              = xerrors.New("time until shutdown must be at least one minute")
	errTTLMax              = xerrors.New("time until shutdown must be less than 7 days")
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

	httpapi.Write(ctx, rw, http.StatusOK, convertWorkspace(
		workspace,
		data.builds[0],
		data.templates[0],
		findUser(workspace.OwnerID, data.users),
	))
}

// workspaces returns all workspaces a user can read.
// Optional filters with query params
//
// @Summary List workspaces
// @ID list-workspaces
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param owner query string false "Filter by owner username"
// @Param template query string false "Filter by template name"
// @Param name query string false "Filter with partial-match by workspace name"
// @Param status query string false "Filter by workspace status" Enums(pending,running,stopping,stopped,failed,canceling,canceled,deleted,deleting)
// @Param has_agent query string false "Filter by agent status" Enums(connected,connecting,disconnected,timeout)
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

	wss, err := convertWorkspaces(workspaces, data)
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
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

	httpapi.Write(ctx, rw, http.StatusOK, convertWorkspace(
		workspace,
		data.builds[0],
		data.templates[0],
		findUser(workspace.OwnerID, data.users),
	))
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
		user                  = httpmw.UserParam(r)
		workspaceResourceInfo = audit.AdditionalFields{
			WorkspaceOwner: user.Username,
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

	if !api.Authorize(r, rbac.ActionCreate,
		rbac.ResourceWorkspace.InOrg(organization.ID).WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var createWorkspace codersdk.CreateWorkspaceRequest
	if !httpapi.Read(ctx, rw, r, &createWorkspace) {
		return
	}

	template, err := api.Database.GetTemplateByID(ctx, createWorkspace.TemplateID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Template %q doesn't exist.", createWorkspace.TemplateID.String()),
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
	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if organization.ID != template.OrganizationID {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
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

	dbTTL, err := validWorkspaceTTLMillis(createWorkspace.TTLMillis, template.DefaultTTL)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid Workspace Time to Shutdown.",
			Validations: []codersdk.ValidationError{{Field: "ttl_ms", Detail: err.Error()}},
		})
		return
	}

	// TODO: This should be a system call as the actor might not be able to
	// read other workspaces. Ideally we check the error on create and look for
	// a postgres conflict error.
	workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: user.ID,
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

	templateVersion, err := api.Database.GetTemplateVersionByID(ctx, template.ActiveVersionID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}

	dbTemplateVersionParameters, err := api.Database.GetTemplateVersionParameters(ctx, templateVersion.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version parameters.",
			Detail:  err.Error(),
		})
		return
	}

	templateVersionParameters, err := convertTemplateVersionParameters(dbTemplateVersionParameters)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting template version parameters.",
			Detail:  err.Error(),
		})
		return
	}

	err = codersdk.ValidateNewWorkspaceParameters(templateVersionParameters, createWorkspace.RichParameterValues)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Error validating workspace build parameters.",
			Detail:  err.Error(),
		})
		return
	}

	templateVersionJob, err := api.Database.GetProvisionerJobByID(ctx, templateVersion.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version job.",
			Detail:  err.Error(),
		})
		return
	}
	templateVersionJobStatus := convertProvisionerJob(templateVersionJob).Status
	switch templateVersionJobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		httpapi.Write(ctx, rw, http.StatusNotAcceptable, codersdk.Response{
			Message: fmt.Sprintf("The provided template version is %s. Wait for it to complete importing!", templateVersionJobStatus),
		})
		return
	case codersdk.ProvisionerJobFailed:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("The provided template version %q has failed to import. You cannot create workspaces using it!", templateVersion.Name),
		})
		return
	case codersdk.ProvisionerJobCanceled:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The provided template version was canceled during import. You cannot create workspaces using it!",
		})
		return
	}

	tags := provisionerdserver.MutateTags(user.ID, templateVersionJob.Tags)
	var (
		provisionerJob database.ProvisionerJob
		workspaceBuild database.WorkspaceBuild
	)
	err = api.Database.InTx(func(db database.Store) error {
		now := database.Now()
		workspaceBuildID := uuid.New()
		// Workspaces are created without any versions.
		workspace, err = db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:                uuid.New(),
			CreatedAt:         now,
			UpdatedAt:         now,
			OwnerID:           user.ID,
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
			// If the value is empty, we don't want to save it on database so
			// Terraform can use the default value
			if parameterValue.SourceValue == "" {
				continue
			}

			_, err = db.InsertParameterValue(ctx, database.InsertParameterValueParams{
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

		input, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
			WorkspaceBuildID: workspaceBuildID,
		})
		if err != nil {
			return xerrors.Errorf("marshal provision job: %w", err)
		}
		provisionerJob, err = db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      now,
			UpdatedAt:      now,
			InitiatorID:    apiKey.UserID,
			OrganizationID: template.OrganizationID,
			Provisioner:    template.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  templateVersionJob.StorageMethod,
			FileID:         templateVersionJob.FileID,
			Input:          input,
			Tags:           tags,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}
		workspaceBuild, err = db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:                workspaceBuildID,
			CreatedAt:         now,
			UpdatedAt:         now,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersion.ID,
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

		names := make([]string, 0, len(createWorkspace.RichParameterValues))
		values := make([]string, 0, len(createWorkspace.RichParameterValues))
		for _, param := range createWorkspace.RichParameterValues {
			names = append(names, param.Name)
			values = append(values, param.Value)
		}
		err = db.InsertWorkspaceBuildParameters(ctx, database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: workspaceBuildID,
			Name:             names,
			Value:            values,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build parameters: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating workspace.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = workspace

	initiator, err := api.Database.GetUserByID(ctx, workspaceBuild.InitiatorID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		Workspaces:      []telemetry.Workspace{telemetry.ConvertWorkspace(workspace)},
		WorkspaceBuilds: []telemetry.WorkspaceBuild{telemetry.ConvertWorkspaceBuild(workspaceBuild)},
	})

	users := []database.User{user, initiator}
	apiBuild, err := api.convertWorkspaceBuild(
		workspaceBuild,
		workspace,
		provisionerJob,
		users,
		[]database.WorkspaceResource{},
		[]database.WorkspaceResourceMetadatum{},
		[]database.WorkspaceAgent{},
		[]database.WorkspaceApp{},
		database.TemplateVersion{},
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace build.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertWorkspace(
		workspace,
		apiBuild,
		template,
		findUser(user.ID, users),
	))
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

	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

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

	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

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

	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.UpdateWorkspaceTTLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var dbTTL sql.NullInt64

	err := api.Database.InTx(func(s database.Store) error {
		var validityErr error
		// don't override 0 ttl with template default here because it indicates disabled auto-stop
		dbTTL, validityErr = validWorkspaceTTLMillis(req.TTLMillis, 0)
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

	if !api.Authorize(r, rbac.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

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

		if _, err := s.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
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
	}, nil)
	if err != nil {
		api.Logger.Info(ctx, "extending workspace", slog.Error(err))
	}
	api.publishWorkspaceUpdate(ctx, workspace.ID)
	httpapi.Write(ctx, rw, code, resp)
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
	if !api.Authorize(r, rbac.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}

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

	// Ignore all trace spans after this, they're not too useful.
	ctx = trace.ContextWithSpan(ctx, tracing.NoopSpan)

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

		_ = sendEvent(ctx, codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: convertWorkspace(
				workspace,
				data.builds[0],
				data.templates[0],
				findUser(workspace.OwnerID, data.users),
			),
		})
	}

	cancelWorkspaceSubscribe, err := api.Pubsub.Subscribe(watchWorkspaceChannel(workspace.ID), sendUpdate)
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
	templates []database.Template
	builds    []codersdk.WorkspaceBuild
	users     []database.User
}

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

	builds, err := api.Database.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, workspaceIDs)
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
		data.templateVersions,
	)
	if err != nil {
		return workspaceData{}, xerrors.Errorf("convert workspace builds: %w", err)
	}

	return workspaceData{
		templates: templates,
		builds:    apiBuilds,
		users:     data.users,
	}, nil
}

func convertWorkspaces(workspaces []database.Workspace, data workspaceData) ([]codersdk.Workspace, error) {
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
		build, exists := buildByWorkspaceID[workspace.ID]
		if !exists {
			return nil, xerrors.Errorf("build not found for workspace %q", workspace.Name)
		}
		template, exists := templateByID[workspace.TemplateID]
		if !exists {
			return nil, xerrors.Errorf("template not found for workspace %q", workspace.Name)
		}
		owner, exists := userByID[workspace.OwnerID]
		if !exists {
			return nil, xerrors.Errorf("owner not found for workspace: %q", workspace.Name)
		}

		apiWorkspaces = append(apiWorkspaces, convertWorkspace(
			workspace,
			build,
			template,
			&owner,
		))
	}
	sort.Slice(apiWorkspaces, func(i, j int) bool {
		iw := apiWorkspaces[i]
		jw := apiWorkspaces[j]
		if jw.LastUsedAt.IsZero() && iw.LastUsedAt.IsZero() {
			return iw.Name < jw.Name
		}
		return iw.LastUsedAt.After(jw.LastUsedAt)
	})

	return apiWorkspaces, nil
}

func convertWorkspace(
	workspace database.Workspace,
	workspaceBuild codersdk.WorkspaceBuild,
	template database.Template,
	owner *database.User,
) codersdk.Workspace {
	var autostartSchedule *string
	if workspace.AutostartSchedule.Valid {
		autostartSchedule = &workspace.AutostartSchedule.String
	}

	ttlMillis := convertWorkspaceTTLMillis(workspace.Ttl)
	return codersdk.Workspace{
		ID:                                   workspace.ID,
		CreatedAt:                            workspace.CreatedAt,
		UpdatedAt:                            workspace.UpdatedAt,
		OwnerID:                              workspace.OwnerID,
		OwnerName:                            owner.Username,
		TemplateID:                           workspace.TemplateID,
		LatestBuild:                          workspaceBuild,
		TemplateName:                         template.Name,
		TemplateIcon:                         template.Icon,
		TemplateDisplayName:                  template.DisplayName,
		TemplateAllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
		Outdated:                             workspaceBuild.TemplateVersionID.String() != template.ActiveVersionID.String(),
		Name:                                 workspace.Name,
		AutostartSchedule:                    autostartSchedule,
		TTLMillis:                            ttlMillis,
		LastUsedAt:                           workspace.LastUsedAt,
	}
}

func convertWorkspaceTTLMillis(i sql.NullInt64) *int64 {
	if !i.Valid {
		return nil
	}

	millis := time.Duration(i.Int64).Milliseconds()
	return &millis
}

func validWorkspaceTTLMillis(millis *int64, def int64) (sql.NullInt64, error) {
	if ptr.NilOrZero(millis) {
		if def == 0 {
			return sql.NullInt64{}, nil
		}

		return sql.NullInt64{
			Int64: def,
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

	return sql.NullInt64{
		Valid: true,
		Int64: int64(truncated),
	}, nil
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

	_, err := schedule.Weekly(*s)
	if err != nil {
		return sql.NullString{}, err
	}

	return sql.NullString{
		Valid:  true,
		String: *s,
	}, nil
}

func watchWorkspaceChannel(id uuid.UUID) string {
	return fmt.Sprintf("workspace:%s", id)
}

func (api *API) publishWorkspaceUpdate(ctx context.Context, workspaceID uuid.UUID) {
	err := api.Pubsub.Publish(watchWorkspaceChannel(workspaceID), []byte{})
	if err != nil {
		api.Logger.Warn(ctx, "failed to publish workspace update",
			slog.F("workspace_id", workspaceID), slog.Error(err))
	}
}
