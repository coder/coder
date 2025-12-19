package aibridgeproxyd_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
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
	listenAddr     string
	coderAccessURL string
	allowedPorts   []string
	certStore      *aibridgeproxyd.CertCache
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

// newTestProxy creates a new AI Bridge Proxy server for testing.
// It uses the shared test CA and registers cleanup automatically.
// It waits for the proxy server to be ready before returning.
func newTestProxy(t *testing.T, opts ...testProxyOption) *aibridgeproxyd.Server {
	t.Helper()

	cfg := &testProxyConfig{
		listenAddr:     "127.0.0.1:0",
		coderAccessURL: "http://localhost:3000",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	certFile, keyFile := getSharedTestCA(t)
	logger := slogtest.Make(t, nil)

	aibridgeOpts := aibridgeproxyd.Options{
		ListenAddr:     cfg.listenAddr,
		CoderAccessURL: cfg.coderAccessURL,
		CertFile:       certFile,
		KeyFile:        keyFile,
		AllowedPorts:   cfg.allowedPorts,
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

// newProxyClient creates an HTTP client configured to use the proxy and trust its CA.
// It adds a Proxy-Authorization header with the provided token for authentication.
func newProxyClient(t *testing.T, srv *aibridgeproxyd.Server, proxyAuth string) *http.Client {
	t.Helper()

	certFile, _ := getSharedTestCA(t)

	// Load the CA certificate so the client trusts the proxy's MITM certificate.
	certPEM, err := os.ReadFile(certFile)
	require.NoError(t, err)
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(certPEM)
	require.True(t, ok)

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
// It returns the server's parsed URL. The server is automatically closed when the test ends.
func newTargetServer(t *testing.T, handler http.HandlerFunc) *url.URL {
	t.Helper()

	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return srvURL
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
			ListenAddr:     "",
			CoderAccessURL: "http://localhost:3000",
			CertFile:       certFile,
			KeyFile:        keyFile,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "listen address is required")
	})

	t.Run("MissingCoderAccessURL", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr: "127.0.0.1:0",
			CertFile:   certFile,
			KeyFile:    keyFile,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "coder access URL is required")
	})

	t.Run("EmptyCoderAccessURL", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:     "127.0.0.1:0",
			CoderAccessURL: " ",
			CertFile:       certFile,
			KeyFile:        keyFile,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "coder access URL is required")
	})

	t.Run("InvalidCoderAccessURL", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:     "127.0.0.1:0",
			CoderAccessURL: "://invalid",
			CertFile:       certFile,
			KeyFile:        keyFile,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid coder access URL")
	})

	t.Run("MissingCertFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:     ":0",
			CoderAccessURL: "http://localhost:3000",
			KeyFile:        "key.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("MissingKeyFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:     ":0",
			CoderAccessURL: "http://localhost:3000",
			CertFile:       "cert.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("InvalidCertFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:     ":0",
			CoderAccessURL: "http://localhost:3000",
			CertFile:       "/nonexistent/cert.pem",
			KeyFile:        "/nonexistent/key.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to load MITM certificate")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr:     "127.0.0.1:0",
			CoderAccessURL: "http://localhost:3000",
			CertFile:       certFile,
			KeyFile:        keyFile,
		})
		require.NoError(t, err)
		require.NotNil(t, srv)

		err = srv.Close()
		require.NoError(t, err)
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	certFile, keyFile := getSharedTestCA(t)
	logger := slogtest.Make(t, nil)

	srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
		ListenAddr:     "127.0.0.1:0",
		CoderAccessURL: "http://localhost:3000",
		CertFile:       certFile,
		KeyFile:        keyFile,
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

	// Create a mock HTTPS server that will be the target of the proxied request.
	targetURL := newTargetServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create a cert cache so we can inspect it after the request.
	certCache := aibridgeproxyd.NewCertCache()

	// Start the proxy server with the certificate cache.
	srv := newTestProxy(t,
		withAllowedPorts(targetURL.Port()),
		withCertStore(certCache),
	)

	// Make a request through the proxy to the target server.
	// This triggers MITM and caches the generated certificate.
	client := newProxyClient(t, srv, makeProxyAuthHeader("test-session-token"))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetURL.String(), nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Fetch with a generator that tracks calls: if the certificate was cached
	// during the request above, the generator should not be called.
	genCalls := 0
	_, err = certCache.Fetch(targetURL.Hostname(), func() (*tls.Certificate, error) {
		genCalls++
		return &tls.Certificate{}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 0, genCalls, "certificate should have been cached during request")
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
			targetURL := newTargetServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from target"))
			})

			// Start the proxy server on a random port to avoid conflicts when running tests in parallel.
			srv := newTestProxy(t, withAllowedPorts(tt.allowedPorts(targetURL)...))

			// Make a request through the proxy to the target server.
			client := newProxyClient(t, srv, makeProxyAuthHeader("test-session-token"))
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
			targetURL := newTargetServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from target"))
			})

			// Start the proxy server on a random port to avoid conflicts when running tests in parallel.
			srv := newTestProxy(t, withAllowedPorts(targetURL.Port()))

			// Make a request through the proxy to the target server.
			client := newProxyClient(t, srv, tt.proxyAuth)
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
		name         string
		targetHost   string
		targetPort   string // optional, if empty uses default HTTPS port (443)
		targetPath   string
		expectedPath string
		passthrough  bool
	}{
		{
			name:         "AnthropicMessages",
			targetHost:   "api.anthropic.com",
			targetPath:   "/v1/messages",
			expectedPath: "/api/v2/aibridge/anthropic/v1/messages",
		},
		{
			name:         "AnthropicNonDefaultPort",
			targetHost:   "api.anthropic.com",
			targetPort:   "8443",
			targetPath:   "/v1/messages",
			expectedPath: "/api/v2/aibridge/anthropic/v1/messages",
		},
		{
			name:         "OpenAIChatCompletions",
			targetHost:   "api.openai.com",
			targetPath:   "/v1/chat/completions",
			expectedPath: "/api/v2/aibridge/openai/v1/chat/completions",
		},
		{
			name:         "OpenAINonDefaultPort",
			targetHost:   "api.openai.com",
			targetPort:   "8443",
			targetPath:   "/v1/chat/completions",
			expectedPath: "/api/v2/aibridge/openai/v1/chat/completions",
		},
		{
			name:        "UnknownHostPassthrough",
			targetPath:  "/some/path",
			passthrough: true,
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
			passthroughURL := newTargetServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from passthrough"))
			})

			// Configure allowed ports based on test case.
			// AI provider tests connect to the specified port, or 443 if not specified.
			// Passthrough tests connect directly to the local target server's random port.
			var allowedPorts []string
			switch {
			case tt.passthrough:
				allowedPorts = []string{passthroughURL.Port()}
			case tt.targetPort != "":
				allowedPorts = []string{tt.targetPort}
			default:
				allowedPorts = []string{"443"}
			}

			// Start the proxy server pointing to our mock aibridged.
			srv := newTestProxy(t,
				withCoderAccessURL(aibridgedServer.URL),
				withAllowedPorts(allowedPorts...),
			)

			// Build the target URL:
			//   - For passthrough, target the local mock TLS server.
			//   - For AI providers, use their real hostnames to trigger routing.
			//     Non-default ports are included explicitly; default port (443) is omitted.
			var targetURL string
			var err error
			switch {
			case tt.passthrough:
				targetURL, err = url.JoinPath(passthroughURL.String(), tt.targetPath)
				require.NoError(t, err)
			case tt.targetPort != "":
				targetURL, err = url.JoinPath("https://"+tt.targetHost+":"+tt.targetPort, tt.targetPath)
				require.NoError(t, err)
			default:
				targetURL, err = url.JoinPath("https://"+tt.targetHost, tt.targetPath)
				require.NoError(t, err)
			}

			// Make a request through the proxy to the target URL.
			client := newProxyClient(t, srv, makeProxyAuthHeader("test-session-token"))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, targetURL, strings.NewReader(`{}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			if tt.passthrough {
				// Verify request went to target server, not aibridged.
				require.Equal(t, "hello from passthrough", string(body))
				require.Empty(t, receivedPath, "aibridged should not receive passthrough requests")
				require.Empty(t, receivedAuth, "aibridged should not receive passthrough requests")
			} else {
				// Verify the request was routed to aibridged correctly.
				require.Equal(t, "hello from aibridged", string(body))
				require.Equal(t, tt.expectedPath, receivedPath)
				require.Equal(t, "Bearer test-session-token", receivedAuth)
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
		ListenAddr:     "127.0.0.1:0",
		CoderAccessURL: "http://localhost:3000",
		CertFile:       compoundCertFile,
		KeyFile:        keyFile,
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
