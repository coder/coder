package aibridgeproxyd_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
	"github.com/coder/coder/v2/testutil"
)

var (
	// testCAOnce ensures the shared CA is generated exactly once.
	// sync.Once guarantees single execution even with parallel tests.
	// Note: no retry on failure.
	testCAOnce sync.Once
	// Shared CA certificate and key paths, and any error from generation.
	// These are set once by testCAOnce and read by all tests.
	testCACert      string
	testCAKey       string
	errTestSharedCA error
)

// getSharedTestCA returns a shared CA certificate for all tests.
// This avoids race conditions with goproxy.GoproxyCa which is a global variable.
// Using sync.Once ensures the CA is generated exactly once, even when tests run
// in parallel. All tests share the same CA, so goproxy.GoproxyCa is only set once.
func getSharedTestCA(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	testCAOnce.Do(func() {
		testCACert, testCAKey, errTestSharedCA = generateSharedTestCA()
	})

	require.NoError(t, errTestSharedCA, "failed to generate shared test CA")
	return testCACert, testCAKey
}

// generateSharedTestCA creates a shared CA certificate and key for testing.
func generateSharedTestCA() (certFile, keyFile string, err error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", xerrors.Errorf("generate CA key: %w", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Shared Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return "", "", xerrors.Errorf("create CA certificate: %w", err)
	}

	tmpDir := os.TempDir()
	certPath := filepath.Join(tmpDir, "aibridgeproxyd_test_ca.crt")
	keyPath := filepath.Join(tmpDir, "aibridgeproxyd_test_ca.key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return "", "", xerrors.Errorf("write cert file: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caKey)})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", "", xerrors.Errorf("write key file: %w", err)
	}

	return certPath, keyPath, nil
}

type testProxyConfig struct {
	listenAddr      string
	coderAccessURL  string
	allowedPorts    []string
	certStore       *aibridgeproxyd.CertCache
	domainAllowlist []string
	upstreamProxy   string
	upstreamProxyCA string
}

type testProxyOption func(*testProxyConfig)

func withAllowedPorts(ports ...string) testProxyOption {
	return func(cfg *testProxyConfig) {
		cfg.allowedPorts = ports
	}
}

func withCoderAccessURL(coderAccessURL string) testProxyOption {
	return func(cfg *testProxyConfig) {
		cfg.coderAccessURL = coderAccessURL
	}
}

func withCertStore(store *aibridgeproxyd.CertCache) testProxyOption {
	return func(cfg *testProxyConfig) {
		cfg.certStore = store
	}
}

func withDomainAllowlist(domains ...string) testProxyOption {
	return func(cfg *testProxyConfig) {
		cfg.domainAllowlist = domains
	}
}

func withUpstreamProxy(upstreamProxy string) testProxyOption {
	return func(cfg *testProxyConfig) {
		cfg.upstreamProxy = upstreamProxy
	}
}

func withUpstreamProxyCA(upstreamProxyCA string) testProxyOption {
	return func(cfg *testProxyConfig) {
		cfg.upstreamProxyCA = upstreamProxyCA
	}
}

// newTestProxy creates a new AI Bridge Proxy server for testing.
// It uses the shared test CA and registers cleanup automatically.
// It waits for the proxy server to be ready before returning.
func newTestProxy(t *testing.T, opts ...testProxyOption) *aibridgeproxyd.Server {
	t.Helper()

	cfg := &testProxyConfig{
		listenAddr:      "127.0.0.1:0",
		coderAccessURL:  "http://localhost:3000",
		domainAllowlist: []string{"127.0.0.1", "localhost"},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	certFile, keyFile := getSharedTestCA(t)
	logger := slogtest.Make(t, nil)

	aibridgeOpts := aibridgeproxyd.Options{
		ListenAddr:      cfg.listenAddr,
		CoderAccessURL:  cfg.coderAccessURL,
		CertFile:        certFile,
		KeyFile:         keyFile,
		AllowedPorts:    cfg.allowedPorts,
		DomainAllowlist: cfg.domainAllowlist,
		UpstreamProxy:   cfg.upstreamProxy,
		UpstreamProxyCA: cfg.upstreamProxyCA,
	}
	if cfg.certStore != nil {
		aibridgeOpts.CertStore = cfg.certStore
	}

	srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeOpts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// Wait for the proxy server to be ready.
	proxyAddr := srv.Addr()
	require.NotEmpty(t, proxyAddr)
	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, testutil.WaitShort, testutil.IntervalFast)

	return srv
}

