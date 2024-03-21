package cli_test

import (
	"bufio"
	"bytes"
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
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"gopkg.in/yaml.v3"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestReadExternalAuthProvidersFromEnv(t *testing.T) {
	t.Parallel()
	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		providers, err := cli.ReadExternalAuthProvidersFromEnv([]string{
			"CODER_EXTERNAL_AUTH_0_ID=1",
			"CODER_EXTERNAL_AUTH_0_TYPE=gitlab",
			"CODER_EXTERNAL_AUTH_1_ID=2",
			"CODER_EXTERNAL_AUTH_1_CLIENT_ID=sid",
			"CODER_EXTERNAL_AUTH_1_CLIENT_SECRET=hunter12",
			"CODER_EXTERNAL_AUTH_1_TOKEN_URL=google.com",
			"CODER_EXTERNAL_AUTH_1_VALIDATE_URL=bing.com",
			"CODER_EXTERNAL_AUTH_1_SCOPES=repo:read repo:write",
			"CODER_EXTERNAL_AUTH_1_NO_REFRESH=true",
			"CODER_EXTERNAL_AUTH_1_DISPLAY_NAME=Google",
			"CODER_EXTERNAL_AUTH_1_DISPLAY_ICON=/icon/google.svg",
		})
		require.NoError(t, err)
		require.Len(t, providers, 2)

		// Validate the first provider.
		assert.Equal(t, "1", providers[0].ID)
		assert.Equal(t, "gitlab", providers[0].Type)

		// Validate the second provider.
		assert.Equal(t, "2", providers[1].ID)
		assert.Equal(t, "sid", providers[1].ClientID)
		assert.Equal(t, "hunter12", providers[1].ClientSecret)
		assert.Equal(t, "google.com", providers[1].TokenURL)
		assert.Equal(t, "bing.com", providers[1].ValidateURL)
		assert.Equal(t, []string{"repo:read", "repo:write"}, providers[1].Scopes)
		assert.Equal(t, true, providers[1].NoRefresh)
		assert.Equal(t, "Google", providers[1].DisplayName)
		assert.Equal(t, "/icon/google.svg", providers[1].DisplayIcon)
	})
}

// TestReadGitAuthProvidersFromEnv ensures that the deprecated `CODER_GITAUTH_`
// environment variables are still supported.
func TestReadGitAuthProvidersFromEnv(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		providers, err := cli.ReadExternalAuthProvidersFromEnv([]string{
			"HOME=/home/frodo",
		})
		require.NoError(t, err)
		require.Empty(t, providers)
	})
	t.Run("InvalidKey", func(t *testing.T) {
		t.Parallel()
		providers, err := cli.ReadExternalAuthProvidersFromEnv([]string{
			"CODER_GITAUTH_XXX=invalid",
		})
		require.Error(t, err, "providers: %+v", providers)
		require.Empty(t, providers)
	})
	t.Run("SkipKey", func(t *testing.T) {
		t.Parallel()
		providers, err := cli.ReadExternalAuthProvidersFromEnv([]string{
			"CODER_GITAUTH_0_ID=invalid",
			"CODER_GITAUTH_2_ID=invalid",
		})
		require.Error(t, err, "%+v", providers)
		require.Empty(t, providers)
	})
	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		providers, err := cli.ReadExternalAuthProvidersFromEnv([]string{
			"CODER_GITAUTH_0_ID=1",
			"CODER_GITAUTH_0_TYPE=gitlab",
			"CODER_GITAUTH_1_ID=2",
			"CODER_GITAUTH_1_CLIENT_ID=sid",
			"CODER_GITAUTH_1_CLIENT_SECRET=hunter12",
			"CODER_GITAUTH_1_TOKEN_URL=google.com",
			"CODER_GITAUTH_1_VALIDATE_URL=bing.com",
			"CODER_GITAUTH_1_SCOPES=repo:read repo:write",
			"CODER_GITAUTH_1_NO_REFRESH=true",
		})
		require.NoError(t, err)
		require.Len(t, providers, 2)

		// Validate the first provider.
		assert.Equal(t, "1", providers[0].ID)
		assert.Equal(t, "gitlab", providers[0].Type)

		// Validate the second provider.
		assert.Equal(t, "2", providers[1].ID)
		assert.Equal(t, "sid", providers[1].ClientID)
		assert.Equal(t, "hunter12", providers[1].ClientSecret)
		assert.Equal(t, "google.com", providers[1].TokenURL)
		assert.Equal(t, "bing.com", providers[1].ValidateURL)
		assert.Equal(t, []string{"repo:read", "repo:write"}, providers[1].Scopes)
		assert.Equal(t, true, providers[1].NoRefresh)
	})
}

