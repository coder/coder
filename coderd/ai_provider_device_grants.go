package coderd

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// Device-code grants pave the in-dashboard OAuth flow for Coder Agents
// BYOK keys. A user starts a grant against their own provider key slot,
// approves the displayed user code at the subscription provider, and the
// runner persists the resulting credential server-side to the encrypted
// BYOK slot on approval, so the refresh token never leaves the server.
// Grants without server-side persistence (persist hook unset, tests) keep
// the legacy shape: the poll response carries the access token for the
// dashboard to save through the existing user-keys endpoint.
//
// The polling discipline mirrors Pi's device-code module
// (packages/ai/src/auth/oauth/device-code.ts): honor the server-provided
// interval with a 5s default, grow it by 5s on slow_down, cap the grant
// at ~15 minutes, and support cancel. Grants live in memory on this
// replica; a restart or a second replica drops them and the user starts
// a fresh round, which the UI states openly.
const (
	aiDeviceGrantTimeoutSeconds = 15 * 60
	// aiDeviceGrantDefaultPollIntervalSeconds applies when the
	// authorization server omits an interval (RFC 8628 section 3.2).
	aiDeviceGrantDefaultPollIntervalSeconds = 5
	aiDeviceGrantMinimumPollIntervalSeconds = 1
	// aiDeviceGrantSlowDownIncrementSeconds grows the poll interval on
	// slow_down (RFC 8628 section 3.5).
	aiDeviceGrantSlowDownIncrementSeconds = 5

	// aiDeviceChatGPTClientID is the public OAuth client for ChatGPT
	// subscription sign-in. It identifies the app to the authorization
	// server and is not a secret.
	aiDeviceChatGPTClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	aiDeviceChatGPTAuthBaseURL     = "https://auth.openai.com"
	aiDeviceChatGPTVerificationURI = "https://auth.openai.com/codex/device"
	aiDeviceChatGPTDeviceRedirect  = "https://auth.openai.com/deviceauth/callback"
	aiDeviceChatGPTScope           = "openid profile email offline_access"
	aiDeviceChatGPTProviderType    = database.AIProviderTypeOpenai
	aiDeviceChatGPTProviderName    = "chatgpt"
	aiDeviceReauthMessage          = "Coder keeps this sign-in fresh automatically. If the sign-in expires, sign in again for a fresh code."
)

// aiDeviceGrantConfig is the provider-generic shape for one subscription
// provider's device-code endpoints. ChatGPT is the first entry; later
// providers add their own without touching the runner.
type aiDeviceGrantConfig struct {
	clientID          string
	deviceUserCodeURL string
	deviceTokenURL    string
	tokenURL          string
	verificationURI   string
	deviceRedirectURI string
	// authorizeURL and browserRedirectURI pave the browser PKCE door
	// for this provider. Empty authorizeURL means browser sign-in is
	// unsupported; the device-code door is unaffected.
	authorizeURL       string
	browserRedirectURI string
	scope              string
	providerType       database.AIProviderType
	providerName       string
	timeoutSeconds     int
	defaultPollSeconds int
}

func aiDeviceGrantConfigForProvider(providerType database.AIProviderType, name string) (aiDeviceGrantConfig, bool) {
	if providerType == aiDeviceChatGPTProviderType && name == aiDeviceChatGPTProviderName {
		return aiDeviceGrantConfig{
			clientID:           aiDeviceChatGPTClientID,
			deviceUserCodeURL:  aiDeviceChatGPTAuthBaseURL + "/api/accounts/deviceauth/usercode",
			deviceTokenURL:     aiDeviceChatGPTAuthBaseURL + "/api/accounts/deviceauth/token",
			tokenURL:           aiDeviceChatGPTAuthBaseURL + "/oauth/token",
			verificationURI:    aiDeviceChatGPTVerificationURI,
			deviceRedirectURI:  aiDeviceChatGPTDeviceRedirect,
			authorizeURL:       aiBrowserChatGPTAuthorizeURL,
			browserRedirectURI: aiBrowserChatGPTRedirectURI,
			scope:              aiDeviceChatGPTScope,
			providerType:       aiDeviceChatGPTProviderType,
			providerName:       aiDeviceChatGPTProviderName,
			timeoutSeconds:     aiDeviceGrantTimeoutSeconds,
			defaultPollSeconds: aiDeviceGrantDefaultPollIntervalSeconds,
		}, true
	}
	return aiDeviceGrantConfig{}, false
}

// deviceFlowSupportedForProvider reports whether the paved device-code
// sign-in exists for this provider row. The list endpoint surfaces it so
// the dashboard only offers sign-in where a grant can start.
func deviceFlowSupportedForProvider(provider database.AIProvider) bool {
	if !provider.Enabled {
		return false
	}
	_, ok := aiDeviceGrantConfigForProvider(provider.Type, provider.Name)
	return ok
}

