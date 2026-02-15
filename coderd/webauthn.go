package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

const (
	webAuthnConnectAudience = "coder-connect"

	// webAuthnSessionTTL is the maximum time a WebAuthn ceremony
	// session (begin → finish) can remain valid. Sessions older
	// than this are rejected on load.
	webAuthnSessionTTL = 5 * time.Minute

	// webAuthnMaxBodySize limits the request body for finish/verify
	// endpoints to prevent memory exhaustion.
	webAuthnMaxBodySize = 64 * 1024 // 64 KB

	// webAuthnMaxCredentialNameLen limits the credential name length.
	webAuthnMaxCredentialNameLen = 128

	// webAuthnJTICacheMaxSize limits the JTI replay cache to
	// prevent unbounded memory growth. Old entries are evicted
	// when this limit is reached.
	webAuthnJTICacheMaxSize = 10000
)

// webAuthnJTICache tracks used JWT IDs to prevent replay of
// single-use tokens. Entries are stored with their expiry time
// and cleaned up lazily.
type webAuthnJTICache struct {
	mu      sync.Mutex
	entries map[string]time.Time // jti → expiry
}

func newWebAuthnJTICache() *webAuthnJTICache {
	return &webAuthnJTICache{
		entries: make(map[string]time.Time),
	}
}

// NewWebAuthnJTICacheForTest creates a JTI cache for testing.
// This is exported only for use in tests.
func NewWebAuthnJTICacheForTest() *webAuthnJTICache {
	return newWebAuthnJTICache()
}

// MarkUsed records a JTI as used. Returns false if the JTI was
// already used (replay detected).
func (c *webAuthnJTICache) MarkUsed(jti string, expiry time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Lazy cleanup: remove expired entries if cache is large.
	if len(c.entries) > webAuthnJTICacheMaxSize/2 {
		now := time.Now()
		for k, exp := range c.entries {
			if now.After(exp) {
				delete(c.entries, k)
			}
		}
	}

	if _, exists := c.entries[jti]; exists {
		return false
	}
	c.entries[jti] = expiry
	return true
}

// webAuthnSession wraps WebAuthn session data with a creation
// timestamp for TTL enforcement.
type webAuthnSession struct {
	data      *webauthn.SessionData
	createdAt time.Time
}

// storeWebAuthnSession stores a session with TTL metadata.
func (api *API) storeWebAuthnSession(key string, session *webauthn.SessionData) {
	api.WebAuthnSessionStore.Store(key, &webAuthnSession{
		data:      session,
		createdAt: time.Now(),
	})
}

// loadWebAuthnSession loads and deletes a session, returning nil if
// it doesn't exist or has expired.
func (api *API) loadWebAuthnSession(key string) *webauthn.SessionData {
	raw, ok := api.WebAuthnSessionStore.LoadAndDelete(key)
	if !ok {
		return nil
	}
	sess, ok := raw.(*webAuthnSession)
	if !ok {
		return nil
	}
	if time.Since(sess.createdAt) > webAuthnSessionTTL {
		return nil
	}
	return sess.data
}

// WebAuthnConnectClaims is the JWT payload issued after a successful
// WebAuthn assertion verification. It grants access to sensitive
// workspace operations (SSH, port forwarding).
type WebAuthnConnectClaims struct {
	jwtutils.RegisteredClaims
}

// webAuthnUser adapts a database.User and its stored WebAuthn
// credentials to the webauthn.User interface required by the
// go-webauthn library.
type webAuthnUser struct {
	user  database.User
	creds []database.WebauthnCredential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	b, _ := u.user.ID.MarshalBinary()
	return b
}

func (u *webAuthnUser) WebAuthnName() string {
	return u.user.Username
}

func (u *webAuthnUser) WebAuthnDisplayName() string {
	name := u.user.Name
	if name == "" {
		name = u.user.Username
	}
	return name
}

func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(u.creds))
	for _, c := range u.creds {
		out = append(out, dbCredentialToWebAuthn(c))
	}
	return out
}

func dbCredentialToWebAuthn(c database.WebauthnCredential) webauthn.Credential {
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.Aaguid,
			SignCount: uint32(c.SignCount),
		},
	}
}

// canonicalOrigin returns scheme://host[:port] from a URL, stripping
// any path, query, or fragment. This is the format WebAuthn expects.
func canonicalOrigin(u *url.URL) string {
	origin := u.Scheme + "://" + u.Host
	return origin
}

