package aibridgeproxyd_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
	"github.com/coder/coder/v2/testutil"
)

// reloadTestHarness wires a real proxy server to a mutable provider
// store and a mock aibridged backend so tests can drive Reload through
// a CRUD-style sequence and observe routing via real proxy requests.
type reloadTestHarness struct {
	srv      *aibridgeproxyd.Server
	store    *providerStore
	client   *http.Client
	bridged  *httptest.Server
	recorder *aibridgedRecorder
	metrics  *aibridgeproxyd.Metrics
}

// aibridgedRecorder captures the path of the last request received by
// the mock aibridged backend. Access is mutex-guarded so the test
// goroutine and the proxy's response goroutine can read/write safely.
type aibridgedRecorder struct {
	mu   sync.Mutex
	path string
}

func (r *aibridgedRecorder) record(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = path
}

func (r *aibridgedRecorder) load() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.path
}

func (r *aibridgedRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = ""
}

// rawProvider is a (name, base URL) pair representing what the database
// holds before classification, mirroring the ai_providers row shape
// that the production refresh function classifies.
type rawProvider struct {
	name    string
	baseURL string
}

// providerStore is a mutable RefreshProvidersFunc backing for
// integration tests. set / setErr mutate the snapshot returned by the
// next Reload, mimicking CRUD against the database.
type providerStore struct {
	mu        sync.Mutex
	providers []rawProvider
	err       error
}

func (s *providerStore) set(providers []rawProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = providers
	s.err = nil
}

func (s *providerStore) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *providerStore) refresh(context.Context) (aibridgeproxyd.ProviderReload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return aibridgeproxyd.ProviderReload{}, s.err
	}
	providers := slices.Clone(s.providers)
	reload := aibridgeproxyd.ProviderReload{
		Providers: make([]aibridgeproxyd.ReloadedProvider, 0, len(providers)),
	}
	seenHost := make(map[string]string, len(providers))
	for _, p := range providers {
		reload.Providers = append(reload.Providers, classifyRaw(p, seenHost))
	}
	return reload, nil
}

// classifyRaw mirrors the production classifier in enterprise/cli so
// the reload tests exercise the same validation rules end-to-end.
func classifyRaw(p rawProvider, seenHost map[string]string) aibridgeproxyd.ReloadedProvider {
	out := aibridgeproxyd.ReloadedProvider{
		ProviderOutcome: aibridged.ProviderOutcome{Name: p.name, Type: "openai"},
	}
	if strings.TrimSpace(p.baseURL) == "" {
		out.Status = aibridged.ProviderStatusError
		out.Err = xerrors.New("base url is empty")
		return out
	}
	u, err := url.Parse(p.baseURL)
	if err != nil {
		out.Status = aibridged.ProviderStatusError
		out.Err = xerrors.Errorf("invalid base url %q: %w", p.baseURL, err)
		return out
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		out.Status = aibridged.ProviderStatusError
		out.Err = xerrors.Errorf("base url %q has no hostname", p.baseURL)
		return out
	}
	if claimedBy, taken := seenHost[host]; taken {
		out.Status = aibridged.ProviderStatusError
		out.Err = xerrors.Errorf("hostname %q already claimed by provider %q", host, claimedBy)
		return out
	}
	seenHost[host] = p.name
	out.Host = host
	out.Status = aibridged.ProviderStatusEnabled
	return out
}

