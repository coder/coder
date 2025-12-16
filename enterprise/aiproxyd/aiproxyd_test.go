package aiproxyd_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/enterprise/aiproxyd"
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

		_, err := aiproxyd.New(t.Context(), logger, aiproxyd.Options{
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

		_, err := aiproxyd.New(t.Context(), logger, aiproxyd.Options{
			ListenAddr: ":0",
			KeyFile:    "key.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("MissingKeyFile", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil)

		_, err := aiproxyd.New(t.Context(), logger, aiproxyd.Options{
			ListenAddr: ":0",
			CertFile:   "cert.pem",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "cert file and key file are required")
	})

	t.Run("InvalidCertFile", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil)

		_, err := aiproxyd.New(t.Context(), logger, aiproxyd.Options{
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

		srv, err := aiproxyd.New(t.Context(), logger, aiproxyd.Options{
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

	srv, err := aiproxyd.New(t.Context(), logger, aiproxyd.Options{
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
