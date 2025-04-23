package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/coderd/wspubsub"
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

	appStatus := codersdk.WorkspaceAppStatus{}
	if len(data.appStatuses) > 0 {
		appStatus = data.appStatuses[0]
	}

	w, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		data.builds[0],
		data.templates[0],
		api.Options.AllowWorkspaceRenames,
		appStatus,
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
	filter, errs := searchquery.Workspaces(ctx, api.Database, queryStr, page, api.AgentInactiveDisconnectTimeout)
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
	prepared, err := api.HTTPAuth.AuthorizeSQLFilter(r, policy.ActionRead, rbac.ResourceWorkspace.Type)
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

	// We need the technical row to present the correct count on every page.
	filter.WithSummary = true

	workspaceRows, err := api.Database.GetAuthorizedWorkspaces(ctx, filter, prepared)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  err.Error(),
		})
		return
	}
	if len(workspaceRows) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  "Workspace summary row is missing.",
		})
		return
	}
	if len(workspaceRows) == 1 {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspacesResponse{
			Workspaces: []codersdk.Workspace{},
			Count:      int(workspaceRows[0].Count),
		})
		return
	}
	// Skip technical summary row
	workspaceRows = workspaceRows[:len(workspaceRows)-1]

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

	appStatus := codersdk.WorkspaceAppStatus{}
	if len(data.appStatuses) > 0 {
		appStatus = data.appStatuses[0]
	}

	w, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		data.builds[0],
		data.templates[0],
		api.Options.AllowWorkspaceRenames,
		appStatus,
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
// @Description Create a new workspace using a template. The request must
// @Description specify either the Template ID or the Template Version ID,
// @Description not both. If the Template ID is specified, the active version
// @Description of the template will be used.
// @Deprecated Use /users/{user}/workspaces instead.
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
		apiKey                = httpmw.APIKey(r)
		auditor               = api.Auditor.Load()
		organization          = httpmw.OrganizationParam(r)
		member                = httpmw.OrganizationMemberParam(r)
		workspaceResourceInfo = audit.AdditionalFields{
			WorkspaceOwner: member.Username,
		}
	)

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:            *auditor,
		Log:              api.Logger,
		Request:          r,
		Action:           database.AuditActionCreate,
		AdditionalFields: workspaceResourceInfo,
		OrganizationID:   organization.ID,
	})

	defer commitAudit()

	var req codersdk.CreateWorkspaceRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	owner := workspaceOwner{
		ID:        member.UserID,
		Username:  member.Username,
		AvatarURL: member.AvatarURL,
	}

	createWorkspace(ctx, aReq, apiKey.UserID, api, owner, req, rw, r)
}

// Create a new workspace for the currently authenticated user.
//
// @Summary Create user workspace
// @Description Create a new workspace using a template. The request must
// @Description specify either the Template ID or the Template Version ID,
// @Description not both. If the Template ID is specified, the active version
// @Description of the template will be used.
// @ID create-user-workspace
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Workspaces
// @Param user path string true "Username, UUID, or me"
// @Param request body codersdk.CreateWorkspaceRequest true "Create workspace request"
// @Success 200 {object} codersdk.Workspace
// @Router /users/{user}/workspaces [post]
func (api *API) postUserWorkspaces(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		apiKey  = httpmw.APIKey(r)
		auditor = api.Auditor.Load()
	)

	var req codersdk.CreateWorkspaceRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var owner workspaceOwner
	// This user fetch is an optimization path for the most common case of creating a
	// workspace for 'Me'.
	//
	// This is also required to allow `owners` to create workspaces for users
	// that are not in an organization.
	user, ok := httpmw.UserParamOptional(r)
	if ok {
		owner = workspaceOwner{
			ID:        user.ID,
			Username:  user.Username,
			AvatarURL: user.AvatarURL,
		}
	} else {
		// A workspace can still be created if the caller can read the organization
		// member. The organization is required, which can be sourced from the
		// template.
		//
		// TODO: This code gets called twice for each workspace build request.
		//   This is inefficient and costs at most 2 extra RTTs to the DB.
		//   This can be optimized. It exists as it is now for code simplicity.
		//   The most common case is to create a workspace for 'Me'. Which does
		//   not enter this code branch.
		template, ok := requestTemplate(ctx, rw, req, api.Database)
		if !ok {
			return
		}

		// We need to fetch the original user as a system user to fetch the
		// user_id. 'ExtractUserContext' handles all cases like usernames,
		// 'Me', etc.
		// nolint:gocritic // The user_id needs to be fetched. This handles all those cases.
		user, ok := httpmw.ExtractUserContext(dbauthz.AsSystemRestricted(ctx), api.Database, rw, r)
		if !ok {
			return
		}

		organizationMember, err := database.ExpectOne(api.Database.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: template.OrganizationID,
			UserID:         user.ID,
			IncludeSystem:  false,
		}))
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching organization member.",
				Detail:  err.Error(),
			})
			return
		}
		owner = workspaceOwner{
			ID:        organizationMember.OrganizationMember.UserID,
			Username:  organizationMember.Username,
			AvatarURL: organizationMember.AvatarURL,
		}
	}

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: owner.Username,
		},
	})

	defer commitAudit()
	createWorkspace(ctx, aReq, apiKey.UserID, api, owner, req, rw, r)
}

