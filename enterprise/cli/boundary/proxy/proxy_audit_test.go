//nolint:paralleltest,testpackage,revive,gocritic,noctx,bodyclose
package proxy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/cli/boundary/audit"
)

// capturingAuditor captures all audit requests for test verification.
type capturingAuditor struct {
	mu       sync.Mutex
	requests []audit.Request
}

func (c *capturingAuditor) AuditRequest(req audit.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
}

func (c *capturingAuditor) getRequests() []audit.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]audit.Request{}, c.requests...)
}

func TestAuditURLIsFullyFormed_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	auditor := &capturingAuditor{}

	pt := NewProxyTest(t,
		WithCertManager(t.TempDir()),
		WithAllowedRule("domain="+serverURL.Hostname()+" path=/allowed/*"),
		WithAuditor(auditor),
	).Start()
	defer pt.Stop()

	t.Run("allowed", func(t *testing.T) {
		resp, err := pt.proxyClient.Get(server.URL + "/allowed/path?q=1")
		require.NoError(t, err)
		defer func() {
			err = resp.Body.Close()
			require.NoError(t, err)
		}()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		requests := auditor.getRequests()
		require.NotEmpty(t, requests)

		req := requests[len(requests)-1]
		require.True(t, req.Allowed)

		expectedURL := "http://" + net.JoinHostPort(serverURL.Hostname(), serverURL.Port()) + "/allowed/path?q=1"
		assert.Equal(t, expectedURL, req.URL)
	})

	t.Run("denied", func(t *testing.T) {
		resp, err := pt.proxyClient.Get(server.URL + "/denied/path")
		require.NoError(t, err)
		defer func() {
			err = resp.Body.Close()
			require.NoError(t, err)
		}()
		require.Equal(t, http.StatusForbidden, resp.StatusCode)

		requests := auditor.getRequests()
		require.NotEmpty(t, requests)

		req := requests[len(requests)-1]
		require.False(t, req.Allowed)

		expectedURL := "http://" + net.JoinHostPort(serverURL.Hostname(), serverURL.Port()) + "/denied/path"
		assert.Equal(t, expectedURL, req.URL)
	})
}

func TestAuditURLIsFullyFormed_HTTPS(t *testing.T) {
	auditor := &capturingAuditor{}

	pt := NewProxyTest(t,
		WithCertManager(t.TempDir()),
		WithAllowedDomain("dev.coder.com"),
		WithAuditor(auditor),
	).Start()
	defer pt.Stop()

	tunnel, err := pt.establishExplicitCONNECT("dev.coder.com:443")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, tunnel.close())
	}()

	t.Run("allowed", func(t *testing.T) {
		_, err := tunnel.sendRequest("dev.coder.com", "/api/v2?q=1")
		require.NoError(t, err)

		requests := auditor.getRequests()
		require.NotEmpty(t, requests)

		req := requests[len(requests)-1]
		require.True(t, req.Allowed)

		assert.Equal(t, "https://dev.coder.com/api/v2?q=1", req.URL)
	})

	t.Run("denied", func(t *testing.T) {
		err := tunnel.sendRequestAndExpectDeny("blocked.example.com", "/some/path")
		require.NoError(t, err)

		requests := auditor.getRequests()
		require.NotEmpty(t, requests)

		req := requests[len(requests)-1]
		require.False(t, req.Allowed)

		assert.Equal(t, "https://blocked.example.com/some/path", req.URL)
	})
}
