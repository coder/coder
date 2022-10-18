package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/coderd/util/slice"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/examples"
)

// Returns whether the initial user has been created or not.
func (api *API) firstUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCount, err := api.Database.GetUserCount(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user count.",
			Detail:  err.Error(),
		})
		return
	}

	if userCount == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The initial user has not been created!",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "The initial user has already been created!",
	})
}

// Creates the initial user for a Coder deployment.
func (api *API) postFirstUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var createUser codersdk.CreateFirstUserRequest
	if !httpapi.Read(ctx, rw, r, &createUser) {
		return
	}

	// This should only function for the first user.
	userCount, err := api.Database.GetUserCount(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user count.",
			Detail:  err.Error(),
		})
		return
	}

	// If a user already exists, the initial admin user no longer can be created.
	if userCount != 0 {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "The initial user has already been created.",
		})
		return
	}

	user, organizationID, err := api.CreateUser(ctx, api.Database, CreateUserRequest{
		CreateUserRequest: codersdk.CreateUserRequest{
			Email:    createUser.Email,
			Username: createUser.Username,
			Password: createUser.Password,
			// Create an org for the first user.
			OrganizationID: uuid.Nil,
		},
		LoginType: database.LoginTypePassword,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating user.",
			Detail:  err.Error(),
		})
		return
	}

	telemetryUser := telemetry.ConvertUser(user)
	// Send the initial users email address!
	telemetryUser.Email = &user.Email
	api.Telemetry.Report(&telemetry.Snapshot{
		Users: []telemetry.User{telemetryUser},
	})

	// TODO: @emyrk this currently happens outside the database tx used to create
	// 	the user. Maybe I add this ability to grant roles in the createUser api
	//	and add some rbac bypass when calling api functions this way??
	// Add the admin role to this first user.
	_, err = api.Database.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
		GrantedRoles: []string{rbac.RoleOwner()},
		ID:           user.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user's roles.",
			Detail:  err.Error(),
		})
		return
	}

	// Auto-import any designated templates into the new organization.
	for _, template := range api.AutoImportTemplates {
		archive, err := examples.Archive(string(template))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error importing template.",
				Detail:  xerrors.Errorf("load template archive for %q: %w", template, err).Error(),
			})
			return
		}

		// Determine which parameter values to use.
		parameters := map[string]string{}
		switch template {
		case AutoImportTemplateKubernetes:

			// Determine the current namespace we're in.
			const namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
			namespace, err := os.ReadFile(namespaceFile)
			if err != nil {
				parameters["use_kubeconfig"] = "true" // use ~/.config/kubeconfig
				parameters["namespace"] = "coder-workspaces"
			} else {
				parameters["use_kubeconfig"] = "false" // use SA auth
				parameters["namespace"] = string(bytes.TrimSpace(namespace))
			}

		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error importing template.",
				Detail:  fmt.Sprintf("cannot auto-import %q template", template),
			})
			return
		}

		tpl, err := api.autoImportTemplate(ctx, autoImportTemplateOpts{
			name:    string(template),
			archive: archive,
			params:  parameters,
			userID:  user.ID,
			orgID:   organizationID,
		})
		if err != nil {
			api.Logger.Warn(ctx, "failed to auto-import template", slog.F("template", template), slog.F("parameters", parameters), slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error importing template.",
				Detail:  xerrors.Errorf("failed to import template %q: %w", template, err).Error(),
			})
			return
		}

		api.Logger.Info(ctx, "auto-imported template", slog.F("id", tpl.ID), slog.F("template", template), slog.F("parameters", parameters))
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: organizationID,
	})
}

func (api *API) users(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	params, errs := userSearchQuery(query)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid user search query.",
			Validations: errs,
		})
		return
	}

	paginationParams, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	users, err := api.Database.GetUsers(ctx, database.GetUsersParams{
		AfterID:   paginationParams.AfterID,
		OffsetOpt: int32(paginationParams.Offset),
		LimitOpt:  int32(paginationParams.Limit),
		Search:    params.Search,
		Status:    params.Status,
		RbacRole:  params.RbacRole,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusOK, []codersdk.User{})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching users.",
			Detail:  err.Error(),
		})
		return
	}

	users, err = AuthorizeFilter(api.HTTPAuth, r, rbac.ActionRead, users)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching users.",
			Detail:  err.Error(),
		})
		return
	}

	userIDs := make([]uuid.UUID, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	organizationIDsByMemberIDsRows, err := api.Database.GetOrganizationIDsByMemberIDs(ctx, userIDs)
	if xerrors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}
	organizationIDsByUserID := map[uuid.UUID][]uuid.UUID{}
	for _, organizationIDsByMemberIDsRow := range organizationIDsByMemberIDsRows {
		organizationIDsByUserID[organizationIDsByMemberIDsRow.UserID] = organizationIDsByMemberIDsRow.OrganizationIDs
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertUsers(users, organizationIDsByUserID))
}