type workspaceOwner struct {
	ID        uuid.UUID
	Username  string
	AvatarURL string
}

func createWorkspace(
	ctx context.Context,
	auditReq *audit.Request[database.WorkspaceTable],
	initiatorID uuid.UUID,
	api *API,
	owner workspaceOwner,
	req codersdk.CreateWorkspaceRequest,
	rw http.ResponseWriter,
	r *http.Request,
) {
	template, ok := requestTemplate(ctx, rw, req, api.Database)
	if !ok {
		return
	}

	// This is a premature auth check to avoid doing unnecessary work if the user
	// doesn't have permission to create a workspace.
	if !api.Authorize(r, policy.ActionCreate,
		rbac.ResourceWorkspace.InOrg(template.OrganizationID).WithOwner(owner.ID.String())) {
		// If this check fails, return a proper unauthorized error to the user to indicate
		// what is going on.
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Unauthorized to create workspace.",
			Detail: "You are unable to create a workspace in this organization. " +
				"It is possible to have access to the template, but not be able to create a workspace. " +
				"Please contact an administrator about your permissions if you feel this is an error.",
			Validations: nil,
		})
		return
	}

	// Update audit log's organization
	auditReq.UpdateOrganizationID(template.OrganizationID)

	// Do this upfront to save work. If this fails, the rest of the work
	// would be wasted.
	if !api.Authorize(r, policy.ActionCreate,
		rbac.ResourceWorkspace.InOrg(template.OrganizationID).WithOwner(owner.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// The user also needs permission to use the template. At this point they have
	// read perms, but not necessarily "use". This is also checked in `db.InsertWorkspace`.
	// Doing this up front can save some work below if the user doesn't have permission.
	if !api.Authorize(r, policy.ActionUse, template) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Unauthorized access to use the template %q.", template.Name),
			Detail: "Although you are able to view the template, you are unable to create a workspace using it. " +
				"Please contact an administrator about your permissions if you feel this is an error.",
			Validations: nil,
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

	dbAutostartSchedule, err := validWorkspaceSchedule(req.AutostartSchedule)
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

	nextStartAt := sql.NullTime{}
	if dbAutostartSchedule.Valid {
		next, err := schedule.NextAllowedAutostart(dbtime.Now(), dbAutostartSchedule.String, templateSchedule)
		if err == nil {
			nextStartAt = sql.NullTime{Valid: true, Time: dbtime.Time(next.UTC())}
		}
	}

	dbTTL, err := validWorkspaceTTLMillis(req.TTLMillis, templateSchedule.DefaultTTL)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid Workspace Time to Shutdown.",
			Validations: []codersdk.ValidationError{{Field: "ttl_ms", Detail: err.Error()}},
		})
		return
	}

	// back-compatibility: default to "never" if not included.
	dbAU := database.AutomaticUpdatesNever
	if req.AutomaticUpdates != "" {
		dbAU, err = validWorkspaceAutomaticUpdates(req.AutomaticUpdates)
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
		OwnerID: owner.ID,
		Name:    req.Name,
	})
	if err == nil {
		// If the workspace already exists, don't allow creation.
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Workspace %q already exists.", req.Name),
			Validations: []codersdk.ValidationError{{
				Field:  "name",
				Detail: "This value is already in use and should be unique.",
			}},
		})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("Internal error fetching workspace by name %q.", req.Name),
			Detail:  err.Error(),
		})
		return
	}

	var (
		provisionerJob     *database.ProvisionerJob
		workspaceBuild     *database.WorkspaceBuild
		provisionerDaemons []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow
	)

	prebuildsClaimer := *api.PrebuildsClaimer.Load()

	err = api.Database.InTx(func(db database.Store) error {
		var (
			workspaceID      uuid.UUID
			claimedWorkspace *database.Workspace
		)

		// If a template preset was chosen, try claim a prebuilt workspace.
		if req.TemplateVersionPresetID != uuid.Nil {
			// Try and claim an eligible prebuild, if available.
			claimedWorkspace, err = claimPrebuild(ctx, prebuildsClaimer, db, api.Logger, req, owner)
			if err != nil && !errors.Is(err, prebuilds.ErrNoClaimablePrebuiltWorkspaces) {
				return xerrors.Errorf("claim prebuild: %w", err)
			}
		}

		// No prebuild found; regular flow.
		if claimedWorkspace == nil {
			now := dbtime.Now()
			// Workspaces are created without any versions.
			minimumWorkspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
				ID:                uuid.New(),
				CreatedAt:         now,
				UpdatedAt:         now,
				OwnerID:           owner.ID,
				OrganizationID:    template.OrganizationID,
				TemplateID:        template.ID,
				Name:              req.Name,
				AutostartSchedule: dbAutostartSchedule,
				NextStartAt:       nextStartAt,
				Ttl:               dbTTL,
				// The workspaces page will sort by last used at, and it's useful to
				// have the newly created workspace at the top of the list!
				LastUsedAt:       dbtime.Now(),
				AutomaticUpdates: dbAU,
			})
			if err != nil {
				return xerrors.Errorf("insert workspace: %w", err)
			}
			workspaceID = minimumWorkspace.ID
		} else {
			// Prebuild found!
			workspaceID = claimedWorkspace.ID
			initiatorID = prebuildsClaimer.Initiator()
		}

		// We have to refetch the workspace for the joined in fields.
		// TODO: We can use WorkspaceTable for the builder to not require
		// this extra fetch.
		workspace, err = db.GetWorkspaceByID(ctx, workspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace by ID: %w", err)
		}

		builder := wsbuilder.New(workspace, database.WorkspaceTransitionStart).
			Reason(database.BuildReasonInitiator).
			Initiator(initiatorID).
			ActiveVersion().
			RichParameterValues(req.RichParameterValues).
			TemplateVersionPresetID(req.TemplateVersionPresetID)
		if req.TemplateVersionID != uuid.Nil {
			builder = builder.VersionID(req.TemplateVersionID)
		}
		if req.TemplateVersionPresetID != uuid.Nil {
			builder = builder.TemplateVersionPresetID(req.TemplateVersionPresetID)
		}

		if claimedWorkspace != nil {
			builder = builder.MarkPrebuildClaimedBy(owner.ID)
		}

		workspaceBuild, provisionerJob, provisionerDaemons, err = builder.Build(
			ctx,
			db,
			func(action policy.Action, object rbac.Objecter) bool {
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

	// nolint:gocritic // Need system context to fetch admins
	admins, err := findTemplateAdmins(dbauthz.AsSystemRestricted(ctx), api.Database)
	if err != nil {
		api.Logger.Error(ctx, "find template admins", slog.Error(err))
	} else {
		for _, admin := range admins {
			// Don't send notifications to user which initiated the event.
			if admin.ID == initiatorID {
				continue
			}

			api.notifyWorkspaceCreated(ctx, admin.ID, workspace, req.RichParameterValues)
		}
	}

	auditReq.New = workspace.WorkspaceTable()

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
		[]database.WorkspaceResource{},
		[]database.WorkspaceResourceMetadatum{},
		[]database.WorkspaceAgent{},
		[]database.WorkspaceApp{},
		[]database.WorkspaceAppStatus{},
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

	w, err := convertWorkspace(
		initiatorID,
		workspace,
		apiBuild,
		template,
		api.Options.AllowWorkspaceRenames,
		codersdk.WorkspaceAppStatus{},
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

func requestTemplate(ctx context.Context, rw http.ResponseWriter, req codersdk.CreateWorkspaceRequest, db database.Store) (database.Template, bool) {
	// If we were given a `TemplateVersionID`, we need to determine the `TemplateID` from it.
	templateID := req.TemplateID

	if templateID == uuid.Nil {
		templateVersion, err := db.GetTemplateVersionByID(ctx, req.TemplateVersionID)
		if httpapi.Is404Error(err) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Template version %q doesn't exist.", req.TemplateVersionID),
				Validations: []codersdk.ValidationError{{
					Field:  "template_version_id",
					Detail: "template not found",
				}},
			})
			return database.Template{}, false
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching template version.",
				Detail:  err.Error(),
			})
			return database.Template{}, false
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
			return database.Template{}, false
		}

		templateID = templateVersion.TemplateID.UUID
	}

	template, err := db.GetTemplateByID(ctx, templateID)
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Template %q doesn't exist.", templateID),
			Validations: []codersdk.ValidationError{{
				Field:  "template_id",
				Detail: "template not found",
			}},
		})
		return database.Template{}, false
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template.",
			Detail:  err.Error(),
		})
		return database.Template{}, false
	}
	if template.Deleted {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Template %q has been deleted!", template.Name),
		})
		return database.Template{}, false
	}
	return template, true
}