func TestServer(t *testing.T) {
	t.Parallel()

	t.Run("BuiltinPostgres", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.SkipNow()
		}

		inv, cfg := clitest.New(t,
			"server",
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--cache-dir", t.TempDir(),
		)

		const superDuperLong = testutil.WaitSuperLong * 3
		ctx := testutil.Context(t, superDuperLong)
		clitest.Start(t, inv.WithContext(ctx))

		//nolint:gocritic // Embedded postgres take a while to fire up.
		require.Eventually(t, func() bool {
			rawURL, err := cfg.URL().Read()
			return err == nil && rawURL != ""
		}, superDuperLong, testutil.IntervalFast, "failed to get access URL")
	})
	t.Run("BuiltinPostgresURL", func(t *testing.T) {
		t.Parallel()
		root, _ := clitest.New(t, "server", "postgres-builtin-url")
		pty := ptytest.New(t)
		root.Stdout = pty.Output()
		err := root.Run()
		require.NoError(t, err)

		pty.ExpectMatch("psql")
	})
	t.Run("BuiltinPostgresURLRaw", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		root, _ := clitest.New(t, "server", "postgres-builtin-url", "--raw-url")
		pty := ptytest.New(t)
		root.Stdout = pty.Output()
		err := root.WithContext(ctx).Run()
		require.NoError(t, err)

		got := pty.ReadLine(ctx)
		if !strings.HasPrefix(got, "postgres://") {
			t.Fatalf("expected postgres URL to start with \"postgres://\", got %q", got)
		}
	})

	// Validate that a warning is printed that it may not be externally
	// reachable.
	t.Run("LocalAccessURL", func(t *testing.T) {
		t.Parallel()
		inv, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "http://localhost:3000/",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("this may cause unexpected problems when creating workspaces")
		pty.ExpectMatch("View the Web UI: http://localhost:3000/")
	})

	// Validate that an https scheme is prepended to a remote access URL
	// and that a warning is printed for a host that cannot be resolved.
	t.Run("RemoteAccessURL", func(t *testing.T) {
		t.Parallel()

		inv, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "https://foobarbaz.mydomain",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("this may cause unexpected problems when creating workspaces")
		pty.ExpectMatch("View the Web UI: https://foobarbaz.mydomain")
	})

	t.Run("NoWarningWithRemoteAccessURL", func(t *testing.T) {
		t.Parallel()
		inv, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "https://google.com",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("View the Web UI: https://google.com")
	})

	t.Run("NoSchemeAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "google.com",
			"--cache-dir", t.TempDir(),
		)
		err := root.WithContext(ctx).Run()
		require.Error(t, err)
	})

	t.Run("TLSBadVersion", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", "",
			"--access-url", "http://example.com",
			"--tls-enable",
			"--tls-address", ":0",
			"--tls-min-version", "tls9",
			"--cache-dir", t.TempDir(),
		)
		err := root.WithContext(ctx).Run()
		require.Error(t, err)
	})
	t.Run("TLSBadClientAuth", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", "",
			"--access-url", "http://example.com",
			"--tls-enable",
			"--tls-address", ":0",
			"--tls-client-auth", "something",
			"--cache-dir", t.TempDir(),
		)
		err := root.WithContext(ctx).Run()
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
					"--http-address", ":0",
					"--access-url", "http://example.com",
					"--cache-dir", t.TempDir(),
				}
				args = append(args, c.args...)
				root, _ := clitest.New(t, args...)
				err := root.WithContext(ctx).Run()
				require.Error(t, err)
				t.Logf("args: %v", args)
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
			"--http-address", "",
			"--access-url", "https://example.com",
			"--tls-enable",
			"--tls-address", ":0",
			"--tls-cert-file", certPath,
			"--tls-key-file", keyPath,
			"--cache-dir", t.TempDir(),
		)
		clitest.Start(t, root.WithContext(ctx))

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
		defer client.HTTPClient.CloseIdleConnections()
		_, err := client.HasFirstUser(ctx)
		require.NoError(t, err)
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
			"--http-address", "",
			"--access-url", "https://example.com",
			"--tls-enable",
			"--tls-address", ":0",
			"--tls-cert-file", cert1Path,
			"--tls-key-file", key1Path,
			"--tls-cert-file", cert2Path,
			"--tls-key-file", key2Path,
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t)
		root.Stdout = pty.Output()
		clitest.Start(t, root.WithContext(ctx))

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
	})

	t.Run("TLSAndHTTP", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		certPath, keyPath := generateTLSCertificate(t)
		inv, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "https://example.com",
			"--tls-enable",
			"--tls-redirect-http-to-https=false",
			"--tls-address", ":0",
			"--tls-cert-file", certPath,
			"--tls-key-file", keyPath,
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		// We can't use waitAccessURL as it will only return the HTTP URL.
		const httpLinePrefix = "Started HTTP listener at"
		pty.ExpectMatch(httpLinePrefix)
		httpLine := pty.ReadLine(ctx)
		httpAddr := strings.TrimSpace(strings.TrimPrefix(httpLine, httpLinePrefix))
		require.NotEmpty(t, httpAddr)
		const tlsLinePrefix = "Started TLS/HTTPS listener at "
		pty.ExpectMatch(tlsLinePrefix)
		tlsLine := pty.ReadLine(ctx)
		tlsAddr := strings.TrimSpace(strings.TrimPrefix(tlsLine, tlsLinePrefix))
		require.NotEmpty(t, tlsAddr)

		// Verify HTTP
		httpURL, err := url.Parse(httpAddr)
		require.NoError(t, err)
		client := codersdk.New(httpURL)
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		_, err = client.HasFirstUser(ctx)
		require.NoError(t, err)

		// Verify TLS
		tlsURL, err := url.Parse(tlsAddr)
		require.NoError(t, err)
		client = codersdk.New(tlsURL)
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					//nolint:gosec
					InsecureSkipVerify: true,
				},
			},
		}
		defer client.HTTPClient.CloseIdleConnections()
		_, err = client.HasFirstUser(ctx)
		require.NoError(t, err)
	})

	t.Run("TLSRedirect", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name         string
			httpListener bool
			tlsListener  bool
			redirect     bool
			accessURL    string
			requestURL   string
			// Empty string means no redirect.
			expectRedirect string
		}{
			{
				name:           "OK",
				httpListener:   true,
				tlsListener:    true,
				redirect:       true,
				accessURL:      "https://example.com",
				expectRedirect: "https://example.com",
			},
			{
				name:           "NoRedirect",
				httpListener:   true,
				tlsListener:    true,
				accessURL:      "https://example.com",
				expectRedirect: "",
			},
			{
				name:           "NoRedirectWithWildcard",
				tlsListener:    true,
				accessURL:      "https://example.com",
				requestURL:     "https://dev.example.com",
				expectRedirect: "",
				redirect:       true,
			},
			{
				name:           "NoTLSListener",
				httpListener:   true,
				tlsListener:    false,
				accessURL:      "https://example.com",
				expectRedirect: "",
			},
			{
				name:           "NoHTTPListener",
				httpListener:   false,
				tlsListener:    true,
				accessURL:      "https://example.com",
				expectRedirect: "",
			},
		}

		for _, c := range cases {
			c := c

			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancelFunc := context.WithCancel(context.Background())
				defer cancelFunc()

				if c.requestURL == "" {
					c.requestURL = c.accessURL
				}

				httpListenAddr := ""
				if c.httpListener {
					httpListenAddr = ":0"
				}

				certPath, keyPath := generateTLSCertificate(t)
				flags := []string{
					"server",
					"--in-memory",
					"--cache-dir", t.TempDir(),
					"--http-address", httpListenAddr,
				}
				if c.tlsListener {
					flags = append(flags,
						"--tls-enable",
						"--tls-address", ":0",
						"--tls-cert-file", certPath,
						"--tls-key-file", keyPath,
						"--wildcard-access-url", "*.example.com",
					)
				}
				if c.accessURL != "" {
					flags = append(flags, "--access-url", c.accessURL)
				}
				if c.redirect {
					flags = append(flags, "--redirect-to-access-url")
				}

				inv, _ := clitest.New(t, flags...)
				pty := ptytest.New(t)
				pty.Attach(inv)

				clitest.Start(t, inv)

				var (
					httpAddr string
					tlsAddr  string
				)
				// We can't use waitAccessURL as it will only return the HTTP URL.
				if c.httpListener {
					const httpLinePrefix = "Started HTTP listener at"
					pty.ExpectMatch(httpLinePrefix)
					httpLine := pty.ReadLine(ctx)
					httpAddr = strings.TrimSpace(strings.TrimPrefix(httpLine, httpLinePrefix))
					require.NotEmpty(t, httpAddr)
				}
				if c.tlsListener {
					const tlsLinePrefix = "Started TLS/HTTPS listener at"
					pty.ExpectMatch(tlsLinePrefix)
					tlsLine := pty.ReadLine(ctx)
					tlsAddr = strings.TrimSpace(strings.TrimPrefix(tlsLine, tlsLinePrefix))
					require.NotEmpty(t, tlsAddr)
				}

				// Verify HTTP redirects (or not)
				if c.httpListener {
					httpURL, err := url.Parse(httpAddr)
					require.NoError(t, err)
					client := codersdk.New(httpURL)
					client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
						return http.ErrUseLastResponse
					}
					resp, err := client.Request(ctx, http.MethodGet, "/api/v2/buildinfo", nil)
					require.NoError(t, err)
					defer resp.Body.Close()
					if c.expectRedirect == "" {
						require.Equal(t, http.StatusOK, resp.StatusCode)
					} else {
						require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
						require.Equal(t, c.expectRedirect, resp.Header.Get("Location"))
					}

					// We should never readirect /healthz
					respHealthz, err := client.Request(ctx, http.MethodGet, "/healthz", nil)
					require.NoError(t, err)
					defer respHealthz.Body.Close()
					require.Equal(t, http.StatusOK, respHealthz.StatusCode, "/healthz should never redirect")

					// We should never redirect DERP
					respDERP, err := client.Request(ctx, http.MethodGet, "/derp", nil)
					require.NoError(t, err)
					defer respDERP.Body.Close()
					require.Equal(t, http.StatusUpgradeRequired, respDERP.StatusCode, "/derp should never redirect")
				}

				// Verify TLS
				if c.tlsListener {
					accessURLParsed, err := url.Parse(c.requestURL)
					require.NoError(t, err)
					client := &http.Client{
						CheckRedirect: func(req *http.Request, via []*http.Request) error {
							return http.ErrUseLastResponse
						},
						Transport: &http.Transport{
							DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
								return tls.Dial(network, strings.TrimPrefix(tlsAddr, "https://"), &tls.Config{
									// nolint:gosec
									InsecureSkipVerify: true,
								})
							},
						},
					}
					defer client.CloseIdleConnections()

					req, err := http.NewRequestWithContext(ctx, http.MethodGet, accessURLParsed.String(), nil)
					require.NoError(t, err)
					resp, err := client.Do(req)
					// We don't care much about the response, just that TLS
					// worked.
					require.NoError(t, err)
					defer resp.Body.Close()
				}
			})
		}
	})

	t.Run("CanListenUnspecifiedv4", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", "0.0.0.0:0",
			"--access-url", "http://example.com",
		)

		pty := ptytest.New(t)
		root.Stdout = pty.Output()
		root.Stderr = pty.Output()
		serverStop := make(chan error, 1)
		go func() {
			err := root.WithContext(ctx).Run()
			if err != nil {
				t.Error(err)
			}
			close(serverStop)
		}()

		pty.ExpectMatch("Started HTTP listener")
		pty.ExpectMatch("http://0.0.0.0:")

		cancelFunc()
		<-serverStop
	})

	t.Run("CanListenUnspecifiedv6", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", "[::]:0",
			"--access-url", "http://example.com",
		)

		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		pty.ExpectMatch("Started HTTP listener at")
		pty.ExpectMatch("http://[::]:")
	})

	t.Run("NoAddress", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		inv, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":80",
			"--tls-enable=false",
			"--tls-address", "",
		)
		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "tls-address")
	})

	t.Run("NoTLSAddress", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		inv, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--tls-enable=true",
			"--tls-address", "",
		)
		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "must not be empty")
	})

	// DeprecatedAddress is a test for the deprecated --address flag. If
	// specified, --http-address and --tls-address are both ignored, a warning
	// is printed, and the server will either be HTTP-only or TLS-only depending
	// on if --tls-enable is set.
	t.Run("DeprecatedAddress", func(t *testing.T) {
		t.Parallel()

		t.Run("HTTP", func(t *testing.T) {
			t.Parallel()
			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			inv, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--address", ":0",
				"--access-url", "http://example.com",
				"--cache-dir", t.TempDir(),
			)
			pty := ptytest.New(t)
			inv.Stdout = pty.Output()
			inv.Stderr = pty.Output()
			clitest.Start(t, inv.WithContext(ctx))

			pty.ExpectMatch("is deprecated")

			accessURL := waitAccessURL(t, cfg)
			require.Equal(t, "http", accessURL.Scheme)
			client := codersdk.New(accessURL)
			_, err := client.HasFirstUser(ctx)
			require.NoError(t, err)
		})

		t.Run("TLS", func(t *testing.T) {
			t.Parallel()
			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			certPath, keyPath := generateTLSCertificate(t)
			root, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--address", ":0",
				"--access-url", "https://example.com",
				"--tls-enable",
				"--tls-cert-file", certPath,
				"--tls-key-file", keyPath,
				"--cache-dir", t.TempDir(),
			)
			pty := ptytest.New(t)
			root.Stdout = pty.Output()
			root.Stderr = pty.Output()
			clitest.Start(t, root.WithContext(ctx))

			pty.ExpectMatch("is deprecated")

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
			defer client.HTTPClient.CloseIdleConnections()
			_, err := client.HasFirstUser(ctx)
			require.NoError(t, err)
		})
	})

	t.Run("TracerNoLeak", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--trace=true",
			"--cache-dir", t.TempDir(),
		)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		clitest.Start(t, inv.WithContext(ctx))
		cancel()
		require.Error(t, goleak.Find())
	})
	t.Run("Telemetry", func(t *testing.T) {
		t.Parallel()

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

		inv, _ := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--telemetry",
			"--telemetry-url", server.URL,
			"--cache-dir", t.TempDir(),
		)
		clitest.Start(t, inv)

		<-deployment
		<-snapshot
	})
	t.Run("Prometheus", func(t *testing.T) {
		t.Parallel()

		t.Run("DBMetricsDisabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			randPort := testutil.RandomPort(t)
			inv, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons", "1",
				"--prometheus-enable",
				"--prometheus-address", ":"+strconv.Itoa(randPort),
				// "--prometheus-collect-db-metrics", // disabled by default
				"--cache-dir", t.TempDir(),
			)

			clitest.Start(t, inv)
			_ = waitAccessURL(t, cfg)

			var res *http.Response
			require.Eventually(t, func() bool {
				req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d", randPort), nil)
				assert.NoError(t, err)
				// nolint:bodyclose
				res, err = http.DefaultClient.Do(req)
				return err == nil
			}, testutil.WaitShort, testutil.IntervalFast)
			defer res.Body.Close()

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
				if strings.HasPrefix(scanner.Text(), "coderd_db_query_latencies_seconds") {
					t.Fatal("db metrics should not be tracked when --prometheus-collect-db-metrics is not enabled")
				}
				t.Logf("scanned %s", scanner.Text())
			}
			require.NoError(t, scanner.Err())
			require.True(t, hasActiveUsers)
			require.True(t, hasWorkspaces)
		})

		t.Run("DBMetricsEnabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			randPort := testutil.RandomPort(t)
			inv, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons", "1",
				"--prometheus-enable",
				"--prometheus-address", ":"+strconv.Itoa(randPort),
				"--prometheus-collect-db-metrics",
				"--cache-dir", t.TempDir(),
			)

			clitest.Start(t, inv)
			_ = waitAccessURL(t, cfg)

			var res *http.Response
			require.Eventually(t, func() bool {
				req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d", randPort), nil)
				assert.NoError(t, err)
				// nolint:bodyclose
				res, err = http.DefaultClient.Do(req)
				return err == nil
			}, testutil.WaitShort, testutil.IntervalFast)
			defer res.Body.Close()

			scanner := bufio.NewScanner(res.Body)
			hasDBMetrics := false
			for scanner.Scan() {
				if strings.HasPrefix(scanner.Text(), "coderd_db_query_latencies_seconds") {
					hasDBMetrics = true
				}
				t.Logf("scanned %s", scanner.Text())
			}
			require.NoError(t, scanner.Err())
			require.True(t, hasDBMetrics)
		})
	})
	t.Run("GitHubOAuth", func(t *testing.T) {
		t.Parallel()

		fakeRedirect := "https://fake-url.com"
		inv, cfg := clitest.New(t,
			"server",
			"--in-memory",
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--oauth2-github-allow-everyone",
			"--oauth2-github-client-id", "fake",
			"--oauth2-github-client-secret", "fake",
			"--oauth2-github-enterprise-base-url", fakeRedirect,
		)
		clitest.Start(t, inv)
		accessURL := waitAccessURL(t, cfg)
		client := codersdk.New(accessURL)
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		githubURL, err := accessURL.Parse("/api/v2/users/oauth2/github")
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(inv.Context(), http.MethodGet, githubURL.String(), nil)
		require.NoError(t, err)
		res, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		fakeURL, err := res.Location()
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(fakeURL.String(), fakeRedirect), fakeURL.String())
	})

	t.Run("OIDC", func(t *testing.T) {
		t.Parallel()

		t.Run("Defaults", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
			defer cancel()

			// Startup a fake server that just responds to .well-known/openid-configuration
			// This is just needed to get Coder to start up.
			oidcServer := httptest.NewServer(nil)
			fakeWellKnownHandler := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				payload := fmt.Sprintf("{\"issuer\": %q}", oidcServer.URL)
				_, _ = w.Write([]byte(payload))
			}
			oidcServer.Config.Handler = http.HandlerFunc(fakeWellKnownHandler)
			t.Cleanup(oidcServer.Close)

			inv, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--oidc-client-id", "fake",
				"--oidc-client-secret", "fake",
				"--oidc-issuer-url", oidcServer.URL,
				// Leaving the rest of the flags as defaults.
			)

			// Ensure that the server starts up without error.
			clitest.Start(t, inv)
			accessURL := waitAccessURL(t, cfg)
			client := codersdk.New(accessURL)

			randPassword, err := cryptorand.String(24)
			require.NoError(t, err)

			_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
				Email:    "admin@coder.com",
				Password: randPassword,
				Username: "admin",
				Trial:    true,
			})
			require.NoError(t, err)

			loginResp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
				Email:    "admin@coder.com",
				Password: randPassword,
			})
			require.NoError(t, err)
			client.SetSessionToken(loginResp.SessionToken)

			deploymentConfig, err := client.DeploymentConfig(ctx)
			require.NoError(t, err)

			// Ensure that the OIDC provider is configured correctly.
			require.Equal(t, "fake", deploymentConfig.Values.OIDC.ClientID.Value())
			// The client secret is not returned from the API.
			require.Empty(t, deploymentConfig.Values.OIDC.ClientSecret.Value())
			require.Equal(t, oidcServer.URL, deploymentConfig.Values.OIDC.IssuerURL.Value())
			// These are the default values returned from the API. See codersdk/deployment.go for the default values.
			require.True(t, deploymentConfig.Values.OIDC.AllowSignups.Value())
			require.Empty(t, deploymentConfig.Values.OIDC.EmailDomain.Value())
			require.Equal(t, []string{"openid", "profile", "email"}, deploymentConfig.Values.OIDC.Scopes.Value())
			require.False(t, deploymentConfig.Values.OIDC.IgnoreEmailVerified.Value())
			require.Equal(t, "preferred_username", deploymentConfig.Values.OIDC.UsernameField.Value())
			require.Equal(t, "email", deploymentConfig.Values.OIDC.EmailField.Value())
			require.Equal(t, map[string]string{"access_type": "offline"}, deploymentConfig.Values.OIDC.AuthURLParams.Value)
			require.False(t, deploymentConfig.Values.OIDC.IgnoreUserInfo.Value())
			require.Empty(t, deploymentConfig.Values.OIDC.GroupField.Value())
			require.Empty(t, deploymentConfig.Values.OIDC.GroupMapping.Value)
			require.Empty(t, deploymentConfig.Values.OIDC.UserRoleField.Value())
			require.Empty(t, deploymentConfig.Values.OIDC.UserRoleMapping.Value)
			require.Equal(t, "OpenID Connect", deploymentConfig.Values.OIDC.SignInText.Value())
			require.Empty(t, deploymentConfig.Values.OIDC.IconURL.Value())
		})

		t.Run("Overrides", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
			defer cancel()

			// Startup a fake server that just responds to .well-known/openid-configuration
			// This is just needed to get Coder to start up.
			oidcServer := httptest.NewServer(nil)
			fakeWellKnownHandler := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				payload := fmt.Sprintf("{\"issuer\": %q}", oidcServer.URL)
				_, _ = w.Write([]byte(payload))
			}
			oidcServer.Config.Handler = http.HandlerFunc(fakeWellKnownHandler)
			t.Cleanup(oidcServer.Close)

			inv, cfg := clitest.New(t,
				"server",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--oidc-client-id", "fake",
				"--oidc-client-secret", "fake",
				"--oidc-issuer-url", oidcServer.URL,
				// The following values have defaults that we want to override.
				"--oidc-allow-signups=false",
				"--oidc-email-domain", "example.com",
				"--oidc-scopes", "360noscope",
				"--oidc-ignore-email-verified",
				"--oidc-username-field", "not_preferred_username",
				"--oidc-email-field", "not_email",
				"--oidc-auth-url-params", `{"prompt":"consent"}`,
				"--oidc-ignore-userinfo",
				"--oidc-group-field", "serious_business_unit",
				"--oidc-group-mapping", `{"serious_business_unit": "serious_business_unit"}`,
				"--oidc-sign-in-text", "Sign In With Coder",
				"--oidc-icon-url", "https://example.com/icon.png",
			)

			// Ensure that the server starts up without error.
			clitest.Start(t, inv)
			accessURL := waitAccessURL(t, cfg)
			client := codersdk.New(accessURL)

			randPassword, err := cryptorand.String(24)
			require.NoError(t, err)

			_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
				Email:    "admin@coder.com",
				Password: randPassword,
				Username: "admin",
				Trial:    true,
			})
			require.NoError(t, err)

			loginResp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
				Email:    "admin@coder.com",
				Password: randPassword,
			})
			require.NoError(t, err)
			client.SetSessionToken(loginResp.SessionToken)

			deploymentConfig, err := client.DeploymentConfig(ctx)
			require.NoError(t, err)

			// Ensure that the OIDC provider is configured correctly.
			require.Equal(t, "fake", deploymentConfig.Values.OIDC.ClientID.Value())
			// The client secret is not returned from the API.
			require.Empty(t, deploymentConfig.Values.OIDC.ClientSecret.Value())
			require.Equal(t, oidcServer.URL, deploymentConfig.Values.OIDC.IssuerURL.Value())
			// These are values that we want to make sure were overridden.
			require.False(t, deploymentConfig.Values.OIDC.AllowSignups.Value())
			require.Equal(t, []string{"example.com"}, deploymentConfig.Values.OIDC.EmailDomain.Value())
			require.Equal(t, []string{"360noscope"}, deploymentConfig.Values.OIDC.Scopes.Value())
			require.True(t, deploymentConfig.Values.OIDC.IgnoreEmailVerified.Value())
			require.Equal(t, "not_preferred_username", deploymentConfig.Values.OIDC.UsernameField.Value())
			require.Equal(t, "not_email", deploymentConfig.Values.OIDC.EmailField.Value())
			require.True(t, deploymentConfig.Values.OIDC.IgnoreUserInfo.Value())
			require.Equal(t, map[string]string{"prompt": "consent"}, deploymentConfig.Values.OIDC.AuthURLParams.Value)
			require.Equal(t, "serious_business_unit", deploymentConfig.Values.OIDC.GroupField.Value())
			require.Equal(t, map[string]string{"serious_business_unit": "serious_business_unit"}, deploymentConfig.Values.OIDC.GroupMapping.Value)
			require.Equal(t, "Sign In With Coder", deploymentConfig.Values.OIDC.SignInText.Value())
			require.Equal(t, "https://example.com/icon.png", deploymentConfig.Values.OIDC.IconURL.Value().String())

			// Verify the option values
			for _, opt := range deploymentConfig.Options {
				switch opt.Flag {
				case "access-url":
					require.Equal(t, "http://example.com", opt.Value.String())
				case "oidc-icon-url":
					require.Equal(t, "https://example.com/icon.png", opt.Value.String())
				case "oidc-sign-in-text":
					require.Equal(t, "Sign In With Coder", opt.Value.String())
				case "redirect-to-access-url":
					require.Equal(t, "false", opt.Value.String())
				case "derp-server-region-id":
					require.Equal(t, "999", opt.Value.String())
				}
			}
		})
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
				"--http-address", ":0",
				"--access-url", "http://example.com",
			)
			serverErr := make(chan error, 1)
			go func() {
				serverErr <- root.WithContext(ctx).Run()
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
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--api-rate-limit", val,
			)
			serverErr := make(chan error, 1)
			go func() {
				serverErr <- root.WithContext(ctx).Run()
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
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--api-rate-limit", "-1",
			)
			serverErr := make(chan error, 1)
			go func() {
				serverErr <- root.WithContext(ctx).Run()
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

	waitFile := func(t *testing.T, fiName string, dur time.Duration) {
		var lastStat os.FileInfo
		require.Eventually(t, func() bool {
			var err error
			lastStat, err = os.Stat(fiName)
			if err != nil {
				if !os.IsNotExist(err) {
					t.Fatalf("unexpected error: %v", err)
				}
				return false
			}
			return lastStat.Size() > 0
		},
			testutil.WaitShort,
			testutil.IntervalFast,
			"file at %s should exist, last stat: %+v",
			fiName, lastStat,
		)
	}

	t.Run("Logging", func(t *testing.T) {
		t.Parallel()

		t.Run("CreatesFile", func(t *testing.T) {
			t.Parallel()
			fiName := testutil.TempFile(t, "", "coder-logging-test-*")

			root, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons-echo",
				"--log-human", fiName,
			)
			clitest.Start(t, root)

			waitFile(t, fiName, testutil.WaitLong)
		})

		t.Run("Human", func(t *testing.T) {
			t.Parallel()
			fi := testutil.TempFile(t, "", "coder-logging-test-*")

			root, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons-echo",
				"--log-human", fi,
			)
			clitest.Start(t, root)

			waitFile(t, fi, testutil.WaitShort)
		})

		t.Run("JSON", func(t *testing.T) {
			t.Parallel()
			fi := testutil.TempFile(t, "", "coder-logging-test-*")

			root, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons-echo",
				"--log-json", fi,
			)
			clitest.Start(t, root)

			waitFile(t, fi, testutil.WaitShort)
		})

		t.Run("Stackdriver", func(t *testing.T) {
			t.Parallel()
			ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
			defer cancelFunc()

			fi := testutil.TempFile(t, "", "coder-logging-test-*")

			inv, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons-echo",
				"--log-stackdriver", fi,
			)
			// Attach pty so we get debug output from the command if this test
			// fails.
			pty := ptytest.New(t).Attach(inv)

			clitest.Start(t, inv.WithContext(ctx))

			// Wait for server to listen on HTTP, this is a good
			// starting point for expecting logs.
			_ = pty.ExpectMatchContext(ctx, "Started HTTP listener at")

			waitFile(t, fi, testutil.WaitSuperLong)
		})

		t.Run("Multiple", func(t *testing.T) {
			t.Parallel()
			ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
			defer cancelFunc()

			fi1 := testutil.TempFile(t, "", "coder-logging-test-*")
			fi2 := testutil.TempFile(t, "", "coder-logging-test-*")
			fi3 := testutil.TempFile(t, "", "coder-logging-test-*")

			// NOTE(mafredri): This test might end up downloading Terraform
			// which can take a long time and end up failing the test.
			// This is why we wait extra long below for server to listen on
			// HTTP.
			inv, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons-echo",
				"--log-human", fi1,
				"--log-json", fi2,
				"--log-stackdriver", fi3,
			)
			// Attach pty so we get debug output from the command if this test
			// fails.
			pty := ptytest.New(t).Attach(inv)

			clitest.Start(t, inv)

			// Wait for server to listen on HTTP, this is a good
			// starting point for expecting logs.
			_ = pty.ExpectMatchContext(ctx, "Started HTTP listener at")

			waitFile(t, fi1, testutil.WaitSuperLong)
			waitFile(t, fi2, testutil.WaitSuperLong)
			waitFile(t, fi3, testutil.WaitSuperLong)
		})
	})

	t.Run("YAML", func(t *testing.T) {
		t.Parallel()

		t.Run("WriteThenReadConfig", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			args := []string{
				"server",
				"--in-memory",
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--log-human", filepath.Join(t.TempDir(), "coder-logging-test-human"),
				// We use ecdsa here because it's the fastest alternative algorithm.
				"--ssh-keygen-algorithm", "ecdsa",
				"--cache-dir", t.TempDir(),
			}

			// First, we get the base config as set via flags (like users before
			// migrating).
			inv, cfg := clitest.New(t,
				args...,
			)
			ptytest.New(t).Attach(inv)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)
			gotURL := waitAccessURL(t, cfg)
			client := codersdk.New(gotURL)

			_ = coderdtest.CreateFirstUser(t, client)
			wantConfig, err := client.DeploymentConfig(ctx)
			require.NoError(t, err)
			cancel()
			w.RequireSuccess()

			// Next, we instruct the same server to display the YAML config
			// and then save it.
			inv = inv.WithContext(testutil.Context(t, testutil.WaitMedium))
			inv.Args = append(args, "--write-config")
			fi, err := os.OpenFile(testutil.TempFile(t, "", "coder-config-test-*"), os.O_WRONLY|os.O_CREATE, 0o600)
			require.NoError(t, err)
			defer fi.Close()
			var conf bytes.Buffer
			inv.Stdout = io.MultiWriter(fi, &conf)
			t.Logf("%+v", inv.Args)
			err = inv.Run()
			require.NoError(t, err)

			// Reset the context.
			ctx = testutil.Context(t, testutil.WaitMedium)
			// Finally, we restart the server with just the config and no flags
			// and ensure that the live configuration is equivalent.
			inv, cfg = clitest.New(t, "server", "--config="+fi.Name())
			w = clitest.StartWithWaiter(t, inv)
			client = codersdk.New(waitAccessURL(t, cfg))
			_ = coderdtest.CreateFirstUser(t, client)
			gotConfig, err := client.DeploymentConfig(ctx)
			require.NoError(t, err, "config:\n%s\nargs: %+v", conf.String(), inv.Args)
			gotConfig.Options.ByName("Config Path").Value.Set("")
			// We check the options individually for better error messages.
			for i := range wantConfig.Options {
				// ValueSource is not going to be correct on the `want`, so just
				// match that field.
				wantConfig.Options[i].ValueSource = gotConfig.Options[i].ValueSource

				// If there is a wrapped value with a validator, unwrap it.
				// The underlying doesn't compare well since it compares go pointers,
				// and not the actual value.
				if validator, isValidator := wantConfig.Options[i].Value.(interface{ Underlying() pflag.Value }); isValidator {
					wantConfig.Options[i].Value = validator.Underlying()
				}

				if validator, isValidator := gotConfig.Options[i].Value.(interface{ Underlying() pflag.Value }); isValidator {
					gotConfig.Options[i].Value = validator.Underlying()
				}

				assert.Equal(
					t, wantConfig.Options[i],
					gotConfig.Options[i],
					"option %q",
					wantConfig.Options[i].Name,
				)
			}
			w.Cancel()
			w.RequireSuccess()
		})
	})
}