// Creates a new user.
func (api *API) postUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.User](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
	})
	defer commitAudit()

	// Create the user on the site.
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceUser) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.CreateUserRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Create the organization member in the org.
	if !api.Authorize(r, rbac.ActionCreate,
		rbac.ResourceOrganizationMember.InOrg(req.OrganizationID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// TODO: @emyrk Authorize the organization create if the createUser will do that.

	_, err := api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Username: req.Username,
		Email:    req.Email,
	})
	if err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "User already exists.",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	_, err = api.Database.GetOrganizationByID(ctx, req.OrganizationID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Organization does not exist with the provided id %q.", req.OrganizationID),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organization.",
			Detail:  err.Error(),
		})
		return
	}

	user, _, err := api.CreateUser(ctx, api.Database, CreateUserRequest{
		CreateUserRequest: req,
		LoginType:         database.LoginTypePassword,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating user.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = user

	// Report when users are added!
	api.Telemetry.Report(&telemetry.Snapshot{
		Users: []telemetry.User{telemetry.ConvertUser(user)},
	})

	httpapi.Write(ctx, rw, http.StatusCreated, convertUser(user, []uuid.UUID{req.OrganizationID}))
}

func (api *API) deleteUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.Auditor.Load()
	user := httpmw.UserParam(r)
	aReq, commitAudit := audit.InitRequest[database.User](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionDelete,
	})
	aReq.Old = user
	defer commitAudit()

	if !api.Authorize(r, rbac.ActionDelete, rbac.ResourceUser) {
		httpapi.Forbidden(rw)
		return
	}

	workspaces, err := api.Database.GetWorkspaces(ctx, database.GetWorkspacesParams{
		OwnerID: user.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  err.Error(),
		})
		return
	}
	if len(workspaces) > 0 {
		httpapi.Write(ctx, rw, http.StatusExpectationFailed, codersdk.Response{
			Message: "You cannot delete a user that has workspaces. Delete their workspaces and try again!",
		})
		return
	}

	err = api.Database.UpdateUserDeletedByID(ctx, database.UpdateUserDeletedByIDParams{
		ID:      user.ID,
		Deleted: true,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting user.",
			Detail:  err.Error(),
		})
		return
	}
	user.Deleted = true
	aReq.New = user
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "User has been deleted!",
	})
}

// Returns the parameterized user requested. All validation
// is completed in the middleware for this route.
func (api *API) userByName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	organizationIDs, err := userOrganizationIDs(ctx, api, user)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertUser(user, organizationIDs))
}

func (api *API) putUserProfile(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		auditor           = *api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = user

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var params codersdk.UpdateUserProfileRequest
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}
	existentUser, err := api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Username: params.Username,
	})
	isDifferentUser := existentUser.ID != user.ID

	if err == nil && isDifferentUser {
		responseErrors := []codersdk.ValidationError{}
		if existentUser.Username == params.Username {
			responseErrors = append(responseErrors, codersdk.ValidationError{
				Field:  "username",
				Detail: "this value is already in use and should be unique",
			})
		}
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message:     "User already exists.",
			Validations: responseErrors,
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) && isDifferentUser {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	updatedUserProfile, err := api.Database.UpdateUserProfile(ctx, database.UpdateUserProfileParams{
		ID:        user.ID,
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
		Username:  params.Username,
		UpdatedAt: database.Now(),
	})
	aReq.New = updatedUserProfile

	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user.",
			Detail:  err.Error(),
		})
		return
	}

	organizationIDs, err := userOrganizationIDs(ctx, api, user)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertUser(updatedUserProfile, organizationIDs))
}