// getProxyCertPool returns a cert pool containing the shared test CA certificate.
// This is used for tests where requests are MITM'd by the proxy, so the client
// needs to trust the proxy's CA to verify the generated certificates.
func getProxyCertPool(t *testing.T) *x509.CertPool {
	t.Helper()

	certFile, _ := getSharedTestCA(t)

	// Load the CA certificate so the client trusts the proxy's MITM certificate.
	certPEM, err := os.ReadFile(certFile)
	require.NoError(t, err)
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(certPEM)
	require.True(t, ok)

	return certPool
}

// newProxyClient creates an HTTP client configured to use the proxy.
// It adds a Proxy-Authorization header with the provided token for authentication.
// The certPool parameter specifies which certificates the client should trust.
// For MITM'd requests, use the proxy's CA. For passthrough, use the target server's cert.
func newProxyClient(t *testing.T, srv *aibridgeproxyd.Server, proxyAuth string, certPool *x509.CertPool) *http.Client {
	t.Helper()

	// Create an HTTP client configured to use the proxy.
	proxyURL, err := url.Parse("http://" + srv.Addr())
	require.NoError(t, err)

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    certPool,
		},
	}

	// Only set the header if proxyAuth is provided. This allows tests to
	// verify behavior when the Proxy-Authorization header is missing.
	if proxyAuth != "" {
		transport.ProxyConnectHeader = http.Header{
			"Proxy-Authorization": []string{proxyAuth},
		}
	}

	return &http.Client{Transport: transport}
}

// newTargetServer creates a mock HTTPS server that will be the target of proxied requests.
// It returns the server and its parsed URL. The server is automatically closed when the test ends.
func newTargetServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *url.URL) {
	t.Helper()

	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return srv, srvURL
}

