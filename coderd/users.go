package coderd

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// User represents a user in Coder.
type User struct {
	ID        string    `json:"id" validate:"required"`
	Email     string    `json:"email" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
	Username  string    `json:"username" validate:"required"`
}

type CreateFirstUserRequest struct {
	Email        string `json:"email" validate:"required,email"`
	Username     string `json:"username" validate:"required,username"`
	Password     string `json:"password" validate:"required"`
	Organization string `json:"organization" validate:"required,username"`
}

// CreateFirstUserResponse contains IDs for newly created user info.
type CreateFirstUserResponse struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
}

type CreateUserRequest struct {
	Email          string `json:"email" validate:"required,email"`
	Username       string `json:"username" validate:"required,username"`
	Password       string `json:"password" validate:"required"`
	OrganizationID string `json:"organization_id" validate:"required"`
}

// LoginWithPasswordRequest enables callers to authenticate with email and password.
type LoginWithPasswordRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginWithPasswordResponse contains a session token for the newly authenticated user.
type LoginWithPasswordResponse struct {
	SessionToken string `json:"session_token" validate:"required"`
}

// GenerateAPIKeyResponse contains an API key for a user.
type GenerateAPIKeyResponse struct {
	Key string `json:"key"`
}

type CreateOrganizationRequest struct {
	Name string `json:"name" validate:"required,username"`
}

// CreateWorkspaceRequest provides options for creating a new workspace.
type CreateWorkspaceRequest struct {
	ProjectID uuid.UUID `json:"project_id" validate:"required"`
	Name      string    `json:"name" validate:"username,required"`
}

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
	var createUser CreateFirstUserRequest
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
	hashedPassword, err := userpassword.Hash(createUser.Password)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("hash password: %s", err.Error()),
		})
		return
	}

	// Create the user, organization, and membership to the user.
	var user database.User
	var organization database.Organization
	err = api.Database.InTx(func(s database.Store) error {
		user, err = api.Database.InsertUser(r.Context(), database.InsertUserParams{
			ID:             uuid.NewString(),
			Email:          createUser.Email,
			HashedPassword: []byte(hashedPassword),
			Username:       createUser.Username,
			LoginType:      database.LoginTypeBuiltIn,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("create user: %w", err)
		}
		organization, err = api.Database.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.NewString(),
			Name:      createUser.Organization,
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
			Roles:          []string{"organization-admin"},
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

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: organization.ID,
	})
}

// Creates a new user.
func (api *api) postUsers(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)

	var createUser CreateUserRequest
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

	hashedPassword, err := userpassword.Hash(createUser.Password)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("hash password: %s", err.Error()),
		})
		return
	}

	var user database.User
	err = api.Database.InTx(func(db database.Store) error {
		user, err = db.InsertUser(r.Context(), database.InsertUserParams{
			ID:             uuid.NewString(),
			Email:          createUser.Email,
			HashedPassword: []byte(hashedPassword),
			Username:       createUser.Username,
			LoginType:      database.LoginTypeBuiltIn,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("create user: %w", err)
		}
		_, err = db.InsertOrganizationMember(r.Context(), database.InsertOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         user.ID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Roles:          []string{},
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

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertUser(user))
}

// Returns the parameterized user requested. All validation
// is completed in the middleware for this route.
func (*api) userByName(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	render.JSON(rw, r, convertUser(user))
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

	publicOrganizations := make([]Organization, 0, len(organizations))
	for _, organization := range organizations {
		publicOrganizations = append(publicOrganizations, convertOrganization(organization))
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, publicOrganizations)
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

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertOrganization(organization))
}

func (api *api) postOrganizationsByUser(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	var req CreateOrganizationRequest
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
			ID:        uuid.NewString(),
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
			Roles:          []string{"organization-admin"},
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

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertOrganization(organization))
}

// Authenticates the user with an email and password.
func (api *api) postLogin(rw http.ResponseWriter, r *http.Request) {
	var loginWithPassword LoginWithPasswordRequest
	if !httpapi.Read(rw, r, &loginWithPassword) {
		return
	}
	user, err := api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
		Email: loginWithPassword.Email,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "invalid email or password",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user: %s", err.Error()),
		})
		return
	}
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

	keyID, keySecret, err := generateAPIKeyIDSecret()
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("generate api key parts: %s", err.Error()),
		})
		return
	}
	hashed := sha256.Sum256([]byte(keySecret))

	_, err = api.Database.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
		ID:           keyID,
		UserID:       user.ID,
		ExpiresAt:    database.Now().Add(24 * time.Hour),
		CreatedAt:    database.Now(),
		UpdatedAt:    database.Now(),
		HashedSecret: hashed[:],
		LoginType:    database.LoginTypeBuiltIn,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert api key: %s", err.Error()),
		})
		return
	}

	// This format is consumed by the APIKey middleware.
	sessionToken := fmt.Sprintf("%s-%s", keyID, keySecret)
	http.SetCookie(rw, &http.Cookie{
		Name:     httpmw.AuthCookie,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, LoginWithPasswordResponse{
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

	keyID, keySecret, err := generateAPIKeyIDSecret()
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("generate api key parts: %s", err.Error()),
		})
		return
	}
	hashed := sha256.Sum256([]byte(keySecret))

	_, err = api.Database.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
		ID:           keyID,
		UserID:       apiKey.UserID,
		ExpiresAt:    database.Now().AddDate(1, 0, 0), // Expire after 1 year (same as v1)
		CreatedAt:    database.Now(),
		UpdatedAt:    database.Now(),
		HashedSecret: hashed[:],
		LoginType:    database.LoginTypeBuiltIn,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert api key: %s", err.Error()),
		})
		return
	}

	// This format is consumed by the APIKey middleware.
	generatedAPIKey := fmt.Sprintf("%s-%s", keyID, keySecret)

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, GenerateAPIKeyResponse{Key: generatedAPIKey})
}

// Clear the user's session cookie
func (*api) postLogout(rw http.ResponseWriter, r *http.Request) {
	// Get a blank token cookie
	cookie := &http.Cookie{
		// MaxAge < 0 means to delete the cookie now
		MaxAge: -1,
		Name:   httpmw.AuthCookie,
		Path:   "/",
	}

	http.SetCookie(rw, cookie)
	render.Status(r, http.StatusOK)
}

// Create a new workspace for the currently authenticated user.
func (api *api) postWorkspacesByUser(rw http.ResponseWriter, r *http.Request) {
	var createWorkspace CreateWorkspaceRequest
	if !httpapi.Read(rw, r, &createWorkspace) {
		return
	}
	apiKey := httpmw.APIKey(r)
	project, err := api.Database.GetProjectByID(r.Context(), createWorkspace.ProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("project %q doesn't exist", createWorkspace.ProjectID.String()),
			Errors: []httpapi.Error{{
				Field: "project_id",
				Code:  "not_found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get project: %s", err),
		})
		return
	}
	_, err = api.Database.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: project.OrganizationID,
		UserID:         apiKey.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "you aren't allowed to access projects in that organization",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization member: %s", err),
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByUserIDAndName(r.Context(), database.GetWorkspaceByUserIDAndNameParams{
		OwnerID: apiKey.UserID,
		Name:    createWorkspace.Name,
	})
	if err == nil {
		// If the workspace already exists, don't allow creation.
		project, err := api.Database.GetProjectByID(r.Context(), workspace.ProjectID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("find project for conflicting workspace name %q: %s", createWorkspace.Name, err),
			})
			return
		}
		// The project is fetched for clarity to the user on where the conflicting name may be.
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("workspace %q already exists in the %q project", createWorkspace.Name, project.Name),
			Errors: []httpapi.Error{{
				Field: "name",
				Code:  "exists",
			}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace by name: %s", err.Error()),
		})
		return
	}

	// Workspaces are created without any versions.
	workspace, err = api.Database.InsertWorkspace(r.Context(), database.InsertWorkspaceParams{
		ID:        uuid.New(),
		CreatedAt: database.Now(),
		UpdatedAt: database.Now(),
		OwnerID:   apiKey.UserID,
		ProjectID: project.ID,
		Name:      createWorkspace.Name,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("insert workspace: %s", err),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertWorkspace(workspace))
}

func (api *api) workspacesByUser(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	workspaces, err := api.Database.GetWorkspacesByUserID(r.Context(), user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces: %s", err),
		})
		return
	}
	apiWorkspaces := make([]Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		apiWorkspaces = append(apiWorkspaces, convertWorkspace(workspace))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiWorkspaces)
}

func (api *api) workspaceByUserAndName(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	workspaceName := chi.URLParam(r, "workspacename")
	workspace, err := api.Database.GetWorkspaceByUserIDAndName(r.Context(), database.GetWorkspaceByUserIDAndNameParams{
		OwnerID: user.ID,
		Name:    workspaceName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("no workspace found by name %q", workspaceName),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace by name: %s", err),
		})
		return
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspace(workspace))
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

func convertUser(user database.User) User {
	return User{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		Username:  user.Username,
	}
}
