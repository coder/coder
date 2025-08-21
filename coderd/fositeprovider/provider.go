package fositeprovider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/token/jwt"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/fositeprovider/fositestorage"
)

type Provider struct {
	// TODO: Make a subset interface for database.Store with only the methods needed for OAuth2 provider functionality.
	//DB database.Store

	logger slog.Logger

	storage  *fositestorage.Storage
	config   *fosite.Config
	provider fosite.OAuth2Provider
}

func New(ctx context.Context, logger slog.Logger, db database.Store) *Provider {
	// privateKey is used to sign JWT tokens. The default strategy uses RS256 (RSA Signature with SHA-256)
	// TODO: Pass in this secret and persist it
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	// TODO: This is unused right now?
	//secret, err := db.GetOAuthSigningKey(ctx)
	//if err != nil {
	//	panic(err)
	//}

	config := &fosite.Config{
		GlobalSecret:        []byte("some-cool-secret-that-is-32bytes"),
		ClientSecretsHasher: clientSecretHasher{},
		// TODO: Configure http client
	}

	// TODO: Persist storage in the database
	store := fositestorage.New(db)
	provider := compose.ComposeAllEnabled(config, store, privateKey)

	return &Provider{
		logger:   logger.Named("oauth2_provider"),
		provider: provider,
		config:   config,
		storage:  store,
	}
}

// A session is passed from the `/auth` to the `/token` endpoint. You probably want to store data like: "Who made the request",
// "What organization does that person belong to" and so on.
// For our use case, the session will meet the requirements imposed by JWT access tokens, HMAC access tokens and OpenID Connect
// ID Tokens plus a custom field
//
// newSession is a helper function for creating a new session. This may look like a lot of code but since we are
// setting up multiple strategies it is a bit longer.
// Usually, you could do:
//
//	session = new(fosite.DefaultSession)
func (p *Provider) newSession(key database.APIKey) *openid.DefaultSession {
	return &openid.DefaultSession{
		Claims: &jwt.IDTokenClaims{
			Issuer:      "https://fosite.my-application.com",
			Subject:     key.UserID.String(),
			Audience:    []string{"https://my-client.my-application.com"},
			ExpiresAt:   time.Now().Add(time.Hour * 6),
			IssuedAt:    time.Now(),
			RequestedAt: time.Now(),
			AuthTime:    time.Now(),
		},
		Headers: &jwt.Headers{
			Extra: make(map[string]interface{}),
		},
	}
}