type aiDevicePollKind string

const (
	aiDevicePollPending  aiDevicePollKind = "pending"
	aiDevicePollSlowDown aiDevicePollKind = "slow_down"
	aiDevicePollComplete aiDevicePollKind = "complete"
	aiDevicePollDenied   aiDevicePollKind = "denied"
	aiDevicePollExpired  aiDevicePollKind = "expired"
	aiDevicePollFailed   aiDevicePollKind = "failed"
)

// AIDevicePollOutcome is one provider poll result. Constructors keep the
// kind internal while letting tests script every path.
type AIDevicePollOutcome struct {
	kind aiDevicePollKind
	// intervalSeconds carries the server-provided replacement interval
	// on slow_down, when given.
	intervalSeconds int
	hasInterval     bool
	// authorizationCode and verifier are set on complete. They are
	// single-use exchange material, never persisted.
	authorizationCode string
	verifier          string
	// message is a short operator-safe failure summary. It never carries
	// key material: outcomes run before any token exists.
	message string
}

// AIDeviceGrantPending reports "not yet approved".
func AIDeviceGrantPending() AIDevicePollOutcome {
	return AIDevicePollOutcome{kind: aiDevicePollPending}
}

// AIDeviceGrantSlowDown asks the runner to back off. A non-positive
// interval applies the RFC 8628 increment instead.
func AIDeviceGrantSlowDown(intervalSeconds int) AIDevicePollOutcome {
	outcome := AIDevicePollOutcome{kind: aiDevicePollSlowDown}
	if intervalSeconds > 0 {
		outcome.intervalSeconds, outcome.hasInterval = intervalSeconds, true
	}
	return outcome
}

// AIDeviceGrantComplete carries approved single-use exchange material.
func AIDeviceGrantComplete(authorizationCode, verifier string) AIDevicePollOutcome {
	return AIDevicePollOutcome{kind: aiDevicePollComplete, authorizationCode: authorizationCode, verifier: verifier}
}

// AIDeviceGrantDenied reports provider-side refusal.
func AIDeviceGrantDenied() AIDevicePollOutcome {
	return AIDevicePollOutcome{kind: aiDevicePollDenied, message: "authorization denied at the provider"}
}

// AIDeviceGrantExpired reports the device code lapsed before approval.
func AIDeviceGrantExpired() AIDevicePollOutcome {
	return AIDevicePollOutcome{kind: aiDevicePollExpired, message: "device code expired before approval"}
}

// AIDeviceGrantFailed reports a retryable provider-side failure.
func AIDeviceGrantFailed(message string) AIDevicePollOutcome {
	return AIDevicePollOutcome{kind: aiDevicePollFailed, message: message}
}

// AIDeviceTokenGrant is the full token shape from one approved device-code
// exchange: access plus refresh plus lifetime. The runner persists all of
// it server-side on approval; the refresh token never leaves the server.
type AIDeviceTokenGrant struct {
	AccessToken string
	// RefreshToken is empty when the provider issues access-only tokens.
	// An empty refresh persists as NULL, which means static-secret
	// behavior: the access token is used as-is, no refresh attempted.
	RefreshToken string
	// ExpiresIn is whole seconds until access-token expiry from expires_in.
	// Zero or negative means unknown: persisted as NULL, no refresh
	// attempted.
	ExpiresIn int
}

// AIDeviceGrantExchanger talks to one subscription provider's
// device-code endpoints. Tests substitute fakes through
// AIDeviceGrantManager.ClientFactory.
type AIDeviceGrantExchanger interface {
	RequestDeviceCode(ctx context.Context) (deviceAuthID, userCode string, intervalSeconds int, err error)
	PollDeviceToken(ctx context.Context, deviceAuthID, userCode string) (AIDevicePollOutcome, error)
	ExchangeAuthorizationCode(ctx context.Context, authorizationCode, verifier string) (AIDeviceTokenGrant, error)
}

// aiDeviceAuthClaimNamespace is the JWT claim namespace carrying the
// ChatGPT account id, per the portable OAuth refresh spec.
const aiDeviceAuthClaimNamespace = "https://api.openai.com/auth"

// aiDeviceAccountIDFromJWT derives the provider account id from the access
// JWT: base64url-decode the payload, read
// [aiDeviceAuthClaimNamespace].chatgpt_account_id, throw if missing. The
// token itself is never logged; errors name only the missing piece.
func aiDeviceAccountIDFromJWT(accessToken string) (string, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return "", xerrors.New("access token is not a JWT")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", xerrors.Errorf("decode access token payload: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", xerrors.Errorf("decode access token claims: %w", err)
	}
	namespace, ok := payload[aiDeviceAuthClaimNamespace].(map[string]any)
	if !ok {
		return "", xerrors.Errorf("access token JWT missing %q claim namespace", aiDeviceAuthClaimNamespace)
	}
	accountID, ok := namespace["chatgpt_account_id"].(string)
	if !ok || strings.TrimSpace(accountID) == "" {
		return "", xerrors.New("access token JWT missing chatgpt_account_id")
	}
	return accountID, nil
}

