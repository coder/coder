package aibridgeproxyd_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

// providerStore is a mutable [aibridgeproxyd.RefreshProvidersFunc]
// backing for integration tests. set / setErr mutate the snapshot
// returned by the next Reload, mimicking CRUD against the database.
type providerStore struct {
	mu        sync.Mutex
	providers []aibridgeproxyd.ProviderRoute
	err       error
}

func (s *providerStore) set(providers []aibridgeproxyd.ProviderRoute) {
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

func (s *providerStore) refresh(context.Context) ([]aibridgeproxyd.ProviderRoute, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	// Return a copy so callers can't mutate our internal snapshot.
	out := make([]aibridgeproxyd.ProviderRoute, len(s.providers))
	copy(out, s.providers)
	return out, nil
}

// newReloadTestHarness boots a proxy with an empty boot allowlist and a
// store-backed RefreshProviders. Production wiring is identical: the
// daemon constructs the proxy without a static allowlist and lets
// Reload populate the router from the database.
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
	srv := newTestProxy(t,
		withCoderAccessURL(bridged.URL),
		withAllowedPorts("443"),
		// Empty boot allowlist: the router must be populated by Reload,
		// matching the production daemon's behavior.
		withDomainAllowlist(),
		withAIBridgeProviderFromHost(nil),
		withRefreshProviders(store.refresh),
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
// to aibridged with the expected /api/v2/aibridge/<name>/<path>.
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

// TestProxy_HotReloadRoutingCRUD drives the proxy through a CRUD-style
// sequence of provider changes and asserts on routing after each
// Reload via real HTTPS requests. Each sub-test mutates the store and
// validates that:
//   - newly created providers are MITM'd to aibridged with the right
//     /api/v2/aibridge/<name>/<path>
//   - renamed providers route under the new name
//   - providers whose BaseURL host changes route the new host and stop
//     MITM'ing the old host
//   - deleted providers stop being MITM'd; aibridged sees nothing
//
// Hostnames are .invalid (RFC 2606) so a request that escapes the MITM
// path fails fast via DNS rather than reaching a real upstream.
func TestProxy_HotReloadRoutingCRUD(t *testing.T) {
	t.Parallel()

	h := newReloadTestHarness(t)

	// InitialEmptyRouter: no Reload has been called and the boot
	// allowlist is empty, so any host falls through to the tunneled
	// middleware.
	h.expectNotRouted(t, "https://alpha.invalid/v1/messages")

	// CreateProvider.
	h.store.set([]aibridgeproxyd.ProviderRoute{
		{Name: "alpha", BaseURL: "https://alpha.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/aibridge/alpha/v1/messages")

	// UpdateProviderName: the same BaseURL with a new name must route
	// under the new name on the next Reload.
	h.store.set([]aibridgeproxyd.ProviderRoute{
		{Name: "alpha-v2", BaseURL: "https://alpha.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/aibridge/alpha-v2/v1/messages")

	// UpdateProviderBaseURLHost: moving the provider to a new host must
	// start MITM'ing the new host and stop MITM'ing the old one.
	h.store.set([]aibridgeproxyd.ProviderRoute{
		{Name: "alpha-v2", BaseURL: "https://alpha-new.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha-new.invalid/v1/messages", "/api/v2/aibridge/alpha-v2/v1/messages")
	h.expectNotRouted(t, "https://alpha.invalid/v1/messages")

	// AddSecondProvider: a second provider added in the same Reload must
	// route independently from the first.
	h.store.set([]aibridgeproxyd.ProviderRoute{
		{Name: "alpha-v2", BaseURL: "https://alpha-new.invalid/v1"},
		{Name: "beta", BaseURL: "https://beta.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha-new.invalid/v1/messages", "/api/v2/aibridge/alpha-v2/v1/messages")
	h.expectRoutedTo(t, "https://beta.invalid/v1/chat/completions", "/api/v2/aibridge/beta/v1/chat/completions")

	// DeleteOneProvider: removing alpha must keep beta routed and stop
	// routing alpha.
	h.store.set([]aibridgeproxyd.ProviderRoute{
		{Name: "beta", BaseURL: "https://beta.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://beta.invalid/v1/chat/completions", "/api/v2/aibridge/beta/v1/chat/completions")
	h.expectNotRouted(t, "https://alpha-new.invalid/v1/messages")

	// DeleteAllProviders: an empty Reload must collapse the router to
	// the fail-closed state with no host MITM'd.
	h.store.set(nil)
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectNotRouted(t, "https://beta.invalid/v1/chat/completions")
	h.expectNotRouted(t, "https://alpha-new.invalid/v1/messages")

	// RecreateAfterDelete: reintroducing a previously-deleted provider
	// must route again without restart, confirming the swap is
	// symmetric.
	h.store.set([]aibridgeproxyd.ProviderRoute{
		{Name: "alpha", BaseURL: "https://alpha.invalid/v1"},
	})
	require.NoError(t, h.srv.Reload(t.Context()))
	h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/aibridge/alpha/v1/messages")
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
		// entry must be silently dropped; the valid one must still
		// route.
		h.store.set([]aibridgeproxyd.ProviderRoute{
			{Name: "no-url"},
			{Name: "valid", BaseURL: "https://valid.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectRoutedTo(t, "https://valid.invalid/v1/messages", "/api/v2/aibridge/valid/v1/messages")
	})

	t.Run("MalformedBaseURLSkipped", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// A BaseURL that fails url.Parse and one whose Hostname() is
		// empty must both be dropped. Mixed with a valid entry, only
		// the valid one routes.
		h.store.set([]aibridgeproxyd.ProviderRoute{
			{Name: "malformed", BaseURL: "://not-a-url"},
			{Name: "no-host", BaseURL: "https://"},
			{Name: "valid", BaseURL: "https://valid.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectRoutedTo(t, "https://valid.invalid/v1/messages", "/api/v2/aibridge/valid/v1/messages")
	})

	t.Run("DuplicateHostFirstWins", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// Two providers with the same BaseURL host: the first one wins,
		// matching buildProviderRouter's documented contract.
		h.store.set([]aibridgeproxyd.ProviderRoute{
			{Name: "first", BaseURL: "https://shared.invalid/v1"},
			{Name: "second", BaseURL: "https://shared.invalid/v2"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectRoutedTo(t, "https://shared.invalid/v1/messages", "/api/v2/aibridge/first/v1/messages")
	})

	t.Run("AllInvalidYieldsEmptyRouter", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// When every provider is invalid, the router contains no
		// entries and the proxy fails closed: no host is MITM'd.
		h.store.set([]aibridgeproxyd.ProviderRoute{
			{Name: "no-url"},
			{Name: "malformed", BaseURL: "://not-a-url"},
			{Name: "no-host", BaseURL: "https://"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))

		h.expectNotRouted(t, "https://anything.invalid/v1/messages")
	})

	t.Run("RefreshErrorPreservesPreviousSnapshot", func(t *testing.T) {
		t.Parallel()

		h := newReloadTestHarness(t)
		// Seed a valid snapshot so we have something to preserve.
		h.store.set([]aibridgeproxyd.ProviderRoute{
			{Name: "alpha", BaseURL: "https://alpha.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))
		h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/aibridge/alpha/v1/messages")

		// A refresh error must NOT clear the router: dropping the
		// allowlist on every transient DB hiccup would amplify the
		// fault into a denial of service.
		h.store.setErr(xerrors.New("simulated db failure"))
		err := h.srv.Reload(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refresh ai providers for proxy routing")
		h.expectRoutedTo(t, "https://alpha.invalid/v1/messages", "/api/v2/aibridge/alpha/v1/messages")

		// Recovery: once the store returns providers again, the next
		// Reload applies the new snapshot.
		h.store.set([]aibridgeproxyd.ProviderRoute{
			{Name: "beta", BaseURL: "https://beta.invalid/v1"},
		})
		require.NoError(t, h.srv.Reload(t.Context()))
		h.expectRoutedTo(t, "https://beta.invalid/v1/messages", "/api/v2/aibridge/beta/v1/messages")
		h.expectNotRouted(t, "https://alpha.invalid/v1/messages")
	})
}
