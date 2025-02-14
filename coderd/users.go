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

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// userDebugOIDC returns the OIDC debug context for the user.
// Not going to expose this via swagger as the return payload is not guaranteed
// to be consistent between releases.
//
// @Summary Debug OIDC context for a user
// @ID debug-oidc-context-for-a-user
// @Security CoderSessionToken
// @Tags Agents
// @Success 200 "Success"
// @Param user path string true "User ID, name, or me"
// @Router /debug/{user}/debug-link [get]
// @x-apidocgen {"skip": true}
func (api *API) userDebugOIDC(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
	)

	if user.LoginType != database.LoginTypeOIDC {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "User is not an OIDC user.",
		})
		return
	}

	link, err := api.Database.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
		UserID:    user.ID,
		LoginType: database.LoginTypeOIDC,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get user links.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, link.Claims)
}

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
		err = api.TrialGenerator(ctx, codersdk.LicensorTrialRequest{
			Email:       createUser.Email,
			FirstName:   createUser.TrialInfo.FirstName,
			LastName:    createUser.TrialInfo.LastName,
			PhoneNumber: createUser.TrialInfo.PhoneNumber,
			JobTitle:    createUser.TrialInfo.JobTitle,
			CompanyName: createUser.TrialInfo.CompanyName,
			Country:     createUser.TrialInfo.Country,
			Developers:  createUser.TrialInfo.Developers,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate trial",
				Detail:  err.Error(),
			})
			return
		}
	}

	//nolint:gocritic // needed to create first user
	defaultOrg, err := api.Database.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching default organization. If you are encountering this error, you will have to restart the Coder deployment.",
			Detail:  err.Error(),
		})
		return
	}

	//nolint:gocritic // needed to create first user
	user, err := api.CreateUser(dbauthz.AsSystemRestricted(ctx), api.Database, CreateUserRequest{
		CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
			Email:    createUser.Email,
			Username: createUser.Username,
			Name:     createUser.Name,
			Password: createUser.Password,
			// There's no reason to create the first user as dormant, since you have
			// to login immediately anyways.
			UserStatus:      ptr.Ref(codersdk.UserStatusActive),
			OrganizationIDs: []uuid.UUID{defaultOrg.ID},
		},
		LoginType:          database.LoginTypePassword,
		accountCreatorName: "coder",
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating user.",
			Detail:  err.Error(),
		})
		return
	}

	if api.RefreshEntitlements != nil {
		err = api.RefreshEntitlements(ctx)
		if err != nil {
			api.Logger.Error(ctx, "failed to refresh entitlements after generating trial license")
			return
		}
	} else {
		api.Logger.Debug(ctx, "entitlements will not be refreshed")
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
		GrantedRoles: []string{rbac.RoleOwner().String()},
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
		OrganizationID: defaultOrg.ID,
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
		CreatedAfter:   params.CreatedAfter,
		CreatedBefore:  params.CreatedBefore,
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
// @Param request body codersdk.CreateUserRequestWithOrgs true "Create user request"
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

	var req codersdk.CreateUserRequestWithOrgs
	if !httpapi.Read(ctx, rw, r, &req) {
		return
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

	if len(req.OrganizationIDs) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "No organization specified to place the user as a member of. It is required to specify at least one organization id to place the user in.",
			Detail:  "required at least 1 value for the array 'organization_ids'",
			Validations: []codersdk.ValidationError{
				{
					Field:  "organization_ids",
					Detail: "Missing values, this cannot be empty",
				},
			},
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

	// If an organization was provided, make sure it exists.
	for i, orgID := range req.OrganizationIDs {
		var orgErr error
		if orgID != uuid.Nil {
			_, orgErr = api.Database.GetOrganizationByID(ctx, orgID)
		} else {
			var defaultOrg database.Organization
			defaultOrg, orgErr = api.Database.GetDefaultOrganization(ctx)
			if orgErr == nil {
				// converts uuid.Nil --> default org.ID
				req.OrganizationIDs[i] = defaultOrg.ID
			}
		}
		if orgErr != nil {
			if httpapi.Is404Error(orgErr) {
				httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
					Message: fmt.Sprintf("Organization does not exist with the provided id %q.", orgID),
				})
				return
			}

			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching organization.",
				Detail:  orgErr.Error(),
			})
			return
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
		if api.OIDCConfig == nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "You must configure OIDC before creating OIDC users.",
			})
			return
		}
		loginType = database.LoginTypeOIDC
	case codersdk.LoginTypeGithub:
		loginType = database.LoginTypeGithub
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Unsupported login type %q for manually creating new users.", req.UserLoginType),
		})
		return
	}

	apiKey := httpmw.APIKey(r)

	accountCreator, err := api.Database.GetUserByID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Unable to determine the details of the actor creating the account.",
		})
		return
	}

	user, err := api.CreateUser(ctx, api.Database, CreateUserRequest{
		CreateUserRequestWithOrgs: req,
		LoginType:                 loginType,
		accountCreatorName:        accountCreator.Name,
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

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.User(user, req.OrganizationIDs))
}