func (api *API) putUserStatus(status database.UserStatus) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx               = r.Context()
			user              = httpmw.UserParam(r)
			apiKey            = httpmw.APIKey(r)
			auditor           = *api.Auditor.Load()
			aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
				Audit:   auditor,
				Log:     api.Logger,
				Request: r,
				Action:  database.AuditActionWrite,
			})
		)
		defer commitAudit()
		aReq.Old = user

		if !api.Authorize(r, rbac.ActionDelete, rbac.ResourceUser) {
			httpapi.ResourceNotFound(rw)
			return
		}

		if status == database.UserStatusSuspended {
			// There are some manual protections when suspending a user to
			// prevent certain situations.
			switch {
			case user.ID == apiKey.UserID:
				// Suspending yourself is not allowed, as you can lock yourself
				// out of the system.
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "You cannot suspend yourself.",
				})
				return
			case slice.Contains(user.RBACRoles, rbac.RoleOwner()):
				// You may not suspend an owner
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("You cannot suspend a user with the %q role. You must remove the role first.", rbac.RoleOwner()),
				})
				return
			}
		}

		suspendedUser, err := api.Database.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:        user.ID,
			Status:    status,
			UpdatedAt: database.Now(),
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: fmt.Sprintf("Internal error updating user's status to %q.", status),
				Detail:  err.Error(),
			})
			return
		}
		aReq.New = suspendedUser

		organizations, err := userOrganizationIDs(ctx, api, user)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching user's organizations.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, convertUser(suspendedUser, organizations))
	}
}

func (api *API) putUserPassword(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		params            codersdk.UpdateUserPasswordRequest
		auditor           = *api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = user

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceUserData.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	err := userpassword.Validate(params.Password)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid password.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "password",
					Detail: err.Error(),
				},
			},
		})
		return
	}

	// admins can change passwords without sending old_password
	if params.OldPassword == "" {
		if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceUser) {
			httpapi.Forbidden(rw)
			return
		}
	} else {
		// if they send something let's validate it
		ok, err := userpassword.Compare(string(user.HashedPassword), params.OldPassword)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error with passwords.",
				Detail:  err.Error(),
			})
			return
		}
		if !ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Old password is incorrect.",
				Validations: []codersdk.ValidationError{
					{
						Field:  "old_password",
						Detail: "Old password is incorrect.",
					},
				},
			})
			return
		}
	}

	hashedPassword, err := userpassword.Hash(params.Password)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error hashing new password.",
			Detail:  err.Error(),
		})
		return
	}
	err = api.Database.UpdateUserHashedPassword(ctx, database.UpdateUserHashedPasswordParams{
		ID:             user.ID,
		HashedPassword: []byte(hashedPassword),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user's password.",
			Detail:  err.Error(),
		})
		return
	}

	newUser := user
	newUser.HashedPassword = []byte(hashedPassword)
	aReq.New = newUser

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

func (api *API) userRoles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUserData.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	resp := codersdk.UserRoles{
		Roles:             user.RBACRoles,
		OrganizationRoles: make(map[uuid.UUID][]string),
	}

	memberships, err := api.Database.GetOrganizationMembershipsByUserID(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organization memberships.",
			Detail:  err.Error(),
		})
		return
	}

	// Only include ones we can read from RBAC.
	memberships, err = AuthorizeFilter(api.HTTPAuth, r, rbac.ActionRead, memberships)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching memberships.",
			Detail:  err.Error(),
		})
		return
	}

	for _, mem := range memberships {
		// If we can read the org member, include the roles.
		if err == nil {
			resp.OrganizationRoles[mem.OrganizationID] = mem.Roles
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) putUserRoles(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx = r.Context()
		// User is the user to modify.
		user              = httpmw.UserParam(r)
		actorRoles        = httpmw.UserAuthorization(r)
		apiKey            = httpmw.APIKey(r)
		auditor           = *api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = user

	if apiKey.UserID == user.ID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "You cannot change your own roles.",
		})
		return
	}

	var params codersdk.UpdateRoles
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// The member role is always implied.
	impliedTypes := append(params.Roles, rbac.RoleMember())
	added, removed := rbac.ChangeRoleSet(user.RBACRoles, impliedTypes)

	// Assigning a role requires the create permission.
	if len(added) > 0 && !api.Authorize(r, rbac.ActionCreate, rbac.ResourceRoleAssignment) {
		httpapi.Forbidden(rw)
		return
	}

	// Removing a role requires the delete permission.
	if len(removed) > 0 && !api.Authorize(r, rbac.ActionDelete, rbac.ResourceRoleAssignment) {
		httpapi.Forbidden(rw)
		return
	}

	// Just treat adding & removing as "assigning" for now.
	for _, roleName := range append(added, removed...) {
		if !rbac.CanAssignRole(actorRoles.Roles, roleName) {
			httpapi.Forbidden(rw)
			return
		}
	}

	updatedUser, err := api.updateSiteUserRoles(ctx, database.UpdateUserRolesParams{
		GrantedRoles: params.Roles,
		ID:           user.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
		})
		return
	}
	aReq.New = updatedUser

	organizationIDs, err := userOrganizationIDs(ctx, api, user)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertUser(updatedUser, organizationIDs))
}

