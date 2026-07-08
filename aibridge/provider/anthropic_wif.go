package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	anthropiccfg "github.com/anthropics/anthropic-sdk-go/config"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/quartz"
)

// wifTokenRefreshMargin is how long before the cached federation token's
// expiry a fresh exchange is performed. Refreshing early keeps passthrough
// requests from racing the token's expiry in flight.
const wifTokenRefreshMargin = time.Minute

// wifExchangeTimeout bounds a single token exchange, covering both the
// identity-token read and the HTTP call. The exchange runs detached from
// any single waiter's context, so this is its only deadline.
const wifExchangeTimeout = 30 * time.Second

// wifTokenSource exchanges an OIDC identity token for a short-lived
// Anthropic access token and caches it until it approaches expiry. It
// authenticates both the bridged /v1/messages path and the passthrough
// reverse proxy. The SDK's own WIF middleware is not used because it
// derives the token-exchange URL from the request's scheme and host only,
// which breaks providers configured with a path-bearing base URL (e.g.
// https://proxy.example/api); exchanges here use the full base URL.
type wifTokenSource struct {
	cfg     config.AnthropicWIF
	baseURL string
	clock   quartz.Clock
	group   singleflight.Group

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
// expiry. Concurrent refreshes collapse into one shared exchange, so a
// token-endpoint outage costs a single in-flight attempt at a time rather
// than a serial convoy, and every waiter still honors its own context.
func (s *wifTokenSource) Token(ctx context.Context) (string, error) {
	if tok, ok := s.cached(); ok {
		return tok, nil
	}
	ch := s.group.DoChan("exchange", func() (any, error) {
		// Re-check under the flight: a refresh that completed between the
		// cache miss and joining the flight already stored a fresh token.
		if tok, ok := s.cached(); ok {
			return tok, nil
		}
		// Detach from the initiating waiter's context so its cancellation
		// cannot fail the exchange for the other waiters sharing it.
		exCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), wifExchangeTimeout)
		defer cancel()
		return s.exchange(exCtx)
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return "", res.Err
		}
		tok, _ := res.Val.(string)
		return tok, nil
	}
}

// cached returns the stored token when it is present and not within the
// refresh margin of its expiry.
func (s *wifTokenSource) cached() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && (s.expiresAt.IsZero() || s.expiresAt.Sub(s.clock.Now()) > wifTokenRefreshMargin) {
		return s.token, true
	}
	return "", false
}

// exchange performs one identity-token exchange and stores the result.
func (s *wifTokenSource) exchange(ctx context.Context) (string, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
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

// wifAuthTransport injects a federation access token into centralized
// requests. Requests that already carry upstream credentials pass through
// unchanged: BYOK requests arrive with their own headers, and when the
// provider also has a key pool the key injection runs before this
// transport. It serves both the passthrough reverse proxy (wrapping the
// proxy transport) and, adapted into an SDK middleware in NewAnthropic,
// the bridged SDK request pipeline.
type wifAuthTransport struct {
	inner  http.RoundTripper
	source *wifTokenSource
}

func (t *wifAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get(intercept.AuthHeaderXAPIKey) != "" || req.Header.Get(intercept.AuthHeaderAuthorization) != "" {
		return t.inner.RoundTrip(req)
	}

	// Buffer the body so a single 401 retry can replay it. Bodies are
	// bounded by the bridge's request size limit.
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
		intercept.AppendAnthropicBeta(out.Header, anthropiccfg.OAuthAPIBetaHeader)
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// requireSecureWIFBaseURL rejects base URLs that would send the identity
// assertion and minted access tokens over cleartext HTTP. Loopback hosts
// are allowed for local development. This mirrors the check the SDK
// applies to its own token exchange; an empty base URL means the SDK
// default, https://api.anthropic.com.
func requireSecureWIFBaseURL(base string) error {
	if base == "" {
		return nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return xerrors.Errorf("invalid WIF base URL %q: %w", base, err)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return xerrors.Errorf("refusing to send WIF credentials over non-https base URL %q", base)
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(host)
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