// newReloadTestHarness boots a proxy with an empty initial router and
// a store-backed RefreshProviders. Production wiring is identical: the
// daemon constructs the proxy without preconfigured provider hosts and
// lets Reload populate the router from the database.
func newReloadTestHarness(t *testing.T) *reloadTestHarness {
	t.Helper()

	recorder := &aibridgedRecorder{}
	bridged := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("aibridged"))
	}))
	t.Cleanup(bridged.Close)

	store := &providerStore{}
	metrics := aibridgeproxyd.NewMetrics(prometheus.NewRegistry())
	srv := newTestProxy(t,
		withCoderAccessURL(bridged.URL),
		withAllowedPorts("443"),
		withRefreshProviders(store.refresh),
		withMetrics(metrics),
	)

	certPool := getProxyCertPool(t)
	client := newProxyClient(t, srv, makeProxyAuthHeader("coder-token"), certPool, false)
	// Disable keep-alives so each request opens a fresh CONNECT through
	// the proxy. Per the Reload contract, already-MITM'd tunnels keep
	// the provider name they captured at CONNECT time; only new
	// connections see the post-Reload snapshot. Tests need a fresh
	// CONNECT between phases to assert on the new routing.
	client.Transport.(*http.Transport).DisableKeepAlives = true

	return &reloadTestHarness{
		srv:      srv,
		store:    store,
		metrics:  metrics,
		client:   client,
		bridged:  bridged,
		recorder: recorder,
	}
}

// requestResult is the outcome of sending a request through the proxy.
// Either err is set (CONNECT failed for a non-MITM'd host whose dial
// fell through to the tunneled path and could not be resolved) or
// status/body carry the MITM'd response from the mock aibridged.
type requestResult struct {
	status int
	body   string
	err    error
}