// updateSiteUserRoles will ensure only site wide roles are passed in as arguments.
// If an organization role is included, an error is returned.
func (api *API) updateSiteUserRoles(ctx context.Context, args database.UpdateUserRolesParams) (database.User, error) {
	// Enforce only site wide roles.
	for _, r := range args.GrantedRoles {
		if _, ok := rbac.IsOrgRole(r); ok {
			return database.User{}, xerrors.Errorf("Must only update site wide roles")
		}

		if _, err := rbac.RoleByName(r); err != nil {
			return database.User{}, xerrors.Errorf("%q is not a supported role", r)
		}
	}

	updatedUser, err := api.Database.UpdateUserRoles(ctx, args)
	if err != nil {
		return database.User{}, xerrors.Errorf("update site roles: %w", err)
	}
	return updatedUser, nil
}

// Returns organizations the parameterized user has access to.
func (api *API) organizationsByUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	organizations, err := api.Database.GetOrganizationsByUserID(ctx, user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		organizations = []database.Organization{}
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	// Only return orgs the user can read.
	organizations, err = AuthorizeFilter(api.HTTPAuth, r, rbac.ActionRead, organizations)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organizations.",
			Detail:  err.Error(),
		})
		return
	}

	publicOrganizations := make([]codersdk.Organization, 0, len(organizations))
	for _, organization := range organizations {
		publicOrganizations = append(publicOrganizations, convertOrganization(organization))
	}

	httpapi.Write(ctx, rw, http.StatusOK, publicOrganizations)
}

func (api *API) organizationByUserAndName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organizationName := chi.URLParam(r, "organizationname")
	organization, err := api.Database.GetOrganizationByName(ctx, organizationName)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organization.",
			Detail:  err.Error(),
		})
		return
	}

	if !api.Authorize(r, rbac.ActionRead,
		rbac.ResourceOrganization.
			InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertOrganization(organization))
}

// Authenticates the user with an email and password.
func (api *API) postLogin(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var loginWithPassword codersdk.LoginWithPasswordRequest
	if !httpapi.Read(ctx, rw, r, &loginWithPassword) {
		return
	}

	user, err := api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Email: loginWithPassword.Email,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return
	}

	// If the user doesn't exist, it will be a default struct.
	equal, err := userpassword.Compare(string(user.HashedPassword), loginWithPassword.Password)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return
	}
	if !equal {
		// This message is the same as above to remove ease in detecting whether
		// users are registered or not. Attackers still could with a timing attack.
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Incorrect email or password.",
		})
		return
	}

	if user.LoginType != database.LoginTypePassword {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Incorrect login type, attempting to use %q but user is of login type %q", database.LoginTypePassword, user.LoginType),
		})
		return
	}

	// If the user logged into a suspended account, reject the login request.
	if user.Status != database.UserStatusActive {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Your account is suspended. Contact an admin to reactivate your account.",
		})
		return
	}

	cookie, err := api.createAPIKey(ctx, createAPIKeyParams{
		UserID:     user.ID,
		LoginType:  database.LoginTypePassword,
		RemoteAddr: r.RemoteAddr,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	http.SetCookie(rw, cookie)

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.LoginWithPasswordResponse{
		SessionToken: cookie.Value,
	})
}