// newWebAuthn creates a webauthn.WebAuthn relying party instance
// from the server's access URL. If --fido2-require-user-verification
// is enabled, the registration and assertion options will request
// user verification (PIN/biometric).
func (api *API) newWebAuthn() (*webauthn.WebAuthn, error) {
	accessURL := api.AccessURL
	rpID := accessURL.Hostname()
	origin := canonicalOrigin(accessURL)

	cfg := &webauthn.Config{
		RPDisplayName: "Coder",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	}

	if api.DeploymentValues.RequireFIDO2UserVerification.Value() {
		cfg.AuthenticatorSelection.UserVerification = protocol.VerificationRequired
	}

	return webauthn.New(cfg)
}

// @Summary Begin WebAuthn registration
// @ID begin-webauthn-registration
// @Security CoderSessionToken
// @Produce json
// @Tags WebAuthn
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} protocol.CredentialCreation
// @Router /users/{user}/webauthn/register/begin [post]
func (api *API) beginWebAuthnRegistration(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	logger := api.Logger.Named("webauthn")

	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceWebauthnCredential.WithOwner(user.ID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	wan, err := api.newWebAuthn()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to initialize WebAuthn.",
			Detail:  err.Error(),
		})
		return
	}

	existing, err := api.Database.GetWebAuthnCredentialsByUserID(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch existing credentials.",
			Detail:  err.Error(),
		})
		return
	}

	wanUser := &webAuthnUser{user: user, creds: existing}

	creation, session, err := wan.BeginRegistration(wanUser,
		webauthn.WithExclusions(webauthn.Credentials(wanUser.WebAuthnCredentials()).CredentialDescriptors()),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to begin WebAuthn registration.",
			Detail:  err.Error(),
		})
		return
	}

	api.storeWebAuthnSession(user.ID.String()+":register", session)

	logger.Debug(ctx, "WebAuthn registration begun",
		slog.F("user_id", user.ID),
		slog.F("existing_credentials", len(existing)),
	)

	httpapi.Write(ctx, rw, http.StatusOK, creation)
}

// @Summary Finish WebAuthn registration
// @ID finish-webauthn-registration
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags WebAuthn
// @Param user path string true "User ID, name, or me"
// @Param name query string true "Credential name"
// @Success 201 {object} codersdk.WebAuthnCredential
// @Router /users/{user}/webauthn/register/finish [post]
func (api *API) finishWebAuthnRegistration(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	logger := api.Logger.Named("webauthn")

	credName := r.URL.Query().Get("name")
	if credName == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing credential name query parameter.",
		})
		return
	}
	if len(credName) > webAuthnMaxCredentialNameLen {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Credential name must be at most %d characters.", webAuthnMaxCredentialNameLen),
		})
		return
	}

	// Limit request body to prevent memory exhaustion.
	r.Body = http.MaxBytesReader(rw, r.Body, webAuthnMaxBodySize)

	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceWebauthnCredential.WithOwner(user.ID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	wan, err := api.newWebAuthn()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to initialize WebAuthn.",
			Detail:  err.Error(),
		})
		return
	}

	session := api.loadWebAuthnSession(user.ID.String() + ":register")
	if session == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "No pending registration session. Call begin first, or the session has expired.",
		})
		return
	}

	existing, err := api.Database.GetWebAuthnCredentialsByUserID(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch existing credentials.",
			Detail:  err.Error(),
		})
		return
	}

	wanUser := &webAuthnUser{user: user, creds: existing}

	// The request body is the raw attestation response JSON from
	// the authenticator. The go-webauthn library parses it directly.
	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		logger.Warn(ctx, "WebAuthn attestation parse failed",
			slog.Error(err),
		)
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to parse attestation response.",
			Detail:  err.Error(),
		})
		return
	}

	credential, err := wan.CreateCredential(wanUser, *session, parsedResponse)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to verify attestation.",
			Detail:  err.Error(),
		})
		return
	}

	dbCred, err := api.Database.InsertWebAuthnCredential(ctx, database.InsertWebAuthnCredentialParams{
		ID:              uuid.New(),
		UserID:          user.ID,
		CredentialID:    credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		Aaguid:          credential.Authenticator.AAGUID,
		SignCount:       int64(credential.Authenticator.SignCount),
		Name:            credName,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to store credential.",
			Detail:  err.Error(),
		})
		return
	}

	logger.Debug(ctx, "WebAuthn credential registered",
		slog.F("user_id", user.ID),
		slog.F("credential_id", dbCred.ID),
		slog.F("credential_name", credName),
	)

	httpapi.Write(ctx, rw, http.StatusCreated, convertWebAuthnCredential(dbCred))
}