// sendRequest issues a single POST through the proxy. It returns rather
// than asserting so callers can branch on whether the host is currently
// routed (MITM'd to aibridged) or not (tunneled, dial of an unresolvable
// host fails).
func (h *reloadTestHarness) sendRequest(t *testing.T, targetURL string) requestResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(`{}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return requestResult{err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return requestResult{status: resp.StatusCode, body: string(body)}
}

// expectRoutedTo asserts the proxy MITM'd the request and forwarded it
// to aibridged with the expected /api/v2/ai-gateway/<name>/<path>.
func (h *reloadTestHarness) expectRoutedTo(t *testing.T, targetURL, expectedPath string) {
	t.Helper()

	h.recorder.reset()
	res := h.sendRequest(t, targetURL)
	require.NoError(t, res.err, "request to routed host must succeed")
	require.Equal(t, http.StatusOK, res.status)
	require.Equal(t, "aibridged", res.body)
	require.Equal(t, expectedPath, h.recorder.load(),
		"aibridged must observe the rewritten path for %s", targetURL)
}

// expectNotRouted asserts the proxy did not MITM the request for the
// given host. The CONNECT either falls through to the tunneled path
// (where the .invalid hostname fails to dial) or to a 502 from the
// proxy. Either way, aibridged never sees the request.
func (h *reloadTestHarness) expectNotRouted(t *testing.T, targetURL string) {
	t.Helper()

	h.recorder.reset()
	_ = h.sendRequest(t, targetURL)
	require.Empty(t, h.recorder.load(),
		"aibridged must not be reached for non-routed host %s", targetURL)
}

// expectProviderStatus asserts the provider_info series for (name,
// status) is present with value 1.
func (h *reloadTestHarness) expectProviderStatus(t *testing.T, name, status string) {
	t.Helper()
	assert.Equal(t, 1.0, promtest.ToFloat64(h.metrics.ProviderInfo.WithLabelValues(name, "openai", status)),
		"expected provider_info{provider_name=%q, status=%q} == 1", name, status)
}

// expectProviderAbsent asserts no series exists for the provider name
// in any status. This verifies the GaugeVec.Reset on each reload
// clears stale entries.
func (h *reloadTestHarness) expectProviderAbsent(t *testing.T, name string) {
	t.Helper()
	for _, status := range []string{"enabled", "disabled", "error"} {
		assert.Equal(t, 0.0, promtest.ToFloat64(h.metrics.ProviderInfo.WithLabelValues(name, "openai", status)),
			"expected no provider_info series for %q, found status %q", name, status)
	}
}

// TestProxy_StaleTunnelStopsRoutingAfterProviderChange is the
// regression test for a bug where a long-lived CONNECT tunnel that was
// established while a provider was enabled kept routing decrypted
// requests to aibridged after the provider was disabled or renamed. The
// fix re-validates the CONNECT-time provider against the live router on
// every decrypted request and covers both shapes of stale mapping:
//
//   - ProviderDisabled: liveProvider == "" (host no longer MITM'd).
//   - ProviderRenamed: liveProvider != reqCtx.Provider (host MITM'd, but
//     under a new provider name).
func TestProxy_StaleTunnelStopsRoutingAfterProviderChange(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// applyChange mutates the store to simulate the provider change
		// after the initial routed request succeeds.
		applyChange func(*providerStore)
		// changeDescription is appended to the second-request assertion
		// message so a failure points at the exercised branch.
		changeDescription string
	}{
		{
			name:              "ProviderDisabled",
			applyChange:       func(s *providerStore) { s.set(nil) },
			changeDescription: "after alpha was disabled",
		},
		{
			name: "ProviderRenamed",
			applyChange: func(s *providerStore) {
				// Same host, new provider name: the live router still
				// MITMs alpha.invalid, but as "alpha-v2". The stale
				// CONNECT-time name "alpha" no longer matches.
				s.set([]rawProvider{
					{name: "alpha-v2", baseURL: "https://alpha.invalid/v1"},
				})
			},
			changeDescription: "after alpha was renamed to alpha-v2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := &aibridgedRecorder{}
			bridged := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorder.record(r.URL.Path)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("aibridged"))
			}))
			t.Cleanup(bridged.Close)

			store := &providerStore{}
			store.set([]rawProvider{
				{name: "alpha", baseURL: "https://alpha.invalid/v1"},
			})

			// newTestProxy seeds the router from the store via the
			// initial Reload, so the first CONNECT is MITM'd as alpha.
			srv := newTestProxy(t,
				withCoderAccessURL(bridged.URL),
				withAllowedPorts("443"),
				withRefreshProviders(store.refresh),
			)

			certPool := getProxyCertPool(t)
			client := newProxyClient(t, srv, makeProxyAuthHeader("coder-token"), certPool, false)
			// Keep-alives are required: the regression exists only when a
			// subsequent request reuses the original CONNECT tunnel. A fresh
			// CONNECT would correctly observe the post-reload router.
			transport := client.Transport.(*http.Transport)
			transport.DisableKeepAlives = false
			transport.MaxConnsPerHost = 1
			transport.MaxIdleConnsPerHost = 1

			sendThroughTunnel := func(path string) (status int, err error) {
				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
				defer cancel()
				req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, "https://alpha.invalid"+path, strings.NewReader(`{}`))
				require.NoError(t, reqErr)
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					return 0, err
				}
				defer resp.Body.Close()
				_, _ = io.Copy(io.Discard, resp.Body)
				return resp.StatusCode, nil
			}

			// First request: alpha is enabled, the proxy MITMs and routes to
			// aibridged under the alpha namespace.
			recorder.reset()
			status, err := sendThroughTunnel("/v1/messages")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "/api/v2/ai-gateway/alpha/v1/messages", recorder.load(),
				"first request must be routed to aibridged while alpha is enabled")

			// Apply the provider change and reload. The atomic router swap
			// takes effect immediately, but the client's connection (and
			// the proxy's hijacked tunnel) remain open.
			tc.applyChange(store)
			require.NoError(t, srv.Reload(t.Context()))

			// Second request on the same tunnel: aibridged must NOT see it.
			// The connection is hijacked so the request reaches the proxy's
			// handleRequest with the stale CONNECT-time provider; the fix
			// re-validates against the live router and passes through to
			// the original upstream (alpha.invalid, which fails DNS).
			recorder.reset()
			_, _ = sendThroughTunnel("/v1/should-not-route")
			require.Empty(t, recorder.load(),
				"%s, aibridged must not receive the request even on a reused tunnel", tc.changeDescription)
		})
	}
}

// TestProxy_HotReloadRoutingCRUD drives the proxy through a CRUD-style
// sequence of provider changes and asserts on routing after each
// Reload via real HTTPS requests.
//
// Hostnames are .invalid (RFC 2606) so a request that escapes the MITM
// path fails fast via DNS rather than reaching a real upstream.
func TestProxy_HotReloadRoutingCRUD(t *testing.T) {
	t.Parallel()

	h := newReloadTestHarness(t)

	// InitialEmptyRouter: no Reload has been called and no provider
	// hosts are configured, so any host falls through to the tunneled
	// middleware.
	h.expectNotRouted(t, "https://alpha.invalid/v1/messages")

	// CreateProvider.
	h.store.set([]rawProvider{
		{name: "alpha", baseURL: "https://alpha.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/ai-gateway/alpha/v1/messages")
	h.expectProviderStatus(t, "alpha", "enabled")

	// UpdateProviderName: the same BaseURL with a new name must route
	// under the new name on the next Reload. The renamed provider must
	// not leave a stale alpha series behind.
	h.store.set([]rawProvider{
		{name: "alpha-v2", baseURL: "https://alpha.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/ai-gateway/alpha-v2/v1/messages")
	h.expectProviderStatus(t, "alpha-v2", "enabled")
	h.expectProviderAbsent(t, "alpha")

	// UpdateProviderBaseURLHost: moving the provider to a new host must
	// start MITM'ing the new host and stop MITM'ing the old one.
	h.store.set([]rawProvider{
		{name: "alpha-v2", baseURL: "https://alpha-new.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha-new.invalid/v1/messages", "/api/v2/ai-gateway/alpha-v2/v1/messages")
	h.expectNotRouted(t, "https://alpha.invalid/v1/messages")
	h.expectProviderStatus(t, "alpha-v2", "enabled")

	// AddSecondProvider: a second provider added in the same Reload must
	// route independently from the first.
	h.store.set([]rawProvider{
		{name: "alpha-v2", baseURL: "https://alpha-new.invalid/v1"},
		{name: "beta", baseURL: "https://beta.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha-new.invalid/v1/messages", "/api/v2/ai-gateway/alpha-v2/v1/messages")
	h.expectRoutedTo(t, "https://beta.invalid/v1/chat/completions", "/api/v2/ai-gateway/beta/v1/chat/completions")
	h.expectProviderStatus(t, "alpha-v2", "enabled")
	h.expectProviderStatus(t, "beta", "enabled")

	// DeleteOneProvider: removing alpha must keep beta routed and stop
	// routing alpha. The deleted name disappears from provider_info.
	h.store.set([]rawProvider{
		{name: "beta", baseURL: "https://beta.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://beta.invalid/v1/chat/completions", "/api/v2/ai-gateway/beta/v1/chat/completions")
	h.expectNotRouted(t, "https://alpha-new.invalid/v1/messages")
	h.expectProviderStatus(t, "beta", "enabled")
	h.expectProviderAbsent(t, "alpha-v2")

	// DeleteAllProviders: an empty Reload must collapse the router to
	// the fail-closed state with no host MITM'd.
	h.store.set(nil)
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectNotRouted(t, "https://beta.invalid/v1/chat/completions")
	h.expectNotRouted(t, "https://alpha-new.invalid/v1/messages")
	h.expectProviderAbsent(t, "beta")

	// RecreateAfterDelete: reintroducing a previously-deleted provider
	// must route again without restart, confirming the swap is
	// symmetric.
	h.store.set([]rawProvider{
		{name: "alpha", baseURL: "https://alpha.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/ai-gateway/alpha/v1/messages")
	h.expectProviderStatus(t, "alpha", "enabled")

	// Both timestamp gauges must have advanced through this sequence.
	assert.Positive(t, promtest.ToFloat64(h.metrics.ProvidersLastReloadTimestampSeconds))
	assert.Positive(t, promtest.ToFloat64(h.metrics.ProvidersLastReloadSuccessTimestampSeconds))
}

// TestProxy_HotReloadRoutingInvalidProviders covers the resilience
// requirements stated in the [aibridgeproxyd.Server.Reload] contract:
// individual invalid provider entries do not poison the snapshot, and
// a refresh-level error does not collapse the previous snapshot to
// empty.
func TestProxy_HotReloadRoutingInvalidProviders(t *testing.T) {
	t.Parallel()

	t.Run("EmptyBaseURLSkipped", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// One valid provider and one with an empty BaseURL. The empty
		// entry must be classified as error and excluded from routing;
		// the valid one must still route.
		h.store.set([]rawProvider{
			{name: "no-url"},
			{name: "valid", baseURL: "https://valid.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectRoutedTo(t, "https://valid.invalid/v1/messages", "/api/v2/ai-gateway/valid/v1/messages")
		h.expectProviderStatus(t, "no-url", "error")
		h.expectProviderStatus(t, "valid", "enabled")
	})

	t.Run("MalformedBaseURLSkipped", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// A BaseURL that fails url.Parse and one whose Hostname() is
		// empty must both be classified as error. Mixed with a valid
		// entry, only the valid one routes.
		h.store.set([]rawProvider{
			{name: "malformed", baseURL: "://not-a-url"},
			{name: "no-host", baseURL: "https://"},
			{name: "valid", baseURL: "https://valid.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectRoutedTo(t, "https://valid.invalid/v1/messages", "/api/v2/ai-gateway/valid/v1/messages")
		h.expectProviderStatus(t, "malformed", "error")
		h.expectProviderStatus(t, "no-host", "error")
		h.expectProviderStatus(t, "valid", "enabled")
	})

	t.Run("DuplicateHostFirstWins", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// Two providers with the same BaseURL host: the second is
		// classified as error and excluded; the first routes.
		h.store.set([]rawProvider{
			{name: "first", baseURL: "https://shared.invalid/v1"},
			{name: "second", baseURL: "https://shared.invalid/v2"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectRoutedTo(t, "https://shared.invalid/v1/messages", "/api/v2/ai-gateway/first/v1/messages")
		h.expectProviderStatus(t, "first", "enabled")
		h.expectProviderStatus(t, "second", "error")
	})

	t.Run("AllInvalidYieldsEmptyRouter", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// When every provider is invalid, the router contains no
		// entries and the proxy fails closed: no host is MITM'd.
		h.store.set([]rawProvider{
			{name: "no-url"},
			{name: "malformed", baseURL: "://not-a-url"},
			{name: "no-host", baseURL: "https://"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectNotRouted(t, "https://anything.invalid/v1/messages")
	})

	t.Run("RefreshErrorPreservesPreviousSnapshot", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// Seed a valid snapshot so we have something to preserve.
		h.store.set([]rawProvider{
			{name: "alpha", baseURL: "https://alpha.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))
		h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/ai-gateway/alpha/v1/messages")

		// A refresh error must NOT clear the router: dropping the
		// provider host set on every transient DB hiccup would
		// amplify the fault into a denial of service.
		h.store.setErr(xerrors.New("simulated db failure"))
		err := h.srv.Reload(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refresh ai providers for proxy routing")
		h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/ai-gateway/alpha/v1/messages")

		// Recovery: once the store returns providers again, the next
		// Reload applies the new snapshot.
		h.store.set([]rawProvider{
			{name: "beta", baseURL: "https://beta.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))
		h.expectRoutedTo(t, "https://beta.invalid/v1/messages", "/api/v2/ai-gateway/beta/v1/messages")
		h.expectNotRouted(t, "https://alpha.invalid/v1/messages")
	})
}
