package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// APIKey: do not ever return the HashedSecret
type APIKey struct {
	ID              string      `json:"id" validate:"required"`
	UserID          uuid.UUID   `json:"user_id" validate:"required" format:"uuid"`
	LastUsed        time.Time   `json:"last_used" validate:"required" format:"date-time"`
	ExpiresAt       time.Time   `json:"expires_at" validate:"required" format:"date-time"`
	CreatedAt       time.Time   `json:"created_at" validate:"required" format:"date-time"`
	UpdatedAt       time.Time   `json:"updated_at" validate:"required" format:"date-time"`
	LoginType       LoginType   `json:"login_type" validate:"required" enums:"password,github,oidc,token"`
	Scope           APIKeyScope `json:"scope" validate:"required" enums:"all,application_connect"`
	TokenName       string      `json:"token_name" validate:"required"`
	LifetimeSeconds int64       `json:"lifetime_seconds" validate:"required"`
}

// LoginType is the type of login used to create the API key.
type LoginType string

const (
	LoginTypeUnknown  LoginType = ""
	LoginTypePassword LoginType = "password"
	LoginTypeGithub   LoginType = "github"
	LoginTypeOIDC     LoginType = "oidc"
	LoginTypeToken    LoginType = "token"
	// LoginTypeNone is used if no login method is available for this user.
	// If this is set, the user has no method of logging in.
	// API keys can still be created by an owner and used by the user.
	// These keys would use the `LoginTypeToken` type.
	LoginTypeNone LoginType = "none"
)

type APIKeyScope string

const (
	// APIKeyScopeAll is a scope that allows the user to do everything.
	APIKeyScopeAll APIKeyScope = "all"
	// APIKeyScopeApplicationConnect is a scope that allows the user
	// to connect to applications in a workspace.
	APIKeyScopeApplicationConnect APIKeyScope = "application_connect"
)

type CreateTokenRequest struct {
	Lifetime  time.Duration `json:"lifetime"`
	Scope     APIKeyScope   `json:"scope" enums:"all,application_connect"`
	TokenName string        `json:"token_name"`
}

// GenerateAPIKeyResponse contains an API key for a user.
type GenerateAPIKeyResponse struct {
	Key string `json:"key"`
}

// CreateToken generates an API key for the user ID provided with
// custom expiration. These tokens can be used for long-lived access,
// like for use with CI.
func (c *Client) CreateToken(ctx context.Context, userID string, req CreateTokenRequest) (GenerateAPIKeyResponse, error) {
	return makeSDKRequest[GenerateAPIKeyResponse](ctx, c, sdkRequestArgs{
		Method:     http.MethodPost,
		URL:        fmt.Sprintf("/api/v2/users/%s/keys/tokens", userID),
		Body:       req,
		ExpectCode: http.StatusCreated,
	})
}

// CreateAPIKey generates an API key for the user ID provided.
// CreateToken should be used over CreateAPIKey. CreateToken allows better
// tracking of the token's usage and allows for custom expiration.
// Only use CreateAPIKey if you want to emulate the session created for
// a browser like login.
func (c *Client) CreateAPIKey(ctx context.Context, user string) (GenerateAPIKeyResponse, error) {
	return makeSDKRequest[GenerateAPIKeyResponse](ctx, c, sdkRequestArgs{
		Method:     http.MethodPost,
		URL:        fmt.Sprintf("/api/v2/users/%s/keys", user),
		Body:       nil,
		ReqOpts:    nil,
		ExpectCode: http.StatusCreated,
	})
}

type TokensFilter struct {
	IncludeAll bool `json:"include_all"`
}

type APIKeyWithOwner struct {
	APIKey
	Username string `json:"username"`
}

type TokenConfig struct {
	MaxTokenLifetime time.Duration `json:"max_token_lifetime"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (f TokensFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		q.Set("include_all", fmt.Sprintf("%t", f.IncludeAll))
		r.URL.RawQuery = q.Encode()
	}
}

// Tokens list machine API keys.
func (c *Client) Tokens(ctx context.Context, userID string, filter TokensFilter) ([]APIKeyWithOwner, error) {
	return makeSDKRequest[[]APIKeyWithOwner](ctx, c, sdkRequestArgs{
		Method:     http.MethodGet,
		URL:        fmt.Sprintf("/api/v2/users/%s/keys/tokens", userID),
		ReqOpts:    []RequestOption{filter.asRequestOption()},
		ExpectCode: http.StatusOK,
	})
}

// APIKeyByID returns the api key by id.
func (c *Client) APIKeyByID(ctx context.Context, userID string, id string) (*APIKey, error) {
	return makeSDKRequest[*APIKey](ctx, c, sdkRequestArgs{
		Method:     http.MethodGet,
		URL:        fmt.Sprintf("/api/v2/users/%s/keys/%s", userID, id),
		ExpectCode: http.StatusOK,
	})
}

// APIKeyByName returns the api key by name.
func (c *Client) APIKeyByName(ctx context.Context, userID string, name string) (*APIKey, error) {
	return makeSDKRequest[*APIKey](ctx, c, sdkRequestArgs{
		Method:     http.MethodGet,
		URL:        fmt.Sprintf("/api/v2/users/%s/keys/tokens/%s", userID, name),
		ExpectCode: http.StatusOK,
	})
}

// DeleteAPIKey deletes API key by id.
func (c *Client) DeleteAPIKey(ctx context.Context, userID string, id string) error {
	_, err := makeSDKRequest[noResponse](ctx, c, sdkRequestArgs{
		Method:     http.MethodDelete,
		URL:        fmt.Sprintf("/api/v2/users/%s/keys/%s", userID, id),
		ExpectCode: http.StatusNoContent,
	})
	return err
}

// GetTokenConfig returns deployment options related to token management
func (c *Client) GetTokenConfig(ctx context.Context, userID string) (TokenConfig, error) {
	return makeSDKRequest[TokenConfig](ctx, c, sdkRequestArgs{
		Method:     http.MethodGet,
		URL:        fmt.Sprintf("/api/v2/users/%s/keys/tokens/tokenconfig", userID),
		ExpectCode: http.StatusOK,
	})
}