// aiDeviceExpiresInSeconds normalizes the provider expires_in field, which
// may arrive as a number or a string, into whole seconds. Non-positive or
// unparsable values report unknown (0): the caller persists NULL expiry
// and attempts no refresh.
func aiDeviceExpiresInSeconds(raw any) int {
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case float32:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case json.Number:
		if parsed, err := v.Int64(); err == nil && parsed > 0 {
			return int(parsed)
		}
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%g", &parsed); err == nil && parsed > 0 {
			return int(parsed)
		}
	}
	return 0
}

type httpAIDeviceGrantExchanger struct {
	config     aiDeviceGrantConfig
	httpClient *http.Client
}

func (e *httpAIDeviceGrantExchanger) RequestDeviceCode(ctx context.Context) (deviceAuthID, userCode string, intervalSeconds int, err error) {
	body, _ := json.Marshal(map[string]string{"client_id": e.config.clientID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.config.deviceUserCodeURL, strings.NewReader(string(body)))
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	var payload struct {
		DeviceAuthID string      `json:"device_auth_id"`
		UserCode     string      `json:"user_code"`
		Interval     interface{} `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", 0, xerrors.Errorf("decode device code response: %w", err)
	}
	if !aiDeviceRespOK(resp.StatusCode) {
		return "", "", 0, xerrors.Errorf("device code request failed with status %d", resp.StatusCode)
	}
	interval := aiDevicePollIntervalSeconds(payload.Interval, e.config.defaultPollSeconds)
	if payload.DeviceAuthID == "" || payload.UserCode == "" {
		return "", "", 0, xerrors.New("device code response missing fields")
	}
	return payload.DeviceAuthID, payload.UserCode, interval, nil
}

func aiDeviceRespOK(status int) bool {
	return status >= 200 && status < 300
}

// aiDevicePollIntervalSeconds normalizes the server-provided interval,
// which may arrive as a number or a string, into whole seconds clamped
// to the minimum.
func aiDevicePollIntervalSeconds(raw interface{}, def int) int {
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return max(int(v), aiDeviceGrantMinimumPollIntervalSeconds)
		}
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%g", &parsed); err == nil && parsed > 0 {
			return max(int(parsed), aiDeviceGrantMinimumPollIntervalSeconds)
		}
	case int:
		if v > 0 {
			return max(v, aiDeviceGrantMinimumPollIntervalSeconds)
		}
	}
	return max(def, aiDeviceGrantMinimumPollIntervalSeconds)
}

func (e *httpAIDeviceGrantExchanger) PollDeviceToken(ctx context.Context, deviceAuthID, userCode string) (AIDevicePollOutcome, error) {
	body, _ := json.Marshal(map[string]string{
		"device_auth_id": deviceAuthID,
		"user_code":      userCode,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.config.deviceTokenURL, strings.NewReader(string(body)))
	if err != nil {
		return AIDevicePollOutcome{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return AIDevicePollOutcome{}, err
	}
	defer resp.Body.Close()
	if aiDeviceRespOK(resp.StatusCode) {
		var payload struct {
			AuthorizationCode string `json:"authorization_code"`
			CodeVerifier      string `json:"code_verifier"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return AIDeviceGrantFailed("device token response unreadable"), nil
		}
		if payload.AuthorizationCode == "" || payload.CodeVerifier == "" {
			return AIDeviceGrantFailed("device token response missing fields"), nil
		}
		return AIDeviceGrantComplete(payload.AuthorizationCode, payload.CodeVerifier), nil
	}
	// The ChatGPT device endpoint reports "not yet approved" as 403/404
	// or an authorization_pending error code: still pending.
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return AIDeviceGrantPending(), nil
	}
	switch code, _ := aiDeviceErrorCode(resp); code {
	case "deviceauth_authorization_pending", "authorization_pending":
		return AIDeviceGrantPending(), nil
	case "slow_down":
		return AIDeviceGrantSlowDown(0), nil
	case "access_denied":
		return AIDeviceGrantDenied(), nil
	case "expired_token":
		return AIDeviceGrantExpired(), nil
	default:
		return AIDeviceGrantFailed(fmt.Sprintf("device authorization failed with status %d", resp.StatusCode)), nil
	}
}

// aiDeviceErrorCode extracts a short machine-readable error code from a
// provider error response without retaining the body.
func aiDeviceErrorCode(resp *http.Response) (string, bool) {
	var payload struct {
		Error interface{} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", false
	}
	switch v := payload.Error.(type) {
	case string:
		return v, v != ""
	case map[string]interface{}:
		if code, ok := v["code"].(string); ok && code != "" {
			return code, true
		}
	}
	return "", false
}