// @Summary List WebAuthn credentials
// @ID list-webauthn-credentials
// @Security CoderSessionToken
// @Produce json
// @Tags WebAuthn
// @Param user path string true "User ID, name, or me"
// @Success 200 {array} codersdk.WebAuthnCredential
// @Router /users/{user}/webauthn/credentials [get]
func (api *API) listWebAuthnCredentials(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	creds, err := api.Database.GetWebAuthnCredentialsByUserID(ctx, user.ID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list credentials.",
			Detail:  err.Error(),
		})
		return
	}

	out := make([]codersdk.WebAuthnCredential, 0, len(creds))
	for _, c := range creds {
		out = append(out, convertWebAuthnCredential(c))
	}

	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Delete WebAuthn credential
// @ID delete-webauthn-credential
// @Security CoderSessionToken
// @Tags WebAuthn
// @Param user path string true "User ID, name, or me"
// @Param credentialID path string true "Credential ID" format(uuid)
// @Success 204
// @Router /users/{user}/webauthn/credentials/{credentialID} [delete]
func (api *API) deleteWebAuthnCredential(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	credIDStr := chi.URLParam(r, "credentialID")
	credID, err := uuid.Parse(credIDStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid credential ID.",
			Detail:  err.Error(),
		})
		return
	}

	// Fetch the credential — dbauthz enforces ownership via
	// RBACObject().WithOwner on the model. Authorization failures
	// are treated as "not found" to avoid leaking existence.
	cred, err := api.Database.GetWebAuthnCredentialByID(ctx, credID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Credential not found.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch credential.",
			Detail:  err.Error(),
		})
		return
	}

	if err := api.Database.DeleteWebAuthnCredential(ctx, cred.ID); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete credential.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Request WebAuthn challenge
// @ID request-webauthn-challenge
// @Security CoderSessionToken
// @Produce json
// @Tags WebAuthn
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} protocol.CredentialAssertion
// @Router /users/{user}/webauthn/challenge [post]
func (api *API) requestWebAuthnChallenge(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceWebauthnCredential.WithOwner(user.ID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	wan, err := api.newWebAuthn()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to initialize WebAuthn.",
			Detail:  err.Error(),
		})
		return
	}

	creds, err := api.Database.GetWebAuthnCredentialsByUserID(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch credentials.",
			Detail:  err.Error(),
		})
		return
	}

	if len(creds) == 0 {
		httpapi.Write(ctx, rw, http.StatusPreconditionFailed, codersdk.Response{
			Message: "No WebAuthn credentials registered. Register a security key first.",
		})
		return
	}

	wanUser := &webAuthnUser{user: user, creds: creds}

	assertion, session, err := wan.BeginLogin(wanUser)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to begin WebAuthn login.",
			Detail:  err.Error(),
		})
		return
	}

	api.storeWebAuthnSession(user.ID.String()+":assert", session)

	httpapi.Write(ctx, rw, http.StatusOK, assertion)
}

