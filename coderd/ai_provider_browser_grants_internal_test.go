package coderd

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// All key material below is throwaway and invalid. These tests never touch
// live credentials, the keychain, or a real subscription provider.

// fakeAIBrowserExchanger scripts one grant's token endpoint.
type fakeAIBrowserExchanger struct {
	mu            sync.Mutex
	exchangeGrant AIDeviceTokenGrant
	exchangeErr   error
	exchanges     int
	lastCode      string
	lastVerifier  string
}

func (f *fakeAIBrowserExchanger) ExchangeAuthorizationCode(_ context.Context, code, verifier string) (AIDeviceTokenGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exchanges++
	f.lastCode = code
	f.lastVerifier = verifier
	if f.exchangeErr != nil {
		return AIDeviceTokenGrant{}, f.exchangeErr
	}
	return f.exchangeGrant, nil
}

func testBrowserManager(t testing.TB, fake *fakeAIBrowserExchanger) (*AIBrowserGrantManager, *quartz.Mock) {
	t.Helper()
	clock := quartz.NewMock(t)
	manager := NewAIBrowserGrantManager(clock)
	manager.ClientFactory(func(_ database.AIProviderType, _ string) AIBrowserGrantExchanger {
		return fake
	})
	return manager, clock
}

func testBrowserTokenGrant(t testing.TB, accountID string) AIDeviceTokenGrant {
	t.Helper()
	return AIDeviceTokenGrant{
		AccessToken:  testDeviceAccessJWT(t, accountID),
		RefreshToken: "test-browser-refresh-token", // #nosec G101 -- test fixture, not a credential.
		ExpiresIn:    3600,
	}
}

func TestAIBrowserPKCE(t *testing.T) {
	t.Parallel()

	verifier, challenge, err := aiBrowserGeneratePKCE()
	require.NoError(t, err)
	require.Len(t, verifier, 43, "32 random bytes render as 43 base64url chars")
	sum := sha256.Sum256([]byte(verifier))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), challenge)

	verifier2, challenge2, err := aiBrowserGeneratePKCE()
	require.NoError(t, err)
	require.NotEqual(t, verifier, verifier2, "verifiers must be random per grant")
	require.NotEqual(t, challenge, challenge2)

	state, err := aiBrowserGenerateState()
	require.NoError(t, err)
	require.Len(t, state, 32, "16 random bytes render as 32 hex chars")
	state2, err := aiBrowserGenerateState()
	require.NoError(t, err)
	require.NotEqual(t, state, state2)
}

func TestAIBrowserAuthorizeURL(t *testing.T) {
	t.Parallel()

	config, ok := aiDeviceGrantConfigForProvider(database.AIProviderTypeOpenai, "chatgpt")
	require.True(t, ok)
	raw := aiBrowserAuthorizeURL(config, "test-challenge", "test-state")
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "auth.openai.com", parsed.Host)
	require.Equal(t, "/oauth/authorize", parsed.Path)
	query := parsed.Query()
	require.Equal(t, "code", query.Get("response_type"))
	require.Equal(t, "app_EMoamEEZ73f0CkXaXp7hrann", query.Get("client_id"))
	require.Equal(t, "http://localhost:1455/auth/callback", query.Get("redirect_uri"))
	require.Equal(t, "openid profile email offline_access", query.Get("scope"))
	require.Equal(t, "test-challenge", query.Get("code_challenge"))
	require.Equal(t, "S256", query.Get("code_challenge_method"))
	require.Equal(t, "test-state", query.Get("state"))
}