func TestServer_Production(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" || testing.Short() {
		// Skip on non-Linux because it spawns a PostgreSQL instance.
		t.SkipNow()
	}
	connectionURL, closeFunc, err := dbtestutil.Open()
	require.NoError(t, err)
	defer closeFunc()

	// Postgres + race detector + CI = slow.
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitSuperLong*3)
	defer cancelFunc()

	inv, cfg := clitest.New(t,
		"server",
		"--http-address", ":0",
		"--access-url", "http://example.com",
		"--postgres-url", connectionURL,
		"--cache-dir", t.TempDir(),
	)
	clitest.Start(t, inv.WithContext(ctx))
	accessURL := waitAccessURL(t, cfg)
	client := codersdk.New(accessURL)

	_, err = client.CreateFirstUser(ctx, coderdtest.FirstUserParams)
	require.NoError(t, err)
}

//nolint:tparallel,paralleltest // This test cannot be run in parallel due to signal handling.
func TestServer_InterruptShutdown(t *testing.T) {
	t.Skip("This test issues an interrupt signal which will propagate to the test runner.")

	if runtime.GOOS == "windows" {
		// Sending interrupt signal isn't supported on Windows!
		t.SkipNow()
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	root, cfg := clitest.New(t,
		"server",
		"--in-memory",
		"--http-address", ":0",
		"--access-url", "http://example.com",
		"--provisioner-daemons", "1",
		"--cache-dir", t.TempDir(),
	)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- root.WithContext(ctx).Run()
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
}

func TestServer_GracefulShutdown(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		// Sending interrupt signal isn't supported on Windows!
		t.SkipNow()
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	root, cfg := clitest.New(t,
		"server",
		"--in-memory",
		"--http-address", ":0",
		"--access-url", "http://example.com",
		"--provisioner-daemons", "1",
		"--cache-dir", t.TempDir(),
	)
	var stopFunc context.CancelFunc
	root = root.WithTestSignalNotifyContext(t, func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
		if !reflect.DeepEqual(cli.StopSignalsNoInterrupt, signals) {
			return context.WithCancel(ctx)
		}
		var ctx context.Context
		ctx, stopFunc = context.WithCancel(parent)
		return ctx, stopFunc
	})
	serverErr := make(chan error, 1)
	pty := ptytest.New(t).Attach(root)
	go func() {
		serverErr <- root.WithContext(ctx).Run()
	}()
	_ = waitAccessURL(t, cfg)
	// It's fair to assume `stopFunc` isn't nil here, because the server
	// has started and access URL is propagated.
	stopFunc()
	pty.ExpectMatch("waiting for provisioner jobs to complete")
	err := <-serverErr
	require.NoError(t, err)
}