// @Summary Verify WebAuthn challenge
// @ID verify-webauthn-challenge
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags WebAuthn
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.WebAuthnVerifyResponse
// @Router /users/{user}/webauthn/verify [post]
func (api *API) verifyWebAuthnChallenge(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	logger := api.Logger.Named("webauthn")

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceWebauthnCredential.WithOwner(user.ID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	// Limit request body to prevent memory exhaustion.
	r.Body = http.MaxBytesReader(rw, r.Body, webAuthnMaxBodySize)

	wan, err := api.newWebAuthn()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to initialize WebAuthn.",
			Detail:  err.Error(),
		})
		return
	}

	session := api.loadWebAuthnSession(user.ID.String() + ":assert")
	if session == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "No pending assertion session. Call challenge first, or the session has expired.",
		})
		return
	}

	creds, err := api.Database.GetWebAuthnCredentialsByUserID(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch credentials.",
			Detail:  err.Error(),
		})
		return
	}

	wanUser := &webAuthnUser{user: user, creds: creds}

	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(r.Body)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to parse assertion response.",
			Detail:  err.Error(),
		})
		return
	}

	updatedCred, err := wan.ValidateLogin(wanUser, *session, parsedResponse)
	if err != nil {
		logger.Warn(ctx, "WebAuthn assertion verification failed",
			slog.F("user_id", user.ID),
			slog.Error(err),
		)
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "WebAuthn assertion verification failed.",
			Detail:  err.Error(),
		})
		return
	}

	logger.Debug(ctx, "WebAuthn assertion verified, issuing connection JWT",
		slog.F("user_id", user.ID),
	)

	// Update the sign count for replay protection. This is an
	// internal server operation after a successful assertion, so
	// we use the system context.
	credDBID, matchErr := matchCredentialID(creds, updatedCred.ID)
	if matchErr != nil {
		logger.Warn(ctx, "could not match credential for sign count update", slog.Error(matchErr))
	} else {
		//nolint:gocritic // System updates sign count after verification.
		if err := api.Database.UpdateWebAuthnCredentialSignCount(dbauthz.AsSystemRestricted(ctx), database.UpdateWebAuthnCredentialSignCountParams{
			ID:         credDBID,
			SignCount:  int64(updatedCred.Authenticator.SignCount),
			LastUsedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}); err != nil {
			// Log but don't fail — the assertion was valid.
			logger.Warn(ctx, "failed to update WebAuthn sign count", slog.Error(err))
		}
	}

	// Issue a short-lived connection JWT. Duration is controlled
	// by the --fido2-token-duration server flag. A value of 0
	// means single-use: we issue a token with a 10-second window,
	// enough for one connection handshake.
	now := time.Now()
	duration := api.DeploymentValues.Sessions.FIDO2TokenDuration.Value()
	if duration == 0 {
		duration = 10 * time.Second
	}
	claims := &WebAuthnConnectClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   user.ID.String(),
			Audience:  jwt.Audience{webAuthnConnectAudience},
			Expiry:    jwt.NewNumericDate(now.Add(duration)),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}

	token, err := jwtutils.Sign(ctx, api.WebAuthnConnectKeyCache, claims)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to sign connection JWT.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WebAuthnVerifyResponse{
		JWT: token,
	})
}

// matchCredentialID finds the database credential ID matching the
// given WebAuthn credential ID bytes. Returns an error if no match
// is found, preventing silent no-op updates.
func matchCredentialID(creds []database.WebauthnCredential, credentialID []byte) (uuid.UUID, error) {
	for _, c := range creds {
		if bytes.Equal(c.CredentialID, credentialID) {
			return c.ID, nil
		}
	}
	return uuid.Nil, xerrors.New("credential ID not found in user's credentials")
}

func convertWebAuthnCredential(c database.WebauthnCredential) codersdk.WebAuthnCredential {
	out := codersdk.WebAuthnCredential{
		ID:        c.ID,
		UserID:    c.UserID,
		Name:      c.Name,
		AAGUID:    c.Aaguid,
		CreatedAt: c.CreatedAt,
	}
	if c.LastUsedAt.Valid {
		out.LastUsedAt = &c.LastUsedAt.Time
	}
	return out
}

// VerifyWebAuthnConnectJWT verifies a connection JWT issued after
// WebAuthn assertion. Returns the user ID from the token's subject
// claim. This is intended to be called from the coordination
// endpoint to authenticate sensitive workspace operations.
func (api *API) VerifyWebAuthnConnectJWT(ctx context.Context, token string) (uuid.UUID, error) {
	var claims WebAuthnConnectClaims
	err := jwtutils.Verify(ctx, api.WebAuthnConnectKeyCache, token, &claims,
		jwtutils.WithVerifyExpected(jwt.Expected{
			AnyAudience: jwt.Audience{webAuthnConnectAudience},
			Issuer:      api.DeploymentID,
			Time:        time.Now(),
		}),
	)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("verify connection JWT: %w", err)
	}

	// In single-use mode (--fido2-token-duration=0), reject replayed
	// tokens by tracking the JTI. When duration > 0, the token is
	// intentionally reusable within its validity window (e.g., for
	// multiple SSH sessions or parallel port-forwards).
	if api.DeploymentValues.Sessions.FIDO2TokenDuration.Value() == 0 && claims.ID != "" {
		expiry := time.Now().Add(10 * time.Minute) // conservative TTL
		if claims.Expiry != nil {
			expiry = claims.Expiry.Time()
		}
		if !api.WebAuthnJTICache.MarkUsed(claims.ID, expiry) {
			return uuid.Nil, xerrors.New("connection JWT has already been used (replay detected)")
		}
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("parse user ID from JWT subject: %w", err)
	}

	return userID, nil
}
