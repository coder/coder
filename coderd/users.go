package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// Returns whether the initial user has been created or not.
//
// @Summary Check initial user created
// @ID check-initial-user-created
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {object} codersdk.Response
// @Router /users/first [get]
func (api *API) firstUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// nolint:gocritic // Getting user count is a system function.
	userCount, err := api.Database.GetUserCount(dbauthz.AsSystemRestricted(ctx))
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
//
// @Summary Create initial user
// @ID create-initial-user
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param request body codersdk.CreateFirstUserRequest true "First user request"
// @Success 201 {object} codersdk.CreateFirstUserResponse
// @Router /users/first [post]
func (api *API) postFirstUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var createUser codersdk.CreateFirstUserRequest
	if !httpapi.Read(ctx, rw, r, &createUser) {
		return
	}

	// This should only function for the first user.
	// nolint:gocritic // Getting user count is a system function.
	userCount, err := api.Database.GetUserCount(dbauthz.AsSystemRestricted(ctx))
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

	err = userpassword.Validate(createUser.Password)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Password not strong enough!",
			Validations: []codersdk.ValidationError{{
				Field:  "password",
				Detail: err.Error(),
			}},
		})
		return
	}

	if createUser.Trial && api.TrialGenerator != nil {
		err = api.TrialGenerator(ctx, createUser.Email)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate trial",
				Detail:  err.Error(),
			})
			return
		}
	}

	//nolint:gocritic // needed to create first user
	user, organizationID, err := api.CreateUser(dbauthz.AsSystemRestricted(ctx), api.Database, CreateUserRequest{
		CreateUserRequest: codersdk.CreateUserRequest{
			Email:    createUser.Email,
			Username: createUser.Username,
			Password: createUser.Password,
			// Create an org for the first user.
			OrganizationID: uuid.Nil,
		},
		CreateOrganization: true,
		LoginType:          database.LoginTypePassword,
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
	//nolint:gocritic // needed to create first user
	_, err = api.Database.UpdateUserRoles(dbauthz.AsSystemRestricted(ctx), database.UpdateUserRolesParams{
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

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: organizationID,
	})
}

// @Summary Get users
// @ID get-users
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param q query string false "Search query"
// @Param after_id query string false "After ID" format(uuid)
// @Param limit query int false "Page limit"
// @Param offset query int false "Page offset"
// @Success 200 {object} codersdk.GetUsersResponse
// @Router /users [get]
func (api *API) users(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, userCount, ok := api.GetUsers(rw, r)
	if !ok {
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
	render.JSON(rw, r, codersdk.GetUsersResponse{
		Users: convertUsers(users, organizationIDsByUserID),
		Count: int(userCount),
	})
}

func (api *API) GetUsers(rw http.ResponseWriter, r *http.Request) ([]database.User, int64, bool) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	params, errs := searchquery.Users(query)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid user search query.",
			Validations: errs,
		})
		return nil, -1, false
	}

	paginationParams, ok := parsePagination(rw, r)
	if !ok {
		return nil, -1, false
	}

	userRows, err := api.Database.GetUsers(ctx, database.GetUsersParams{
		AfterID:        paginationParams.AfterID,
		Search:         params.Search,
		Status:         params.Status,
		RbacRole:       params.RbacRole,
		LastSeenBefore: params.LastSeenBefore,
		LastSeenAfter:  params.LastSeenAfter,
		OffsetOpt:      int32(paginationParams.Offset),
		LimitOpt:       int32(paginationParams.Limit),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching users.",
			Detail:  err.Error(),
		})
		return nil, -1, false
	}

	// GetUsers does not return ErrNoRows because it uses a window function to get the count.
	// So we need to check if the userRows is empty and return an empty array if so.
	if len(userRows) == 0 {
		return []database.User{}, 0, true
	}

	users := database.ConvertUserRows(userRows)
	return users, userRows[0].Count, true
}