func (e *httpAIDeviceGrantExchanger) ExchangeAuthorizationCode(ctx context.Context, authorizationCode, verifier string) (AIDeviceTokenGrant, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {e.config.clientID},
		"code":          {authorizationCode},
		"code_verifier": {verifier},
		"redirect_uri":  {e.config.deviceRedirectURI},
	}
	// The full triple leaves this function for server-side persistence in
	// complete(). Only the access token may reach the browser poll
	// response, and only when the grant was not server-persisted. The
	// refresh token never leaves the server.
	return aiOAuthExchangeCodeForTokenGrant(ctx, e.httpClient, e.config.tokenURL, form)
}

// AIDeviceOAuthCredential is the server-persisted credential from one
// approved grant. The refresh token never leaves the server: it is written
// to the user key row through persistAuthorized, never carried in a poll
// response. Values are plaintext key material; persist immediately, log
// never.
type AIDeviceOAuthCredential struct {
	AccessToken string
	// RefreshToken is empty for access-only provider responses. An empty
	// refresh persists as NULL: static-secret behavior, no refresh.
	RefreshToken string
	// ExpiresAt is the access-token expiry from expires_in. Zero means
	// unknown: persisted as NULL, no refresh attempted.
	ExpiresAt time.Time
	AccountID string
}

// aiDeviceGrant is one in-flight device-code authorization. AccessToken
// is transient: set once on approval, read by the owning dashboard poll,
// and cleared when the grant is deleted or expires. It is never logged.
// oauth holds the full exchanged triple (also never logged) so a failed
// server-side persist can retry the persist on the next poll without
// re-exchanging the single-use authorization code.
type aiDeviceGrant struct {
	id           uuid.UUID
	ownerID      uuid.UUID
	providerID   uuid.UUID
	config       aiDeviceGrantConfig
	deviceAuthID string
	userCode     string
	createdAt    time.Time
	expiresAt    time.Time
	intervalSecs int
	status       codersdk.AIDeviceGrantStatus
	accessToken  string
	oauth        *AIDeviceOAuthCredential
	// persisted reports the credential reached the user key row
	// server-side. A persisted grant carries no key material in its poll
	// response: the dashboard must not PUT after it.
	persisted bool
}

// AIDeviceGrantManager tracks in-flight device-code grants for one API
// replica. Tests replace ClientFactory to avoid network use.
type AIDeviceGrantManager struct {
	mu      sync.Mutex
	clock   quartz.Clock
	grants  map[uuid.UUID]*aiDeviceGrant
	factory func(aiDeviceGrantConfig) AIDeviceGrantExchanger
	// persistAuthorized, when non-nil, writes the approved credential to
	// the user key row. See SetPersistAuthorized.
	persistAuthorized func(ctx context.Context, ownerID, providerID uuid.UUID, cred AIDeviceOAuthCredential) error
}

// ClientFactory builds the provider talker for a grant. Overriding it is
// the test seam; production always uses the HTTP exchanger.
func (m *AIDeviceGrantManager) ClientFactory(factory func(providerType database.AIProviderType, name string) AIDeviceGrantExchanger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if factory == nil {
		m.factory = defaultAIDeviceGrantFactory
		return
	}
	m.factory = func(config aiDeviceGrantConfig) AIDeviceGrantExchanger {
		return factory(config.providerType, config.providerName)
	}
}