func TestAIBrowserParseAuthorizationInput(t *testing.T) {
	t.Parallel()

	const callback = "http://localhost:1455/auth/callback?code=auth-code-123&state=state-abc"
	cases := []struct {
		name      string
		input     string
		wantCode  string
		wantState string
	}{
		{name: "FullCallbackURL", input: callback, wantCode: "auth-code-123", wantState: "state-abc"},
		{name: "CallbackURLCodeOnly", input: "http://localhost:1455/auth/callback?code=auth-code-123", wantCode: "auth-code-123"},
		{name: "QueryString", input: "code=auth-code-123&state=state-abc", wantCode: "auth-code-123", wantState: "state-abc"},
		{name: "PiCodeStateShorthand", input: "auth-code-123#state-abc", wantCode: "auth-code-123", wantState: "state-abc"},
		{name: "RawCode", input: "auth-code-123", wantCode: "auth-code-123"},
		{name: "PaddedCode", input: "  auth-code-123\n", wantCode: "auth-code-123"},
		{name: "Empty", input: "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, state := aiBrowserParseAuthorizationInput(tc.input)
			require.Equal(t, tc.wantCode, code)
			require.Equal(t, tc.wantState, state)
		})
	}
}

func TestAIBrowserGrantLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := uuid.New()
	provider := testChatGPTProvider()
	fake := &fakeAIBrowserExchanger{exchangeGrant: testBrowserTokenGrant(t, "acct-browser-123")}
	manager, _ := testBrowserManager(t, fake)

	var persisted *AIDeviceOAuthCredential
	manager.SetPersistAuthorized(func(_ context.Context, _, _ uuid.UUID, cred AIDeviceOAuthCredential) error {
		c := cred
		persisted = &c
		return nil
	})

	grant, err := manager.initiate(ctx, owner, provider.ID, provider)
	require.NoError(t, err)
	require.Equal(t, codersdk.AIDeviceGrantStatusPending, grant.status)
	require.NotEmpty(t, grant.verifier, "verifier stays server-side on the stored grant")
	require.NotEmpty(t, grant.state)
	require.Contains(t, grant.authorizeURL, "https://auth.openai.com/oauth/authorize?")
	require.NotContains(t, grant.authorizeURL, grant.verifier, "verifier never travels in the URL")

	callback := "http://localhost:1455/auth/callback?code=test-auth-code&state=" + grant.state
	settled, err := manager.exchange(ctx, grant.id, owner, callback)
	require.NoError(t, err)
	require.Equal(t, codersdk.AIDeviceGrantStatusAuthorized, settled.status)
	require.True(t, settled.persisted)
	require.Equal(t, "test-auth-code", fake.lastCode)
	require.Equal(t, grant.verifier, fake.lastVerifier, "exchange uses the stored verifier")

	require.NotNil(t, persisted, "exchange persists through the shared server-side path")
	require.Equal(t, testDeviceAccessJWT(t, "acct-browser-123"), persisted.AccessToken)
	require.Equal(t, "test-browser-refresh-token", persisted.RefreshToken)
	require.Equal(t, "acct-browser-123", persisted.AccountID)
	require.False(t, persisted.ExpiresAt.IsZero())
	require.Equal(t, time.Hour, persisted.ExpiresAt.Sub(grant.createdAt), "expiry derives from expires_in")

	again, err := manager.exchange(ctx, grant.id, owner, callback)
	require.NoError(t, err, "exchange on a settled grant returns its snapshot")
	require.Equal(t, codersdk.AIDeviceGrantStatusAuthorized, again.status)
	require.Equal(t, 1, fake.exchanges, "settled grants never re-exchange")
}

func TestAIBrowserGrantStateMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := uuid.New()
	provider := testChatGPTProvider()
	fake := &fakeAIBrowserExchanger{exchangeGrant: testBrowserTokenGrant(t, "acct-browser-123")}
	manager, _ := testBrowserManager(t, fake)
	manager.SetPersistAuthorized(func(_ context.Context, _, _ uuid.UUID, _ AIDeviceOAuthCredential) error {
		return nil
	})

	grant, err := manager.initiate(ctx, owner, provider.ID, provider)
	require.NoError(t, err)

	settled, err := manager.exchange(ctx, grant.id, owner, "http://localhost:1455/auth/callback?code=test-auth-code&state=stale-state")
	require.ErrorIs(t, err, errAIBrowserGrantState)
	require.Equal(t, codersdk.AIDeviceGrantStatusPending, settled.status)
	require.Equal(t, 0, fake.exchanges, "state mismatch never reaches the provider")
}

func TestAIBrowserGrantMissingCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := uuid.New()
	provider := testChatGPTProvider()
	fake := &fakeAIBrowserExchanger{exchangeGrant: testBrowserTokenGrant(t, "acct-browser-123")}
	manager, _ := testBrowserManager(t, fake)

	grant, err := manager.initiate(ctx, owner, provider.ID, provider)
	require.NoError(t, err)

	settled, err := manager.exchange(ctx, grant.id, owner, "   ")
	require.ErrorIs(t, err, errAIBrowserGrantMissing)
	require.Equal(t, codersdk.AIDeviceGrantStatusPending, settled.status)
	require.Equal(t, 0, fake.exchanges)
}

func TestAIBrowserGrantAccountIDDerivation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := uuid.New()
	provider := testChatGPTProvider()
	fake := &fakeAIBrowserExchanger{exchangeGrant: AIDeviceTokenGrant{
		AccessToken:  "not-a-jwt",
		RefreshToken: "test-browser-refresh-token", // #nosec G101 -- test fixture, not a credential.
		ExpiresIn:    3600,
	}}
	manager, _ := testBrowserManager(t, fake)
	persisted := false
	manager.SetPersistAuthorized(func(_ context.Context, _, _ uuid.UUID, _ AIDeviceOAuthCredential) error {
		persisted = true
		return nil
	})

	grant, err := manager.initiate(ctx, owner, provider.ID, provider)
	require.NoError(t, err)

	settled, err := manager.exchange(ctx, grant.id, owner, "test-auth-code")
	require.NoError(t, err, "derivation failure leaves the grant pending like the device door")
	require.Equal(t, codersdk.AIDeviceGrantStatusPending, settled.status)
	require.False(t, persisted, "nothing persists without a derived account id")
}

func TestAIBrowserGrantPersistRetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := uuid.New()
	provider := testChatGPTProvider()
	fake := &fakeAIBrowserExchanger{exchangeGrant: testBrowserTokenGrant(t, "acct-browser-123")}
	manager, _ := testBrowserManager(t, fake)
	calls := 0
	manager.SetPersistAuthorized(func(_ context.Context, _, _ uuid.UUID, _ AIDeviceOAuthCredential) error {
		calls++
		if calls == 1 {
			return xerrors.New("test persist failure")
		}
		return nil
	})

	grant, err := manager.initiate(ctx, owner, provider.ID, provider)
	require.NoError(t, err)

	pending, err := manager.exchange(ctx, grant.id, owner, "test-auth-code")
	require.NoError(t, err, "persist failure leaves the grant pending with the triple held")
	require.Equal(t, codersdk.AIDeviceGrantStatusPending, pending.status)

	settled, err := manager.exchange(ctx, grant.id, owner, "test-auth-code")
	require.NoError(t, err)
	require.Equal(t, codersdk.AIDeviceGrantStatusAuthorized, settled.status)
	require.Equal(t, 1, fake.exchanges, "the held triple retries the persist without re-exchanging")
	require.Equal(t, 2, calls)
}

func TestAIBrowserGrantExpiryCancelIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := uuid.New()
	other := uuid.New()
	provider := testChatGPTProvider()
	fake := &fakeAIBrowserExchanger{exchangeGrant: testBrowserTokenGrant(t, "acct-browser-123")}
	manager, clock := testBrowserManager(t, fake)
	manager.SetPersistAuthorized(func(_ context.Context, _, _ uuid.UUID, _ AIDeviceOAuthCredential) error {
		return nil
	})

	grant, err := manager.initiate(ctx, owner, provider.ID, provider)
	require.NoError(t, err)

	_, err = manager.exchange(ctx, grant.id, other, "test-auth-code")
	require.ErrorIs(t, err, errAIBrowserGrantNotFound)
	require.ErrorIs(t, manager.cancel(grant.id, other), errAIBrowserGrantNotFound)

	clock.Advance(aiBrowserGrantTimeoutSeconds*time.Second + time.Minute)
	expired, err := manager.exchange(ctx, grant.id, owner, "test-auth-code")
	require.NoError(t, err, "expired grants report expired instead of exchanging")
	require.Equal(t, codersdk.AIDeviceGrantStatusExpired, expired.status)
	require.Equal(t, 0, fake.exchanges)

	require.NoError(t, manager.cancel(grant.id, owner))
	canceled, err := manager.peek(grant.id, owner)
	require.NoError(t, err)
	require.Equal(t, codersdk.AIDeviceGrantStatusExpired, canceled.status, "cancel past expiry keeps the terminal state")

	fresh, err := manager.initiate(ctx, owner, provider.ID, provider)
	require.NoError(t, err)
	require.NoError(t, manager.cancel(fresh.id, owner))
	canceled, err = manager.peek(fresh.id, owner)
	require.NoError(t, err)
	require.Equal(t, codersdk.AIDeviceGrantStatusCanceled, canceled.status)
}

func TestAIBrowserGrantUnsupportedProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager, _ := testBrowserManager(t, &fakeAIBrowserExchanger{})
	plain := database.AIProvider{ID: uuid.New(), Type: database.AIProviderTypeOpenai, Name: "plain-api-keys", Enabled: true}
	_, err := manager.initiate(ctx, uuid.New(), plain.ID, plain)
	require.ErrorContains(t, err, "not supported for this provider")

	require.True(t, browserFlowSupportedForProvider(database.AIProvider{Type: database.AIProviderTypeOpenai, Name: "chatgpt", Enabled: true}))
	require.False(t, browserFlowSupportedForProvider(database.AIProvider{Type: database.AIProviderTypeOpenai, Name: "chatgpt"}))
	require.False(t, browserFlowSupportedForProvider(database.AIProvider{Type: database.AIProviderTypeOpenai, Name: "other", Enabled: true}))
	require.False(t, browserFlowSupportedForProvider(database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "claude", Enabled: true}))
}

// TestHTTPBrowserExchangeShape pins the provider exchange wire format:
// form fields, redirect URI, and the accepted response triple. It runs
// against a local stub so no live provider is involved.
func TestHTTPBrowserExchangeShape(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotForm = r.PostForm
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"access_token":  testDeviceAccessJWT(t, "acct-wire-test"),
			"refresh_token": "test-wire-refresh", // #nosec G101 -- test fixture, not a credential.
			"expires_in":    7200,
		})
	}))
	t.Cleanup(server.Close)

	config, ok := aiDeviceGrantConfigForProvider(database.AIProviderTypeOpenai, "chatgpt")
	require.True(t, ok)
	exchanger := &httpAIBrowserGrantExchanger{
		config:     config,
		httpClient: server.Client(),
	}
	// Point the stub config at the local server without touching the
	// shared provider table.
	exchanger.config.tokenURL = server.URL

	grant, err := exchanger.ExchangeAuthorizationCode(ctx, "wire-code", "wire-verifier")
	require.NoError(t, err)
	require.Equal(t, "authorization_code", gotForm.Get("grant_type"))
	require.Equal(t, "app_EMoamEEZ73f0CkXaXp7hrann", gotForm.Get("client_id"))
	require.Equal(t, "wire-code", gotForm.Get("code"))
	require.Equal(t, "wire-verifier", gotForm.Get("code_verifier"))
	require.Equal(t, "http://localhost:1455/auth/callback", gotForm.Get("redirect_uri"))
	require.Equal(t, testDeviceAccessJWT(t, "acct-wire-test"), grant.AccessToken)
	require.Equal(t, "test-wire-refresh", grant.RefreshToken)
	require.Equal(t, 7200, grant.ExpiresIn)
}