// Creates a new user.
//
// @Summary Create new user
// @ID create-new-user
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param request body codersdk.CreateUserRequest true "Create user request"
// @Success 201 {object} codersdk.User
// @Router /users [post]
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

	var req codersdk.CreateUserRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.UserLoginType == "" && req.DisableLogin {
		// Handle the deprecated field
		req.UserLoginType = codersdk.LoginTypeNone
	}
	if req.UserLoginType == "" {
		// Default to password auth
		req.UserLoginType = codersdk.LoginTypePassword
	}

	if req.UserLoginType != codersdk.LoginTypePassword && req.Password != "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Password cannot be set for non-password (%q) authentication.", req.UserLoginType),
		})
		return
	}

	// If password auth is disabled, don't allow new users to be
	// created with a password!
	if api.DeploymentValues.DisablePasswordAuth && req.UserLoginType == codersdk.LoginTypePassword {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Password based authentication is disabled! Unable to provision new users with password authentication.",
		})
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

	if req.OrganizationID != uuid.Nil {
		// If an organization was provided, make sure it exists.
		_, err := api.Database.GetOrganizationByID(ctx, req.OrganizationID)
		if err != nil {
			if httpapi.Is404Error(err) {
				httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
					Message: fmt.Sprintf("Organization does not exist with the provided id %q.", req.OrganizationID),
				})
				return
			}

			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching organization.",
				Detail:  err.Error(),
			})
			return
		}
	} else {
		// If no organization is provided, add the user to the first
		// organization.
		organizations, err := api.Database.GetOrganizations(ctx)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching orgs.",
				Detail:  err.Error(),
			})
			return
		}

		if len(organizations) > 0 {
			// Add the user to the first organization. Once multi-organization
			// support is added, we should enable a configuration map of user
			// email to organization.
			req.OrganizationID = organizations[0].ID
		}
	}

	var loginType database.LoginType
	switch req.UserLoginType {
	case codersdk.LoginTypeNone:
		loginType = database.LoginTypeNone
	case codersdk.LoginTypePassword:
		err = userpassword.Validate(req.Password)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Password not strong enough!",
				Validations: []codersdk.ValidationError{{
					Field:  "password",
					Detail: err.Error(),
				}},
			})
			return
		}
		loginType = database.LoginTypePassword
	case codersdk.LoginTypeOIDC:
		loginType = database.LoginTypeOIDC
	case codersdk.LoginTypeGithub:
		loginType = database.LoginTypeGithub
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Unsupported login type %q for manually creating new users.", req.UserLoginType),
		})
	}

	user, _, err := api.CreateUser(ctx, api.Database, CreateUserRequest{
		CreateUserRequest: req,
		LoginType:         loginType,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You are not authorized to create users.",
		})
		return
	}
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

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.User(user, []uuid.UUID{req.OrganizationID}))
}

// @Summary Delete user
// @ID delete-user
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.User
// @Router /users/{user} [delete]
func (api *API) deleteUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.Auditor.Load()
	user := httpmw.UserParam(r)
	auth := httpmw.UserAuthorization(r)
	aReq, commitAudit := audit.InitRequest[database.User](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionDelete,
	})
	aReq.Old = user
	defer commitAudit()

	if auth.Actor.ID == user.ID.String() {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You cannot delete yourself!",
		})
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
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
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
//
// @Summary Get user by name
// @ID get-user-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.User
// @Router /users/{user} [get]
func (api *API) userByName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	organizationIDs, err := userOrganizationIDs(ctx, api, user)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.User(user, organizationIDs))
}

// Returns the user's login type. This only works if the api key for authorization
// and the requested user match. Eg: 'me'
//
// @Summary Get user login type
// @ID get-user-login-type
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.UserLoginType
// @Router /users/{user}/login-type [get]
func (*API) userLoginType(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
		key  = httpmw.APIKey(r)
	)

	if key.UserID != user.ID {
		// Currently this is only valid for querying yourself.
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You are not authorized to view this user's login type.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserLoginType{
		LoginType: codersdk.LoginType(user.LoginType),
	})
}

