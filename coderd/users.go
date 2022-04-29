package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

// Returns whether the initial user has been created or not.
func (api *api) firstUser(rw http.ResponseWriter, r *http.Request) {
	userCount, err := api.Database.GetUserCount(r.Context())
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user count: %s", err.Error()),
		})
		return
	}

	if userCount == 0 {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "The initial user has not been created!",
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, httpapi.Response{
		Message: "The initial user has already been created!",
	})
}

// Creates the initial user for a Coder deployment.
func (api *api) postFirstUser(rw http.ResponseWriter, r *http.Request) {
	var createUser codersdk.CreateFirstUserRequest
	if !httpapi.Read(rw, r, &createUser) {
		return
	}

	// This should only function for the first user.
	userCount, err := api.Database.GetUserCount(r.Context())
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user count: %s", err.Error()),
		})
		return
	}

	// If a user already exists, the initial admin user no longer can be created.
	if userCount != 0 {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: "the initial user has already been created",
		})
		return
	}

	user, organizationID, err := api.createUser(r.Context(), codersdk.CreateUserRequest{
		Email:    createUser.Email,
		Username: createUser.Username,
		Password: createUser.Password,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	// TODO: @emyrk this currently happens outside the database tx used to create
	// 	the user. Maybe I add this ability to grant roles in the createUser api
	//	and add some rbac bypass when calling api functions this way??
	// Add the admin role to this first user
	_, err = api.Database.UpdateUserRoles(r.Context(), database.UpdateUserRolesParams{
		GrantedRoles: []string{rbac.RoleAdmin(), rbac.RoleMember()},
		ID:           user.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: organizationID,
	})
}

func (api *api) users(rw http.ResponseWriter, r *http.Request) {
	var (
		afterArg     = r.URL.Query().Get("after_user")
		limitArg     = r.URL.Query().Get("limit")
		offsetArg    = r.URL.Query().Get("offset")
		searchName   = r.URL.Query().Get("search")
		statusFilter = r.URL.Query().Get("status")
	)

	// createdAfter is a user uuid.
	createdAfter := uuid.Nil
	if afterArg != "" {
		after, err := uuid.Parse(afterArg)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: fmt.Sprintf("after_user must be a valid uuid: %s", err.Error()),
			})
			return
		}
		createdAfter = after
	}

	// Default to no limit and return all users.
	pageLimit := -1
	if limitArg != "" {
		limit, err := strconv.Atoi(limitArg)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: fmt.Sprintf("limit must be an integer: %s", err.Error()),
			})
			return
		}
		pageLimit = limit
	}

	// The default for empty string is 0.
	offset, err := strconv.ParseInt(offsetArg, 10, 64)
	if offsetArg != "" && err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("offset must be an integer: %s", err.Error()),
		})
		return
	}

	users, err := api.Database.GetUsers(r.Context(), database.GetUsersParams{
		AfterUser: createdAfter,
		OffsetOpt: int32(offset),
		LimitOpt:  int32(pageLimit),
		Search:    searchName,
		Status:    statusFilter,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
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
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
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
func (api *api) postUser(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)

	var createUser codersdk.CreateUserRequest
	if !httpapi.Read(rw, r, &createUser) {
		return
	}
	_, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
		Username: createUser.Username,
		Email:    createUser.Email,
	})
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: "user already exists",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user: %s", err),
		})
		return
	}

	organization, err := api.Database.GetOrganizationByID(r.Context(), createUser.OrganizationID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: "organization does not exist with the provided id",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization: %s", err),
		})
		return
	}
	// Check if the caller has permissions to the organization requested.
	_, err = api.Database.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: organization.ID,
		UserID:         apiKey.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "you are not authorized to add members to that organization",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization member: %s", err),
		})
		return
	}

	user, _, err := api.createUser(r.Context(), createUser)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertUser(user, []uuid.UUID{createUser.OrganizationID}))
}

// Returns the parameterized user requested. All validation
// is completed in the middleware for this route.
func (api *api) userByName(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	organizationIDs, err := userOrganizationIDs(r.Context(), api, user)

	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization IDs: %s", err.Error()),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertUser(user, organizationIDs))
}

func (api *api) putUserProfile(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	var params codersdk.UpdateUserProfileRequest
	if !httpapi.Read(rw, r, &params) {
		return
	}
	existentUser, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
		Email:    params.Email,
		Username: params.Username,
	})
	isDifferentUser := existentUser.ID != user.ID

	if err == nil && isDifferentUser {
		responseErrors := []httpapi.Error{}
		if existentUser.Email == params.Email {
			responseErrors = append(responseErrors, httpapi.Error{
				Field:  "email",
				Detail: "this value is already in use and should be unique",
			})
		}
		if existentUser.Username == params.Username {
			responseErrors = append(responseErrors, httpapi.Error{
				Field:  "username",
				Detail: "this value is already in use and should be unique",
			})
		}
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("user already exists"),
			Errors:  responseErrors,
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) && isDifferentUser {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user: %s", err),
		})
		return
	}

	updatedUserProfile, err := api.Database.UpdateUserProfile(r.Context(), database.UpdateUserProfileParams{
		ID:        user.ID,
		Email:     params.Email,
		Username:  params.Username,
		UpdatedAt: database.Now(),
	})

	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("patch user: %s", err.Error()),
		})
		return
	}

	organizationIDs, err := userOrganizationIDs(r.Context(), api, user)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization IDs: %s", err.Error()),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertUser(updatedUserProfile, organizationIDs))
}

