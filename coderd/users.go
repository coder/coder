package coderd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/tabbed/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/coderd/util/slice"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/examples"
)

// Returns whether the initial user has been created or not.
func (api *API) firstUser(rw http.ResponseWriter, r *http.Request) {
	userCount, err := api.Database.GetUserCount(r.Context())
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user count.",
			Detail:  err.Error(),
		})
		return
	}

	if userCount == 0 {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: "The initial user has not been created!",
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, codersdk.Response{
		Message: "The initial user has already been created!",
	})
}

// Creates the initial user for a Coder deployment.
func (api *API) postFirstUser(rw http.ResponseWriter, r *http.Request) {
	var createUser codersdk.CreateFirstUserRequest
	if !httpapi.Read(rw, r, &createUser) {
		return
	}

	// This should only function for the first user.
	userCount, err := api.Database.GetUserCount(r.Context())
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user count.",
			Detail:  err.Error(),
		})
		return
	}

	// If a user already exists, the initial admin user no longer can be created.
	if userCount != 0 {
		httpapi.Write(rw, http.StatusConflict, codersdk.Response{
			Message: "The initial user has already been created.",
		})
		return
	}

	user, organizationID, err := api.createUser(r.Context(), api.Database, createUserRequest{
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
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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
	_, err = api.Database.UpdateUserRoles(r.Context(), database.UpdateUserRolesParams{
		GrantedRoles: []string{rbac.RoleOwner()},
		ID:           user.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user's roles.",
			Detail:  err.Error(),
		})
		return
	}

	// Auto-import any designated templates into the new organization.
	for _, template := range api.AutoImportTemplates {
		archive, err := examples.Archive(string(template))
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error importing template.",
				Detail:  fmt.Sprintf("cannot auto-import %q template", template),
			})
			return
		}

		tpl, err := api.autoImportTemplate(r.Context(), autoImportTemplateOpts{
			name:    string(template),
			archive: archive,
			params:  parameters,
			userID:  user.ID,
			orgID:   organizationID,
		})
		if err != nil {
			api.Logger.Warn(r.Context(), "failed to auto-import template", slog.F("template", template), slog.F("parameters", parameters), slog.Error(err))
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error importing template.",
				Detail:  xerrors.Errorf("failed to import template %q: %w", template, err).Error(),
			})
			return
		}

		api.Logger.Info(r.Context(), "auto-imported template", slog.F("id", tpl.ID), slog.F("template", template), slog.F("parameters", parameters))
	}

	httpapi.Write(rw, http.StatusCreated, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: organizationID,
	})
}

func (api *API) users(rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	params, errs := userSearchQuery(query)
	if len(errs) > 0 {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid user search query.",
			Validations: errs,
		})
		return
	}

	paginationParams, ok := parsePagination(rw, r)
	if !ok {
		return
	}

	users, err := api.Database.GetUsers(r.Context(), database.GetUsersParams{
		AfterID:   paginationParams.AfterID,
		OffsetOpt: int32(paginationParams.Offset),
		LimitOpt:  int32(paginationParams.Limit),
		Search:    params.Search,
		Status:    params.Status,
		RbacRole:  params.RbacRole,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusOK, []codersdk.User{})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching users.",
			Detail:  err.Error(),
		})
		return
	}

	users, err = AuthorizeFilter(api.httpAuth, r, rbac.ActionRead, users)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching users.",
			Detail:  err.Error(),
		})
		return
	}

	userIDs := make([]uuid.UUID, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	organizationIDsByMemberIDsRows, err := api.Database.GetOrganizationIDsByMemberIDs(r.Context(), userIDs)
	if xerrors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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
	// Create the user on the site.
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceUser) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.CreateUserRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	// Create the organization member in the org.
	if !api.Authorize(r, rbac.ActionCreate,
		rbac.ResourceOrganizationMember.InOrg(req.OrganizationID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// TODO: @emyrk Authorize the organization create if the createUser will do that.

	_, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
		Username: req.Username,
		Email:    req.Email,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, codersdk.Response{
			Message: "User already exists.",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	_, err = api.Database.GetOrganizationByID(r.Context(), req.OrganizationID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Organization does not exist with the provided id %q.", req.OrganizationID),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organization.",
			Detail:  err.Error(),
		})
		return
	}

	user, _, err := api.createUser(r.Context(), api.Database, createUserRequest{
		CreateUserRequest: req,
		LoginType:         database.LoginTypePassword,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating user.",
			Detail:  err.Error(),
		})
		return
	}

	// Report when users are added!
	api.Telemetry.Report(&telemetry.Snapshot{
		Users: []telemetry.User{telemetry.ConvertUser(user)},
	})

	httpapi.Write(rw, http.StatusCreated, convertUser(user, []uuid.UUID{req.OrganizationID}))
}

// Returns the parameterized user requested. All validation
// is completed in the middleware for this route.
func (api *API) userByName(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	organizationIDs, err := userOrganizationIDs(r.Context(), api, user)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertUser(user, organizationIDs))
}