// @Summary Delete user
// @ID delete-user
// @Security CoderSessionToken
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200
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

	if auth.ID == user.ID.String() {
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

	err = api.Database.UpdateUserDeletedByID(ctx, user.ID)
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

	userAdmins, err := findUserAdmins(ctx, api.Database)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user admins.",
			Detail:  err.Error(),
		})
		return
	}

	apiKey := httpmw.APIKey(r)

	accountDeleter, err := api.Database.GetUserByID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Unable to determine the details of the actor deleting the account.",
		})
		return
	}

	for _, u := range userAdmins {
		// nolint: gocritic // Need notifier actor to enqueue notifications
		if _, err := api.NotificationsEnqueuer.Enqueue(dbauthz.AsNotifier(ctx), u.ID, notifications.TemplateUserAccountDeleted,
			map[string]string{
				"deleted_account_name":      user.Username,
				"deleted_account_user_name": user.Name,
				"initiator":                 accountDeleter.Name,
			},
			"api-users-delete",
			user.ID,
		); err != nil {
			api.Logger.Warn(ctx, "unable to notify about deleted user", slog.F("deleted_user", user.Username), slog.Error(err))
		}
	}

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
// @Param user path string true "User ID, username, or me"
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

// Returns recent build parameters for the signed-in user.
//
// @Summary Get autofill build parameters for user
// @ID get-autofill-build-parameters-for-user
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param user path string true "User ID, username, or me"
// @Param template_id query string true "Template ID"
// @Success 200 {array} codersdk.UserParameter
// @Router /users/{user}/autofill-parameters [get]
func (api *API) userAutofillParameters(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	p := httpapi.NewQueryParamParser().RequiredNotEmpty("template_id")
	templateID := p.UUID(r.URL.Query(), uuid.UUID{}, "template_id")
	p.ErrorExcessParams(r.URL.Query())
	if len(p.Errors) > 0 {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: p.Errors,
		})
		return
	}

	params, err := api.Database.GetUserWorkspaceBuildParameters(
		r.Context(),
		database.GetUserWorkspaceBuildParametersParams{
			OwnerID:    user.ID,
			TemplateID: templateID,
		},
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's parameters.",
			Detail:  err.Error(),
		})
		return
	}

	sdkParams := []codersdk.UserParameter{}
	for _, param := range params {
		sdkParams = append(sdkParams, codersdk.UserParameter{
			Name:  param.Name,
			Value: param.Value,
		})
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, sdkParams)
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
		responseErrors := []codersdk.ValidationError{{
			Field:  "username",
			Detail: "This username is already in use.",
		}}
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message:     "A user with this username already exists.",
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
		Name:      params.Name,
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
			case slice.Contains(user.RBACRoles, rbac.RoleOwner().String()):
				// You may not suspend an owner
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("You cannot suspend a user with the %q role. You must remove the role first.", rbac.RoleOwner()),
				})
				return
			}
		}

		actingUser, err := api.Database.GetUserByID(ctx, apiKey.UserID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Unable to determine the details of the actor creating the account.",
			})
			return
		}

		targetUser, err := api.Database.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
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
		aReq.New = targetUser

		err = api.notifyUserStatusChanged(ctx, actingUser.Name, user, status)
		if err != nil {
			api.Logger.Warn(ctx, "unable to notify about changed user's status", slog.F("affected_user", user.Username), slog.Error(err))
		}

		organizations, err := userOrganizationIDs(ctx, api, user)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching user's organizations.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, db2sdk.User(targetUser, organizations))
	}
}

