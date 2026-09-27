package coderd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// Browser PKCE grants pave the second ChatGPT sign-in door next to the
// device-code flow. The shape mirrors Pi's browser login
// (packages/ai/src/auth/oauth/openai-codex.ts: loginOpenAICodex plus
// createAuthorizationFlow): the server mints PKCE verifier/challenge and
// state, builds the provider authorize URL with the pinned parameters,
// and exchanges the returned authorization code server-side. The
// credential then persists through the same server-side custody path as
// device-code approvals, so the refresh token never reaches the browser.
//
// Pi also starts a localhost callback listener for the CLI case. That has
// no server-side equivalent here: the provider redirects the user's own
// browser to localhost:1455, which lands on the user's machine, never on
// coderd. The working door is Pi's manual_code fallback instead: the user
// pastes the callback URL or code back into the dashboard, and the server
// parses, state-checks, and exchanges it. Only the single-use
// authorization code crosses from the browser to the server, over the
// authenticated API; key material stays server-side throughout.
//
// Grants live in memory on this replica like device-code grants do; a
// restart drops them and the user starts a fresh round.
const (
	aiBrowserGrantTimeoutSeconds = 15 * 60

	// aiBrowserChatGPTAuthorizeURL is the provider's authorization
	// endpoint, mirroring Pi's AUTHORIZE_URL.
	aiBrowserChatGPTAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	// aiBrowserChatGPTRedirectURI is the pinned loopback redirect Pi
	// registers against the public client. The provider only redirects to
	// registered URIs, so this must match Pi exactly.
	aiBrowserChatGPTRedirectURI = "http://localhost:1455/auth/callback"
	// aiBrowserChatGPTOriginator identifies this product in the
	// provider's originator telemetry. Pi sends "pi"; we send our own.
	aiBrowserChatGPTOriginator = "coder"
)

// browserFlowSupportedForProvider reports whether the paved browser PKCE
// sign-in exists for this provider row. It shares the provider table with
// the device-code flow: a provider with an authorize URL offers the
// browser door.
func browserFlowSupportedForProvider(provider database.AIProvider) bool {
	if !provider.Enabled {
		return false
	}
	config, ok := aiDeviceGrantConfigForProvider(provider.Type, provider.Name)
	return ok && config.authorizeURL != ""
}

// aiBrowserGeneratePKCE mints an RFC 7636 S256 pair: a 32-byte random
// verifier and its base64url SHA-256 challenge.
func aiBrowserGeneratePKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", xerrors.Errorf("generate PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// aiBrowserGenerateState mints the grant's CSRF state: 16 random bytes
// rendered as hex, mirroring Pi's createState.
func aiBrowserGenerateState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", xerrors.Errorf("generate OAuth state: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// aiBrowserAuthorizeURL builds the provider authorize URL with the pinned
// parameters: response_type=code, the public client id, the loopback
// redirect, the subscription scope, and the PKCE challenge plus state. The
// two Codex-flow flags ride along from Pi's createAuthorizationFlow.
func aiBrowserAuthorizeURL(config aiDeviceGrantConfig, challenge, state string) string {
	params := url.Values{
		"response_type":              {"code"},
		"client_id":                  {config.clientID},
		"redirect_uri":               {config.browserRedirectURI},
		"scope":                      {config.scope},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"state":                      {state},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"originator":                 {aiBrowserChatGPTOriginator},
	}
	return config.authorizeURL + "?" + params.Encode()
}

// aiBrowserParseAuthorizationInput extracts the authorization code and
// state from whatever the user pastes back: the full callback URL, a
// bare query string, Pi's code#state shorthand, or the raw code. It
// mirrors Pi's parseAuthorizationInput so dashboard paste behavior
// matches the CLI fallback exactly.
func aiBrowserParseAuthorizationInput(input string) (code, state string) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() && parsed.Host != "" {
		return parsed.Query().Get("code"), parsed.Query().Get("state")
	}
	if before, after, ok := strings.Cut(value, "#"); ok {
		return before, after
	}
	if strings.Contains(value, "code=") {
		params, err := url.ParseQuery(value)
		if err == nil {
			return params.Get("code"), params.Get("state")
		}
		return "", ""
	}
	return value, ""
}