func (api *API) putUserProfile(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var params codersdk.UpdateUserProfileRequest
	if !httpapi.Read(rw, r, &params) {
		return
	}
	existentUser, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
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
		httpapi.Write(rw, http.StatusConflict, codersdk.Response{
			Message:     "User already exists.",
			Validations: responseErrors,
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) && isDifferentUser {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	updatedUserProfile, err := api.Database.UpdateUserProfile(r.Context(), database.UpdateUserProfileParams{
		ID:        user.ID,
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
		Username:  params.Username,
		UpdatedAt: database.Now(),
	})

	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user.",
			Detail:  err.Error(),
		})
		return
	}

	organizationIDs, err := userOrganizationIDs(r.Context(), api, user)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertUser(updatedUserProfile, organizationIDs))
}

func (api *API) putUserStatus(status database.UserStatus) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		user := httpmw.UserParam(r)
		apiKey := httpmw.APIKey(r)

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
				httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
					Message: "You cannot suspend yourself.",
				})
				return
			case slice.Contains(user.RBACRoles, rbac.RoleOwner()):
				// You may not suspend an owner
				httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("You cannot suspend a user with the %q role. You must remove the role first.", rbac.RoleOwner()),
				})
				return
			}
		}

		suspendedUser, err := api.Database.UpdateUserStatus(r.Context(), database.UpdateUserStatusParams{
			ID:        user.ID,
			Status:    status,
			UpdatedAt: database.Now(),
		})

		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: fmt.Sprintf("Internal error updating user's status to %q.", status),
				Detail:  err.Error(),
			})
			return
		}

		organizations, err := userOrganizationIDs(r.Context(), api, user)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching user's organizations.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(rw, http.StatusOK, convertUser(suspendedUser, organizations))
	}
}

func (api *API) putUserPassword(rw http.ResponseWriter, r *http.Request) {
	var (
		user   = httpmw.UserParam(r)
		params codersdk.UpdateUserPasswordRequest
	)

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceUserData.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !httpapi.Read(rw, r, &params) {
		return
	}

	err := userpassword.Validate(params.Password)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
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
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error with passwords.",
				Detail:  err.Error(),
			})
			return
		}
		if !ok {
			httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
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
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error hashing new password.",
			Detail:  err.Error(),
		})
		return
	}
	err = api.Database.UpdateUserHashedPassword(r.Context(), database.UpdateUserHashedPasswordParams{
		ID:             user.ID,
		HashedPassword: []byte(hashedPassword),
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user's password.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusNoContent, nil)
}

func (api *API) userRoles(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceUserData.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	resp := codersdk.UserRoles{
		Roles:             user.RBACRoles,
		OrganizationRoles: make(map[uuid.UUID][]string),
	}

	memberships, err := api.Database.GetOrganizationMembershipsByUserID(r.Context(), user.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organization memberships.",
			Detail:  err.Error(),
		})
		return
	}

	// Only include ones we can read from RBAC.
	memberships, err = AuthorizeFilter(api.httpAuth, r, rbac.ActionRead, memberships)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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

	httpapi.Write(rw, http.StatusOK, resp)
}

func (api *API) putUserRoles(rw http.ResponseWriter, r *http.Request) {
	// User is the user to modify.
	user := httpmw.UserParam(r)
	actorRoles := httpmw.AuthorizationUserRoles(r)
	apiKey := httpmw.APIKey(r)

	if apiKey.UserID == user.ID {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "You cannot change your own roles.",
		})
		return
	}

	var params codersdk.UpdateRoles
	if !httpapi.Read(rw, r, &params) {
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

	updatedUser, err := api.updateSiteUserRoles(r.Context(), database.UpdateUserRolesParams{
		GrantedRoles: params.Roles,
		ID:           user.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	organizationIDs, err := userOrganizationIDs(r.Context(), api, user)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertUser(updatedUser, organizationIDs))
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
	user := httpmw.UserParam(r)

	organizations, err := api.Database.GetOrganizationsByUserID(r.Context(), user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		organizations = []database.Organization{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	// Only return orgs the user can read.
	organizations, err = AuthorizeFilter(api.httpAuth, r, rbac.ActionRead, organizations)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organizations.",
			Detail:  err.Error(),
		})
		return
	}

	publicOrganizations := make([]codersdk.Organization, 0, len(organizations))
	for _, organization := range organizations {
		publicOrganizations = append(publicOrganizations, convertOrganization(organization))
	}

	httpapi.Write(rw, http.StatusOK, publicOrganizations)
}

func (api *API) organizationByUserAndName(rw http.ResponseWriter, r *http.Request) {
	organizationName := chi.URLParam(r, "organizationname")
	organization, err := api.Database.GetOrganizationByName(r.Context(), organizationName)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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

	httpapi.Write(rw, http.StatusOK, convertOrganization(organization))
}