// makeProxyAuthHeader creates a Proxy-Authorization header value with the given token.
// Format: "Basic base64(username:token)" where username is "ignored".
func makeProxyAuthHeader(token string) string {
	credentials := base64.StdEncoding.EncodeToString([]byte("ignored:" + token))
	return "Basic " + credentials
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("MissingListenAddr", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "listen address is required")
	})

	t.Run("EmptyListenAddr", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "listen address is required")
	})

	t.Run("MissingCoderAccessURL", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "coder access URL is required")
	})

	t.Run("EmptyCoderAccessURL", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  " ",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "coder access URL is required")
	})

	t.Run("InvalidCoderAccessURL", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "://invalid",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid coder access URL")
	})

	t.Run("MissingCertFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      ":0",
			CoderAccessURL:  "http://localhost:3000",
			KeyFile:         "key.pem",
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("MissingKeyFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      ":0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        "cert.pem",
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("InvalidCertFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      ":0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        "/nonexistent/cert.pem",
			KeyFile:         "/nonexistent/key.pem",
			DomainAllowlist: []string{"127.0.0.1", "localhost"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to load MITM certificate")
	})

	t.Run("MissingDomainAllowlist", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:     ":0",
			CoderAccessURL: "http://localhost:3000",
			CertFile:       certFile,
			KeyFile:        keyFile,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "domain allow list is required")
	})

	t.Run("EmptyDomainAllowlist", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      ":0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{""},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "domain allowlist is empty, at least one domain is required")
	})

	t.Run("InvalidDomainAllowlist", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"[invalid:domain"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid domain")
	})

	t.Run("DomainWithNonAllowedPort", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"api.anthropic.com:8443"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid port in domain")
	})

	t.Run("InvalidUpstreamProxy", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"api.anthropic.com"},
			UpstreamProxy:   "://invalid-url",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid upstream proxy URL")
	})

	t.Run("UpstreamProxyCAFileNotFound", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"api.anthropic.com"},
			UpstreamProxy:   "https://proxy.example.com:8080",
			UpstreamProxyCA: "/nonexistent/ca.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to read upstream proxy CA certificate")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"api.anthropic.com", "api.openai.com"},
		})
		require.NoError(t, err)
		require.NotNil(t, srv)
	})

	t.Run("SuccessWithUpstreamProxy", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"api.anthropic.com", "api.openai.com"},
			UpstreamProxy:   "http://proxy.example.com:8080",
		})
		require.NoError(t, err)
		require.NotNil(t, srv)
	})

	t.Run("SuccessWithHTTPSUpstreamProxyAndCA", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		// Use the shared test CA as the upstream proxy CA (it's a valid PEM cert)
		srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:      "127.0.0.1:0",
			CoderAccessURL:  "http://localhost:3000",
			CertFile:        certFile,
			KeyFile:         keyFile,
			DomainAllowlist: []string{"api.anthropic.com"},
			UpstreamProxy:   "https://proxy.example.com:8080",
			UpstreamProxyCA: certFile,
		})
		require.NoError(t, err)
		require.NotNil(t, srv)
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	certFile, keyFile := getSharedTestCA(t)
	logger := slogtest.Make(t, nil)

	srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
		ListenAddr:      "127.0.0.1:0",
		CoderAccessURL:  "http://localhost:3000",
		CertFile:        certFile,
		KeyFile:         keyFile,
		DomainAllowlist: []string{"127.0.0.1", "localhost"},
	})
	require.NoError(t, err)

	err = srv.Close()
	require.NoError(t, err)

	// Calling Close again should not error
	err = srv.Close()
	require.NoError(t, err)
}

func TestProxy_CertCaching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		domainAllowlist []string
		passthrough     bool
	}{
		{
			name:            "AllowlistedDomainCached",
			domainAllowlist: nil, // will use targetURL.Hostname()
			passthrough:     false,
		},
		{
			name:            "NonAllowlistedDomainNotCached",
			domainAllowlist: []string{"other.example.com"},
			passthrough:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a mock HTTPS server that will be the target of the proxied request.
			targetServer, targetURL := newTargetServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Create a cert cache so we can inspect it after the request.
			certCache := aibridgeproxyd.NewCertCache()

			// Configure domain allowlist.
			domainAllowlist := tt.domainAllowlist
			if domainAllowlist == nil {
				domainAllowlist = []string{targetURL.Hostname()}
			}

			// Start the proxy server with the certificate cache.
			srv := newTestProxy(t,
				withAllowedPorts(targetURL.Port()),
				withCertStore(certCache),
				withDomainAllowlist(domainAllowlist...),
			)

			// Build the cert pool for the client to trust.
			//   - For MITM'd requests, the client connects through the proxy which generates
			//     certificates signed by our test CA, so it needs to trust the proxy's CA.
			//   - For passthrough requests, the client connects directly to the target server
			//     through a tunnel, so it needs to trust the target's self-signed certificate.
			var certPool *x509.CertPool
			if tt.passthrough {
				certPool = x509.NewCertPool()
				certPool.AddCert(targetServer.Certificate())
			} else {
				certPool = getProxyCertPool(t)
			}

			// Make a request through the proxy to the target server.
			client := newProxyClient(t, srv, makeProxyAuthHeader("test-session-token"), certPool)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetURL.String(), nil)
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Fetch with a generator that tracks calls.
			genCalls := 0
			_, err = certCache.Fetch(targetURL.Hostname(), func() (*tls.Certificate, error) {
				genCalls++
				return &tls.Certificate{}, nil
			})
			require.NoError(t, err)

			if tt.passthrough {
				// Certificate should NOT have been cached since request was tunneled.
				require.Equal(t, 1, genCalls, "certificate should NOT have been cached for non-allowlisted domain")
			} else {
				// Certificate should have been cached during MITM.
				require.Equal(t, 0, genCalls, "certificate should have been cached during request")
			}
		})
	}
}