func claimPrebuild(ctx context.Context, claimer prebuilds.Claimer, db database.Store, logger slog.Logger, req codersdk.CreateWorkspaceRequest, owner workspaceOwner) (*database.Workspace, error) {
	prebuildsCtx := dbauthz.AsPrebuildsOrchestrator(ctx)

	// TODO: do we need a timeout here?
	claimCtx, cancel := context.WithTimeout(prebuildsCtx, time.Second*10)
	defer cancel()

	claimedID, err := claimer.Claim(claimCtx, owner.ID, req.Name, req.TemplateVersionPresetID)
	if err != nil {
		// TODO: enhance this by clarifying whether this *specific* prebuild failed or whether there are none to claim.
		return nil, xerrors.Errorf("claim prebuild: %w", err)
	}

	lookup, err := db.GetWorkspaceByID(prebuildsCtx, *claimedID)
	if err != nil {
		logger.Error(ctx, "unable to find claimed workspace by ID", slog.Error(err), slog.F("claimed_prebuild_id", (*claimedID).String()))
		return nil, xerrors.Errorf("find claimed workspace by ID %q: %w", (*claimedID).String(), err)
	}
	return &lookup, nil
}

func (api *API) notifyWorkspaceCreated(
	ctx context.Context,
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

	owner, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		log.Warn(ctx, "failed to fetch user for workspace creation notification", slog.F("owner_id", workspace.OwnerID), slog.Error(err))
		return
	}

	version, err := api.Database.GetTemplateVersionByID(ctx, template.ActiveVersionID)
	if err != nil {
		log.Warn(ctx, "failed to fetch template version for workspace creation notification", slog.F("template_version_id", template.ActiveVersionID), slog.Error(err))
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
		notifications.TemplateWorkspaceCreated,
		map[string]string{
			"workspace":                workspace.Name,
			"template":                 template.Name,
			"version":                  version.Name,
			"workspace_owner_username": owner.Username,
		},
		map[string]any{
			"workspace":        map[string]any{"id": workspace.ID, "name": workspace.Name},
			"template":         map[string]any{"id": template.ID, "name": template.Name},
			"template_version": map[string]any{"id": version.ID, "name": version.Name},
			"owner":            map[string]any{"id": owner.ID, "name": owner.Name, "email": owner.Email},
			"parameters":       buildParameters,
		},
		"api-workspaces-create",
		// Associate this notification with all the related entities
		workspace.ID, workspace.OwnerID, workspace.TemplateID, workspace.OrganizationID,
	); err != nil {
		log.Warn(ctx, "failed to notify of workspace creation", slog.Error(err))
	}
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
		aReq, commitAudit = audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: workspace.OrganizationID,
		})
	)
	defer commitAudit()
	aReq.Old = workspace.WorkspaceTable()

	var req codersdk.UpdateWorkspaceRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == "" || req.Name == workspace.Name {
		aReq.New = workspace.WorkspaceTable()
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

	api.publishWorkspaceUpdate(ctx, workspace.OwnerID, wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindMetadataUpdate,
		WorkspaceID: workspace.ID,
	})

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
		aReq, commitAudit = audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: workspace.OrganizationID,
		})
	)
	defer commitAudit()
	aReq.Old = workspace.WorkspaceTable()

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

	nextStartAt := sql.NullTime{}
	if dbSched.Valid {
		next, err := schedule.NextAllowedAutostart(dbtime.Now(), dbSched.String, templateSchedule)
		if err == nil {
			nextStartAt = sql.NullTime{Valid: true, Time: dbtime.Time(next.UTC())}
		}
	}

	err = api.Database.UpdateWorkspaceAutostart(ctx, database.UpdateWorkspaceAutostartParams{
		ID:                workspace.ID,
		AutostartSchedule: dbSched,
		NextStartAt:       nextStartAt,
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
	aReq.New = newWorkspace.WorkspaceTable()

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
		aReq, commitAudit = audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: workspace.OrganizationID,
		})
	)
	defer commitAudit()
	aReq.Old = workspace.WorkspaceTable()

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

		// don't override 0 ttl with template default here because it indicates
		// disabled autostop
		var validityErr error
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

		// If autostop has been disabled, we want to remove the deadline from the
		// existing workspace build (if there is one).
		if !dbTTL.Valid {
			build, err := s.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
			if err != nil {
				return xerrors.Errorf("get latest workspace build: %w", err)
			}

			if build.Transition == database.WorkspaceTransitionStart {
				if err = s.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
					ID:          build.ID,
					Deadline:    time.Time{},
					MaxDeadline: build.MaxDeadline,
					UpdatedAt:   dbtime.Time(api.Clock.Now()),
				}); err != nil {
					return xerrors.Errorf("update workspace build deadline: %w", err)
				}
			}
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
	aReq.New = newWorkspace.WorkspaceTable()

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
		oldWorkspace      = httpmw.WorkspaceParam(r)
		apiKey            = httpmw.APIKey(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: oldWorkspace.OrganizationID,
		})
	)
	aReq.Old = oldWorkspace.WorkspaceTable()
	defer commitAudit()

	var req codersdk.UpdateWorkspaceDormancy
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// If the workspace is already in the desired state do nothing!
	if oldWorkspace.DormantAt.Valid == req.Dormant {
		rw.WriteHeader(http.StatusNotModified)
		return
	}

	dormantAt := sql.NullTime{
		Valid: req.Dormant,
	}
	if req.Dormant {
		dormantAt.Time = dbtime.Now()
	}

	newWorkspace, err := api.Database.UpdateWorkspaceDormantDeletingAt(ctx, database.UpdateWorkspaceDormantDeletingAtParams{
		ID:        oldWorkspace.ID,
		DormantAt: dormantAt,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating workspace locked status.",
			Detail:  err.Error(),
		})
		return
	}

	// We don't need to notify the owner if they are the one making the request.
	if req.Dormant && apiKey.UserID != newWorkspace.OwnerID {
		initiator, initiatorErr := api.Database.GetUserByID(ctx, apiKey.UserID)
		if initiatorErr != nil {
			api.Logger.Warn(
				ctx,
				"failed to fetch the user that marked the workspace as dormant",
				slog.Error(err),
				slog.F("workspace_id", newWorkspace.ID),
				slog.F("user_id", apiKey.UserID),
			)
		}

		tmpl, tmplErr := api.Database.GetTemplateByID(ctx, newWorkspace.TemplateID)
		if tmplErr != nil {
			api.Logger.Warn(
				ctx,
				"failed to fetch the template of the workspace marked as dormant",
				slog.Error(err),
				slog.F("workspace_id", newWorkspace.ID),
				slog.F("template_id", newWorkspace.TemplateID),
			)
		}

		if initiatorErr == nil && tmplErr == nil {
			dormantTime := dbtime.Now().Add(time.Duration(tmpl.TimeTilDormant))
			_, err = api.NotificationsEnqueuer.Enqueue(
				// nolint:gocritic // Need notifier actor to enqueue notifications
				dbauthz.AsNotifier(ctx),
				newWorkspace.OwnerID,
				notifications.TemplateWorkspaceDormant,
				map[string]string{
					"name":           newWorkspace.Name,
					"reason":         "a " + initiator.Username + " request",
					"timeTilDormant": humanize.Time(dormantTime),
				},
				"api",
				newWorkspace.ID,
				newWorkspace.OwnerID,
				newWorkspace.TemplateID,
				newWorkspace.OrganizationID,
			)
			if err != nil {
				api.Logger.Warn(ctx, "failed to notify of workspace marked as dormant", slog.Error(err))
			}
		}
	}

	// We have to refetch the workspace to get the joined in fields.
	workspace, err := api.Database.GetWorkspaceByID(ctx, newWorkspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
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

	// TODO: This is a strange error since it occurs after the mutatation.
	// An example of why we should join in fields to prevent this forbidden error
	// from being sent, when the action did succeed.
	if len(data.templates) == 0 {
		httpapi.Forbidden(rw)
		return
	}

	aReq.New = newWorkspace

	appStatus := codersdk.WorkspaceAppStatus{}
	if len(data.appStatuses) > 0 {
		appStatus = data.appStatuses[0]
	}

	w, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		data.builds[0],
		data.templates[0],
		api.Options.AllowWorkspaceRenames,
		appStatus,
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

	api.publishWorkspaceUpdate(ctx, workspace.OwnerID, wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindMetadataUpdate,
		WorkspaceID: workspace.ID,
	})
	httpapi.Write(ctx, rw, code, resp)
}