func (api *API) notifyUserStatusChanged(ctx context.Context, actingUserName string, targetUser database.User, status database.UserStatus) error {
	var labels map[string]string
	var data map[string]any
	var adminTemplateID, personalTemplateID uuid.UUID
	switch status {
	case database.UserStatusSuspended:
		labels = map[string]string{
			"suspended_account_name":      targetUser.Username,
			"suspended_account_user_name": targetUser.Name,
			"initiator":                   actingUserName,
		}
		data = map[string]any{
			"user": map[string]any{"id": targetUser.ID, "name": targetUser.Name, "email": targetUser.Email},
		}
		adminTemplateID = notifications.TemplateUserAccountSuspended
		personalTemplateID = notifications.TemplateYourAccountSuspended
	case database.UserStatusActive:
		labels = map[string]string{
			"activated_account_name":      targetUser.Username,
			"activated_account_user_name": targetUser.Name,
			"initiator":                   actingUserName,
		}
		data = map[string]any{
			"user": map[string]any{"id": targetUser.ID, "name": targetUser.Name, "email": targetUser.Email},
		}
		adminTemplateID = notifications.TemplateUserAccountActivated
		personalTemplateID = notifications.TemplateYourAccountActivated
	default:
		api.Logger.Error(ctx, "user status is not supported", slog.F("username", targetUser.Username), slog.F("user_status", string(status)))
		return xerrors.Errorf("unable to notify admins as the user's status is unsupported")
	}

	userAdmins, err := findUserAdmins(ctx, api.Database)
	if err != nil {
		api.Logger.Error(ctx, "unable to find user admins", slog.Error(err))
	}

	// Send notifications to user admins and affected user
	for _, u := range userAdmins {
		// nolint:gocritic // Need notifier actor to enqueue notifications
		if _, err := api.NotificationsEnqueuer.EnqueueWithData(dbauthz.AsNotifier(ctx), u.ID, adminTemplateID,
			labels, data, "api-put-user-status",
			targetUser.ID,
		); err != nil {
			api.Logger.Warn(ctx, "unable to notify about changed user's status", slog.F("affected_user", targetUser.Username), slog.Error(err))
		}
	}
	// nolint:gocritic // Need notifier actor to enqueue notifications
	if _, err := api.NotificationsEnqueuer.EnqueueWithData(dbauthz.AsNotifier(ctx), targetUser.ID, personalTemplateID,
		labels, data, "api-put-user-status",
		targetUser.ID,
	); err != nil {
		api.Logger.Warn(ctx, "unable to notify user about status change of their account", slog.F("affected_user", targetUser.Username), slog.Error(err))
	}
	return nil
}

// @Summary Get user appearance settings
// @ID get-user-appearance-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.UserAppearanceSettings
// @Router /users/{user}/appearance [get]
func (api *API) userAppearanceSettings(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
	)

	themePreference, err := api.Database.GetUserAppearanceSettings(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserAppearanceSettings{
		ThemePreference: themePreference,
	})
}

// @Summary Update user appearance settings
// @ID update-user-appearance-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.UpdateUserAppearanceSettingsRequest true "New appearance settings"
// @Success 200 {object} codersdk.UserAppearanceSettings
// @Router /users/{user}/appearance [put]
func (api *API) putUserAppearanceSettings(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
	)

	var params codersdk.UpdateUserAppearanceSettingsRequest
	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	updatedSettings, err := api.Database.UpdateUserAppearanceSettings(ctx, database.UpdateUserAppearanceSettingsParams{
		UserID:          user.ID,
		ThemePreference: params.ThemePreference,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating user.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserAppearanceSettings{
		ThemePreference: updatedSettings.Value,
	})
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

	if !api.Authorize(r, policy.ActionUpdatePersonal, user) {
		httpapi.ResourceNotFound(rw)
		return
	}

	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	if user.LoginType != database.LoginTypePassword {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Users without password login type cannot change their password.",
		})
		return
	}

	// A user need to put its own password to update it
	if apiKey.UserID == user.ID && params.OldPassword == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Old password is required.",
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

	rw.WriteHeader(http.StatusNoContent)
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

	if !api.Authorize(r, policy.ActionReadPersonal, user) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// TODO: Replace this with "GetAuthorizationUserRoles"
	resp := codersdk.UserRoles{
		Roles:             user.RBACRoles,
		OrganizationRoles: make(map[uuid.UUID][]string),
	}

	memberships, err := api.Database.OrganizationMembers(ctx, database.OrganizationMembersParams{
		UserID:         user.ID,
		OrganizationID: uuid.Nil,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user's organization memberships.",
			Detail:  err.Error(),
		})
		return
	}

	for _, mem := range memberships {
		resp.OrganizationRoles[mem.OrganizationMember.OrganizationID] = mem.OrganizationMember.Roles
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

	if user.LoginType == database.LoginTypeOIDC && api.IDPSync.SiteRoleSyncEnabled() {
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

	updatedUser, err := api.Database.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
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
	organizations, err = AuthorizeFilter(api.HTTPAuth, r, policy.ActionRead, organizations)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching organizations.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.List(organizations, db2sdk.Organization))
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

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Organization(organization))
}