func TestProxy_PortValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		allowedPorts func(targetURL *url.URL) []string
		expectError  bool
	}{
		{
			name: "AllowedPort",
			// Include the target's random port so the request is allowed.
			allowedPorts: func(targetURL *url.URL) []string {
				return []string{targetURL.Port()}
			},
		},
		{
			name: "RejectedPort",
			// Only allow port 443 which doesn't match the target.
			allowedPorts: func(_ *url.URL) []string {
				return []string{"443"}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a target HTTPS server that will be the destination of our proxied request.
			_, targetURL := newTargetServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from target"))
			})

			// Start the proxy server on a random port to avoid conflicts when running tests in parallel.
			srv := newTestProxy(t,
				withAllowedPorts(tt.allowedPorts(targetURL)...),
				withDomainAllowlist(targetURL.Hostname()),
			)

			// Make a request through the proxy to the target server.
			client := newProxyClient(t, srv, makeProxyAuthHeader("test-session-token"), getProxyCertPool(t))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetURL.String(), nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify the request was successful and reached the target server.
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "hello from target", string(body))
		})
	}
}

func TestProxy_Authentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		proxyAuth   string
		expectError bool
	}{
		{
			name:        "ValidCredentials",
			proxyAuth:   makeProxyAuthHeader("test-coder-session-token"),
			expectError: false,
		},
		{
			name:        "MissingCredentials",
			proxyAuth:   "",
			expectError: true,
		},
		{
			name:        "InvalidBase64",
			proxyAuth:   "Basic not-valid-base64!",
			expectError: true,
		},
		{
			name:        "EmptyToken",
			proxyAuth:   makeProxyAuthHeader(""),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a mock HTTPS server that will be the target of our proxied request.
			_, targetURL := newTargetServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from target"))
			})

			// Start the proxy server on a random port to avoid conflicts when running tests in parallel.
			srv := newTestProxy(t,
				withAllowedPorts(targetURL.Port()),
				withDomainAllowlist(targetURL.Hostname()),
			)

			// Make a request through the proxy to the target server.
			client := newProxyClient(t, srv, tt.proxyAuth, getProxyCertPool(t))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetURL.String(), nil)
			require.NoError(t, err)
			resp, err := client.Do(req)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				defer resp.Body.Close()

				// Verify the response was successfully proxied.
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.Equal(t, "hello from target", string(body))
			}
		})
	}
}

