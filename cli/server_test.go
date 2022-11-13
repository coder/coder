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
	"sync/atomic"
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
			"--access-url", "http://example.com",
			"--postgres-url", connectionURL,
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t)
		root.SetOutput(pty.Output())
		root.SetErr(pty.Output())
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
			"--access-url", "http://example.com",
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

	// Validate that a warning is printed that it may not be externally
	// reachable.
	t.Run("LocalAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "http://localhost:3000/",
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
	t.Run("RemoteAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "https://foobarbaz.mydomain",
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

	t.Run("NoSchemeAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "google.com",
			"--cache-dir", t.TempDir(),
		)
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})

	t.Run("TLSBadVersion", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "http://example.com",
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
			"--access-url", "http://example.com",
			"--tls-enable",
			"--tls-client-auth", "something",
			"--cache-dir", t.TempDir(),
		)
		err := root.ExecuteContext(ctx)
		require.Error(t, err)
	})
	t.Run("TLSInvalid", func(t *testing.T) {
		t.Parallel()

		cert1Path, key1Path := generateTLSCertificate(t)
		cert2Path, key2Path := generateTLSCertificate(t)

		cases := []struct {
			name        string
			args        []string
			errContains string
		}{
			{
				name:        "NoCertAndKey",
				args:        []string{"--tls-enable"},
				errContains: "--tls-cert-file is required when tls is enabled",
			},
			{
				name:        "NoCert",
				args:        []string{"--tls-enable", "--tls-key-file", key1Path},
				errContains: "--tls-cert-file and --tls-key-file must be used the same amount of times",
			},
			{
				name:        "NoKey",
				args:        []string{"--tls-enable", "--tls-cert-file", cert1Path},
				errContains: "--tls-cert-file and --tls-key-file must be used the same amount of times",
			},
			{
				name:        "MismatchedCount",
				args:        []string{"--tls-enable", "--tls-cert-file", cert1Path, "--tls-key-file", key1Path, "--tls-cert-file", cert2Path},
				errContains: "--tls-cert-file and --tls-key-file must be used the same amount of times",
			},
			{
				name:        "MismatchedCertAndKey",
				args:        []string{"--tls-enable", "--tls-cert-file", cert1Path, "--tls-key-file", key2Path},
				errContains: "load TLS key pair",
			},
		}

		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				ctx, cancelFunc := context.WithCancel(context.Background())
				defer cancelFunc()

				args := []string{
					"server",
					"--in-memory",
					"--address", ":0",
					"--access-url", "http://example.com",
					"--cache-dir", t.TempDir(),
				}
				args = append(args, c.args...)
				root, _ := clitest.New(t, args...)
				err := root.ExecuteContext(ctx)
				require.Error(t, err)
				require.ErrorContains(t, err, c.errContains)
			})
		}
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
			"--access-url", "http://example.com",
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
	t.Run("TLSValidMultiple", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		cert1Path, key1Path := generateTLSCertificate(t, "alpaca.com")
		cert2Path, key2Path := generateTLSCertificate(t, "*.llama.com")
		root, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--address", ":0",
			"--access-url", "http://example.com",
			"--tls-enable",
			"--tls-cert-file", cert1Path,
			"--tls-key-file", key1Path,
			"--tls-cert-file", cert2Path,
			"--tls-key-file", key2Path,
			"--cache-dir", t.TempDir(),
		)
		errC := make(chan error, 1)
		go func() {
			errC <- root.ExecuteContext(ctx)
		}()
		accessURL := waitAccessURL(t, cfg)
		require.Equal(t, "https", accessURL.Scheme)
		originalHost := accessURL.Host

		var (
			expectAddr string
			dials      int64
		)
		client := codersdk.New(accessURL)
		client.HTTPClient = &http.Client{
			Transport: &http.Transport{
				DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					atomic.AddInt64(&dials, 1)
					assert.Equal(t, expectAddr, addr)

					host, _, err := net.SplitHostPort(addr)
					require.NoError(t, err)

					// Always connect to the accessURL ip:port regardless of
					// hostname.
					conn, err := tls.Dial(network, originalHost, &tls.Config{
						MinVersion: tls.VersionTLS12,
						//nolint:gosec
						InsecureSkipVerify: true,
						ServerName:         host,
					})
					if err != nil {
						return nil, err
					}

					// We can't call conn.VerifyHostname because it requires
					// that the certificates are valid, so we call
					// VerifyHostname on the first certificate instead.
					require.Len(t, conn.ConnectionState().PeerCertificates, 1)
					err = conn.ConnectionState().PeerCertificates[0].VerifyHostname(host)
					assert.NoError(t, err, "invalid cert common name")
					return conn, nil
				},
			},
		}
		defer client.HTTPClient.CloseIdleConnections()

		// Use the first certificate and hostname.
		client.URL.Host = "alpaca.com:443"
		expectAddr = "alpaca.com:443"
		_, err := client.HasFirstUser(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, atomic.LoadInt64(&dials))

		// Use the second certificate (wildcard) and hostname.
		client.URL.Host = "hi.llama.com:443"
		expectAddr = "hi.llama.com:443"
		_, err = client.HasFirstUser(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 2, atomic.LoadInt64(&dials))

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
			"--access-url", "http://example.com",
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
			"--access-url", "http://example.com",
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
			"--access-url", "http://example.com",
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
			"--access-url", "http://example.com",
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
			"--access-url", "http://example.com",
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

	t.Run("RateLimit", func(t *testing.T) {
		t.Parallel()

		t.Run("Default", func(t *testing.T) {
			t.Parallel()
			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			root, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--address", ":0",
				"--access-url", "http://example.com",
			)
			serverErr := make(chan error, 1)
			go func() {
				serverErr <- root.ExecuteContext(ctx)
			}()
			accessURL := waitAccessURL(t, cfg)
			client := codersdk.New(accessURL)

			resp, err := client.Request(ctx, http.MethodGet, "/api/v2/buildinfo", nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "512", resp.Header.Get("X-Ratelimit-Limit"))
			cancelFunc()
			<-serverErr
		})

		t.Run("Changed", func(t *testing.T) {
			t.Parallel()
			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			val := "100"
			root, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--address", ":0",
				"--access-url", "http://example.com",
				"--api-rate-limit", val,
			)
			serverErr := make(chan error, 1)
			go func() {
				serverErr <- root.ExecuteContext(ctx)
			}()
			accessURL := waitAccessURL(t, cfg)
			client := codersdk.New(accessURL)

			resp, err := client.Request(ctx, http.MethodGet, "/api/v2/buildinfo", nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, val, resp.Header.Get("X-Ratelimit-Limit"))
			cancelFunc()
			<-serverErr
		})

		t.Run("Disabled", func(t *testing.T) {
			t.Parallel()
			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			root, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--address", ":0",
				"--access-url", "http://example.com",
				"--api-rate-limit", "-1",
			)
			serverErr := make(chan error, 1)
			go func() {
				serverErr <- root.ExecuteContext(ctx)
			}()
			accessURL := waitAccessURL(t, cfg)
			client := codersdk.New(accessURL)

			resp, err := client.Request(ctx, http.MethodGet, "/api/v2/buildinfo", nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "", resp.Header.Get("X-Ratelimit-Limit"))
			cancelFunc()
			<-serverErr
		})
	})
}

func generateTLSCertificate(t testing.TB, commonName ...string) (certPath, keyPath string) {
	dir := t.TempDir()

	commonNameStr := "localhost"
	if len(commonName) > 0 {
		commonNameStr = commonName[0]
	}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
			CommonName:   commonNameStr,
		},
		DNSNames:  []string{commonNameStr},
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