// @Summary Update user profile
// @ID update-user-profile
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.UpdateUserProfileRequest true "Updated profile"
// @Success 200 {object} codersdk.User
// @Router /users/{user}/profile [put]
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
		UpdatedAt: dbtime.Now(),
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

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.User(updatedUserProfile, organizationIDs))
}

// @Summary Suspend user account
// @ID suspend-user-account
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.User
// @Router /users/{user}/status/suspend [put]
func (api *API) putSuspendUserAccount() func(rw http.ResponseWriter, r *http.Request) {
	return api.putUserStatus(database.UserStatusSuspended)
}

// @Summary Activate user account
// @ID activate-user-account
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.User
// @Router /users/{user}/status/activate [put]
func (api *API) putActivateUserAccount() func(rw http.ResponseWriter, r *http.Request) {
	return api.putUserStatus(database.UserStatusActive)
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
			UpdatedAt: dbtime.Now(),
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

		httpapi.Write(ctx, rw, http.StatusOK, db2sdk.User(suspendedUser, organizations))
	}
}

// @Summary Update user password
// @ID update-user-password
// @Security CoderSessionToken
// @Accept json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.UpdateUserPasswordRequest true "Update password request"
// @Success 204
// @Router /users/{user}/password [put]
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

	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	if user.LoginType != database.LoginTypePassword {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Users without password login type cannot change their password.",
		})
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
	if params.OldPassword != "" {
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

	// Prevent users reusing their old password.
	if match, _ := userpassword.Compare(string(user.HashedPassword), params.Password); match {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "New password cannot match old password.",
		})
		return
	}

	hashedPassword, err := userpassword.Hash(params.Password)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error hashing new password.",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.InTx(func(tx database.Store) error {
		err = tx.UpdateUserHashedPassword(ctx, database.UpdateUserHashedPasswordParams{
			ID:             user.ID,
			HashedPassword: []byte(hashedPassword),
		})
		if err != nil {
			return xerrors.Errorf("update user hashed password: %w", err)
		}

		err = tx.DeleteAPIKeysByUserID(ctx, user.ID)
		if err != nil {
			return xerrors.Errorf("delete api keys by user ID: %w", err)
		}

		return nil
	}, nil)
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

