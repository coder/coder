package cli_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/database/postgres"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/codersdk"
)

// This cannot be ran in parallel because it uses a signal.
// nolint:paralleltest
func TestServer(t *testing.T) {
	t.Run("Production", func(t *testing.T) {
		if runtime.GOOS != "linux" || testing.Short() {
			// Skip on non-Linux because it spawns a PostgreSQL instance.
			t.SkipNow()
		}
		connectionURL, closeFunc, err := postgres.Open()
		require.NoError(t, err)
		defer closeFunc()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, cfg := clitest.New(t, "server", "--address", ":0", "--postgres-url", connectionURL)
		errC := make(chan error)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()
		var client *codersdk.Client
		require.Eventually(t, func() bool {
			rawURL, err := cfg.URL().Read()
			if err != nil {
				return false
			}
			accessURL, err := url.Parse(rawURL)
			assert.NoError(t, err)
			client = codersdk.New(accessURL)
			return true
		}, time.Minute, 50*time.Millisecond)
		_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:            "some@one.com",
			Username:         "example",
			Password:         "password",
			OrganizationName: "example",
		})
		require.NoError(t, err)
		cancelFunc()
		require.ErrorIs(t, <-errC, context.Canceled)
	})
	t.Run("BuiltinPostgres", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.SkipNow()
		}
		ctx, cancelFunc := context.WithCancel(context.Background())
		root, cfg := clitest.New(t, "server", "--address", ":0")
		errC := make(chan error)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()
		require.Eventually(t, func() bool {
			_, err := cfg.URL().Read()
			return err == nil
		}, time.Minute, 25*time.Millisecond)
		cancelFunc()
		require.ErrorIs(t, <-errC, context.Canceled)
	})
	t.Run("BuiltinPostgresURL", func(t *testing.T) {
		t.Parallel()
		root, _ := clitest.New(t, "server", "postgres-builtin-url")
		var buf strings.Builder
		root.SetOutput(&buf)
		err := root.Execute()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "psql")
	})

	t.Run("NoWarningWithRemoteAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, cfg := clitest.New(t, "server", "--in-memory", "--address", ":0", "--access-url", "http://1.2.3.4:3000/")
		var buf strings.Builder
		errC := make(chan error)
		root.SetOutput(&buf)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		// Just wait for startup
		require.Eventually(t, func() bool {
			var err error
			_, err = cfg.URL().Read()
			return err == nil
		}, 15*time.Second, 25*time.Millisecond)

		cancelFunc()
		require.ErrorIs(t, <-errC, context.Canceled)

		assert.NotContains(t, buf.String(), "Workspaces must be able to reach Coder from this URL")
	})

	t.Run("TLSBadVersion", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, _ := clitest.New(t, "server", "--in-memory", "--address", ":0",
			"--tls-enable", "--tls-min-version", "tls9")
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})
	t.Run("TLSBadClientAuth", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, _ := clitest.New(t, "server", "--in-memory", "--address", ":0",
			"--tls-enable", "--tls-client-auth", "something")
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})
	t.Run("TLSNoCertFile", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, _ := clitest.New(t, "server", "--in-memory", "--address", ":0",
			"--tls-enable")
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})
	t.Run("TLSValid", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		certPath, keyPath := generateTLSCertificate(t)
		root, cfg := clitest.New(t, "server", "--in-memory", "--address", ":0",
			"--tls-enable", "--tls-cert-file", certPath, "--tls-key-file", keyPath)
		errC := make(chan error)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		// Verify HTTPS
		var accessURLRaw string
		require.Eventually(t, func() bool {
			var err error
			accessURLRaw, err = cfg.URL().Read()
			return err == nil
		}, 15*time.Second, 25*time.Millisecond)
		accessURL, err := url.Parse(accessURLRaw)
		require.NoError(t, err)
		require.Equal(t, "https", accessURL.Scheme)
		client := codersdk.New(accessURL)
		client.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					//nolint:gosec
					InsecureSkipVerify: true,
				},
			},
		}
		_, err = client.HasFirstUser(ctx)
		require.NoError(t, err)

		cancelFunc()
		require.ErrorIs(t, <-errC, context.Canceled)
	})
	// This cannot be ran in parallel because it uses a signal.
	//nolint:paralleltest
	t.Run("Shutdown", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			// Sending interrupt signal isn't supported on Windows!
			t.SkipNow()
		}
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, cfg := clitest.New(t, "server", "--in-memory", "--address", ":0", "--provisioner-daemons", "1")
		serverErr := make(chan error)
		go func() {
			err := root.ExecuteContext(ctx)
			serverErr <- err
		}()
		require.Eventually(t, func() bool {
			var err error
			_, err = cfg.URL().Read()
			return err == nil
		}, 15*time.Second, 25*time.Millisecond)

		currentProcess, err := os.FindProcess(os.Getpid())
		require.NoError(t, err)
		err = currentProcess.Signal(os.Interrupt)
		require.NoError(t, err)
		// Send a two more signal, which should be ignored.  Send 2 because the channel has a buffer
		// of 1 and we want to make sure that nothing strange happens if we exceed the buffer.
		err = currentProcess.Signal(os.Interrupt)
		require.NoError(t, err)
		err = currentProcess.Signal(os.Interrupt)
		require.NoError(t, err)
		err = <-serverErr
		require.NoError(t, err)
	})
	t.Run("TracerNoLeak", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		root, _ := clitest.New(t, "server", "--in-memory", "--address", ":0", "--trace=true")
		errC := make(chan error)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()
		cancelFunc()
		require.ErrorIs(t, <-errC, context.Canceled)
		require.Error(t, goleak.Find())
	})
	t.Run("Telemetry", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		deployment := make(chan struct{}, 64)
		snapshot := make(chan *telemetry.Snapshot, 64)
		r := chi.NewRouter()
		r.Post("/deployment", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			deployment <- struct{}{}
		})
		r.Post("/snapshot", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			ss := &telemetry.Snapshot{}
			err := json.NewDecoder(r.Body).Decode(ss)
			require.NoError(t, err)
			snapshot <- ss
		})
		server := httptest.NewServer(r)
		t.Cleanup(server.Close)

		root, _ := clitest.New(t, "server", "--in-memory", "--address", ":0", "--telemetry", "--telemetry-url", server.URL)
		errC := make(chan error)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		<-deployment
		<-snapshot
	})
}

func generateTLSCertificate(t testing.TB) (certPath, keyPath string) {
	dir := t.TempDir()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	certFile, err := os.CreateTemp(dir, "")
	require.NoError(t, err)
	defer certFile.Close()
	_, err = certFile.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}))
	require.NoError(t, err)
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	keyFile, err := os.CreateTemp(dir, "")
	require.NoError(t, err)
	defer keyFile.Close()
	err = pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})
	require.NoError(t, err)
	return certFile.Name(), keyFile.Name()
}