// AIBrowserGrantExchanger trades one approved authorization code for the
// token triple. Tests substitute fakes through
// AIBrowserGrantManager.ClientFactory.
type AIBrowserGrantExchanger interface {
	ExchangeAuthorizationCode(ctx context.Context, code, verifier string) (AIDeviceTokenGrant, error)
}

type httpAIBrowserGrantExchanger struct {
	config     aiDeviceGrantConfig
	httpClient *http.Client
}

func (e *httpAIBrowserGrantExchanger) ExchangeAuthorizationCode(ctx context.Context, code, verifier string) (AIDeviceTokenGrant, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {e.config.clientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {e.config.browserRedirectURI},
	}
	return aiOAuthExchangeCodeForTokenGrant(ctx, e.httpClient, e.config.tokenURL, form)
}

// aiOAuthExchangeCodeForTokenGrant posts one authorization-code exchange
// and decodes the full token triple: access plus refresh plus lifetime.
// Both the device-code and browser doors exchange through here so the
// accepted shape cannot drift between doors. The triple is returned for
// server-side persistence only; callers must never place it in a browser
// response.
func aiOAuthExchangeCodeForTokenGrant(ctx context.Context, httpClient *http.Client, tokenURL string, form url.Values) (AIDeviceTokenGrant, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return AIDeviceTokenGrant{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return AIDeviceTokenGrant{}, err
	}
	defer resp.Body.Close()
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    any    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return AIDeviceTokenGrant{}, xerrors.Errorf("decode token response: %w", err)
	}
	if !aiDeviceRespOK(resp.StatusCode) {
		return AIDeviceTokenGrant{}, xerrors.Errorf("token exchange failed with status %d", resp.StatusCode)
	}
	if payload.AccessToken == "" {
		return AIDeviceTokenGrant{}, xerrors.New("token exchange response missing access token")
	}
	if err := validateChatProviderAPIKeySize(payload.AccessToken); err != nil {
		return AIDeviceTokenGrant{}, err
	}
	if payload.RefreshToken != "" {
		if err := validateChatProviderAPIKeySize(payload.RefreshToken); err != nil {
			return AIDeviceTokenGrant{}, err
		}
	}
	return AIDeviceTokenGrant{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		ExpiresIn:    aiDeviceExpiresInSeconds(payload.ExpiresIn),
	}, nil
}

// Sentinels for the browser exchange endpoint. Expired grants are not
// errors: exchange returns the expired snapshot so the dashboard can
// render the re-auth path, mirroring the device poll's expired state.
var (
	errAIBrowserGrantNotFound = xerrors.New("grant not found")
	errAIBrowserGrantState    = xerrors.New("authorization state mismatch: start a fresh sign-in and paste the newest callback")
	errAIBrowserGrantMissing  = xerrors.New("no authorization code found: paste the full callback URL or code")
)

// aiBrowserGrant is one in-flight browser PKCE authorization. The
// verifier is the grant's half of the PKCE pair: it never leaves the
// server and is single-use per exchange. oauth holds the exchanged
// triple (never logged) so a failed server-side persist can retry
// without re-exchanging the single-use code.
type aiBrowserGrant struct {
	id           uuid.UUID
	ownerID      uuid.UUID
	providerID   uuid.UUID
	config       aiDeviceGrantConfig
	verifier     string
	state        string
	createdAt    time.Time
	expiresAt    time.Time
	status       codersdk.AIDeviceGrantStatus
	oauth        *AIDeviceOAuthCredential
	authorizeURL string
	// persisted reports the credential reached the user key row
	// server-side. A persisted grant carries no key material anywhere:
	// the dashboard must not PUT after it.
	persisted bool
}

// AIBrowserGrantManager tracks in-flight browser PKCE grants for one API
// replica. Tests replace ClientFactory to avoid network use.
type AIBrowserGrantManager struct {
	mu      sync.Mutex
	clock   quartz.Clock
	grants  map[uuid.UUID]*aiBrowserGrant
	factory func(aiDeviceGrantConfig) AIBrowserGrantExchanger
	// persistAuthorized, when non-nil, writes the approved credential to
	// the user key row. It is the same server-side custody hook the
	// device-code manager uses: one persist path for both doors.
	persistAuthorized func(ctx context.Context, ownerID, providerID uuid.UUID, cred AIDeviceOAuthCredential) error
}