func (api *api) putUserSuspend(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	suspendedUser, err := api.Database.UpdateUserStatus(r.Context(), database.UpdateUserStatusParams{
		ID:        user.ID,
		Status:    database.UserStatusSuspended,
		UpdatedAt: database.Now(),
	})

	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("put user suspended: %s", err.Error()),
		})
		return
	}

	organizations, err := userOrganizationIDs(r.Context(), api, user)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization IDs: %s", err.Error()),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertUser(suspendedUser, organizations))
}

func (api *api) userRoles(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	resp := codersdk.UserRoles{
		Roles:             user.RBACRoles,
		OrganizationRoles: make(map[uuid.UUID][]string),
	}

	memberships, err := api.Database.GetOrganizationMembershipsByUserID(r.Context(), user.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user memberships: %s", err),
		})
		return
	}

	for _, mem := range memberships {
		resp.OrganizationRoles[mem.OrganizationID] = mem.Roles
	}

	httpapi.Write(rw, http.StatusOK, resp)
}

func (api *api) putUserRoles(rw http.ResponseWriter, r *http.Request) {
	// User is the user to modify
	// TODO: Until rbac authorize is implemented, only be able to change your
	//		own roles. This also means you can grant yourself whatever roles you want.
	user := httpmw.UserParam(r)
	apiKey := httpmw.APIKey(r)
	if apiKey.UserID != user.ID {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("modifying other users is not supported at this time"),
		})
		return
	}

	var params codersdk.UpdateRoles
	if !httpapi.Read(rw, r, &params) {
		return
	}

	updatedUser, err := api.updateSiteUserRoles(r.Context(), database.UpdateUserRolesParams{
		GrantedRoles: params.Roles,
		ID:           user.ID,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	organizationIDs, err := userOrganizationIDs(r.Context(), api, user)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization IDs: %s", err.Error()),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertUser(updatedUser, organizationIDs))
}

func (api *api) updateSiteUserRoles(ctx context.Context, args database.UpdateUserRolesParams) (database.User, error) {
	// Enforce only site wide roles
	for _, r := range args.GrantedRoles {
		if _, ok := rbac.IsOrgRole(r); ok {
			return database.User{}, xerrors.Errorf("must only update site wide roles")
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
func (api *api) organizationsByUser(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	organizations, err := api.Database.GetOrganizationsByUserID(r.Context(), user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		organizations = []database.Organization{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organizations: %s", err.Error()),
		})
		return
	}

	publicOrganizations := make([]codersdk.Organization, 0, len(organizations))
	for _, organization := range organizations {
		publicOrganizations = append(publicOrganizations, convertOrganization(organization))
	}

	httpapi.Write(rw, http.StatusOK, publicOrganizations)
}

func (api *api) organizationByUserAndName(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	organizationName := chi.URLParam(r, "organizationname")
	organization, err := api.Database.GetOrganizationByName(r.Context(), organizationName)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no organization found by name %q", organizationName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization by name: %s", err),
		})
		return
	}
	_, err = api.Database.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: organization.ID,
		UserID:         user.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "you are not a member of that organization",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization member: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertOrganization(organization))
}

func (api *api) postOrganizationsByUser(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	var req codersdk.CreateOrganizationRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}
	_, err := api.Database.GetOrganizationByName(r.Context(), req.Name)
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: "organization already exists with that name",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization: %s", err.Error()),
		})
		return
	}

	var organization database.Organization
	err = api.Database.InTx(func(db database.Store) error {
		organization, err = api.Database.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.New(),
			Name:      req.Name,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("create organization: %w", err)
		}
		_, err = api.Database.InsertOrganizationMember(r.Context(), database.InsertOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Roles: []string{
				// Also assign member role incase they get demoted from admin
				rbac.RoleOrgMember(organization.ID),
				rbac.RoleOrgAdmin(organization.ID),
			},
		})
		if err != nil {
			return xerrors.Errorf("create organization member: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertOrganization(organization))
}

// Authenticates the user with an email and password.
func (api *api) postLogin(rw http.ResponseWriter, r *http.Request) {
	var loginWithPassword codersdk.LoginWithPasswordRequest
	if !httpapi.Read(rw, r, &loginWithPassword) {
		return
	}

	user, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
		Email: loginWithPassword.Email,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user: %s", err.Error()),
		})
		return
	}

	// If the user doesn't exist, it will be a default struct.

	equal, err := userpassword.Compare(string(user.HashedPassword), loginWithPassword.Password)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("compare: %s", err.Error()),
		})
	}
	if !equal {
		// This message is the same as above to remove ease in detecting whether
		// users are registered or not. Attackers still could with a timing attack.
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "invalid email or password",
		})
		return
	}

	sessionToken, created := api.createAPIKey(rw, r, database.InsertAPIKeyParams{
		UserID:    user.ID,
		LoginType: database.LoginTypePassword,
	})
	if !created {
		return
	}

	httpapi.Write(rw, http.StatusCreated, codersdk.LoginWithPasswordResponse{
		SessionToken: sessionToken,
	})
}

