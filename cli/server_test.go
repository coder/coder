package cli_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd/database/postgres"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

// This cannot be ran in parallel because it uses a signal.
// nolint:tparallel,paralleltest
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

		root, cfg := clitest.New(t,
			"server",
			"--address", ":0",
			"--postgres-url", connectionURL,
			"--cache-dir", t.TempDir(),
		)
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()
		accessURL := waitAccessURL(t, cfg)
		client := codersdk.New(accessURL)

		_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:            "some@one.com",
			Username:         "example",
			Password:         "password",
			OrganizationName: "example",
		})
		require.NoError(t, err)
		cancelFunc()
		require.NoError(t, <-errC)
	})
	t.Run("BuiltinPostgres", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.SkipNow()
		}
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, cfg := clitest.New(t,
			"server",
			"--address", ":0",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t)
		root.SetOutput(pty.Output())
		root.SetErr(pty.Output())
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()
		//nolint:gocritic // Embedded postgres take a while to fire up.
		require.Eventually(t, func() bool {
			rawURL, err := cfg.URL().Read()
			return err == nil && rawURL != ""
		}, 3*time.Minute, testutil.IntervalFast, "failed to get access URL")
		cancelFunc()
		require.NoError(t, <-errC)
	})
	t.Run("BuiltinPostgresURL", func(t *testing.T) {
		t.Parallel()
		root, _ := clitest.New(t, "server", "postgres-builtin-url")
		pty := ptytest.New(t)
		root.SetOutput(pty.Output())
		err := root.Execute()
		require.NoError(t, err)

		pty.ExpectMatch("psql")
	})

	// Validate that an http scheme is prepended to a loopback
	// access URL and that a warning is printed that it may not be externally
	// reachable.
	t.Run("NoSchemeLocalAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "localhost:3000/",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t)
		root.SetIn(pty.Input())
		root.SetOut(pty.Output())
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("this may cause unexpected problems when creating workspaces")
		pty.ExpectMatch("View the Web UI: http://localhost:3000/")

		cancelFunc()
		require.NoError(t, <-errC)
	})

	// Validate that an https scheme is prepended to a remote access URL
	// and that a warning is printed for a host that cannot be resolved.
	t.Run("NoSchemeRemoteAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "foobarbaz.mydomain",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t)
		root.SetIn(pty.Input())
		root.SetOut(pty.Output())
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("this may cause unexpected problems when creating workspaces")
		pty.ExpectMatch("View the Web UI: https://foobarbaz.mydomain")

		cancelFunc()
		require.NoError(t, <-errC)
	})

	t.Run("NoWarningWithRemoteAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "https://google.com",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t)
		root.SetIn(pty.Input())
		root.SetOut(pty.Output())
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("View the Web UI: https://google.com")

		cancelFunc()
		require.NoError(t, <-errC)
	})

	t.Run("TLSBadVersion", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--tls-enable",
			"--tls-min-version", "tls9",
			"--cache-dir", t.TempDir(),
		)
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})
	t.Run("TLSBadClientAuth", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--tls-enable",
			"--tls-client-auth", "something",
			"--cache-dir", t.TempDir(),
		)
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})
	t.Run("TLSNoCertFile", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--tls-enable",
			"--cache-dir", t.TempDir(),
		)
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})
	t.Run("TLSValid", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		certPath, keyPath := generateTLSCertificate(t)
		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--tls-enable",
			"--tls-cert-file", certPath,
			"--tls-key-file", keyPath,
			"--cache-dir", t.TempDir(),
		)
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		// Verify HTTPS
		accessURL := waitAccessURL(t, cfg)
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
		_, err := client.HasFirstUser(ctx)
		require.NoError(t, err)

		cancelFunc()
		require.NoError(t, <-errC)
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

		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--provisioner-daemons", "1",
			"--cache-dir", t.TempDir(),
		)
		serverErr := make(chan error, 1)
		go func() {
			serverErr <- root.ExecuteContext(ctx)
		}()
		_ = waitAccessURL(t, cfg)
		currentProcess, err := os.FindProcess(os.Getpid())
		require.NoError(t, err)
		err = currentProcess.Signal(os.Interrupt)
		require.NoError(t, err)
		// We cannot send more signals here, because it's possible Coder
		// has already exited, which could cause the test to fail due to interrupt.
		err = <-serverErr
		require.NoError(t, err)
	})
	t.Run("TracerNoLeak", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--trace=true",
			"--cache-dir", t.TempDir(),
		)
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()
		cancelFunc()
		require.NoError(t, <-errC)
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
		defer server.Close()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--telemetry",
			"--telemetry-url", server.URL,
			"--cache-dir", t.TempDir(),
		)
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()

		<-deployment
		<-snapshot
		cancelFunc()
		<-errC
	})
	t.Run("Prometheus", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		random, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		_ = random.Close()
		tcpAddr, valid := random.Addr().(*net.TCPAddr)
		require.True(t, valid)
		randomPort := tcpAddr.Port

		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--provisioner-daemons", "1",
			"--prometheus-enable",
			"--prometheus-address", ":"+strconv.Itoa(randomPort),
			"--cache-dir", t.TempDir(),
		)
		serverErr := make(chan error, 1)
		go func() {
			serverErr <- root.ExecuteContext(ctx)
		}()
		_ = waitAccessURL(t, cfg)

		var res *http.Response
		require.Eventually(t, func() bool {
			req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d", randomPort), nil)
			assert.NoError(t, err)
			// nolint:bodyclose
			res, err = http.DefaultClient.Do(req)
			return err == nil
		}, testutil.WaitShort, testutil.IntervalFast)

		scanner := bufio.NewScanner(res.Body)
		hasActiveUsers := false
		hasWorkspaces := false
		for scanner.Scan() {
			// This metric is manually registered to be tracked in the server. That's
			// why we test it's tracked here.
			if strings.HasPrefix(scanner.Text(), "coderd_api_active_users_duration_hour") {
				hasActiveUsers = true
				continue
			}
			if strings.HasPrefix(scanner.Text(), "coderd_api_workspace_latest_build_total") {
				hasWorkspaces = true
				continue
			}
			t.Logf("scanned %s", scanner.Text())
		}
		require.NoError(t, scanner.Err())
		require.True(t, hasActiveUsers)
		require.True(t, hasWorkspaces)
		cancelFunc()
		<-serverErr
	})
	t.Run("GitHubOAuth", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		fakeRedirect := "https://fake-url.com"
		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--oauth2-github-client-id", "fake",
			"--oauth2-github-client-secret", "fake",
			"--oauth2-github-enterprise-base-url", fakeRedirect,
		)
		serverErr := make(chan error, 1)
		go func() {
			serverErr <- root.ExecuteContext(ctx)
		}()
		accessURL := waitAccessURL(t, cfg)
		client := codersdk.New(accessURL)
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		githubURL, err := accessURL.Parse("/api/v2/users/oauth2/github")
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubURL.String(), nil)
		require.NoError(t, err)
		res, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		fakeURL, err := res.Location()
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(fakeURL.String(), fakeRedirect), fakeURL.String())
		cancelFunc()
		<-serverErr
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

func waitAccessURL(t *testing.T, cfg config.Root) *url.URL {
	t.Helper()

	var err error
	var rawURL string
	require.Eventually(t, func() bool {
		rawURL, err = cfg.URL().Read()
		return err == nil && rawURL != ""
	}, testutil.WaitLong, testutil.IntervalFast, "failed to get access URL")

	accessURL, err := url.Parse(rawURL)
	require.NoError(t, err, "failed to parse access URL")

	return accessURL
}
