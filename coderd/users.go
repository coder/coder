package coderd

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

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

// CreateInitialUserRequest provides options to create the initial
// user for a Coder deployment. The organization provided will be
// created as well.
type CreateInitialUserRequest struct {
	Email        string `json:"email" validate:"required,email"`
	Username     string `json:"username" validate:"required,username"`
	Password     string `json:"password" validate:"required"`
	Organization string `json:"organization" validate:"required,username"`
}

// CreateUserRequest provides options for creating a new user.
type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Username string `json:"username" validate:"required,username"`
	Password string `json:"password" validate:"required"`
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

type users struct {
	Database database.Store
}

// Creates the initial user for a Coder deployment.
func (users *users) createInitialUser(rw http.ResponseWriter, r *http.Request) {
	var createUser CreateInitialUserRequest
	if !httpapi.Read(rw, r, &createUser) {
		return
	}
	// This should only function for the first user.
	userCount, err := users.Database.GetUserCount(r.Context())
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
	err = users.Database.InTx(func(s database.Store) error {
		user, err = users.Database.InsertUser(r.Context(), database.InsertUserParams{
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
		organization, err := users.Database.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.NewString(),
			Name:      createUser.Organization,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("create organization: %w", err)
		}
		_, err = users.Database.InsertOrganizationMember(r.Context(), database.InsertOrganizationMemberParams{
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
	render.JSON(rw, r, convertUser(user))
}

// Creates a new user.
func (users *users) createUser(rw http.ResponseWriter, r *http.Request) {
	var createUser CreateUserRequest
	if !httpapi.Read(rw, r, &createUser) {
		return
	}
	_, err := users.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
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

	hashedPassword, err := userpassword.Hash(createUser.Password)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("hash password: %s", err.Error()),
		})
		return
	}

	user, err := users.Database.InsertUser(r.Context(), database.InsertUserParams{
		ID:             uuid.NewString(),
		Email:          createUser.Email,
		HashedPassword: []byte(hashedPassword),
		Username:       createUser.Username,
		LoginType:      database.LoginTypeBuiltIn,
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("create user: %s", err.Error()),
		})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(rw, r, convertUser(user))
}

// Returns the parameterized user requested. All validation
// is completed in the middleware for this route.
func (*users) user(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	render.JSON(rw, r, convertUser(user))
}

// Returns organizations the parameterized user has access to.
func (users *users) userOrganizations(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	organizations, err := users.Database.GetOrganizationsByUserID(r.Context(), user.ID)
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

// Authenticates the user with an email and password.
func (users *users) loginWithPassword(rw http.ResponseWriter, r *http.Request) {
	var loginWithPassword LoginWithPasswordRequest
	if !httpapi.Read(rw, r, &loginWithPassword) {
		return
	}
	user, err := users.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
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

	_, err = users.Database.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
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

// Clear the user's session cookie
func (*users) logout(rw http.ResponseWriter, r *http.Request) {
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