// Creates a new session key, used for logging in via the CLI
func (api *api) postAPIKey(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	apiKey := httpmw.APIKey(r)

	if user.ID != apiKey.UserID {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "Keys can only be generated for the authenticated user",
		})
		return
	}

	sessionToken, created := api.createAPIKey(rw, r, database.InsertAPIKeyParams{
		UserID:    user.ID,
		LoginType: database.LoginTypePassword,
	})
	if !created {
		return
	}

	httpapi.Write(rw, http.StatusCreated, codersdk.GenerateAPIKeyResponse{Key: sessionToken})
}

// Clear the user's session cookie
func (*api) postLogout(rw http.ResponseWriter, _ *http.Request) {
	// Get a blank token cookie
	cookie := &http.Cookie{
		// MaxAge < 0 means to delete the cookie now
		MaxAge: -1,
		Name:   httpmw.AuthCookie,
		Path:   "/",
	}

	http.SetCookie(rw, cookie)
	httpapi.Write(rw, http.StatusOK, httpapi.Response{
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

func (api *api) createAPIKey(rw http.ResponseWriter, r *http.Request, params database.InsertAPIKeyParams) (string, bool) {
	keyID, keySecret, err := generateAPIKeyIDSecret()
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("generate api key parts: %s", err.Error()),
		})
		return "", false
	}
	hashed := sha256.Sum256([]byte(keySecret))

	_, err = api.Database.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
		ID:                keyID,
		UserID:            params.UserID,
		ExpiresAt:         database.Now().Add(24 * time.Hour),
		CreatedAt:         database.Now(),
		UpdatedAt:         database.Now(),
		HashedSecret:      hashed[:],
		LoginType:         params.LoginType,
		OAuthAccessToken:  params.OAuthAccessToken,
		OAuthRefreshToken: params.OAuthRefreshToken,
		OAuthIDToken:      params.OAuthIDToken,
		OAuthExpiry:       params.OAuthExpiry,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert api key: %s", err.Error()),
		})
		return "", false
	}

	// This format is consumed by the APIKey middleware.
	sessionToken := fmt.Sprintf("%s-%s", keyID, keySecret)
	http.SetCookie(rw, &http.Cookie{
		Name:     httpmw.AuthCookie,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   api.SecureAuthCookie,
	})
	return sessionToken, true
}

func (api *api) createUser(ctx context.Context, req codersdk.CreateUserRequest) (database.User, uuid.UUID, error) {
	var user database.User
	return user, req.OrganizationID, api.Database.InTx(func(db database.Store) error {
		var orgRoles []string
		// If no organization is provided, create a new one for the user.
		if req.OrganizationID == uuid.Nil {
			organization, err := db.InsertOrganization(ctx, database.InsertOrganizationParams{
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
		// Always also be a member
		orgRoles = append(orgRoles, rbac.RoleOrgMember(req.OrganizationID))

		params := database.InsertUserParams{
			ID:        uuid.New(),
			Email:     req.Email,
			Username:  req.Username,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
			// All new users are defaulted to members of the site.
			RBACRoles: []string{rbac.RoleMember()},
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
		user, err = db.InsertUser(ctx, params)
		if err != nil {
			return xerrors.Errorf("create user: %w", err)
		}

		privateKey, publicKey, err := gitsshkey.Generate(api.SSHKeygenAlgorithm)
		if err != nil {
			return xerrors.Errorf("generate user gitsshkey: %w", err)
		}
		_, err = db.InsertGitSSHKey(ctx, database.InsertGitSSHKeyParams{
			UserID:     user.ID,
			CreatedAt:  database.Now(),
			UpdatedAt:  database.Now(),
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		})
		if err != nil {
			return xerrors.Errorf("insert user gitsshkey: %w", err)
		}
		_, err = db.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
			OrganizationID: req.OrganizationID,
			UserID:         user.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			// By default give them membership to the organization
			Roles: orgRoles,
		})
		if err != nil {
			return xerrors.Errorf("create organization member: %w", err)
		}
		return nil
	})
}

func convertUser(user database.User, organizationIDs []uuid.UUID) codersdk.User {
	return codersdk.User{
		ID:              user.ID,
		Email:           user.Email,
		CreatedAt:       user.CreatedAt,
		Username:        user.Username,
		Status:          codersdk.UserStatus(user.Status),
		OrganizationIDs: organizationIDs,
	}
}

func convertUsers(users []database.User, organizationIDsByUserID map[uuid.UUID][]uuid.UUID) []codersdk.User {
	converted := make([]codersdk.User, 0, len(users))
	for _, u := range users {
		userOrganizationIDs := organizationIDsByUserID[u.ID]
		converted = append(converted, convertUser(u, userOrganizationIDs))
	}
	return converted
}

func userOrganizationIDs(ctx context.Context, api *api, user database.User) ([]uuid.UUID, error) {
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
