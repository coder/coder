package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// User is the JSON representation of a Coder user.
type User struct {
	ID        string    `json:"id" validate:"required"`
	Email     string    `json:"email" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
	Username  string    `json:"username" validate:"required"`
}

// CreateUserRequest enables callers to create a new user.
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
	var createUser CreateUserRequest
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
	user, err := users.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
		Email:    createUser.Email,
		Username: createUser.Username,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get user: %s", err.Error()),
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

	user, err = users.Database.InsertUser(context.Background(), database.InsertUserParams{
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
	render.JSON(rw, r, user)
}

// Returns the currently authenticated user.
func (users *users) getAuthenticatedUser(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.User(r)

	render.JSON(rw, r, User{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		Username:  user.Username,
	})
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

	id, secret, err := generateAPIKeyIDSecret()
	hashed := sha256.Sum256([]byte(secret))

	_, err = users.Database.InsertAPIKey(r.Context(), database.InsertAPIKeyParams{
		ID:           id,
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
	sessionToken := fmt.Sprintf("%s-%s", id, secret)
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

// Generates a new ID and secret for an API key.
func generateAPIKeyIDSecret() (string, string, error) {
	// Length of an API Key ID.
	id, err := cryptorand.String(10)
	if err != nil {
		return "", "", err
	}
	// Length of an API Key secret.
	secret, err := cryptorand.String(22)
	if err != nil {
		return "", "", err
	}
	return id, secret, nil
}