// @Summary Post Workspace Usage by ID
// @ID post-workspace-usage-by-id
// @Security CoderSessionToken
// @Tags Workspaces
// @Accept json
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.PostWorkspaceUsageRequest false "Post workspace usage request"
// @Success 204
// @Router /workspaces/{workspace}/usage [post]
func (api *API) postWorkspaceUsage(rw http.ResponseWriter, r *http.Request) {
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, policy.ActionUpdate, workspace) {
		httpapi.Forbidden(rw)
		return
	}

	api.statsReporter.TrackUsage(workspace.ID)

	if !api.Experiments.Enabled(codersdk.ExperimentWorkspaceUsage) {
		// Continue previous behavior if the experiment is not enabled.
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Body == http.NoBody {
		// Continue previous behavior if no body is present.
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()
	var req codersdk.PostWorkspaceUsageRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.AgentID == uuid.Nil && req.AppName == "" {
		// Continue previous behavior if body is empty.
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	if req.AgentID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request",
			Validations: []codersdk.ValidationError{{
				Field:  "agent_id",
				Detail: "must be set when app_name is set",
			}},
		})
		return
	}
	if req.AppName == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request",
			Validations: []codersdk.ValidationError{{
				Field:  "app_name",
				Detail: "must be set when agent_id is set",
			}},
		})
		return
	}
	if !slices.Contains(codersdk.AllowedAppNames, req.AppName) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request",
			Validations: []codersdk.ValidationError{{
				Field:  "app_name",
				Detail: fmt.Sprintf("must be one of %v", codersdk.AllowedAppNames),
			}},
		})
		return
	}

	stat := &proto.Stats{
		ConnectionCount: 1,
	}
	switch req.AppName {
	case codersdk.UsageAppNameVscode:
		stat.SessionCountVscode = 1
	case codersdk.UsageAppNameJetbrains:
		stat.SessionCountJetbrains = 1
	case codersdk.UsageAppNameReconnectingPty:
		stat.SessionCountReconnectingPty = 1
	case codersdk.UsageAppNameSSH:
		stat.SessionCountSsh = 1
	default:
		// This means the app_name is in the codersdk.AllowedAppNames but not being
		// handled by this switch statement.
		httpapi.InternalServerError(rw, xerrors.Errorf("unknown app_name %q", req.AppName))
		return
	}

	agent, err := api.Database.GetWorkspaceAgentByID(ctx, req.AgentID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	err = api.statsReporter.ReportAgentStats(ctx, dbtime.Now(), workspace, agent, template.Name, stat, true)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
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

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: workspace.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = workspace.WorkspaceTable()

	err := api.Database.FavoriteWorkspace(ctx, workspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error setting workspace as favorite",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = workspace.WorkspaceTable()
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

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: workspace.OrganizationID,
	})

	defer commitAudit()
	aReq.Old = workspace.WorkspaceTable()

	err := api.Database.UnfavoriteWorkspace(ctx, workspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error unsetting workspace as favorite",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = workspace.WorkspaceTable()
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
		aReq, commitAudit = audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: workspace.OrganizationID,
		})
	)
	defer commitAudit()
	aReq.Old = workspace.WorkspaceTable()

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
	aReq.New = newWorkspace.WorkspaceTable()

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
// @Deprecated Use /workspaces/{workspace}/watch-ws instead
func (api *API) watchWorkspaceSSE(rw http.ResponseWriter, r *http.Request) {
	api.watchWorkspace(rw, r, httpapi.ServerSentEventSender)
}

