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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"

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
			ListenAddr: "",
			CertFile:   certFile,
			KeyFile:    keyFile,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "listen address is required")
	})

	t.Run("MissingCertFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr: ":0",
			KeyFile:    "key.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("MissingKeyFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr: ":0",
			CertFile:   "cert.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("InvalidCertFile", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, nil)

		_, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr: ":0",
			CertFile:   "/nonexistent/cert.pem",
			KeyFile:    "/nonexistent/key.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to load MITM certificate")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := getSharedTestCA(t)
		logger := slogtest.Make(t, nil)

		srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
			ListenAddr: "127.0.0.1:0",
			CertFile:   certFile,
			KeyFile:    keyFile,
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
		ListenAddr: "127.0.0.1:0",
		CertFile:   certFile,
		KeyFile:    keyFile,
	})
	require.NoError(t, err)

	err = srv.Close()
	require.NoError(t, err)

	// Calling Close again should not error
	err = srv.Close()
	require.NoError(t, err)
}

func TestProxy_PortValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		allowPort   bool
		expectError bool
	}{
		{
			name:      "AllowedPort",
			allowPort: true,
		},
		{
			name:        "RejectedPort",
			allowPort:   false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a target HTTPS server that will be the destination of our proxied request.
			targetServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from target"))
			}))
			defer targetServer.Close()

			targetURL, err := url.Parse(targetServer.URL)
			require.NoError(t, err)

			certFile, keyFile := generateTestCA(t)
			logger := slogtest.Make(t, nil)

			// Configure allowed ports based on test case.
			// For allowed case, include the target's random port.
			// For rejected case, only allow port 443 which doesn't match the target.
			var allowedPorts []string
			if tt.allowPort {
				allowedPorts = []string{targetURL.Port()}
			} else {
				allowedPorts = []string{"443"}
			}

			// Start the proxy server on a random port to avoid conflicts when running tests in parallel.
			srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
				ListenAddr:   "127.0.0.1:0",
				CertFile:     certFile,
				KeyFile:      keyFile,
				AllowedPorts: allowedPorts,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = srv.Close() })

			proxyAddr := srv.Addr()
			require.NotEmpty(t, proxyAddr)

			// Wait for the proxy server to be ready.
			require.Eventually(t, func() bool {
				conn, err := net.Dial("tcp", proxyAddr)
				if err != nil {
					return false
				}
				_ = conn.Close()
				return true
			}, testutil.WaitShort, testutil.IntervalFast)

			// Load the CA certificate so the client trusts the proxy's MITM certificate.
			certPEM, err := os.ReadFile(certFile)
			require.NoError(t, err)
			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM(certPEM)

			// Create an HTTP client configured to use the proxy.
			proxyURL, err := url.Parse("http://" + proxyAddr)
			require.NoError(t, err)

			client := &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyURL),
					ProxyConnectHeader: http.Header{
						"Proxy-Authorization": []string{makeProxyAuthHeader("test-session-token")},
					},
					TLSClientConfig: &tls.Config{
						MinVersion: tls.VersionTLS12,
						RootCAs:    certPool,
					},
				},
			}

			// Make a request through the proxy to the target server.
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetServer.URL, nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			defer resp.Body.Close()

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
			targetServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from target"))
			}))
			defer targetServer.Close()

			targetURL, err := url.Parse(targetServer.URL)
			require.NoError(t, err)

			certFile, keyFile := generateTestCA(t)
			logger := slogtest.Make(t, nil)

			// Start the proxy server on a random port to avoid conflicts when running tests in parallel.
			// The actual port is accessible via srv.Addr().
			srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
				ListenAddr:   "127.0.0.1:0",
				CertFile:     certFile,
				KeyFile:      keyFile,
				AllowedPorts: []string{targetURL.Port()},
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = srv.Close() })

			proxyAddr := srv.Addr()
			require.NotEmpty(t, proxyAddr)

			// Wait for the proxy server to be ready.
			require.Eventually(t, func() bool {
				conn, err := net.Dial("tcp", proxyAddr)
				if err != nil {
					return false
				}
				_ = conn.Close()
				return true
			}, testutil.WaitShort, testutil.IntervalFast)

			// Load the CA certificate so the client trusts the proxy's MITM certificate.
			certPEM, err := os.ReadFile(certFile)
			require.NoError(t, err)
			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM(certPEM)

			// Create an HTTP client configured to use the proxy.
			proxyURL, err := url.Parse("http://" + proxyAddr)
			require.NoError(t, err)

			transport := &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
					RootCAs:    certPool,
				},
			}

			if tt.proxyAuth != "" {
				transport.ProxyConnectHeader = http.Header{
					"Proxy-Authorization": []string{tt.proxyAuth},
				}
			}

			client := &http.Client{Transport: transport}

			// Make a request through the proxy to the target server.
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetServer.URL, nil)
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