// Clear the user's session cookie.
func (api *API) postLogout(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Get a blank token cookie.
	cookie := &http.Cookie{
		// MaxAge < 0 means to delete the cookie now.
		MaxAge: -1,
		Name:   codersdk.SessionTokenKey,
		Path:   "/",
	}
	http.SetCookie(rw, cookie)

	// Delete the session token from database.
	apiKey := httpmw.APIKey(r)
	err := api.Database.DeleteAPIKeyByID(ctx, apiKey.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Deployments should not host app tokens on the same domain as the
	// primary deployment. But in the case they are, we should also delete this
	// token.
	if appCookie, _ := r.Cookie(httpmw.DevURLSessionTokenCookie); appCookie != nil {
		appCookieRemove := &http.Cookie{
			// MaxAge < 0 means to delete the cookie now.
			MaxAge: -1,
			Name:   httpmw.DevURLSessionTokenCookie,
			Path:   "/",
			Domain: "." + api.AccessURL.Hostname(),
		}
		http.SetCookie(rw, appCookieRemove)

		id, _, err := httpmw.SplitAPIToken(appCookie.Value)
		if err == nil {
			err = api.Database.DeleteAPIKeyByID(ctx, id)
			if err != nil {
				// Don't block logout, just log any errors.
				api.Logger.Warn(r.Context(), "failed to delete devurl token on logout",
					slog.Error(err),
					slog.F("id", id),
				)
			}
		}
	}

	// This code should be removed after Jan 1 2023.
	// This code logs out of the old session cookie before we renamed it
	// if it is a valid coder token. Otherwise, this old cookie hangs around
	// and we never log out of the user.
	oldCookie, err := r.Cookie("session_token")
	if err == nil && oldCookie != nil {
		_, _, err := httpmw.SplitAPIToken(oldCookie.Value)
		if err == nil {
			cookie := &http.Cookie{
				// MaxAge < 0 means to delete the cookie now.
				MaxAge: -1,
				Name:   "session_token",
				Path:   "/",
			}
			http.SetCookie(rw, cookie)
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Logged out!",
	})
}

type CreateUserRequest struct {
	codersdk.CreateUserRequest
	LoginType database.LoginType
}

func (api *API) CreateUser(ctx context.Context, store database.Store, req CreateUserRequest) (database.User, uuid.UUID, error) {
	var user database.User
	return user, req.OrganizationID, store.InTx(func(tx database.Store) error {
		orgRoles := make([]string, 0)
		// If no organization is provided, create a new one for the user.
		if req.OrganizationID == uuid.Nil {
			organization, err := tx.InsertOrganization(ctx, database.InsertOrganizationParams{
				ID:        uuid.New(),
				Name:      req.Username,
				CreatedAt: database.Now(),
				UpdatedAt: database.Now(),
			})
			if err != nil {
				return xerrors.Errorf("create organization: %w", err)
			}
			req.OrganizationID = organization.ID
			orgRoles = append(orgRoles, rbac.RoleOrgAdmin(req.OrganizationID))

			_, err = tx.InsertAllUsersGroup(ctx, organization.ID)
			if err != nil {
				return xerrors.Errorf("create %q group: %w", database.AllUsersGroup, err)
			}
		}

		params := database.InsertUserParams{
			ID:        uuid.New(),
			Email:     req.Email,
			Username:  req.Username,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
			// All new users are defaulted to members of the site.
			RBACRoles: []string{},
			LoginType: req.LoginType,
		}
		// If a user signs up with OAuth, they can have no password!
		if req.Password != "" {
			hashedPassword, err := userpassword.Hash(req.Password)
			if err != nil {
				return xerrors.Errorf("hash password: %w", err)
			}
			params.HashedPassword = []byte(hashedPassword)
		}

		var err error
		user, err = tx.InsertUser(ctx, params)
		if err != nil {
			return xerrors.Errorf("create user: %w", err)
		}

		privateKey, publicKey, err := gitsshkey.Generate(api.SSHKeygenAlgorithm)
		if err != nil {
			return xerrors.Errorf("generate user gitsshkey: %w", err)
		}
		_, err = tx.InsertGitSSHKey(ctx, database.InsertGitSSHKeyParams{
			UserID:     user.ID,
			CreatedAt:  database.Now(),
			UpdatedAt:  database.Now(),
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		})
		if err != nil {
			return xerrors.Errorf("insert user gitsshkey: %w", err)
		}
		_, err = tx.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
			OrganizationID: req.OrganizationID,
			UserID:         user.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			// By default give them membership to the organization.
			Roles: orgRoles,
		})
		if err != nil {
			return xerrors.Errorf("create organization member: %w", err)
		}
		return nil
	})
}