func BenchmarkServerHelp(b *testing.B) {
	// server --help is a good proxy for measuring the
	// constant overhead of each command.

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inv, _ := clitest.New(b, "server", "--help")
		inv.Stdout = io.Discard
		inv.Stderr = io.Discard
		err := inv.Run()
		require.NoError(b, err)
	}
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

func TestServerYAMLConfig(t *testing.T) {
	t.Parallel()

	var deployValues codersdk.DeploymentValues
	opts := deployValues.Options()

	err := opts.SetDefaults()
	require.NoError(t, err)

	n, err := opts.MarshalYAML()
	require.NoError(t, err)

	// Sanity-check that we can read the config back in.
	err = opts.UnmarshalYAML(n.(*yaml.Node))
	require.NoError(t, err)

	var wantBuf bytes.Buffer
	enc := yaml.NewEncoder(&wantBuf)
	enc.SetIndent(2)
	err = enc.Encode(n)
	require.NoError(t, err)

	wantByt := wantBuf.Bytes()

	goldenPath := filepath.Join("testdata", "server-config.yaml.golden")

	wantByt = clitest.NormalizeGoldenFile(t, wantByt)
	if *clitest.UpdateGoldenFiles {
		require.NoError(t, os.WriteFile(goldenPath, wantByt, 0o600))
		return
	}

	got, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	got = clitest.NormalizeGoldenFile(t, got)

	require.Equal(t, string(wantByt), string(got))
}

func TestConnectToPostgres(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test does not make sense without postgres")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	t.Cleanup(cancel)

	log := slogtest.Make(t, nil)

	dbURL, closeFunc, err := dbtestutil.Open()
	require.NoError(t, err)
	t.Cleanup(closeFunc)

	sqlDB, err := cli.ConnectToPostgres(ctx, log, "postgres", dbURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	require.NoError(t, sqlDB.PingContext(ctx))
}

func TestServer_InvalidDERP(t *testing.T) {
	t.Parallel()

	// Try to start a server with the built-in DERP server disabled and no
	// external DERP map.
	inv, _ := clitest.New(t,
		"server",
		"--in-memory",
		"--http-address", ":0",
		"--access-url", "http://example.com",
		"--derp-server-enable=false",
		"--derp-server-stun-addresses", "disable",
		"--block-direct-connections",
	)
	err := inv.Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "A valid DERP map is required for networking to work")
}
