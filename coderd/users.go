package coderd

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

// Lists all the users
func (api *api) users(rw http.ResponseWriter, r *http.Request) {
	users, err := api.Database.GetUsers(r.Context())

	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get users: %s", err.Error()),
		})
		return
	}

	var res []codersdk.User
	for _, user := range users {
		res = append(res, convertUser(user))
	}

	httpapi.Write(rw, http.StatusOK, res)
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
	err = api.Database.InTx(func(db database.Store) error {
		user, err = api.Database.InsertUser(r.Context(), database.InsertUserParams{
			ID:             uuid.New(),
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

		privateKey, publicKey, err := gitsshkey.Generate(api.SSHKeygenAlgorithm)
		if err != nil {
			return xerrors.Errorf("generate user gitsshkey: %w", err)
		}
		_, err = db.InsertGitSSHKey(r.Context(), database.InsertGitSSHKeyParams{
			UserID:     user.ID,
			CreatedAt:  database.Now(),
			UpdatedAt:  database.Now(),
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		})
		if err != nil {
			return xerrors.Errorf("insert user gitsshkey: %w", err)
		}

		organization, err = api.Database.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.New(),
			Name:      createUser.OrganizationName,
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

	httpapi.Write(rw, http.StatusCreated, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: organization.ID,
	})
}

// Creates a new user.
func (api *api) postUsers(rw http.ResponseWriter, r *http.Request) {
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
			ID:             uuid.New(),
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

		privateKey, publicKey, err := gitsshkey.Generate(api.SSHKeygenAlgorithm)
		if err != nil {
			return xerrors.Errorf("generate user gitsshkey: %w", err)
		}
		_, err = db.InsertGitSSHKey(r.Context(), database.InsertGitSSHKeyParams{
			UserID:     user.ID,
			CreatedAt:  database.Now(),
			UpdatedAt:  database.Now(),
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		})
		if err != nil {
			return xerrors.Errorf("insert user gitsshkey: %w", err)
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

	httpapi.Write(rw, http.StatusCreated, convertUser(user))
}

// Returns the parameterized user requested. All validation
// is completed in the middleware for this route.
func (*api) userByName(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	httpapi.Write(rw, http.StatusOK, convertUser(user))
}

func (api *api) putUserProfile(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	var params codersdk.UpdateUserProfileRequest
	if !httpapi.Read(rw, r, &params) {
		return
	}

	if params.Name == nil {
		params.Name = &user.Name
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
		Name:      *params.Name,
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

	httpapi.Write(rw, http.StatusOK, convertUser(updatedUserProfile))
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
		Secure:   api.SecureAuthCookie,
	})

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

	httpapi.Write(rw, http.StatusCreated, codersdk.GenerateAPIKeyResponse{Key: generatedAPIKey})
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

// Create a new workspace for the currently authenticated user.
func (api *api) postWorkspacesByUser(rw http.ResponseWriter, r *http.Request) {
	var createWorkspace codersdk.CreateWorkspaceRequest
	if !httpapi.Read(rw, r, &createWorkspace) {
		return
	}
	apiKey := httpmw.APIKey(r)
	template, err := api.Database.GetTemplateByID(r.Context(), createWorkspace.TemplateID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("template %q doesn't exist", createWorkspace.TemplateID.String()),
			Errors: []httpapi.Error{{
				Field:  "template_id",
				Detail: "template not found",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template: %s", err),
		})
		return
	}
	_, err = api.Database.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: template.OrganizationID,
		UserID:         apiKey.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "you aren't allowed to access templates in that organization",
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
		template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("find template for conflicting workspace name %q: %s", createWorkspace.Name, err),
			})
			return
		}
		// The template is fetched for clarity to the user on where the conflicting name may be.
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: fmt.Sprintf("workspace %q already exists in the %q template", createWorkspace.Name, template.Name),
			Errors: []httpapi.Error{{
				Field:  "name",
				Detail: "this value is already in use and should be unique",
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

	templateVersion, err := api.Database.GetTemplateVersionByID(r.Context(), template.ActiveVersionID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version: %s", err),
		})
		return
	}
	templateVersionJob, err := api.Database.GetProvisionerJobByID(r.Context(), templateVersion.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template version job: %s", err),
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
		workspaceBuildID := uuid.New()
		// Workspaces are created without any versions.
		workspace, err = db.InsertWorkspace(r.Context(), database.InsertWorkspaceParams{
			ID:         uuid.New(),
			CreatedAt:  database.Now(),
			UpdatedAt:  database.Now(),
			OwnerID:    apiKey.UserID,
			TemplateID: template.ID,
			Name:       createWorkspace.Name,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace: %w", err)
		}
		for _, parameterValue := range createWorkspace.ParameterValues {
			_, err = db.InsertParameterValue(r.Context(), database.InsertParameterValueParams{
				ID:                uuid.New(),
				Name:              parameterValue.Name,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				Scope:             database.ParameterScopeWorkspace,
				ScopeID:           workspace.ID,
				SourceScheme:      parameterValue.SourceScheme,
				SourceValue:       parameterValue.SourceValue,
				DestinationScheme: parameterValue.DestinationScheme,
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
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
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
			CreatedAt:         database.Now(),
			UpdatedAt:         database.Now(),
			WorkspaceID:       workspace.ID,
			TemplateVersionID: templateVersion.ID,
			Name:              namesgenerator.GetRandomName(1),
			InitiatorID:       apiKey.UserID,
			Transition:        database.WorkspaceTransitionStart,
			JobID:             provisionerJob.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("create workspace: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertWorkspace(workspace,
		convertWorkspaceBuild(workspaceBuild, convertProvisionerJob(templateVersionJob)), template))
}

func (api *api) workspacesByUser(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)
	workspaces, err := api.Database.GetWorkspacesByUserID(r.Context(), database.GetWorkspacesByUserIDParams{
		OwnerID: user.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspaces: %s", err),
		})
		return
	}
	workspaceIDs := make([]uuid.UUID, 0, len(workspaces))
	templateIDs := make([]uuid.UUID, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
		templateIDs = append(templateIDs, workspace.TemplateID)
	}
	workspaceBuilds, err := api.Database.GetWorkspaceBuildsByWorkspaceIDsWithoutAfter(r.Context(), workspaceIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace builds: %s", err),
		})
		return
	}
	templates, err := api.Database.GetTemplatesByIDs(r.Context(), templateIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get templates: %s", err),
		})
		return
	}
	jobIDs := make([]uuid.UUID, 0, len(workspaceBuilds))
	for _, build := range workspaceBuilds {
		jobIDs = append(jobIDs, build.JobID)
	}
	jobs, err := api.Database.GetProvisionerJobsByIDs(r.Context(), jobIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner jobs: %s", err),
		})
		return
	}

	buildByWorkspaceID := map[uuid.UUID]database.WorkspaceBuild{}
	for _, workspaceBuild := range workspaceBuilds {
		buildByWorkspaceID[workspaceBuild.WorkspaceID] = workspaceBuild
	}
	templateByID := map[uuid.UUID]database.Template{}
	for _, template := range templates {
		templateByID[template.ID] = template
	}
	jobByID := map[uuid.UUID]database.ProvisionerJob{}
	for _, job := range jobs {
		jobByID[job.ID] = job
	}
	apiWorkspaces := make([]codersdk.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		build, exists := buildByWorkspaceID[workspace.ID]
		if !exists {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("build not found for workspace %q", workspace.Name),
			})
			return
		}
		template, exists := templateByID[workspace.TemplateID]
		if !exists {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("template not found for workspace %q", workspace.Name),
			})
			return
		}
		job, exists := jobByID[build.JobID]
		if !exists {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("build job not found for workspace %q", workspace.Name),
			})
			return
		}
		apiWorkspaces = append(apiWorkspaces,
			convertWorkspace(workspace, convertWorkspaceBuild(build, convertProvisionerJob(job)), template))
	}

	httpapi.Write(rw, http.StatusOK, apiWorkspaces)
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
	build, err := api.Database.GetWorkspaceBuildByWorkspaceIDWithoutAfter(r.Context(), workspace.ID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace build: %s", err),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), build.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get provisioner job: %s", err),
		})
		return
	}
	template, err := api.Database.GetTemplateByID(r.Context(), workspace.TemplateID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get template: %s", err),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspace(workspace,
		convertWorkspaceBuild(build, convertProvisionerJob(job)), template))
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

func convertUser(user database.User) codersdk.User {
	return codersdk.User{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		Username:  user.Username,
		Name:      user.Name,
	}
}