// @Summary Get user roles
// @ID get-user-roles
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.User
// @Router /users/{user}/roles [get]
func (api *API) userRoles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	if !api.Authorize(r, rbac.ActionRead, user.UserDataRBACObject()) {
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

	for _, mem := range memberships {
		// If we can read the org member, include the roles.
		if err == nil {
			resp.OrganizationRoles[mem.OrganizationID] = mem.Roles
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Assign role to user
// @ID assign-role-to-user
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.UpdateRoles true "Update roles request"
// @Success 200 {object} codersdk.User
// @Router /users/{user}/roles [put]
func (api *API) putUserRoles(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx = r.Context()
		// User is the user to modify.
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

	if user.LoginType == database.LoginTypeOIDC && api.OIDCConfig.RoleSyncEnabled() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot modify roles for OIDC users when role sync is enabled.",
			Detail:  "'User Role Field' is set in the OIDC configuration. All role changes must come from the oidc identity provider.",
		})
		return
	}

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

	updatedUser, err := UpdateSiteUserRoles(ctx, api.Database, database.UpdateUserRolesParams{
		GrantedRoles: params.Roles,
		ID:           user.ID,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
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

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.User(updatedUser, organizationIDs))
}

// UpdateSiteUserRoles will ensure only site wide roles are passed in as arguments.
// If an organization role is included, an error is returned.
func UpdateSiteUserRoles(ctx context.Context, db database.Store, args database.UpdateUserRolesParams) (database.User, error) {
	// Enforce only site wide roles.
	for _, r := range args.GrantedRoles {
		if _, ok := rbac.IsOrgRole(r); ok {
			return database.User{}, xerrors.Errorf("Must only update site wide roles")
		}

		if _, err := rbac.RoleByName(r); err != nil {
			return database.User{}, xerrors.Errorf("%q is not a supported role", r)
		}
	}

	updatedUser, err := db.UpdateUserRoles(ctx, args)
	if err != nil {
		return database.User{}, xerrors.Errorf("update site roles: %w", err)
	}
	return updatedUser, nil
}

// Returns organizations the parameterized user has access to.
//
// @Summary Get organizations by user
// @ID get-organizations-by-user
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {array} codersdk.Organization
// @Router /users/{user}/organizations [get]
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

// @Summary Get organization by user and organization name
// @ID get-organization-by-user-and-organization-name
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param organizationname path string true "Organization name"
// @Success 200 {object} codersdk.Organization
// @Router /users/{user}/organizations/{organizationname} [get]
func (api *API) organizationByUserAndName(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organizationName := chi.URLParam(r, "organizationname")
	organization, err := api.Database.GetOrganizationByName(ctx, organizationName)
	if httpapi.Is404Error(err) {
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

	httpapi.Write(ctx, rw, http.StatusOK, convertOrganization(organization))
}

type CreateUserRequest struct {
	codersdk.CreateUserRequest
	CreateOrganization bool
	LoginType          database.LoginType
}

func (api *API) CreateUser(ctx context.Context, store database.Store, req CreateUserRequest) (database.User, uuid.UUID, error) {
	// Ensure the username is valid. It's the caller's responsibility to ensure
	// the username is valid and unique.
	if usernameValid := httpapi.NameValid(req.Username); usernameValid != nil {
		return database.User{}, uuid.Nil, xerrors.Errorf("invalid username %q: %w", req.Username, usernameValid)
	}

	var user database.User
	return user, req.OrganizationID, store.InTx(func(tx database.Store) error {
		orgRoles := make([]string, 0)
		// If no organization is provided, create a new one for the user.
		if req.OrganizationID == uuid.Nil {
			if !req.CreateOrganization {
				return xerrors.Errorf("organization ID must be provided")
			}

			organization, err := tx.InsertOrganization(ctx, database.InsertOrganizationParams{
				ID:        uuid.New(),
				Name:      req.Username,
				CreatedAt: dbtime.Now(),
				UpdatedAt: dbtime.Now(),
			})
			if err != nil {
				return xerrors.Errorf("create organization: %w", err)
			}
			req.OrganizationID = organization.ID
			// TODO: When organizations are allowed to be created, we should
			// come back to determining the default role of the person who
			// creates the org. Until that happens, all users in an organization
			// should be just regular members.
			orgRoles = append(orgRoles, rbac.RoleOrgMember(req.OrganizationID))

			_, err = tx.InsertAllUsersGroup(ctx, organization.ID)
			if err != nil {
				return xerrors.Errorf("create %q group: %w", database.EveryoneGroup, err)
			}
		}

		params := database.InsertUserParams{
			ID:        uuid.New(),
			Email:     req.Email,
			Username:  req.Username,
			CreatedAt: dbtime.Now(),
			UpdatedAt: dbtime.Now(),
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
			CreatedAt:  dbtime.Now(),
			UpdatedAt:  dbtime.Now(),
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		})
		if err != nil {
			return xerrors.Errorf("insert user gitsshkey: %w", err)
		}
		_, err = tx.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
			OrganizationID: req.OrganizationID,
			UserID:         user.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			// By default give them membership to the organization.
			Roles: orgRoles,
		})
		if err != nil {
			return xerrors.Errorf("create organization member: %w", err)
		}
		return nil
	}, nil)
}

func convertUsers(users []database.User, organizationIDsByUserID map[uuid.UUID][]uuid.UUID) []codersdk.User {
	converted := make([]codersdk.User, 0, len(users))
	for _, u := range users {
		userOrganizationIDs := organizationIDsByUserID[u.ID]
		converted = append(converted, db2sdk.User(u, userOrganizationIDs))
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
		TokenName:       k.TokenName,
	}
}