// @Summary Watch workspace by ID via WebSockets
// @ID watch-workspace-by-id-via-websockets
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {object} codersdk.ServerSentEvent
// @Router /workspaces/{workspace}/watch-ws [get]
func (api *API) watchWorkspaceWS(rw http.ResponseWriter, r *http.Request) {
	api.watchWorkspace(rw, r, httpapi.OneWayWebSocketEventSender)
}

func (api *API) watchWorkspace(
	rw http.ResponseWriter,
	r *http.Request,
	connect httpapi.EventSender,
) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	apiKey := httpmw.APIKey(r)

	sendEvent, senderClosed, err := connect(rw, r)
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
			_ = sendEvent(codersdk.ServerSentEvent{
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
			_ = sendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Internal error fetching workspace data.",
					Detail:  err.Error(),
				},
			})
			return
		}
		if len(data.templates) == 0 {
			_ = sendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Forbidden reading template of selected workspace.",
				},
			})
			return
		}

		appStatus := codersdk.WorkspaceAppStatus{}
		if len(data.appStatuses) > 0 {
			appStatus = data.appStatuses[0]
		}
		w, err := convertWorkspace(
			apiKey.UserID,
			workspace,
			data.builds[0],
			data.templates[0],
			api.Options.AllowWorkspaceRenames,
			appStatus,
		)
		if err != nil {
			_ = sendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeError,
				Data: codersdk.Response{
					Message: "Internal error converting workspace.",
					Detail:  err.Error(),
				},
			})
		}
		_ = sendEvent(codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: w,
		})
	}

	cancelWorkspaceSubscribe, err := api.Pubsub.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
		wspubsub.HandleWorkspaceEvent(
			func(ctx context.Context, payload wspubsub.WorkspaceEvent, err error) {
				if err != nil {
					return
				}
				if payload.WorkspaceID != workspace.ID {
					return
				}
				sendUpdate(ctx, nil)
			}))
	if err != nil {
		_ = sendEvent(codersdk.ServerSentEvent{
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
		_ = sendEvent(codersdk.ServerSentEvent{
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
	_ = sendEvent(codersdk.ServerSentEvent{
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

// @Summary Get workspace timings by ID
// @ID get-workspace-timings-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceBuildTimings
// @Router /workspaces/{workspace}/timings [get]
func (api *API) workspaceTimings(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
		workspace = httpmw.WorkspaceParam(r)
	)

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}

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

type workspaceData struct {
	templates    []database.Template
	builds       []codersdk.WorkspaceBuild
	appStatuses  []codersdk.WorkspaceAppStatus
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

	var (
		templates   []database.Template
		builds      []database.WorkspaceBuild
		appStatuses []database.WorkspaceAppStatus
		eg          errgroup.Group
	)
	eg.Go(func() (err error) {
		templates, err = api.Database.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
			IDs: templateIDs,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get templates: %w", err)
		}
		return nil
	})
	eg.Go(func() (err error) {
		// This query must be run as system restricted to be efficient.
		// nolint:gocritic
		builds, err = api.Database.GetLatestWorkspaceBuildsByWorkspaceIDs(dbauthz.AsSystemRestricted(ctx), workspaceIDs)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get workspace builds: %w", err)
		}
		return nil
	})
	eg.Go(func() (err error) {
		// This query must be run as system restricted to be efficient.
		// nolint:gocritic
		appStatuses, err = api.Database.GetLatestWorkspaceAppStatusesByWorkspaceIDs(dbauthz.AsSystemRestricted(ctx), workspaceIDs)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get workspace app statuses: %w", err)
		}
		return nil
	})
	err := eg.Wait()
	if err != nil {
		return workspaceData{}, err
	}

	data, err := api.workspaceBuildsData(ctx, builds)
	if err != nil {
		return workspaceData{}, xerrors.Errorf("get workspace builds data: %w", err)
	}

	apiBuilds, err := api.convertWorkspaceBuilds(
		builds,
		workspaces,
		data.jobs,
		data.resources,
		data.metadata,
		data.agents,
		data.apps,
		data.appStatuses,
		data.scripts,
		data.logSources,
		data.templateVersions,
		data.provisionerDaemons,
	)
	if err != nil {
		return workspaceData{}, xerrors.Errorf("convert workspace builds: %w", err)
	}

	return workspaceData{
		templates:    templates,
		appStatuses:  db2sdk.WorkspaceAppStatuses(appStatuses),
		builds:       apiBuilds,
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
	appStatusesByWorkspaceID := map[uuid.UUID]codersdk.WorkspaceAppStatus{}
	for _, appStatus := range data.appStatuses {
		appStatusesByWorkspaceID[appStatus.WorkspaceID] = appStatus
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
		appStatus := appStatusesByWorkspaceID[workspace.ID]

		w, err := convertWorkspace(
			requesterID,
			workspace,
			build,
			template,
			data.allowRenames,
			appStatus,
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
	allowRenames bool,
	latestAppStatus codersdk.WorkspaceAppStatus,
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

	var nextStartAt *time.Time
	if workspace.NextStartAt.Valid {
		nextStartAt = &workspace.NextStartAt.Time
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
	// If the template doesn't allow a workspace-configured value, then report the
	// template value instead.
	if !template.AllowUserAutostop {
		ttlMillis = convertWorkspaceTTLMillis(sql.NullInt64{Valid: true, Int64: template.DefaultTTL})
	}

	// Only show favorite status if you own the workspace.
	requesterFavorite := workspace.OwnerID == requesterID && workspace.Favorite

	appStatus := &latestAppStatus
	if latestAppStatus.ID == uuid.Nil {
		appStatus = nil
	}
	return codersdk.Workspace{
		ID:                                   workspace.ID,
		CreatedAt:                            workspace.CreatedAt,
		UpdatedAt:                            workspace.UpdatedAt,
		OwnerID:                              workspace.OwnerID,
		OwnerName:                            workspace.OwnerUsername,
		OwnerAvatarURL:                       workspace.OwnerAvatarUrl,
		OrganizationID:                       workspace.OrganizationID,
		OrganizationName:                     workspace.OrganizationName,
		TemplateID:                           workspace.TemplateID,
		LatestBuild:                          workspaceBuild,
		LatestAppStatus:                      appStatus,
		TemplateName:                         workspace.TemplateName,
		TemplateIcon:                         workspace.TemplateIcon,
		TemplateDisplayName:                  workspace.TemplateDisplayName,
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
		NextStartAt:      nextStartAt,
	}, nil
}

func convertWorkspaceTTLMillis(i sql.NullInt64) *int64 {
	if !i.Valid {
		return nil
	}

	millis := time.Duration(i.Int64).Milliseconds()
	return &millis
}

func validWorkspaceTTLMillis(millis *int64, templateDefault time.Duration) (sql.NullInt64, error) {
	if ptr.NilOrZero(millis) {
		if templateDefault == 0 {
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

func (api *API) publishWorkspaceUpdate(ctx context.Context, ownerID uuid.UUID, event wspubsub.WorkspaceEvent) {
	err := event.Validate()
	if err != nil {
		api.Logger.Warn(ctx, "invalid workspace update event",
			slog.F("workspace_id", event.WorkspaceID),
			slog.F("event_kind", event.Kind), slog.Error(err))
		return
	}
	msg, err := json.Marshal(event)
	if err != nil {
		api.Logger.Warn(ctx, "failed to marshal workspace update",
			slog.F("workspace_id", event.WorkspaceID), slog.Error(err))
		return
	}
	err = api.Pubsub.Publish(wspubsub.WorkspaceEventChannel(ownerID), msg)
	if err != nil {
		api.Logger.Warn(ctx, "failed to publish workspace update",
			slog.F("workspace_id", event.WorkspaceID), slog.Error(err))
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
