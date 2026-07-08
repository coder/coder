package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	anthropiccfg "github.com/anthropics/anthropic-sdk-go/config"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/quartz"
)

// anthropicBetaHeaderName is the header carrying Anthropic beta flags.
const anthropicBetaHeaderName = "anthropic-beta"

// wifTokenRefreshMargin is how long before the cached federation token's
// expiry a fresh exchange is performed. Refreshing early keeps passthrough
// requests from racing the token's expiry in flight.
const wifTokenRefreshMargin = time.Minute

// wifTokenSource exchanges an OIDC identity token for a short-lived
// Anthropic access token and caches it until it approaches expiry. The
// bridged /v1/messages path gets the same behavior from the SDK's auth
// middleware; this source exists for the passthrough reverse proxy, which
// performs raw HTTP requests outside the SDK request pipeline.
type wifTokenSource struct {
	cfg     config.AnthropicWIF
	baseURL string
	clock   quartz.Clock

	mu    sync.Mutex
	token string
	// expiresAt is zero when the server reported no expiry; the token is
	// then cached until a 401 invalidates it.
	expiresAt time.Time
}

func newWIFTokenSource(cfg config.AnthropicWIF, baseURL string) *wifTokenSource {
	return &wifTokenSource{cfg: cfg, baseURL: baseURL, clock: quartz.NewReal()}
}

// Token returns a valid access token, re-exchanging the identity token when
// no token is cached or the cached one is within wifTokenRefreshMargin of
// expiry. The lock is held across the exchange so concurrent requests do
// not stampede the token endpoint.
func (s *wifTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && (s.expiresAt.IsZero() || s.expiresAt.Sub(s.clock.Now()) > wifTokenRefreshMargin) {
		return s.token, nil
	}
	jwt, err := s.cfg.IdentityToken(ctx)
	if err != nil {
		return "", xerrors.Errorf("get WIF identity token: %w", err)
	}
	creds, err := anthropiccfg.ExchangeFederationAssertion(ctx, anthropiccfg.FederationExchangeParams{
		Assertion:        jwt,
		FederationRuleID: s.cfg.FederationRuleID,
		OrganizationID:   s.cfg.OrganizationID,
		ServiceAccountID: s.cfg.ServiceAccountID,
		WorkspaceID:      s.cfg.WorkspaceID,
		BaseURL:          s.baseURL,
		UserAgent:        "coder-aibridge",
	})
	if err != nil {
		return "", xerrors.Errorf("exchange WIF identity token: %w", err)
	}
	s.token = creds.AccessToken
	s.expiresAt = time.Time{}
	if creds.ExpiresAt != nil {
		s.expiresAt = *creds.ExpiresAt
	}
	return s.token, nil
}

// invalidate drops the cached token if it is still the one the caller used,
// forcing the next Token call to re-exchange. The comparison prevents a
// stale 401 from discarding a token another request already refreshed.
func (s *wifTokenSource) invalidate(used string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == used {
		s.token = ""
		s.expiresAt = time.Time{}
	}
}

// wifPassthroughTransport injects a federation access token into
// centralized passthrough requests. Requests that already carry upstream
// credentials pass through unchanged: BYOK requests arrive with their own
// headers, and when the provider also has a key pool the enclosing
// key-failover transport injects a key before this transport runs.
type wifPassthroughTransport struct {
	inner  http.RoundTripper
	source *wifTokenSource
}

func (t *wifPassthroughTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get(intercept.AuthHeaderXAPIKey) != "" || req.Header.Get(intercept.AuthHeaderAuthorization) != "" {
		return t.inner.RoundTrip(req)
	}

	// Buffer the body so a single 401 retry can replay it. Passthrough
	// bodies are bounded by the bridge's request size limit.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, xerrors.Errorf("buffer passthrough request body: %w", err)
		}
	}

	// Clone per attempt so the caller's request is never mutated.
	attempt := func(token string) (*http.Response, error) {
		out := req.Clone(req.Context())
		if body != nil {
			out.Body = io.NopCloser(bytes.NewReader(body))
		}
		out.Header.Set(intercept.AuthHeaderAuthorization, "Bearer "+token)
		// Bearer-token API access requires the oauth beta flag; append it
		// without clobbering any flags the client already sent.
		switch existing := out.Header.Get(anthropicBetaHeaderName); {
		case existing == "":
			out.Header.Set(anthropicBetaHeaderName, anthropiccfg.OAuthAPIBetaHeader)
		case !strings.Contains(existing, anthropiccfg.OAuthAPIBetaHeader):
			out.Header.Set(anthropicBetaHeaderName, existing+","+anthropiccfg.OAuthAPIBetaHeader)
		}
		return t.inner.RoundTrip(out)
	}

	token, err := t.source.Token(req.Context())
	if err != nil {
		return nil, xerrors.Errorf("resolve WIF federation token: %w", err)
	}
	resp, err := attempt(token)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	// The cached token was rejected (revoked, or expired server-side).
	// Re-exchange once and retry; if the exchange yields the same token,
	// surface the original 401 rather than repeating a doomed request.
	t.source.invalidate(token)
	fresh, tokenErr := t.source.Token(req.Context())
	if tokenErr != nil || fresh == token {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return attempt(fresh)
}