func convertUser(user database.User, organizationIDs []uuid.UUID) codersdk.User {
	convertedUser := codersdk.User{
		ID:              user.ID,
		Email:           user.Email,
		CreatedAt:       user.CreatedAt,
		LastSeenAt:      user.LastSeenAt,
		Username:        user.Username,
		Status:          codersdk.UserStatus(user.Status),
		OrganizationIDs: organizationIDs,
		Roles:           make([]codersdk.Role, 0, len(user.RBACRoles)),
		AvatarURL:       user.AvatarURL.String,
	}

	for _, roleName := range user.RBACRoles {
		rbacRole, _ := rbac.RoleByName(roleName)
		convertedUser.Roles = append(convertedUser.Roles, convertRole(rbacRole))
	}

	return convertedUser
}

func convertUsers(users []database.User, organizationIDsByUserID map[uuid.UUID][]uuid.UUID) []codersdk.User {
	converted := make([]codersdk.User, 0, len(users))
	for _, u := range users {
		userOrganizationIDs := organizationIDsByUserID[u.ID]
		converted = append(converted, convertUser(u, userOrganizationIDs))
	}
	return converted
}

func userOrganizationIDs(ctx context.Context, api *API, user database.User) ([]uuid.UUID, error) {
	organizationIDsByMemberIDsRows, err := api.Database.GetOrganizationIDsByMemberIDs(ctx, []uuid.UUID{user.ID})
	if errors.Is(err, sql.ErrNoRows) || len(organizationIDsByMemberIDsRows) == 0 {
		return []uuid.UUID{}, nil
	}
	if err != nil {
		return []uuid.UUID{}, err
	}
	member := organizationIDsByMemberIDsRows[0]
	return member.OrganizationIDs, nil
}

func findUser(id uuid.UUID, users []database.User) *database.User {
	for _, u := range users {
		if u.ID == id {
			return &u
		}
	}
	return nil
}

func userSearchQuery(query string) (database.GetUsersParams, []codersdk.ValidationError) {
	searchParams := make(url.Values)
	if query == "" {
		// No filter
		return database.GetUsersParams{}, nil
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
			// No key:value pair.
			searchParams.Set("search", parts[0])
		case 2:
			searchParams.Set(parts[0], parts[1])
		default:
			return database.GetUsersParams{}, []codersdk.ValidationError{
				{Field: "q", Detail: fmt.Sprintf("Query element %q can only contain 1 ':'", element)},
			}
		}
	}

	parser := httpapi.NewQueryParamParser()
	filter := database.GetUsersParams{
		Search:   parser.String(searchParams, "", "search"),
		Status:   httpapi.ParseCustom(parser, searchParams, []database.UserStatus{}, "status", parseUserStatus),
		RbacRole: parser.Strings(searchParams, []string{}, "role"),
	}

	return filter, parser.Errors
}

// parseUserStatus ensures proper enums are used for user statuses
func parseUserStatus(v string) ([]database.UserStatus, error) {
	var statuses []database.UserStatus
	if v == "" {
		return statuses, nil
	}
	parts := strings.Split(v, ",")
	for _, part := range parts {
		switch database.UserStatus(part) {
		case database.UserStatusActive, database.UserStatusSuspended:
			statuses = append(statuses, database.UserStatus(part))
		default:
			return []database.UserStatus{}, xerrors.Errorf("%q is not a valid user status", part)
		}
	}
	return statuses, nil
}

func convertAPIKey(k database.APIKey) codersdk.APIKey {
	return codersdk.APIKey{
		ID:              k.ID,
		UserID:          k.UserID,
		LastUsed:        k.LastUsed,
		ExpiresAt:       k.ExpiresAt,
		CreatedAt:       k.CreatedAt,
		UpdatedAt:       k.UpdatedAt,
		LoginType:       codersdk.LoginType(k.LoginType),
		Scope:           codersdk.APIKeyScope(k.Scope),
		LifetimeSeconds: k.LifetimeSeconds,
	}
}