// NewAIBrowserGrantManager creates the per-replica grant registry.
func NewAIBrowserGrantManager(clock quartz.Clock) *AIBrowserGrantManager {
	return &AIBrowserGrantManager{
		clock:   clock,
		grants:  make(map[uuid.UUID]*aiBrowserGrant),
		factory: defaultAIBrowserGrantFactory,
	}
}

func defaultAIBrowserGrantFactory(config aiDeviceGrantConfig) AIBrowserGrantExchanger {
	return &httpAIBrowserGrantExchanger{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ClientFactory builds the provider talker for a grant. Overriding it is
// the test seam; production always uses the HTTP exchanger.
func (m *AIBrowserGrantManager) ClientFactory(factory func(providerType database.AIProviderType, name string) AIBrowserGrantExchanger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if factory == nil {
		m.factory = defaultAIBrowserGrantFactory
		return
	}
	m.factory = func(config aiDeviceGrantConfig) AIBrowserGrantExchanger {
		return factory(config.providerType, config.providerName)
	}
}

// SetPersistAuthorized wires server-side credential custody: on exchange
// the triple is written to the user key row so the refresh token never
// leaves the server. A nil hook keeps grants authorized in memory
// without persisting; production always sets it.
func (m *AIBrowserGrantManager) SetPersistAuthorized(hook func(ctx context.Context, ownerID, providerID uuid.UUID, cred AIDeviceOAuthCredential) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistAuthorized = hook
}

// initiate mints PKCE plus state, builds the provider authorize URL, and
// stores the grant. The verifier never leaves this response: only the
// challenge travels inside the URL.
func (m *AIBrowserGrantManager) initiate(_ context.Context, ownerID, providerID uuid.UUID, provider database.AIProvider) (*aiBrowserGrant, error) {
	config, ok := aiDeviceGrantConfigForProvider(provider.Type, provider.Name)
	if !ok || config.authorizeURL == "" {
		return nil, xerrors.New("browser sign-in is not supported for this provider")
	}
	verifier, challenge, err := aiBrowserGeneratePKCE()
	if err != nil {
		return nil, err
	}
	state, err := aiBrowserGenerateState()
	if err != nil {
		return nil, err
	}
	now := m.clock.Now()
	grant := &aiBrowserGrant{
		id:           uuid.New(),
		ownerID:      ownerID,
		providerID:   providerID,
		config:       config,
		verifier:     verifier,
		state:        state,
		createdAt:    now,
		expiresAt:    now.Add(aiBrowserGrantTimeoutSeconds * time.Second),
		status:       codersdk.AIDeviceGrantStatusPending,
		authorizeURL: aiBrowserAuthorizeURL(config, challenge, state),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(now)
	m.grants[grant.id] = grant
	snapshot := *grant
	return &snapshot, nil
}

// exchange parses the pasted callback input, state-checks it, trades the
// code for the token triple, derives the provider account id, and
// persists through the shared server-side path. Failures leave the grant
// pending so the user can retry the paste; only a successful persist
// authorizes the grant. A triple already exchanged but not yet persisted
// retries the persist without re-exchanging the single-use code.
func (m *AIBrowserGrantManager) exchange(ctx context.Context, grantID, callerID uuid.UUID, input string) (*aiBrowserGrant, error) {
	grant, ok := m.snapshot(grantID, callerID)
	if !ok {
		return nil, errAIBrowserGrantNotFound
	}
	if grant.status != codersdk.AIDeviceGrantStatusPending {
		return grant, nil
	}

	m.mu.Lock()
	current, ok := m.grants[grantID]
	if !ok || current.ownerID != callerID {
		m.mu.Unlock()
		return nil, errAIBrowserGrantNotFound
	}
	// A held triple from an earlier exchange whose persist failed
	// retries the persist without touching the provider again.
	if current.oauth != nil && m.persistAuthorized != nil {
		persist := m.persistAuthorized
		cred := *current.oauth
		ownerID, providerID := current.ownerID, current.providerID
		m.mu.Unlock()
		if err := persist(ctx, ownerID, providerID, cred); err != nil {
			return m.snapshotChecked(grantID, callerID)
		}
		return m.markAuthorized(grantID, callerID)
	}
	verifier := current.verifier
	expectedState := current.state
	client := m.factory(current.config)
	persist := m.persistAuthorized
	now := m.clock.Now()
	m.mu.Unlock()

	code, state := aiBrowserParseAuthorizationInput(input)
	if code == "" {
		return m.snapshotWith(grantID, callerID, errAIBrowserGrantMissing)
	}
	if state != "" && state != expectedState {
		return m.snapshotWith(grantID, callerID, errAIBrowserGrantState)
	}
	token, err := client.ExchangeAuthorizationCode(ctx, code, verifier)
	if err != nil {
		return m.snapshotChecked(grantID, callerID)
	}
	accountID, err := aiDeviceAccountIDFromJWT(token.AccessToken)
	if err != nil {
		return m.snapshotChecked(grantID, callerID)
	}
	cred := AIDeviceOAuthCredential{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		AccountID:    accountID,
	}
	if token.ExpiresIn > 0 {
		cred.ExpiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	m.mu.Lock()
	current, ok = m.grants[grantID]
	if !ok || current.ownerID != callerID {
		m.mu.Unlock()
		return nil, errAIBrowserGrantNotFound
	}
	if current.status != codersdk.AIDeviceGrantStatusPending {
		snapshot := *current
		m.mu.Unlock()
		return &snapshot, nil
	}
	// First exchange wins: a concurrent exchange that already stored the
	// triple keeps it, and this call reuses it.
	if current.oauth == nil {
		current.oauth = &cred
	}
	held := *current.oauth
	ownerID, providerID := current.ownerID, current.providerID
	m.mu.Unlock()

	if persist != nil {
		if err := persist(ctx, ownerID, providerID, held); err != nil {
			return m.snapshotChecked(grantID, callerID)
		}
	}
	return m.markAuthorized(grantID, callerID)
}

// markAuthorized flips a pending grant with a persisted (or persist-free)
// triple to authorized. Server-persisted grants carry no key material
// onward: the dashboard must not PUT after them.
func (m *AIBrowserGrantManager) markAuthorized(grantID, callerID uuid.UUID) (*aiBrowserGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.grants[grantID]
	if !ok || current.ownerID != callerID {
		return nil, errAIBrowserGrantNotFound
	}
	if current.status == codersdk.AIDeviceGrantStatusPending && current.oauth != nil {
		if m.persistAuthorized != nil {
			current.persisted = true
		}
		current.status = codersdk.AIDeviceGrantStatusAuthorized
	}
	snapshot := *current
	return &snapshot, nil
}

func (m *AIBrowserGrantManager) cancel(grantID, callerID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	grant, ok := m.grants[grantID]
	if !ok || grant.ownerID != callerID {
		return errAIBrowserGrantNotFound
	}
	if grant.status == codersdk.AIDeviceGrantStatusPending {
		grant.status = codersdk.AIDeviceGrantStatusCanceled
	}
	grant.oauth = nil
	return nil
}

// peek returns a grant snapshot for its owner without advancing it.
func (m *AIBrowserGrantManager) peek(grantID, callerID uuid.UUID) (*aiBrowserGrant, error) {
	snapshot, ok := m.snapshot(grantID, callerID)
	if !ok {
		return nil, errAIBrowserGrantNotFound
	}
	return snapshot, nil
}

// snapshot returns the grant for its owner, lazily expiring it.
func (m *AIBrowserGrantManager) snapshot(grantID, callerID uuid.UUID) (*aiBrowserGrant, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	grant, ok := m.grants[grantID]
	if !ok || grant.ownerID != callerID {
		return nil, false
	}
	m.expireLocked(grant)
	snapshot := *grant
	return &snapshot, true
}

// snapshotWith returns the current snapshot paired with a caller-provided
// error, used when the pasted input itself is unusable. A vanished grant
// surfaces as not-found instead.
func (m *AIBrowserGrantManager) snapshotWith(grantID, callerID uuid.UUID, err error) (*aiBrowserGrant, error) {
	snapshot, snapErr := m.snapshotChecked(grantID, callerID)
	if snapErr != nil {
		return nil, snapErr
	}
	return snapshot, err
}

// snapshotChecked returns the current snapshot or a not-found error,
// used after an exchange failure leaves the grant pending.
func (m *AIBrowserGrantManager) snapshotChecked(grantID, callerID uuid.UUID) (*aiBrowserGrant, error) {
	snapshot, ok := m.snapshot(grantID, callerID)
	if !ok {
		return nil, errAIBrowserGrantNotFound
	}
	return snapshot, nil
}

// expireLocked lazily marks a pending grant expired past its deadline.
func (m *AIBrowserGrantManager) expireLocked(grant *aiBrowserGrant) bool {
	if grant.status == codersdk.AIDeviceGrantStatusPending && !m.clock.Now().Before(grant.expiresAt) {
		grant.status = codersdk.AIDeviceGrantStatusExpired
		grant.oauth = nil
	}
	return grant.status != codersdk.AIDeviceGrantStatusPending
}

// sweepLocked drops grants that can no longer complete meaningfully:
// canceled grants and grants expired for over an hour. Expired grants
// stay briefly so exchanges report expired instead of vanishing.
func (m *AIBrowserGrantManager) sweepLocked(now time.Time) {
	for id, grant := range m.grants {
		if grant.status == codersdk.AIDeviceGrantStatusCanceled ||
			(now.Sub(grant.expiresAt) > time.Hour) {
			delete(m.grants, id)
		}
	}
}

// grantSecondsLeft reports whole seconds until expiry, floored at 0.
func aiBrowserGrantSecondsLeft(grant *aiBrowserGrant, now time.Time) int {
	secs := int(grant.expiresAt.Sub(now).Seconds())
	return max(secs, 0)
}

func aiBrowserGrantInitiateResponse(grant *aiBrowserGrant, now time.Time) codersdk.AIBrowserGrantInitiateResponse {
	return codersdk.AIBrowserGrantInitiateResponse{
		GrantID:               grant.id,
		ProviderID:            grant.providerID,
		AuthorizeURL:          grant.authorizeURL,
		ExpiresIn:             aiBrowserGrantSecondsLeft(grant, now),
		StoresAccessTokenOnly: false,
		RefreshSupported:      true,
		ReauthMessage:         aiDeviceReauthMessage,
	}
}

func aiBrowserGrantExchangeResponse(grant *aiBrowserGrant, now time.Time) codersdk.AIBrowserGrantExchangeResponse {
	return codersdk.AIBrowserGrantExchangeResponse{
		GrantID:               grant.id,
		ProviderID:            grant.providerID,
		Status:                grant.status,
		ExpiresIn:             aiBrowserGrantSecondsLeft(grant, now),
		StoresAccessTokenOnly: false,
		RefreshSupported:      true,
		ReauthMessage:         aiDeviceReauthMessage,
	}
}

// @Summary Initiate an AI provider browser PKCE grant
// @ID initiate-ai-provider-browser-grant
// @Security CoderSessionToken
// @Tags Chats
// @Param user path string true "User ID, username, or me"
// @Param aiProvider path string true "AI provider ID" format(uuid)
// @Produce json
// @Success 201 {object} codersdk.AIBrowserGrantInitiateResponse
// @Router /api/v2/users/{user}/ai-provider-keys/{aiProvider}/browser-grants [post]
func (api *API) postUserAIBrowserGrant(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUser, provider, ok := api.resolveAIDeviceGrantTarget(ctx, rw, r)
	if !ok {
		return
	}
	if !api.DeploymentValues.AI.BridgeConfig.AllowBYOK.Value() {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: "BYOK is disabled."})
		return
	}
	if !provider.Enabled {
		writeChatProviderPreconditionError(ctx, rw, errChatProviderDisabled)
		return
	}
	if !browserFlowSupportedForProvider(provider) {
		httpapi.Write(ctx, rw, http.StatusUnprocessableEntity, codersdk.Response{Message: "Browser sign-in is not supported for this provider."})
		return
	}
	grant, err := api.AIBrowserGrants.initiate(ctx, targetUser.ID, provider.ID, provider)
	if err != nil {
		if strings.Contains(err.Error(), "not supported for this provider") {
			httpapi.Write(ctx, rw, http.StatusUnprocessableEntity, codersdk.Response{Message: "Browser sign-in is not supported for this provider."})
			return
		}
		api.Logger.Error(ctx, "failed to initiate AI browser grant",
			slog.F("user_id", targetUser.ID),
			slog.F("ai_provider_id", provider.ID),
			slog.Error(err),
		)
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{Message: "Failed to start browser sign-in with the provider."})
		return
	}
	api.Logger.Info(ctx, "ai browser grant initiated",
		slog.F("user_id", targetUser.ID),
		slog.F("ai_provider_id", provider.ID),
		slog.F("grant_id", grant.id),
	)
	httpapi.Write(ctx, rw, http.StatusCreated, aiBrowserGrantInitiateResponse(grant, api.Clock.Now()))
}

// @Summary Exchange an AI provider browser PKCE grant code
// @ID exchange-ai-provider-browser-grant
// @Security CoderSessionToken
// @Tags Chats
// @Param user path string true "User ID, username, or me"
// @Param aiProvider path string true "AI provider ID" format(uuid)
// @Param grant path string true "Browser grant ID" format(uuid)
// @Accept json
// @Param request body codersdk.AIBrowserGrantExchangeRequest true "Pasted callback URL or code"
// @Produce json
// @Success 200 {object} codersdk.AIBrowserGrantExchangeResponse
// @Router /api/v2/users/{user}/ai-provider-keys/{aiProvider}/browser-grants/{grant}/exchange [post]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) postUserAIBrowserGrantExchange(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUser, provider, ok := api.resolveAIDeviceGrantTarget(ctx, rw, r)
	if !ok {
		return
	}
	grantID, err := uuid.Parse(chi.URLParam(r, "grant"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid browser grant ID."})
		return
	}
	var req codersdk.AIBrowserGrantExchangeRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	grant, err := api.AIBrowserGrants.peek(grantID, targetUser.ID)
	if err != nil {
		httpapi.ResourceNotFound(rw)
		return
	}
	if grant.providerID != provider.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	grant, err = api.AIBrowserGrants.exchange(ctx, grantID, targetUser.ID, req.Input)
	if err != nil {
		switch {
		case xerrors.Is(err, errAIBrowserGrantNotFound):
			httpapi.ResourceNotFound(rw)
		case xerrors.Is(err, errAIBrowserGrantState), xerrors.Is(err, errAIBrowserGrantMissing):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: err.Error()})
		default:
			api.Logger.Error(ctx, "failed to exchange AI browser grant",
				slog.F("user_id", targetUser.ID),
				slog.F("ai_provider_id", provider.ID),
				slog.F("grant_id", grantID),
				slog.Error(err),
			)
			httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{Message: "Failed to exchange the sign-in code with the provider."})
		}
		return
	}
	if grant.providerID != provider.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	if grant.status != codersdk.AIDeviceGrantStatusPending {
		api.Logger.Info(ctx, "ai browser grant terminal state",
			slog.F("user_id", targetUser.ID),
			slog.F("ai_provider_id", provider.ID),
			slog.F("grant_id", grantID),
			slog.F("status", string(grant.status)),
		)
	}
	httpapi.Write(ctx, rw, http.StatusOK, aiBrowserGrantExchangeResponse(grant, api.Clock.Now()))
}

// @Summary Cancel an AI provider browser PKCE grant
// @ID cancel-ai-provider-browser-grant
// @Security CoderSessionToken
// @Tags Chats
// @Param user path string true "User ID, username, or me"
// @Param aiProvider path string true "AI provider ID" format(uuid)
// @Param grant path string true "Browser grant ID" format(uuid)
// @Success 204
// @Router /api/v2/users/{user}/ai-provider-keys/{aiProvider}/browser-grants/{grant} [delete]
func (api *API) deleteUserAIBrowserGrant(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUser, provider, ok := api.resolveAIDeviceGrantTarget(ctx, rw, r)
	if !ok {
		return
	}
	grantID, err := uuid.Parse(chi.URLParam(r, "grant"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid browser grant ID."})
		return
	}
	managed, peekErr := api.AIBrowserGrants.peek(grantID, targetUser.ID)
	if peekErr == nil && managed.providerID != provider.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err := api.AIBrowserGrants.cancel(grantID, targetUser.ID); err != nil {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}
