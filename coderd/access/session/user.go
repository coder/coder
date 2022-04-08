package session

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
)

const (
	// AuthCookie represents the name of the cookie the API key is stored in.
	AuthCookie = "session_token"

	// nolint:gosec // this is not a credential
	apiKeyInvalidMessage = "API key is invalid"
	apiKeyLifetime       = 24 * time.Hour
)

type userActor struct {
	user   database.User
	apiKey database.APIKey
}

var _ UserActor = &userActor{}

func NewUserActor(u database.User, apiKey database.APIKey) UserActor {
	return &userActor{
		user:   u,
		apiKey: apiKey,
	}
}

func (*userActor) Type() ActorType {
	return ActorTypeUser
}

func (ua *userActor) ID() string {
	return ua.user.ID.String()
}

func (ua *userActor) Name() string {
	return ua.user.Username
}

func (ua *userActor) User() *database.User {
	return &ua.user
}

func (ua *userActor) APIKey() *database.APIKey {
	return &ua.apiKey
}

// UserActorFromRequest tries to get a UserActor from the API key supplied in
// the request cookies. If the cookie doesn't exist, nil is returned. If there
// was an error that was responded to, false is returned.
//
// You should probably be calling session.ExtractActor as a middleware, or
// session.RequestActor instead.
func UserActorFromRequest(ctx context.Context, db database.Store, rw http.ResponseWriter, r *http.Request) (UserActor, bool) {
	cookie, err := r.Cookie(AuthCookie)
	if err != nil || cookie.Value == "" {
		// No cookie provided, return true so any actor handlers further down
		// the chain can make their attempt.
		return nil, true
	}

	// APIKeys are formatted: ${id}-${secret}. The ID is 10 characters and the
	// secret is 22.
	parts := strings.Split(cookie.Value, "-")
	// TODO: Dean - workspace agent token auth should not share the same cookie
	// name as regular auth
	if len(parts) == 5 {
		// Skip anything that looks like a UUID for now.
		return nil, true
	}
	if len(parts) != 2 || len(parts[0]) != 10 || len(parts[1]) != 22 {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("invalid API key cookie %q format", AuthCookie),
		})
		return nil, false
	}

	// We hash the secret before getting the key from the database to ensure we
	// keep this function fixed time.
	var (
		keyID        = parts[0]
		keySecret    = parts[1]
		hashedSecret = sha256.Sum256([]byte(keySecret))
	)

	// Get the API key from the database.
	key, err := db.GetAPIKeyByID(ctx, keyID)
	if xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: apiKeyInvalidMessage,
		})
		return nil, false
	} else if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get API key by id: %s", err.Error()),
		})
		return nil, false
	}

	// Checking to see if the secret is valid.
	if subtle.ConstantTimeCompare(key.HashedSecret, hashedSecret[:]) != 1 {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: apiKeyInvalidMessage,
		})
		return nil, false
	}

	// Check if the key has expired.
	now := database.Now()
	if key.ExpiresAt.Before(now) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: apiKeyInvalidMessage,
		})
		return nil, false
	}

	// TODO: Dean - check if the corresponding OIDC or OAuth token has expired
	// once OIDC is implemented

	// Only update LastUsed and key expiry once an hour to prevent database
	// spam.
	if now.Sub(key.LastUsed) > time.Hour || key.ExpiresAt.Sub(now) <= apiKeyLifetime-time.Hour {
		err := db.UpdateAPIKeyByID(ctx, database.UpdateAPIKeyByIDParams{
			ID:               key.ID,
			ExpiresAt:        now.Add(apiKeyLifetime),
			LastUsed:         now,
			OIDCAccessToken:  key.OIDCAccessToken,
			OIDCRefreshToken: key.OIDCRefreshToken,
			OIDCExpiry:       key.OIDCExpiry,
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("could not refresh API key: %s", err.Error()),
			})
			return nil, false
		}
	}

	// Get the associated user.
	u, err := db.GetUserByID(ctx, key.UserID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("could not fetch current user: %s", err.Error()),
		})
		return nil, false
	}

	return NewUserActor(u, key), true
}