func defaultAIDeviceGrantFactory(config aiDeviceGrantConfig) AIDeviceGrantExchanger {
	return &httpAIDeviceGrantExchanger{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewAIDeviceGrantManager creates the per-replica grant registry.
func NewAIDeviceGrantManager(clock quartz.Clock) *AIDeviceGrantManager {
	return &AIDeviceGrantManager{
		clock:   clock,
		grants:  make(map[uuid.UUID]*aiDeviceGrant),
		factory: defaultAIDeviceGrantFactory,
	}
}

// SetPersistAuthorized wires server-side credential custody: on approval
// the exchanged triple is written to the user key row so the refresh token
// never leaves the server and never appears in a poll response. A nil hook
// keeps the legacy shape (access token in the poll response for the
// dashboard to PUT); production always sets it. A hook failure leaves the
// grant pending so the next poll retries the persist without re-exchanging.
func (m *AIDeviceGrantManager) SetPersistAuthorized(hook func(ctx context.Context, ownerID, providerID uuid.UUID, cred AIDeviceOAuthCredential) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistAuthorized = hook
}

func (m *AIDeviceGrantManager) initiate(ctx context.Context, ownerID, providerID uuid.UUID, provider database.AIProvider) (*aiDeviceGrant, error) {
	config, ok := aiDeviceGrantConfigForProvider(provider.Type, provider.Name)
	if !ok {
		return nil, xerrors.New("device sign-in is not supported for this provider")
	}
	m.mu.Lock()
	client := m.factory(config)
	m.mu.Unlock()
	deviceAuthID, userCode, interval, err := client.RequestDeviceCode(ctx)
	if err != nil {
		return nil, err
	}
	now := m.clock.Now()
	grant := &aiDeviceGrant{
		id:           uuid.New(),
		ownerID:      ownerID,
		providerID:   providerID,
		config:       config,
		deviceAuthID: deviceAuthID,
		userCode:     userCode,
		createdAt:    now,
		expiresAt:    now.Add(time.Duration(config.timeoutSeconds) * time.Second),
		intervalSecs: interval,
		status:       codersdk.AIDeviceGrantStatusPending,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(now)
	m.grants[grant.id] = grant
	return grant, nil
}

// poll advances one grant for its owner. Each call performs at most one
// provider round-trip plus, on approval, one code exchange; the dashboard
// re-polls at the returned interval.
func (m *AIDeviceGrantManager) poll(ctx context.Context, grantID, callerID uuid.UUID) (*aiDeviceGrant, error) {
	grant, ok := m.snapshot(grantID, callerID)
	if !ok {
		return nil, xerrors.New("grant not found")
	}
	if grant.status != codersdk.AIDeviceGrantStatusPending {
		return grant, nil
	}

	m.mu.Lock()
	client := m.factory(grant.config)
	m.mu.Unlock()
	outcome, err := client.PollDeviceToken(ctx, grant.deviceAuthID, grant.userCode)
	if err != nil {
		return nil, err
	}
	if outcome.kind == aiDevicePollComplete {
		return m.complete(ctx, grantID, callerID, outcome)
	}

	m.mu.Lock()
	current, ok := m.grants[grantID]
	if !ok || current.ownerID != callerID {
		m.mu.Unlock()
		return nil, xerrors.New("grant not found")
	}
	if m.expireLocked(current) || current.status != codersdk.AIDeviceGrantStatusPending {
		snapshot := *current
		m.mu.Unlock()
		return &snapshot, nil
	}
	// An earlier poll exchanged the code but failed to persist
	// server-side; the triple is held in memory. Retry the persist
	// without re-exchanging the single-use code and without depending on
	// the provider re-reporting approval. Terminal provider states win
	// over the held triple.
	if current.oauth != nil && m.persistAuthorized != nil &&
		(outcome.kind == aiDevicePollPending || outcome.kind == aiDevicePollFailed || outcome.kind == aiDevicePollSlowDown) {
		persist := m.persistAuthorized
		cred := *current.oauth
		ownerID, providerID := current.ownerID, current.providerID
		m.mu.Unlock()
		if err := persist(ctx, ownerID, providerID, cred); err != nil {
			return m.snapshotChecked(grantID, callerID)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		current, ok = m.grants[grantID]
		if !ok || current.ownerID != callerID {
			return nil, xerrors.New("grant not found")
		}
		if current.status == codersdk.AIDeviceGrantStatusPending && current.oauth != nil {
			current.persisted = true
			current.accessToken = ""
			current.status = codersdk.AIDeviceGrantStatusAuthorized
		}
		snapshot := *current
		return &snapshot, nil
	}
	defer m.mu.Unlock()
	switch outcome.kind {
	case aiDevicePollPending:
		// No state change; the dashboard re-polls at intervalSecs.
	case aiDevicePollSlowDown:
		if outcome.hasInterval {
			current.intervalSecs = max(outcome.intervalSeconds, aiDeviceGrantMinimumPollIntervalSeconds)
		} else {
			current.intervalSecs += aiDeviceGrantSlowDownIncrementSeconds
		}
	case aiDevicePollDenied:
		current.status = codersdk.AIDeviceGrantStatusDenied
		current.accessToken = ""
		current.oauth = nil
	case aiDevicePollExpired:
		current.status = codersdk.AIDeviceGrantStatusExpired
		current.accessToken = ""
		current.oauth = nil
	case aiDevicePollFailed:
		// Provider-side failures stay pending: poll again. Terminal
		// mapping (denied/expired) arrives as its own kind above.
	}
	snapshot := *current
	return &snapshot, nil
}

// snapshot returns the grant for its owner, lazily expiring it. The
// second return reports whether the grant exists and is owner-held.
func (m *AIDeviceGrantManager) snapshot(grantID, callerID uuid.UUID) (*aiDeviceGrant, bool) {
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

// expireLocked lazily marks a pending grant expired past its deadline.
// It reports whether the grant is no longer pending.
func (m *AIDeviceGrantManager) expireLocked(grant *aiDeviceGrant) bool {
	if grant.status == codersdk.AIDeviceGrantStatusPending && !m.clock.Now().Before(grant.expiresAt) {
		grant.status = codersdk.AIDeviceGrantStatusExpired
		grant.accessToken = ""
		grant.oauth = nil
	}
	return grant.status != codersdk.AIDeviceGrantStatusPending
}

// complete exchanges an approved device authorization for the full token
// triple without holding the manager lock across the network round-trip.
// An exchange or account-derivation failure leaves the grant pending so
// the next poll retries; only a successful exchange authorizes the grant.
// When persistAuthorized is set, the triple is written to the user key row
// before the grant authorizes, so the refresh token never leaves the
// server. A persist failure leaves the grant pending with the exchanged
// triple held in memory: the next poll retries the persist without
// re-exchanging the single-use authorization code.
func (m *AIDeviceGrantManager) complete(ctx context.Context, grantID, callerID uuid.UUID, outcome AIDevicePollOutcome) (*aiDeviceGrant, error) {
	m.mu.Lock()
	grant, ok := m.grants[grantID]
	if !ok || grant.ownerID != callerID {
		m.mu.Unlock()
		return nil, xerrors.New("grant not found")
	}
	if m.expireLocked(grant) || grant.status != codersdk.AIDeviceGrantStatusPending {
		snapshot := *grant
		m.mu.Unlock()
		return &snapshot, nil
	}
	client := m.factory(grant.config)
	persist := m.persistAuthorized
	needExchange := grant.oauth == nil
	now := m.clock.Now()
	m.mu.Unlock()

	var exchanged *AIDeviceOAuthCredential
	if needExchange {
		token, err := client.ExchangeAuthorizationCode(ctx, outcome.authorizationCode, outcome.verifier)
		if err != nil {
			return m.snapshotChecked(grantID, callerID)
		}
		accountID, err := aiDeviceAccountIDFromJWT(token.AccessToken)
		if err != nil {
			return m.snapshotChecked(grantID, callerID)
		}
		exchanged = &AIDeviceOAuthCredential{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			AccountID:    accountID,
		}
		if token.ExpiresIn > 0 {
			exchanged.ExpiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second)
		}
	}

	m.mu.Lock()
	current, ok := m.grants[grantID]
	if !ok || current.ownerID != callerID {
		m.mu.Unlock()
		return nil, xerrors.New("grant not found")
	}
	if current.status != codersdk.AIDeviceGrantStatusPending {
		snapshot := *current
		m.mu.Unlock()
		return &snapshot, nil
	}
	// First exchange wins: a concurrent poll that already stored the
	// triple keeps it, and this poll reuses it.
	if current.oauth == nil && exchanged != nil {
		current.oauth = exchanged
		if persist == nil {
			current.accessToken = exchanged.AccessToken
		}
	}
	if current.oauth == nil {
		snapshot := *current
		m.mu.Unlock()
		return &snapshot, nil
	}
	cred := *current.oauth
	ownerID, providerID := current.ownerID, current.providerID
	m.mu.Unlock()

	if persist != nil {
		if err := persist(ctx, ownerID, providerID, cred); err != nil {
			return m.snapshotChecked(grantID, callerID)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok = m.grants[grantID]
	if !ok || current.ownerID != callerID {
		return nil, xerrors.New("grant not found")
	}
	if current.status == codersdk.AIDeviceGrantStatusPending && current.oauth != nil {
		if persist != nil {
			current.persisted = true
			// A persisted grant carries no key material in its poll
			// response: the dashboard must not PUT after it.
			current.accessToken = ""
		}
		current.status = codersdk.AIDeviceGrantStatusAuthorized
	}
	snapshot := *current
	return &snapshot, nil
}

// snapshotChecked returns the current snapshot or a not-found error,
// used after an exchange failure leaves the grant pending.
func (m *AIDeviceGrantManager) snapshotChecked(grantID, callerID uuid.UUID) (*aiDeviceGrant, error) {
	snapshot, ok := m.snapshot(grantID, callerID)
	if !ok {
		return nil, xerrors.New("grant not found")
	}
	return snapshot, nil
}

func (m *AIDeviceGrantManager) cancel(grantID, callerID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	grant, ok := m.grants[grantID]
	if !ok || grant.ownerID != callerID {
		return xerrors.New("grant not found")
	}
	if grant.status == codersdk.AIDeviceGrantStatusPending {
		grant.status = codersdk.AIDeviceGrantStatusCanceled
	}
	grant.accessToken = ""
	grant.oauth = nil
	return nil
}

// peek returns a grant snapshot for its owner without advancing it.
func (m *AIDeviceGrantManager) peek(grantID, callerID uuid.UUID) (*aiDeviceGrant, error) {
	snapshot, ok := m.snapshot(grantID, callerID)
	if !ok {
		return nil, xerrors.New("grant not found")
	}
	return snapshot, nil
}

// sweepLocked drops grants that can no longer be polled meaningfully:
// canceled grants and grants expired for over an hour. Expired grants
// stay briefly so polls report expired instead of vanishing.
func (m *AIDeviceGrantManager) sweepLocked(now time.Time) {
	for id, grant := range m.grants {
		if grant.status == codersdk.AIDeviceGrantStatusCanceled ||
			(now.Sub(grant.expiresAt) > time.Hour) {
			delete(m.grants, id)
		}
	}
}

// grantSecondsLeft reports whole seconds until expiry, floored at 0.
func grantSecondsLeft(grant *aiDeviceGrant, now time.Time) int {
	secs := int(grant.expiresAt.Sub(now).Seconds())
	return max(secs, 0)
}

func aiDeviceGrantInitiateResponse(grant *aiDeviceGrant, now time.Time) codersdk.AIDeviceGrantInitiateResponse {
	return codersdk.AIDeviceGrantInitiateResponse{
		GrantID:                 grant.id,
		ProviderID:              grant.providerID,
		UserCode:                grant.userCode,
		VerificationURI:         grant.config.verificationURI,
		VerificationURIComplete: grant.config.verificationURI + "/" + grant.userCode,
		ExpiresIn:               grantSecondsLeft(grant, now),
		PollInterval:            grant.intervalSecs,
		StoresAccessTokenOnly:   false,
		RefreshSupported:        true,
		ReauthMessage:           aiDeviceReauthMessage,
	}
}

// persistAIDeviceGrantCredential writes one approved device-grant credential
// to the user's key row. It is the server-side custody path implementing
// the captain's Q1 decision: the refresh token is written here and never
// appears in a poll response. A fresh approval clears any stale refresh
// failure bookkeeping. The Upsert overwrites OAuth state wholesale: a later
// pasted/static replace carries no OAuth material and drops it by
// definition.
func persistAIDeviceGrantCredential(db database.Store, clock quartz.Clock) func(ctx context.Context, ownerID, providerID uuid.UUID, cred AIDeviceOAuthCredential) error {
	return func(ctx context.Context, ownerID, providerID uuid.UUID, cred AIDeviceOAuthCredential) error {
		now := clock.Now()
		params := database.UpsertUserAIProviderKeyParams{
			ID:                        uuid.New(),
			UserID:                    ownerID,
			AIProviderID:              providerID,
			APIKey:                    cred.AccessToken,
			ApiKeyKeyID:               sql.NullString{},
			OAuthRefreshToken:         sql.NullString{},
			OAuthRefreshTokenKeyID:    sql.NullString{},
			OAuthExpiry:               sql.NullTime{},
			AccountID:                 sql.NullString{},
			OAuthExtra:                pqtype.NullRawMessage{},
			OauthRefreshFailureReason: sql.NullString{},
			CreatedAt:                 now,
			UpdatedAt:                 now,
		}
		if strings.TrimSpace(cred.AccountID) != "" {
			params.AccountID = sql.NullString{String: cred.AccountID, Valid: true}
		}
		if strings.TrimSpace(cred.RefreshToken) != "" {
			params.OAuthRefreshToken = sql.NullString{String: cred.RefreshToken, Valid: true}
		}
		if !cred.ExpiresAt.IsZero() {
			params.OAuthExpiry = sql.NullTime{Time: cred.ExpiresAt, Valid: true}
		}
		_, err := db.UpsertUserAIProviderKey(ctx, params)
		return err
	}
}

func aiDeviceGrantPollResponse(grant *aiDeviceGrant, now time.Time) codersdk.AIDeviceGrantPollResponse {
	return codersdk.AIDeviceGrantPollResponse{
		GrantID:                 grant.id,
		ProviderID:              grant.providerID,
		Status:                  grant.status,
		UserCode:                grant.userCode,
		VerificationURI:         grant.config.verificationURI,
		VerificationURIComplete: grant.config.verificationURI + "/" + grant.userCode,
		ExpiresIn:               grantSecondsLeft(grant, now),
		PollInterval:            grant.intervalSecs,
		APIKey:                  grant.accessToken,
		StoresAccessTokenOnly:   false,
		RefreshSupported:        true,
		ReauthMessage:           aiDeviceReauthMessage,
	}
}

// resolveAIDeviceGrantTarget loads the provider row and enforces the
// strict user scope: a caller can only mint into their own key slot,
// never another user's. Unknown grants, other users' slots, and other
// users' grants all surface as 404 so nothing about another user leaks.
func (api *API) resolveAIDeviceGrantTarget(ctx context.Context, rw http.ResponseWriter, r *http.Request) (targetUser database.User, provider database.AIProvider, ok bool) {
	targetUser = httpmw.UserParam(r)
	callerID := httpmw.APIKey(r).UserID
	if targetUser.ID != callerID {
		httpapi.ResourceNotFound(rw)
		return database.User{}, database.AIProvider{}, false
	}
	providerID, err := parseUserAIProviderID(r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid AI provider ID."})
		return database.User{}, database.AIProvider{}, false
	}
	//nolint:gocritic // Device-grant metadata needs provider type and name only.
	provider, err = api.Database.GetAIProviderByID(dbauthz.AsAIProviderMetadataReader(ctx), providerID)
	if err != nil {
		httpapi.ResourceNotFound(rw)
		return database.User{}, database.AIProvider{}, false
	}
	return targetUser, provider, true
}

// @Summary Initiate an AI provider device-code grant
// @ID initiate-ai-provider-device-code-grant
// @Security CoderSessionToken
// @Tags Chats
// @Param user path string true "User ID, username, or me"
// @Param aiProvider path string true "AI provider ID" format(uuid)
// @Produce json
// @Success 201 {object} codersdk.AIDeviceGrantInitiateResponse
// @Router /api/v2/users/{user}/ai-provider-keys/{aiProvider}/device-grants [post]
func (api *API) postUserAIDeviceGrant(rw http.ResponseWriter, r *http.Request) {
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
	if !deviceFlowSupportedForProvider(provider) {
		httpapi.Write(ctx, rw, http.StatusUnprocessableEntity, codersdk.Response{Message: "Device sign-in is not supported for this provider."})
		return
	}
	grant, err := api.AIDeviceGrants.initiate(ctx, targetUser.ID, provider.ID, provider)
	if err != nil {
		if strings.Contains(err.Error(), "not supported for this provider") {
			httpapi.Write(ctx, rw, http.StatusUnprocessableEntity, codersdk.Response{Message: "Device sign-in is not supported for this provider."})
			return
		}
		api.Logger.Error(ctx, "failed to initiate AI device grant",
			slog.F("user_id", targetUser.ID),
			slog.F("ai_provider_id", provider.ID),
			slog.Error(err),
		)
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{Message: "Failed to start device sign-in with the provider."})
		return
	}
	api.Logger.Info(ctx, "ai device grant initiated",
		slog.F("user_id", targetUser.ID),
		slog.F("ai_provider_id", provider.ID),
		slog.F("grant_id", grant.id),
	)
	httpapi.Write(ctx, rw, http.StatusCreated, aiDeviceGrantInitiateResponse(grant, api.Clock.Now()))
}

// @Summary Poll an AI provider device-code grant
// @ID poll-ai-provider-device-code-grant
// @Security CoderSessionToken
// @Tags Chats
// @Param user path string true "User ID, username, or me"
// @Param aiProvider path string true "AI provider ID" format(uuid)
// @Param grant path string true "Device grant ID" format(uuid)
// @Produce json
// @Success 200 {object} codersdk.AIDeviceGrantPollResponse
// @Router /api/v2/users/{user}/ai-provider-keys/{aiProvider}/device-grants/{grant} [get]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getUserAIDeviceGrant(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUser, provider, ok := api.resolveAIDeviceGrantTarget(ctx, rw, r)
	if !ok {
		return
	}
	grantID, err := uuid.Parse(chi.URLParam(r, "grant"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid device grant ID."})
		return
	}
	grant, err := api.AIDeviceGrants.poll(ctx, grantID, targetUser.ID)
	if err != nil {
		if strings.Contains(err.Error(), "grant not found") {
			httpapi.ResourceNotFound(rw)
			return
		}
		api.Logger.Error(ctx, "failed to poll AI device grant",
			slog.F("user_id", targetUser.ID),
			slog.F("ai_provider_id", provider.ID),
			slog.F("grant_id", grantID),
			slog.Error(err),
		)
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{Message: "Failed to check device sign-in status."})
		return
	}
	if grant.providerID != provider.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	if grant.status != codersdk.AIDeviceGrantStatusPending {
		api.Logger.Info(ctx, "ai device grant terminal state",
			slog.F("user_id", targetUser.ID),
			slog.F("ai_provider_id", provider.ID),
			slog.F("grant_id", grantID),
			slog.F("status", string(grant.status)),
		)
	}
	httpapi.Write(ctx, rw, http.StatusOK, aiDeviceGrantPollResponse(grant, api.Clock.Now()))
}

// @Summary Cancel an AI provider device-code grant
// @ID cancel-ai-provider-device-code-grant
// @Security CoderSessionToken
// @Tags Chats
// @Param user path string true "User ID, username, or me"
// @Param aiProvider path string true "AI provider ID" format(uuid)
// @Param grant path string true "Device grant ID" format(uuid)
// @Success 204
// @Router /api/v2/users/{user}/ai-provider-keys/{aiProvider}/device-grants/{grant} [delete]
func (api *API) deleteUserAIDeviceGrant(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetUser, provider, ok := api.resolveAIDeviceGrantTarget(ctx, rw, r)
	if !ok {
		return
	}
	grantID, err := uuid.Parse(chi.URLParam(r, "grant"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid device grant ID."})
		return
	}
	managed, peekErr := api.AIDeviceGrants.peek(grantID, targetUser.ID)
	if peekErr == nil && managed.providerID != provider.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err := api.AIDeviceGrants.cancel(grantID, targetUser.ID); err != nil {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}
