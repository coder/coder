package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

// People who manage an agent connect external auth accounts for the agent's
// user, which cannot sign in. The browser runs the provider's normal flow
// through the provider's registered callback; a signed cookie bound to the
// OAuth state tells the callback to store the link for the agent's user
// instead of the signed-in person.

const (
	chatProjectExternalAuthCookie      = "coder_agent_external_auth"
	chatProjectExternalAuthStatePrefix = "agent-"
	chatProjectExternalAuthAudience    = "chat-project-external-auth"
)

type chatProjectExternalAuthClaims struct {
	jwtutils.RegisteredClaims

	ActorID     uuid.UUID `json:"actor_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	AgentUserID uuid.UUID `json:"agent_user_id"`
	ProviderID  string    `json:"provider_id"`
	State       string    `json:"state"`
}

func (c *chatProjectExternalAuthClaims) Validate(e jwt.Expected) error {
	return c.RegisteredClaims.Validate(e)
}

func isChatProjectExternalAuthState(state string) bool {
	return strings.HasPrefix(state, chatProjectExternalAuthStatePrefix)
}

// chatProjectForExternalAuth loads the route's project with its agent user
// and checks the caller may manage it.
func (api *API) chatProjectForExternalAuth(rw http.ResponseWriter, r *http.Request, action policy.Action) (database.ChatProject, bool) {
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, action, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return database.ChatProject{}, false
	}
	project, err := api.ensureChatProjectAgentUser(r.Context(), project)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return database.ChatProject{}, false
	}
	return project, true
}

// chatProjectForAgentAccountChange is chatProjectForExternalAuth for
// connecting or disconnecting the agent's accounts. Chats act with these
// accounts, so like triggers and MCP server tokens it is limited to the
// agent's owner.
func (api *API) chatProjectForAgentAccountChange(rw http.ResponseWriter, r *http.Request) (database.ChatProject, bool) {
	project, ok := api.chatProjectForExternalAuth(rw, r, policy.ActionUpdate)
	if !ok {
		return database.ChatProject{}, false
	}
	if project.OwnerID != httpmw.APIKey(r).UserID {
		httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
			Message: chatProjectAgentAccountOwnerOnly,
		})
		return database.ChatProject{}, false
	}
	return project, true
}

const chatProjectAgentAccountOwnerOnly = "Only the agent owner can connect or disconnect the agent's accounts."

// @Summary List chat project external auth
// @ID list-chat-project-external-auth
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ListUserExternalAuthResponse
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/external-auth [get]
// @x-apidocgen {"skip": true}
func (api *API) chatProjectExternalAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, ok := api.chatProjectForExternalAuth(rw, r, policy.ActionRead)
	if !ok {
		return
	}
	//nolint:gocritic // Reading the agent user's links is authorized by the project check above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	links, err := api.Database.GetExternalAuthLinksByUserID(sysCtx, project.AgentUserID.UUID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("get agent external auth links: %w", err))
		return
	}
	configs := make(map[string]*externalauth.Config, len(api.ExternalAuthConfigs))
	for _, cfg := range api.ExternalAuthConfigs {
		configs[cfg.ID] = cfg
	}
	linkMeta := make(map[string]db2sdk.ExternalAuthMeta)
	for i, link := range links {
		cfg, ok := configs[link.ProviderID]
		if !ok || link.OAuthAccessToken == "" {
			continue
		}
		refreshed, err := cfg.RefreshToken(sysCtx, api.Database, link)
		meta := db2sdk.ExternalAuthMeta{Authenticated: err == nil}
		if err != nil {
			meta.ValidateError = err.Error()
		} else {
			links[i] = refreshed
		}
		linkMeta[link.ProviderID] = meta
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ListUserExternalAuthResponse{
		Providers: ExternalAuthConfigs(api.ExternalAuthConfigs),
		Links:     db2sdk.ExternalAuths(links, linkMeta),
	})
}

// @Summary Connect chat project external auth
// @ID connect-chat-project-external-auth
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param externalauth path string true "External auth provider ID"
// @Param request body codersdk.ConnectChatProjectExternalAuthRequest true "Connect request"
// @Success 200 {object} codersdk.ConnectChatProjectExternalAuthResponse
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/external-auth/{externalauth}/connect [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProjectExternalAuthConnect(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	config := httpmw.ExternalAuthParam(r)
	project, ok := api.chatProjectForAgentAccountChange(rw, r)
	if !ok {
		return
	}
	var req codersdk.ConnectChatProjectExternalAuthRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if config.DeviceAuth != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This provider uses the device flow. Exchange a device code instead.",
		})
		return
	}

	random, err := cryptorand.String(32)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("generate state: %w", err))
		return
	}
	state := chatProjectExternalAuthStatePrefix + random
	now := time.Now()
	claims := &chatProjectExternalAuthClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   state,
			Audience:  []string{chatProjectExternalAuthAudience},
			Expiry:    jwt.NewNumericDate(now.Add(10 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
		ActorID:     apiKey.UserID,
		ProjectID:   project.ID,
		AgentUserID: project.AgentUserID.UUID,
		ProviderID:  config.ID,
		State:       state,
	}
	// The OIDC convert key signs short-lived, state-bound browser flow
	// tokens. The audience keeps these tokens from being accepted there.
	token, err := jwtutils.Sign(ctx, api.OIDCConvertKeyCache, claims)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("sign external auth state: %w", err))
		return
	}
	callbackPath := "/external-auth/" + url.PathEscape(config.ID) + "/callback"
	http.SetCookie(rw, api.DeploymentValues.HTTPCookies.Apply(&http.Cookie{
		Name:     chatProjectExternalAuthCookie,
		Path:     callbackPath,
		Value:    token,
		Expires:  claims.Expiry.Time(),
		HttpOnly: true,
		// The provider redirects back with a top-level navigation.
		SameSite: http.SameSiteLaxMode,
	}))

	query := url.Values{}
	query.Set("oidc_merge_state", state)
	if req.Redirect != "" {
		query.Set("redirect", httpapi.SafeRedirectPath(req.Redirect))
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ConnectChatProjectExternalAuthResponse{
		URL: callbackPath + "?" + query.Encode(),
	})
}

// chatProjectExternalAuthTarget resolves the agent user an external auth
// callback links for. It writes an error and returns false when the flow is
// not authorized.
func (api *API) chatProjectExternalAuthTarget(rw http.ResponseWriter, r *http.Request, config *externalauth.Config, state string) (uuid.UUID, bool) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	// The cookie is single use whatever the outcome.
	http.SetCookie(rw, &http.Cookie{
		Name:   chatProjectExternalAuthCookie,
		Path:   "/external-auth/" + url.PathEscape(config.ID) + "/callback",
		MaxAge: -1,
	})
	deny := func(message string) (uuid.UUID, bool) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: message})
		return uuid.Nil, false
	}
	cookie, err := r.Cookie(chatProjectExternalAuthCookie)
	if err != nil {
		return deny("The request to connect an account for an agent expired. Try again from the agent's settings.")
	}
	var claims chatProjectExternalAuthClaims
	err = jwtutils.Verify(ctx, api.OIDCConvertKeyCache, cookie.Value, &claims, jwtutils.WithVerifyExpected(jwt.Expected{
		Issuer:      api.DeploymentID,
		AnyAudience: jwt.Audience{chatProjectExternalAuthAudience},
		Time:        time.Now(),
	}))
	if err != nil {
		return deny("The request to connect an account for an agent is invalid. Try again from the agent's settings.")
	}
	if claims.State != state || claims.ActorID != apiKey.UserID || claims.ProviderID != config.ID {
		return deny("The request to connect an account for an agent does not match this sign-in. Try again from the agent's settings.")
	}
	// Access may have changed since the flow started.
	//nolint:gocritic // Authorization is checked against the caller below.
	project, err := api.Database.GetChatProjectByID(dbauthz.AsSystemRestricted(ctx), claims.ProjectID)
	if err != nil || project.AgentUserID != (uuid.NullUUID{UUID: claims.AgentUserID, Valid: true}) ||
		!api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		return deny("You can no longer manage this agent.")
	}
	if project.OwnerID != apiKey.UserID {
		return deny(chatProjectAgentAccountOwnerOnly)
	}
	return claims.AgentUserID, true
}

// @Summary Exchange chat project external auth device code
// @ID exchange-chat-project-external-auth-device-code
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param externalauth path string true "External auth provider ID"
// @Param request body codersdk.ExternalAuthDeviceExchange true "Device code"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/external-auth/{externalauth}/device [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProjectExternalAuthDevice(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.ExternalAuthParam(r)
	project, ok := api.chatProjectForAgentAccountChange(rw, r)
	if !ok {
		return
	}
	var req codersdk.ExternalAuthDeviceExchange
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if config.DeviceAuth == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "This provider does not support the device flow.",
		})
		return
	}
	token, err := config.DeviceAuth.ExchangeDeviceCode(ctx, req.DeviceCode)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to exchange device code.",
			Detail:  err.Error(),
		})
		return
	}
	//nolint:gocritic // Writing the agent user's link is authorized by the project check above.
	if err := upsertExternalAuthLink(dbauthz.AsSystemRestricted(ctx), api.Database, config.ID, project.AgentUserID.UUID, token, pqtype.NullRawMessage{}); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Disconnect chat project external auth
// @ID disconnect-chat-project-external-auth
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param externalauth path string true "External auth provider ID"
// @Success 200 {object} codersdk.DeleteExternalAuthByIDResponse
// @Router /api/experimental/organizations/{organization}/chats/projects/{project}/external-auth/{externalauth} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProjectExternalAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.ExternalAuthParam(r)
	project, ok := api.chatProjectForAgentAccountChange(rw, r)
	if !ok {
		return
	}
	//nolint:gocritic // Removing the agent user's link is authorized by the project check above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	params := database.GetExternalAuthLinkParams{ProviderID: config.ID, UserID: project.AgentUserID.UUID}
	link, err := api.Database.GetExternalAuthLink(sysCtx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	if err := api.Database.DeleteExternalAuthLink(sysCtx, database.DeleteExternalAuthLinkParams(params)); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	revoked, err := config.RevokeToken(ctx, link)
	resp := codersdk.DeleteExternalAuthByIDResponse{TokenRevoked: revoked}
	if err != nil {
		resp.TokenRevocationError = err.Error()
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// upsertExternalAuthLink stores a provider token for a user.
func upsertExternalAuthLink(ctx context.Context, store database.Store, providerID string, userID uuid.UUID, token *oauth2.Token, extra pqtype.NullRawMessage) error {
	_, err := store.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{ProviderID: providerID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		_, err = store.InsertExternalAuthLink(ctx, database.InsertExternalAuthLinkParams{
			ProviderID:             providerID,
			UserID:                 userID,
			CreatedAt:              dbtime.Now(),
			UpdatedAt:              dbtime.Now(),
			OAuthAccessToken:       token.AccessToken,
			OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will set as required
			OAuthRefreshToken:      token.RefreshToken,
			OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will set as required
			OAuthExpiry:            token.Expiry,
			OAuthExtra:             extra,
		})
		if err != nil {
			return xerrors.Errorf("insert external auth link: %w", err)
		}
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get external auth link: %w", err)
	}
	_, err = store.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
		ProviderID:                providerID,
		UserID:                    userID,
		UpdatedAt:                 dbtime.Now(),
		OAuthAccessToken:          token.AccessToken,
		OAuthAccessTokenKeyID:     sql.NullString{}, // dbcrypt will update as required
		OAuthRefreshToken:         token.RefreshToken,
		OAuthRefreshTokenKeyID:    sql.NullString{}, // dbcrypt will update as required
		OAuthExpiry:               token.Expiry,
		OAuthExtra:                extra,
		OauthRefreshFailureReason: "",
		RefreshLeaseExpiresAt:     sql.NullTime{},
	})
	if err != nil {
		return xerrors.Errorf("update external auth link: %w", err)
	}
	return nil
}