// Authenticates the user with an email and password.
func (api *API) postLogin(rw http.ResponseWriter, r *http.Request) {
	var loginWithPassword codersdk.LoginWithPasswordRequest
	if !httpapi.Read(rw, r, &loginWithPassword) {
		return
	}

	user, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
		Email: loginWithPassword.Email,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return
	}

	// If the user doesn't exist, it will be a default struct.
	equal, err := userpassword.Compare(string(user.HashedPassword), loginWithPassword.Password)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return
	}
	if !equal {
		// This message is the same as above to remove ease in detecting whether
		// users are registered or not. Attackers still could with a timing attack.
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Incorrect email or password.",
		})
		return
	}

	if user.LoginType != database.LoginTypePassword {
		httpapi.Write(rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Incorrect login type, attempting to use %q but user is of login type %q", database.LoginTypePassword, user.LoginType),
		})
		return
	}

	// If the user logged into a suspended account, reject the login request.
	if user.Status != database.UserStatusActive {
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Your account is suspended. Contact an admin to reactivate your account.",
		})
		return
	}

	cookie, err := api.createAPIKey(r, createAPIKeyParams{
		UserID:    user.ID,
		LoginType: database.LoginTypePassword,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	http.SetCookie(rw, cookie)

	httpapi.Write(rw, http.StatusCreated, codersdk.LoginWithPasswordResponse{
		SessionToken: cookie.Value,
	})
}

// Creates a new session key, used for logging in via the CLI.
func (api *API) postAPIKey(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceAPIKey.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	lifeTime := time.Hour * 24 * 7
	cookie, err := api.createAPIKey(r, createAPIKeyParams{
		UserID:    user.ID,
		LoginType: database.LoginTypePassword,
		// All api generated keys will last 1 week. Browser login tokens have
		// a shorter life.
		ExpiresAt:       database.Now().Add(lifeTime),
		LifetimeSeconds: int64(lifeTime.Seconds()),
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	// We intentionally do not set the cookie on the response here.
	// Setting the cookie will couple the browser sesion to the API
	// key we return here, meaning logging out of the website would
	// invalid your CLI key.
	httpapi.Write(rw, http.StatusCreated, codersdk.GenerateAPIKeyResponse{Key: cookie.Value})
}

func (api *API) apiKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
	)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceAPIKey.WithOwner(user.ID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}

	keyID := chi.URLParam(r, "keyid")
	key, err := api.Database.GetAPIKeyByID(ctx, keyID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching API key.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertAPIKey(key))
}

// Clear the user's session cookie.
func (api *API) postLogout(rw http.ResponseWriter, r *http.Request) {
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
	err := api.Database.DeleteAPIKeyByID(r.Context(), apiKey.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting API key.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, codersdk.Response{
		Message: "Logged out!",
	})
}

// Generates a new ID and secret for an API key.
func generateAPIKeyIDSecret() (id string, secret string, err error) {
	// Length of an API Key ID.
	id, err = cryptorand.String(10)
	if err != nil {
		return "", "", err
	}
	// Length of an API Key secret.
	secret, err = cryptorand.String(22)
	if err != nil {
		return "", "", err
	}
	return id, secret, nil
}

type createAPIKeyParams struct {
	UserID    uuid.UUID
	LoginType database.LoginType

	// Optional.
	ExpiresAt       time.Time
	LifetimeSeconds int64
}

func (api *API) createAPIKey(r *http.Request, params createAPIKeyParams) (*http.Cookie, error) {
	keyID, keySecret, err := generateAPIKeyIDSecret()
	if err != nil {
		return nil, xerrors.Errorf("generate API key: %w", err)
	}
	hashed := sha256.Sum256([]byte(keySecret))

	// Default expires at to now+lifetime, or just 24hrs if not set
	if params.ExpiresAt.IsZero() {
		if params.LifetimeSeconds != 0 {
			params.ExpiresAt = database.Now().Add(time.Duration(params.LifetimeSeconds) * time.Second)
		} else {
			params.ExpiresAt = database.Now().Add(24 * time.Hour)
		}
	}

	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if ip == nil {
		ip = net.IPv4(0, 0, 0, 0)
	}
	bitlen := len(ip) * 8
	key, err := api.Database.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
		ID:              keyID,
		UserID:          params.UserID,
		LifetimeSeconds: params.LifetimeSeconds,
		IPAddress: pqtype.Inet{
			IPNet: net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(bitlen, bitlen),
			},
			Valid: true,
		},
		// Make sure in UTC time for common time zone
		ExpiresAt:    params.ExpiresAt.UTC(),
		CreatedAt:    database.Now(),
		UpdatedAt:    database.Now(),
		HashedSecret: hashed[:],
		LoginType:    params.LoginType,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert API key: %w", err)
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		APIKeys: []telemetry.APIKey{telemetry.ConvertAPIKey(key)},
	})

	// This format is consumed by the APIKey middleware.
	sessionToken := fmt.Sprintf("%s-%s", keyID, keySecret)
	return &http.Cookie{
		Name:     codersdk.SessionTokenKey,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   api.SecureAuthCookie,
	}, nil
}

type createUserRequest struct {
	codersdk.CreateUserRequest
	LoginType database.LoginType
}

func (api *API) createUser(ctx context.Context, store database.Store, req createUserRequest) (database.User, uuid.UUID, error) {
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
		LifetimeSeconds: k.LifetimeSeconds,
	}
}