func TestProxy_MITM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		domainAllowlist   []string
		allowedPorts      []string
		buildTargetURL    func(passthroughURL *url.URL) (string, error)
		passthrough       bool
		noAIBridgeRouting bool
		expectedPath      string
	}{
		{
			name:            "MitmdAnthropic",
			domainAllowlist: []string{"api.anthropic.com"},
			allowedPorts:    []string{"443"},
			buildTargetURL: func(_ *url.URL) (string, error) {
				return "https://api.anthropic.com/v1/messages", nil
			},
			expectedPath: "/api/v2/aibridge/anthropic/v1/messages",
		},
		{
			name:            "MitmdAnthropicNonDefaultPort",
			domainAllowlist: []string{"api.anthropic.com"},
			allowedPorts:    []string{"8443"},
			buildTargetURL: func(_ *url.URL) (string, error) {
				return "https://api.anthropic.com:8443/v1/messages", nil
			},
			expectedPath: "/api/v2/aibridge/anthropic/v1/messages",
		},
		{
			name:            "MitmdOpenAI",
			domainAllowlist: []string{"api.openai.com"},
			allowedPorts:    []string{"443"},
			buildTargetURL: func(_ *url.URL) (string, error) {
				return "https://api.openai.com/v1/chat/completions", nil
			},
			expectedPath: "/api/v2/aibridge/openai/v1/chat/completions",
		},
		{
			name:            "MitmdOpenAINonDefaultPort",
			domainAllowlist: []string{"api.openai.com"},
			allowedPorts:    []string{"8443"},
			buildTargetURL: func(_ *url.URL) (string, error) {
				return "https://api.openai.com:8443/v1/chat/completions", nil
			},
			expectedPath: "/api/v2/aibridge/openai/v1/chat/completions",
		},
		{
			name:            "PassthroughUnknownHost",
			domainAllowlist: []string{"other.example.com"},
			allowedPorts:    nil, // will use passthroughURL.Port()
			buildTargetURL: func(passthroughURL *url.URL) (string, error) {
				return url.JoinPath(passthroughURL.String(), "/some/path")
			},
			passthrough: true,
		},
		// The host is MITM'd but has no provider mapping.
		// The request is decrypted but passed through to the original destination
		// instead of being routed to aibridge.
		{
			name:            "MitmdWithoutAIBridgeRouting",
			domainAllowlist: nil, // will use passthroughURL.Hostname()
			allowedPorts:    nil, // will use passthroughURL.Port()
			buildTargetURL: func(passthroughURL *url.URL) (string, error) {
				return url.JoinPath(passthroughURL.String(), "/some/path")
			},
			passthrough:       false,
			noAIBridgeRouting: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Track what aibridged receives.
			var receivedPath string
			var receivedAuth string

			// Create a mock aibridged server that captures requests.
			aibridgedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				receivedAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from aibridged"))
			}))
			t.Cleanup(func() { aibridgedServer.Close() })

			// Create a mock target server for passthrough tests.
			passthroughServer, passthroughURL := newTargetServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from passthrough"))
			})

			// Configure allowed ports.
			allowedPorts := tt.allowedPorts
			if allowedPorts == nil {
				allowedPorts = []string{passthroughURL.Port()}
			}

			// Configure domain allowlist.
			domainAllowlist := tt.domainAllowlist
			if domainAllowlist == nil {
				domainAllowlist = []string{passthroughURL.Hostname()}
			}

			// Start the proxy server pointing to our mock aibridged.
			srv := newTestProxy(t,
				withCoderAccessURL(aibridgedServer.URL),
				withAllowedPorts(allowedPorts...),
				withDomainAllowlist(domainAllowlist...),
			)

			// Build the target URL:
			targetURL, err := tt.buildTargetURL(passthroughURL)
			require.NoError(t, err)

			// Build the cert pool for the client to trust.
			//   - For MITM'd requests, the client connects through the proxy which generates
			//     certificates signed by our test CA, so it needs to trust the proxy's CA.
			//   - For passthrough requests, the client connects directly to the target server
			//     through a tunnel, so it needs to trust the target's self-signed certificate.
			var certPool *x509.CertPool
			if tt.passthrough {
				certPool = x509.NewCertPool()
				certPool.AddCert(passthroughServer.Certificate())
			} else {
				certPool = getProxyCertPool(t)
			}

			// Make a request through the proxy to the target URL.
			client := newProxyClient(t, srv, makeProxyAuthHeader("test-session-token"), certPool)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, targetURL, strings.NewReader(`{}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			if tt.passthrough || tt.noAIBridgeRouting {
				// Verify request went to target server, not aibridged.
				require.Equal(t, "hello from passthrough", string(body))
				require.Empty(t, receivedPath, "aibridged should not receive passthrough requests")
				require.Empty(t, receivedAuth, "passthrough requests are not authenticated by the proxy")
			} else {
				// Verify the request was routed to aibridged correctly.
				require.Equal(t, "hello from aibridged", string(body))
				require.Equal(t, tt.expectedPath, receivedPath)
				require.Equal(t, "Bearer test-session-token", receivedAuth, "MITM'd requests must include authentication")
			}
		})
	}
}

// TestServeCACert validates that a configured certificate file can be served correctly by the API.
//
// Note: Tests for certificate file errors (missing file, invalid PEM) are
// covered by [TestNew] since certificate validation happens at initialization.
// The serveCACert handler returns the pre-loaded, pre-validated certificate.
func TestServeCACert(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		srv := newTestProxy(t)

		// Create a request to the CA cert endpoint via the Handler.
		req := httptest.NewRequest(http.MethodGet, "/ca-cert.pem", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "application/x-pem-file", rec.Header().Get("Content-Type"))
		require.Equal(t, "attachment; filename=ca-cert.pem", rec.Header().Get("Content-Disposition"))

		// Verify the certificate is valid PEM.
		body := rec.Body.Bytes()
		block, _ := pem.Decode(body)
		require.NotNil(t, block, "response should be valid PEM")
		require.Equal(t, "CERTIFICATE", block.Type)

		// Verify the certificate is valid X.509.
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		require.NotNil(t, cert)

		// Verify it matches the original certificate.
		certFile, _ := getSharedTestCA(t)
		expectedCertPEM, err := os.ReadFile(certFile)
		require.NoError(t, err)
		require.Equal(t, expectedCertPEM, body)
	})
}

// TestServeCACert_CompoundPEM validates that a compound PEM certificate which contains a private key
// will only have its certificate type returned from the /ca-cert.pem endpoint.
func TestServeCACert_CompoundPEM(t *testing.T) {
	t.Parallel()

	certFile, keyFile := getSharedTestCA(t)

	// Read the shared CA cert and key to create a compound PEM file.
	certPEM, err := os.ReadFile(certFile)
	require.NoError(t, err)
	keyPEM, err := os.ReadFile(keyFile)
	require.NoError(t, err)

	// Create a compound PEM file containing both the certificate and the private key.
	compoundPEM := make([]byte, 0, len(certPEM)+len(keyPEM))
	compoundPEM = append(compoundPEM, certPEM...)
	compoundPEM = append(compoundPEM, keyPEM...)

	tmpDir := t.TempDir()
	compoundCertFile := filepath.Join(tmpDir, "compound.pem")

	err = os.WriteFile(compoundCertFile, compoundPEM, 0o600)
	require.NoError(t, err)

	logger := slogtest.Make(t, nil)

	srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
		ListenAddr:      "127.0.0.1:0",
		CoderAccessURL:  "http://localhost:3000",
		CertFile:        compoundCertFile,
		KeyFile:         keyFile,
		DomainAllowlist: []string{"127.0.0.1", "localhost"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// Create a request to the CA cert endpoint via the Handler.
	req := httptest.NewRequest(http.MethodGet, "/ca-cert.pem", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Verify the response contains only the certificate, not the private key.
	body := rec.Body.Bytes()

	// Parse all PEM blocks from the response.
	var pemBlocks []*pem.Block
	remaining := body
	for {
		var block *pem.Block
		block, remaining = pem.Decode(remaining)
		if block == nil {
			break
		}
		pemBlocks = append(pemBlocks, block)
	}

	// There should be exactly one PEM block (the certificate).
	require.Len(t, pemBlocks, 1, "response should contain exactly one PEM block")
	require.Equal(t, "CERTIFICATE", pemBlocks[0].Type, "the PEM block should be a certificate")

	// Verify no private key material is present by checking for common key block types.
	bodyStr := string(body)
	require.NotContains(t, bodyStr, "PRIVATE KEY", "response should not contain any private key")
	require.NotContains(t, bodyStr, "RSA PRIVATE KEY", "response should not contain RSA private key")
	require.NotContains(t, bodyStr, "EC PRIVATE KEY", "response should not contain EC private key")

	// Verify the certificate is valid X.509.
	cert, err := x509.ParseCertificate(pemBlocks[0].Bytes)
	require.NoError(t, err)
	require.Equal(t, "Shared Test CA", cert.Subject.CommonName)
}

func TestUpstreamProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// passthrough determines whether the request should be tunneled through
		// the upstream proxy (true) or MITM'd by aiproxy (false).
		// When true, the target domain is NOT in the allowlist.
		// When false, the target domain IS in the allowlist.
		passthrough bool
		// upstreamProxyTLS determines whether the upstream proxy uses TLS.
		// When true, aiproxy must be configured with the upstream proxy's CA.
		upstreamProxyTLS bool
		// buildTargetURL constructs the request URL. For passthrough, it uses
		// the final destination URL. For MITM, it uses api.anthropic.com.
		buildTargetURL func(finalDestinationURL *url.URL) string
		// expectedAIBridgePath is the path aibridge should receive for MITM requests.
		expectedAIBridgePath string
	}{
		{
			name:             "NonAllowlistedDomain_PassthroughToHTTPUpstreamProxy",
			passthrough:      true,
			upstreamProxyTLS: false,
			buildTargetURL: func(finalDestinationURL *url.URL) string {
				return fmt.Sprintf("https://%s/passthrough-path", finalDestinationURL.Host)
			},
		},
		{
			name:             "NonAllowlistedDomain_PassthroughToHTTPSUpstreamProxy",
			passthrough:      true,
			upstreamProxyTLS: true,
			buildTargetURL: func(finalDestinationURL *url.URL) string {
				return fmt.Sprintf("https://%s/passthrough-path", finalDestinationURL.Host)
			},
		},
		{
			name:             "AllowlistedDomain_MITMByAIProxy",
			passthrough:      false,
			upstreamProxyTLS: false,
			buildTargetURL: func(_ *url.URL) string {
				return "https://api.anthropic.com:443/v1/messages"
			},
			expectedAIBridgePath: "/api/v2/aibridge/anthropic/v1/messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Track requests received by each component to verify the flow.
			var (
				upstreamProxyCONNECTReceived bool
				upstreamProxyCONNECTHost     string
				finalDestinationReceived     bool
				finalDestinationPath         string
				finalDestinationBody         string
				aibridgeReceived             bool
				aibridgePath                 string
				aibridgeAuthHeader           string
				aibridgeBody                 string
			)

			// Create mock final destination server representing the actual target:
			//   - For passthrough requests, traffic should reach this server.
			//   - For MITM requests, traffic should NOT reach this server.
			finalDestination, finalDestinationURL := newTargetServer(t, func(w http.ResponseWriter, r *http.Request) {
				finalDestinationReceived = true
				finalDestinationPath = r.URL.Path
				body, _ := io.ReadAll(r.Body)
				finalDestinationBody = string(body)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("final destination response"))
			})

			// Upstream proxy handler: same logic for both HTTP and HTTPS.
			upstreamProxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodConnect {
					http.Error(w, "expected CONNECT request", http.StatusBadRequest)
					return
				}

				upstreamProxyCONNECTReceived = true
				upstreamProxyCONNECTHost = r.Host

				// Connect to the mock final destination server.
				targetConn, err := net.Dial("tcp", finalDestinationURL.Host)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
				defer targetConn.Close()

				// Hijack the connection to take over the raw TCP socket.
				// After responding "200 Connection Established", the proxy stops being
				// an HTTP server and becomes a transparent tunnel that copies bytes
				// bidirectionally. The http package can't handle this mode, so we
				// hijack and manage the connection ourselves.
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					http.Error(w, "hijacking not supported", http.StatusInternalServerError)
					return
				}

				clientConn, _, err := hijacker.Hijack()
				if err != nil {
					return
				}
				defer clientConn.Close()

				// Send 200 Connection Established to signal tunnel is ready.
				_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

				// Copy data bidirectionally between aiproxy and final destination.
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, _ = io.Copy(targetConn, clientConn)
				}()
				go func() {
					defer wg.Done()
					_, _ = io.Copy(clientConn, targetConn)
				}()
				wg.Wait()
			})

			// Create upstream proxy: HTTP or HTTPS based on test case.
			var upstreamProxy *httptest.Server
			var upstreamProxyCAFile string
			if tt.upstreamProxyTLS {
				upstreamProxy = httptest.NewTLSServer(upstreamProxyHandler)
				// Write the upstream proxy's CA cert to a temp file for aiproxy to trust.
				upstreamProxyCAFile = filepath.Join(t.TempDir(), "upstream-proxy-ca.pem")
				certPEM := pem.EncodeToMemory(&pem.Block{
					Type:  "CERTIFICATE",
					Bytes: upstreamProxy.Certificate().Raw,
				})
				err := os.WriteFile(upstreamProxyCAFile, certPEM, 0o600)
				require.NoError(t, err)
			} else {
				upstreamProxy = httptest.NewServer(upstreamProxyHandler)
			}
			t.Cleanup(upstreamProxy.Close)

			// Create a mock aibridged server:
			//   - For passthrough requests, traffic should NOT reach this server.
			//   - For MITM requests, aiproxy rewrites the URL and forwards here.
			aibridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				aibridgeReceived = true
				aibridgePath = r.URL.Path
				aibridgeAuthHeader = r.Header.Get("Authorization")
				body, _ := io.ReadAll(r.Body)
				aibridgeBody = string(body)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("aibridge response"))
			}))
			t.Cleanup(aibridgeServer.Close)

			// Build the target URL for this test case.
			targetURL := tt.buildTargetURL(finalDestinationURL)
			parsedTargetURL, err := url.Parse(targetURL)
			require.NoError(t, err)

			// Configure allowlist based on test case:
			//   - For passthrough, api.anthropic.com is in allowlist, but we target a different host.
			//   - For MITM, api.anthropic.com must be in the allowlist.
			domainAllowlist := []string{"api.anthropic.com"}

			// Create aiproxy with upstream proxy configured.
			proxyOpts := []testProxyOption{
				withCoderAccessURL(aibridgeServer.URL),
				withDomainAllowlist(domainAllowlist...),
				withUpstreamProxy(upstreamProxy.URL),
				withAllowedPorts("80", "443", finalDestinationURL.Port(), parsedTargetURL.Port()),
			}
			if upstreamProxyCAFile != "" {
				proxyOpts = append(proxyOpts, withUpstreamProxyCA(upstreamProxyCAFile))
			}
			srv := newTestProxy(t, proxyOpts...)

			// Configure certificate trust based on test case:
			//   - For passthrough: client trusts final destination's CA.
			//   - For MITM: client trusts aiproxy's CA (fake certs).
			var certPool *x509.CertPool
			if tt.passthrough {
				certPool = x509.NewCertPool()
				certPool.AddCert(finalDestination.Certificate())
			} else {
				certPool = getProxyCertPool(t)
			}

			// Create HTTP client configured to use aiproxy.
			client := newProxyClient(t, srv, makeProxyAuthHeader("test-coder-token"), certPool)

			// Make request through aiproxy.
			requestBody := `{"test": "data", "foo": "bar"}`
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, targetURL, strings.NewReader(requestBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Verify the request flow based on test case.
			if tt.passthrough {
				require.True(t, upstreamProxyCONNECTReceived,
					"upstream proxy should receive CONNECT for non-allowlisted domain")
				require.Equal(t, finalDestinationURL.Host, upstreamProxyCONNECTHost,
					"upstream proxy should receive CONNECT to correct host")
				require.True(t, finalDestinationReceived,
					"final destination should receive the passthrough request")
				require.Equal(t, parsedTargetURL.Path, finalDestinationPath,
					"final destination should receive correct path")
				require.Equal(t, requestBody, finalDestinationBody,
					"final destination should receive the exact request body")
				require.False(t, aibridgeReceived,
					"aibridge should NOT receive request for non-allowlisted domain")
			} else {
				require.False(t, upstreamProxyCONNECTReceived,
					"upstream proxy should NOT receive CONNECT for allowlisted domain")
				require.True(t, aibridgeReceived,
					"aibridge should receive the MITM'd request")
				require.Equal(t, tt.expectedAIBridgePath, aibridgePath,
					"aibridge should receive rewritten path")
				require.Equal(t, "Bearer test-coder-token", aibridgeAuthHeader,
					"aibridge should receive auth header extracted from proxy auth")
				require.Equal(t, requestBody, aibridgeBody,
					"aibridge should receive the exact request body")
				require.False(t, finalDestinationReceived,
					"final destination should NOT receive request for allowlisted domain")
			}
		})
	}
}
