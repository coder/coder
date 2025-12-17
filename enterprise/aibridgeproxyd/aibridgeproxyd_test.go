package aibridgeproxyd_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
	"github.com/coder/coder/v2/testutil"
)

// generateTestCA creates a temporary CA certificate and key for testing.
func generateTestCA(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	tempDir := t.TempDir()
	certFile = filepath.Join(tempDir, "ca.crt")
	keyFile = filepath.Join(tempDir, "ca.key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	err = os.WriteFile(certFile, certPEM, 0o600)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	err = os.WriteFile(keyFile, keyPEM, 0o600)
	require.NoError(t, err)

	return certFile, keyFile
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("MissingListenAddr", func(t *testing.T) {
		t.Parallel()

		certFile, keyFile := generateTestCA(t)
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

		certFile, keyFile := generateTestCA(t)
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

	certFile, keyFile := generateTestCA(t)
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

func TestProxy_MITM(t *testing.T) {
	t.Parallel()

	// Create a mock HTTPS server that will be the target of our proxied request.
	targetServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from target"))
	}))
	defer targetServer.Close()

	certFile, keyFile := generateTestCA(t)
	logger := slogtest.Make(t, nil)

	// Start the proxy server.
	srv, err := aibridgeproxyd.New(t.Context(), logger, aibridgeproxyd.Options{
		ListenAddr: "127.0.0.1:8888",
		CertFile:   certFile,
		KeyFile:    keyFile,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// Wait for the proxy server to be ready.
	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", "127.0.0.1:8888")
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
	proxyURL, err := url.Parse("http://127.0.0.1:8888")
	require.NoError(t, err)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
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
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify the response was successfully proxied.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "hello from target", string(body))
}