type CreateUserRequest struct {
	codersdk.CreateUserRequestWithOrgs
	LoginType          database.LoginType
	SkipNotifications  bool
	accountCreatorName string
}

func (api *API) CreateUser(ctx context.Context, store database.Store, req CreateUserRequest) (database.User, error) {
	// Ensure the username is valid. It's the caller's responsibility to ensure
	// the username is valid and unique.
	if usernameValid := codersdk.NameValid(req.Username); usernameValid != nil {
		return database.User{}, xerrors.Errorf("invalid username %q: %w", req.Username, usernameValid)
	}

	var user database.User
	err := store.InTx(func(tx database.Store) error {
		orgRoles := make([]string, 0)

		status := ""
		if req.UserStatus != nil {
			status = string(*req.UserStatus)
		}
		params := database.InsertUserParams{
			ID:             uuid.New(),
			Email:          req.Email,
			Username:       req.Username,
			Name:           codersdk.NormalizeRealUsername(req.Name),
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			HashedPassword: []byte{},
			// All new users are defaulted to members of the site.
			RBACRoles: []string{},
			LoginType: req.LoginType,
			Status:    status,
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

		for _, orgID := range req.OrganizationIDs {
			_, err = tx.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
				OrganizationID: orgID,
				UserID:         user.ID,
				CreatedAt:      dbtime.Now(),
				UpdatedAt:      dbtime.Now(),
				// By default give them membership to the organization.
				Roles: orgRoles,
			})
			if err != nil {
				return xerrors.Errorf("create organization member for %q: %w", orgID.String(), err)
			}
		}

		return nil
	}, nil)
	if err != nil || req.SkipNotifications {
		return user, err
	}

	userAdmins, err := findUserAdmins(ctx, store)
	if err != nil {
		return user, xerrors.Errorf("find user admins: %w", err)
	}

	for _, u := range userAdmins {
		if _, err := api.NotificationsEnqueuer.EnqueueWithData(
			// nolint:gocritic // Need notifier actor to enqueue notifications
			dbauthz.AsNotifier(ctx),
			u.ID,
			notifications.TemplateUserAccountCreated,
			map[string]string{
				"created_account_name":      user.Username,
				"created_account_user_name": user.Name,
				"initiator":                 req.accountCreatorName,
			},
			map[string]any{
				"user": map[string]any{"id": user.ID, "name": user.Name, "email": user.Email},
			},
			"api-users-create",
			user.ID,
		); err != nil {
			api.Logger.Warn(ctx, "unable to notify about created user", slog.F("created_user", user.Username), slog.Error(err))
		}
	}

	return user, err
}

// findUserAdmins fetches all users with user admin permission including owners.
func findUserAdmins(ctx context.Context, store database.Store) ([]database.GetUsersRow, error) {
	// Notice: we can't scrape the user information in parallel as pq
	// fails with: unexpected describe rows response: 'D'
	owners, err := store.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleOwner},
	})
	if err != nil {
		return nil, xerrors.Errorf("get owners: %w", err)
	}
	userAdmins, err := store.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleUserAdmin},
	})
	if err != nil {
		return nil, xerrors.Errorf("get user admins: %w", err)
	}
	return append(owners, userAdmins...), nil
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
	if err != nil {
		return []uuid.UUID{}, err
	}

	// If you are in no orgs, then return an empty list.
	if len(organizationIDsByMemberIDsRows) == 0 {
		return []uuid.UUID{}, nil
	}

	member := organizationIDsByMemberIDsRows[0]
	return member.OrganizationIDs, nil
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
