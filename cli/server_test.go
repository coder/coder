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
	"regexp"
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
	"tailscale.com/derp/derphttp"
	"tailscale.com/types/key"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func dbArg(t *testing.T) string {
	dbURL, err := dbtestutil.Open(t)
	require.NoError(t, err)
	return "--postgres-url=" + dbURL
}

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
	t.Run("EphemeralDeployment", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.SkipNow()
		}

		inv, _ := clitest.New(t,
			"server",
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--ephemeral",
		)
		pty := ptytest.New(t).Attach(inv)

		// Embedded postgres takes a while to fire up.
		const superDuperLong = testutil.WaitSuperLong * 3
		ctx, cancelFunc := context.WithCancel(testutil.Context(t, superDuperLong))
		errCh := make(chan error, 1)
		go func() {
			errCh <- inv.WithContext(ctx).Run()
		}()
		matchCh1 := make(chan string, 1)
		go func() {
			matchCh1 <- pty.ExpectMatchContext(ctx, "Using an ephemeral deployment directory")
		}()
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-matchCh1:
			// OK!
		}
		rootDirLine := pty.ReadLine(ctx)
		rootDir := strings.TrimPrefix(rootDirLine, "Using an ephemeral deployment directory")
		rootDir = strings.TrimSpace(rootDir)
		rootDir = strings.TrimPrefix(rootDir, "(")
		rootDir = strings.TrimSuffix(rootDir, ")")
		require.NotEmpty(t, rootDir)
		require.DirExists(t, rootDir)

		matchCh2 := make(chan string, 1)
		go func() {
			// The "View the Web UI" log is a decent indicator that the server was successfully started.
			matchCh2 <- pty.ExpectMatchContext(ctx, "View the Web UI")
		}()
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-matchCh2:
			// OK!
		}

		cancelFunc()
		<-errCh

		require.NoDirExists(t, rootDir)
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
	t.Run("SpammyLogs", func(t *testing.T) {
		// The purpose of this test is to ensure we don't show excessive logs when the server starts.
		t.Parallel()
		inv, cfg := clitest.New(t,
			"server",
			dbArg(t),
			"--http-address", ":0",
			"--access-url", "http://localhost:3000/",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, pty.Resize(20, 80))
		clitest.Start(t, inv)

		// Wait for startup
		_ = waitAccessURL(t, cfg)

		// Wait a bit for more logs to be printed.
		time.Sleep(testutil.WaitShort)

		// Lines containing these strings are printed because we're
		// running the server with a test config. They wouldn't be
		// normally shown to the user, so we'll ignore them.
		ignoreLines := []string{
			"isn't externally reachable",
			"open install.sh: file does not exist",
			"telemetry disabled, unable to notify of security issues",
			"installed terraform version newer than expected",
		}

		countLines := func(fullOutput string) int {
			terminalWidth := 80
			linesByNewline := strings.Split(fullOutput, "\n")
			countByWidth := 0
		lineLoop:
			for _, line := range linesByNewline {
				for _, ignoreLine := range ignoreLines {
					if strings.Contains(line, ignoreLine) {
						t.Logf("Ignoring: %q", line)
						continue lineLoop
					}
				}
				t.Logf("Counting: %q", line)
				if line == "" {
					// Empty lines take up one line.
					countByWidth++
				} else {
					countByWidth += (len(line) + terminalWidth - 1) / terminalWidth
				}
			}
			return countByWidth
		}

		out := pty.ReadAll()
		numLines := countLines(string(out))
		t.Logf("numLines: %d", numLines)
		require.Less(t, numLines, 20, "expected less than 20 lines of output (terminal width 80), got %d", numLines)
	})

	t.Run("OAuth2GitHubDefaultProvider", func(t *testing.T) {
		type testCase struct {
			name                                  string
			githubDefaultProviderEnabled          string
			githubClientID                        string
			githubClientSecret                    string
			allowedOrg                            string
			expectGithubEnabled                   bool
			expectGithubDefaultProviderConfigured bool
			createUserPreStart                    bool
			createUserPostRestart                 bool
		}

		runGitHubProviderTest := func(t *testing.T, tc testCase) {
			t.Parallel()
			if !dbtestutil.WillUsePostgres() {
				t.Skip("test requires postgres")
			}

			ctx, cancelFunc := context.WithCancel(testutil.Context(t, testutil.WaitLong))
			defer cancelFunc()

			dbURL, err := dbtestutil.Open(t)
			require.NoError(t, err)
			db, _ := dbtestutil.NewDB(t, dbtestutil.WithURL(dbURL))

			if tc.createUserPreStart {
				_ = dbgen.User(t, db, database.User{})
			}

			args := []string{
				"server",
				"--postgres-url", dbURL,
				"--http-address", ":0",
				"--access-url", "https://example.com",
			}
			if tc.githubClientID != "" {
				args = append(args, fmt.Sprintf("--oauth2-github-client-id=%s", tc.githubClientID))
			}
			if tc.githubClientSecret != "" {
				args = append(args, fmt.Sprintf("--oauth2-github-client-secret=%s", tc.githubClientSecret))
			}
			if tc.githubClientID != "" || tc.githubClientSecret != "" {
				args = append(args, "--oauth2-github-allow-everyone")
			}
			if tc.githubDefaultProviderEnabled != "" {
				args = append(args, fmt.Sprintf("--oauth2-github-default-provider-enable=%s", tc.githubDefaultProviderEnabled))
			}
			if tc.allowedOrg != "" {
				args = append(args, fmt.Sprintf("--oauth2-github-allowed-orgs=%s", tc.allowedOrg))
			}
			inv, cfg := clitest.New(t, args...)
			errChan := make(chan error, 1)
			go func() {
				errChan <- inv.WithContext(ctx).Run()
			}()
			accessURLChan := make(chan *url.URL, 1)
			go func() {
				accessURLChan <- waitAccessURL(t, cfg)
			}()

			var accessURL *url.URL
			select {
			case err := <-errChan:
				require.NoError(t, err)
			case accessURL = <-accessURLChan:
				require.NotNil(t, accessURL)
			}

			client := codersdk.New(accessURL)

			authMethods, err := client.AuthMethods(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expectGithubEnabled, authMethods.Github.Enabled)
			require.Equal(t, tc.expectGithubDefaultProviderConfigured, authMethods.Github.DefaultProviderConfigured)

			cancelFunc()
			select {
			case err := <-errChan:
				require.NoError(t, err)
			case <-time.After(testutil.WaitLong):
				t.Fatal("server did not exit")
			}

			if tc.createUserPostRestart {
				_ = dbgen.User(t, db, database.User{})
			}

			// Ensure that it stays at that setting after the server restarts.
			inv, cfg = clitest.New(t, args...)
			clitest.Start(t, inv)
			accessURL = waitAccessURL(t, cfg)
			client = codersdk.New(accessURL)

			ctx = testutil.Context(t, testutil.WaitLong)
			authMethods, err = client.AuthMethods(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expectGithubEnabled, authMethods.Github.Enabled)
			require.Equal(t, tc.expectGithubDefaultProviderConfigured, authMethods.Github.DefaultProviderConfigured)
		}

		for _, tc := range []testCase{
			{
				name:                                  "NewDeployment",
				expectGithubEnabled:                   true,
				expectGithubDefaultProviderConfigured: true,
				createUserPreStart:                    false,
				createUserPostRestart:                 true,
			},
			{
				name:                                  "ExistingDeployment",
				expectGithubEnabled:                   false,
				expectGithubDefaultProviderConfigured: false,
				createUserPreStart:                    true,
				createUserPostRestart:                 false,
			},
			{
				name:                                  "ManuallyDisabled",
				githubDefaultProviderEnabled:          "false",
				expectGithubEnabled:                   false,
				expectGithubDefaultProviderConfigured: false,
			},
			{
				name:                                  "ConfiguredClientID",
				githubClientID:                        "123",
				expectGithubEnabled:                   true,
				expectGithubDefaultProviderConfigured: false,
			},
			{
				name:                                  "ConfiguredClientSecret",
				githubClientSecret:                    "456",
				expectGithubEnabled:                   true,
				expectGithubDefaultProviderConfigured: false,
			},
			{
				name:                                  "AllowedOrg",
				allowedOrg:                            "coder",
				expectGithubEnabled:                   true,
				expectGithubDefaultProviderConfigured: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runGitHubProviderTest(t, tc)
			})
		}
	})

	// Validate that a warning is printed that it may not be externally
	// reachable.
	t.Run("LocalAccessURL", func(t *testing.T) {
		t.Parallel()
		inv, cfg := clitest.New(t,
			"server",
			dbArg(t),
			"--http-address", ":0",
			"--access-url", "http://localhost:3000/",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("this may cause unexpected problems when creating workspaces")
		pty.ExpectMatch("View the Web UI:")
		pty.ExpectMatch("http://localhost:3000/")
	})

	// Validate that an https scheme is prepended to a remote access URL
	// and that a warning is printed for a host that cannot be resolved.
	t.Run("RemoteAccessURL", func(t *testing.T) {
		t.Parallel()

		inv, cfg := clitest.New(t,
			"server",
			dbArg(t),
			"--http-address", ":0",
			"--access-url", "https://foobarbaz.mydomain",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv)

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("this may cause unexpected problems when creating workspaces")
		pty.ExpectMatch("View the Web UI:")
		pty.ExpectMatch("https://foobarbaz.mydomain")
	})

	t.Run("NoWarningWithRemoteAccessURL", func(t *testing.T) {
		t.Parallel()
		inv, cfg := clitest.New(t,
			"server",
			dbArg(t),
			"--http-address", ":0",
			"--access-url", "https://google.com",
			"--cache-dir", t.TempDir(),
		)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		// Just wait for startup
		_ = waitAccessURL(t, cfg)

		pty.ExpectMatch("View the Web UI:")
		pty.ExpectMatch("https://google.com")
	})

	t.Run("NoSchemeAccessURL", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		root, _ := clitest.New(t,
			"server",
			dbArg(t),
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
			dbArg(t),
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
			dbArg(t),
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
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				ctx, cancelFunc := context.WithCancel(context.Background())
				defer cancelFunc()

				args := []string{
					"server",
					dbArg(t),
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
			dbArg(t),
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
			dbArg(t),
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
			dbArg(t),
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
					dbArg(t),
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

		inv, _ := clitest.New(t,
			"server",
			dbArg(t),
			"--http-address", "0.0.0.0:0",
			"--access-url", "http://example.com",
		)

		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)

		pty.ExpectMatch("Started HTTP listener")
		pty.ExpectMatch("http://0.0.0.0:")
	})

	t.Run("CanListenUnspecifiedv6", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t,
			"server",
			dbArg(t),
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
			dbArg(t),
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
			dbArg(t),
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
				dbArg(t),
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
				dbArg(t),
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
			dbArg(t),
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

		telemetryServerURL, deployment, snapshot := mockTelemetryServer(t)

		inv, cfg := clitest.New(t,
			"server",
			dbArg(t),
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--telemetry",
			"--telemetry-url", telemetryServerURL.String(),
			"--cache-dir", t.TempDir(),
		)
		clitest.Start(t, inv)

		<-deployment
		<-snapshot

		accessURL := waitAccessURL(t, cfg)

		ctx := testutil.Context(t, testutil.WaitMedium)
		client := codersdk.New(accessURL)
		body, err := client.Request(ctx, http.MethodGet, "/", nil)
		require.NoError(t, err)
		require.NoError(t, body.Body.Close())

		require.Eventually(t, func() bool {
			snap := <-snapshot
			htmlFirstServedFound := false
			for _, item := range snap.TelemetryItems {
				if item.Key == string(telemetry.TelemetryItemKeyHTMLFirstServedAt) {
					htmlFirstServedFound = true
				}
			}
			return htmlFirstServedFound
		}, testutil.WaitLong, testutil.IntervalSlow, "no html_first_served telemetry item")
	})
	t.Run("Prometheus", func(t *testing.T) {
		t.Parallel()

		t.Run("DBMetricsDisabled", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			inv, _ := clitest.New(t,
				"server",
				dbArg(t),
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons", "1",
				"--prometheus-enable",
				"--prometheus-address", ":0",
				// "--prometheus-collect-db-metrics", // disabled by default
				"--cache-dir", t.TempDir(),
			)

			pty := ptytest.New(t)
			inv.Stdout = pty.Output()
			inv.Stderr = pty.Output()

			clitest.Start(t, inv)

			// Wait until we see the prometheus address in the logs.
			addrMatchExpr := `http server listening\s+addr=(\S+)\s+name=prometheus`
			lineMatch := pty.ExpectRegexMatchContext(ctx, addrMatchExpr)
			promAddr := regexp.MustCompile(addrMatchExpr).FindStringSubmatch(lineMatch)[1]

			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/metrics", promAddr), nil)
				if err != nil {
					t.Logf("error creating request: %s", err.Error())
					return false
				}
				// nolint:bodyclose
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Logf("error hitting prometheus endpoint: %s", err.Error())
					return false
				}
				defer res.Body.Close()
				scanner := bufio.NewScanner(res.Body)
				var activeUsersFound bool
				var scannedOnce bool
				for scanner.Scan() {
					line := scanner.Text()
					if !scannedOnce {
						t.Logf("scanned: %s", line) // avoid spamming logs
						scannedOnce = true
					}
					if strings.HasPrefix(line, "coderd_db_query_latencies_seconds") {
						t.Errorf("db metrics should not be tracked when --prometheus-collect-db-metrics is not enabled")
					}
					// This metric is manually registered to be tracked in the server. That's
					// why we test it's tracked here.
					if strings.HasPrefix(line, "coderd_api_active_users_duration_hour") {
						activeUsersFound = true
					}
				}
				return activeUsersFound
			}, testutil.IntervalSlow, "didn't find coderd_api_active_users_duration_hour in time")
		})

		t.Run("DBMetricsEnabled", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			inv, _ := clitest.New(t,
				"server",
				dbArg(t),
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons", "1",
				"--prometheus-enable",
				"--prometheus-address", ":0",
				"--prometheus-collect-db-metrics",
				"--cache-dir", t.TempDir(),
			)

			pty := ptytest.New(t)
			inv.Stdout = pty.Output()
			inv.Stderr = pty.Output()

			clitest.Start(t, inv)

			// Wait until we see the prometheus address in the logs.
			addrMatchExpr := `http server listening\s+addr=(\S+)\s+name=prometheus`
			lineMatch := pty.ExpectRegexMatchContext(ctx, addrMatchExpr)
			promAddr := regexp.MustCompile(addrMatchExpr).FindStringSubmatch(lineMatch)[1]

			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/metrics", promAddr), nil)
				if err != nil {
					t.Logf("error creating request: %s", err.Error())
					return false
				}
				// nolint:bodyclose
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Logf("error hitting prometheus endpoint: %s", err.Error())
					return false
				}
				defer res.Body.Close()
				scanner := bufio.NewScanner(res.Body)
				var dbMetricsFound bool
				var scannedOnce bool
				for scanner.Scan() {
					line := scanner.Text()
					if !scannedOnce {
						t.Logf("scanned: %s", line) // avoid spamming logs
						scannedOnce = true
					}
					if strings.HasPrefix(line, "coderd_db_query_latencies_seconds") {
						dbMetricsFound = true
					}
				}
				return dbMetricsFound
			}, testutil.IntervalSlow, "didn't find coderd_db_query_latencies_seconds in time")
		})
	})
	t.Run("GitHubOAuth", func(t *testing.T) {
		t.Parallel()

		fakeRedirect := "https://fake-url.com"
		inv, cfg := clitest.New(t,
			"server",
			dbArg(t),
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
				dbArg(t),
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
				dbArg(t),
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
				dbArg(t),
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
				dbArg(t),
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
				dbArg(t),
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

	t.Run("Logging", func(t *testing.T) {
		t.Parallel()

		t.Run("CreatesFile", func(t *testing.T) {
			t.Parallel()
			fiName := testutil.TempFile(t, "", "coder-logging-test-*")

			root, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				dbArg(t),
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons=3",
				"--provisioner-types=echo",
				"--log-human", fiName,
			)
			clitest.Start(t, root)

			loggingWaitFile(t, fiName, testutil.WaitLong)
		})

		t.Run("Human", func(t *testing.T) {
			t.Parallel()
			fi := testutil.TempFile(t, "", "coder-logging-test-*")

			root, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				dbArg(t),
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons=3",
				"--provisioner-types=echo",
				"--log-human", fi,
			)
			clitest.Start(t, root)

			loggingWaitFile(t, fi, testutil.WaitShort)
		})

		t.Run("JSON", func(t *testing.T) {
			t.Parallel()
			fi := testutil.TempFile(t, "", "coder-logging-test-*")

			root, _ := clitest.New(t,
				"server",
				"--log-filter=.*",
				dbArg(t),
				"--http-address", ":0",
				"--access-url", "http://example.com",
				"--provisioner-daemons=3",
				"--provisioner-types=echo",
				"--log-json", fi,
			)
			clitest.Start(t, root)

			loggingWaitFile(t, fi, testutil.WaitShort)
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
				dbArg(t),
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
			//nolint:gocritic
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
			inv, cfg = clitest.New(t, "server", "--config="+fi.Name(), dbArg(t))
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

//nolint:tparallel,paralleltest // This test sets environment variables.
func TestServer_Logging_NoParallel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { server.Close() })

	// Speed up stackdriver test by using custom host. This is like
	// saying we're running on GCE, so extra checks are skipped.
	//
	// Note, that the server isn't actually hit by the test, unsure why
	// but kept just in case.
	//
	// From cloud.google.com/go/compute/metadata/metadata.go (used by coder/slog):
	//
	// metadataHostEnv is the environment variable specifying the
	// GCE metadata hostname.  If empty, the default value of
	// metadataIP ("169.254.169.254") is used instead.
	// This is variable name is not defined by any spec, as far as
	// I know; it was made up for the Go package.
	t.Setenv("GCE_METADATA_HOST", server.URL)

	t.Run("Stackdriver", func(t *testing.T) {
		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancelFunc()

		fi := testutil.TempFile(t, "", "coder-logging-test-*")

		inv, _ := clitest.New(t,
			"server",
			"--log-filter=.*",
			dbArg(t),
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--provisioner-daemons=3",
			"--provisioner-types=echo",
			"--log-stackdriver", fi,
		)
		// Attach pty so we get debug output from the command if this test
		// fails.
		pty := ptytest.New(t).Attach(inv)

		clitest.Start(t, inv.WithContext(ctx))

		// Wait for server to listen on HTTP, this is a good
		// starting point for expecting logs.
		_ = pty.ExpectMatchContext(ctx, "Started HTTP listener at")

		loggingWaitFile(t, fi, testutil.WaitSuperLong)
	})

	t.Run("Multiple", func(t *testing.T) {
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
			dbArg(t),
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--provisioner-daemons=3",
			"--provisioner-types=echo",
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

		loggingWaitFile(t, fi1, testutil.WaitSuperLong)
		loggingWaitFile(t, fi2, testutil.WaitSuperLong)
		loggingWaitFile(t, fi3, testutil.WaitSuperLong)
	})
}

func loggingWaitFile(t *testing.T, fiName string, dur time.Duration) {
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
		dur, //nolint:gocritic
		testutil.IntervalFast,
		"file at %s should exist, last stat: %+v",
		fiName, lastStat,
	)
}

func TestServer_Production(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" || testing.Short() {
		// Skip on non-Linux because it spawns a PostgreSQL instance.
		t.SkipNow()
	}

	// Postgres + race detector + CI = slow.
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitSuperLong*3)
	defer cancelFunc()

	inv, cfg := clitest.New(t,
		"server",
		"--http-address", ":0",
		"--access-url", "http://example.com",
		dbArg(t),
		"--cache-dir", t.TempDir(),
	)
	clitest.Start(t, inv.WithContext(ctx))
	accessURL := waitAccessURL(t, cfg)
	client := codersdk.New(accessURL)

	_, err := client.CreateFirstUser(ctx, coderdtest.FirstUserParams)
	require.NoError(t, err)
}

//nolint:tparallel,paralleltest // This test sets environment variables.
func TestServer_TelemetryDisable(t *testing.T) {
	// Set the default telemetry to true (normally disabled in tests).
	t.Setenv("CODER_TEST_TELEMETRY_DEFAULT_ENABLE", "true")

	//nolint:paralleltest // No need to reinitialise the variable tt (Go version).
	for _, tt := range []struct {
		key  string
		val  string
		want bool
	}{
		{"", "", true},
		{"CODER_TELEMETRY_ENABLE", "true", true},
		{"CODER_TELEMETRY_ENABLE", "false", false},
		{"CODER_TELEMETRY", "true", true},
		{"CODER_TELEMETRY", "false", false},
	} {
		t.Run(fmt.Sprintf("%s=%s", tt.key, tt.val), func(t *testing.T) {
			t.Parallel()
			var b bytes.Buffer
			inv, _ := clitest.New(t, "server", "--write-config")
			inv.Stdout = &b
			inv.Environ.Set(tt.key, tt.val)
			clitest.Run(t, inv)

			var dv codersdk.DeploymentValues
			err := yaml.Unmarshal(b.Bytes(), &dv)
			require.NoError(t, err)
			assert.Equal(t, tt.want, dv.Telemetry.Enable.Value())
		})
	}
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
		dbArg(t),
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
		dbArg(t),
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

	clitest.TestGoldenFile(t, "server-config.yaml", wantBuf.Bytes(), nil)
}

func TestConnectToPostgres(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test does not make sense without postgres")
	}

	t.Run("Migrate", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)

		log := testutil.Logger(t)

		dbURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := cli.ConnectToPostgres(ctx, log, "postgres", dbURL, migrations.Up)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
		require.NoError(t, sqlDB.PingContext(ctx))
	})

	t.Run("NoMigrate", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)

		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		dbURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		okDB, err := cli.ConnectToPostgres(ctx, log, "postgres", dbURL, nil)
		require.NoError(t, err)
		defer okDB.Close()

		// Set the migration number forward
		_, err = okDB.Exec(`UPDATE schema_migrations SET version = version + 1`)
		require.NoError(t, err)

		_, err = cli.ConnectToPostgres(ctx, log, "postgres", dbURL, nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "database needs migration")

		require.NoError(t, okDB.PingContext(ctx))
	})
}

func TestServer_InvalidDERP(t *testing.T) {
	t.Parallel()

	// Try to start a server with the built-in DERP server disabled and no
	// external DERP map.

	inv, _ := clitest.New(t,
		"server",
		dbArg(t),
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

func TestServer_DisabledDERP(t *testing.T) {
	t.Parallel()

	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(context.Background(), w, http.StatusOK, derpMap)
	}))
	t.Cleanup(srv.Close)

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancelFunc()

	// Try to start a server with the built-in DERP server disabled and an
	// external DERP map.
	inv, cfg := clitest.New(t,
		"server",
		dbArg(t),
		"--http-address", ":0",
		"--access-url", "http://example.com",
		"--derp-server-enable=false",
		"--derp-config-url", srv.URL,
	)
	clitest.Start(t, inv.WithContext(ctx))
	accessURL := waitAccessURL(t, cfg)
	derpURL, err := accessURL.Parse("/derp")
	require.NoError(t, err)

	c, err := derphttp.NewClient(key.NewNode(), derpURL.String(), func(format string, args ...any) {})
	require.NoError(t, err)

	// DERP should fail to connect
	err = c.Connect(ctx)
	require.Error(t, err)
}

type runServerOpts struct {
	waitForSnapshot               bool
	telemetryDisabled             bool
	waitForTelemetryDisabledCheck bool
}

func TestServer_TelemetryDisabled_FinalReport(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

	telemetryServerURL, deployment, snapshot := mockTelemetryServer(t)
	dbConnURL, err := dbtestutil.Open(t)
	require.NoError(t, err)

	cacheDir := t.TempDir()
	runServer := func(t *testing.T, opts runServerOpts) (chan error, context.CancelFunc) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		inv, _ := clitest.New(t,
			"server",
			"--postgres-url", dbConnURL,
			"--http-address", ":0",
			"--access-url", "http://example.com",
			"--telemetry="+strconv.FormatBool(!opts.telemetryDisabled),
			"--telemetry-url", telemetryServerURL.String(),
			"--cache-dir", cacheDir,
			"--log-filter", ".*",
		)
		finished := make(chan bool, 2)
		errChan := make(chan error, 1)
		pty := ptytest.New(t).Attach(inv)
		go func() {
			errChan <- inv.WithContext(ctx).Run()
			finished <- true
		}()
		go func() {
			defer func() {
				finished <- true
			}()
			if opts.waitForSnapshot {
				pty.ExpectMatchContext(testutil.Context(t, testutil.WaitLong), "submitted snapshot")
			}
			if opts.waitForTelemetryDisabledCheck {
				pty.ExpectMatchContext(testutil.Context(t, testutil.WaitLong), "finished telemetry status check")
			}
		}()
		<-finished
		return errChan, cancelFunc
	}
	waitForShutdown := func(t *testing.T, errChan chan error) error {
		t.Helper()
		select {
		case err := <-errChan:
			return err
		case <-time.After(testutil.WaitMedium):
			t.Fatalf("timed out waiting for server to shutdown")
		}
		return nil
	}

	errChan, cancelFunc := runServer(t, runServerOpts{telemetryDisabled: true, waitForTelemetryDisabledCheck: true})
	cancelFunc()
	require.NoError(t, waitForShutdown(t, errChan))

	// Since telemetry was disabled, we expect no deployments or snapshots.
	require.Empty(t, deployment)
	require.Empty(t, snapshot)

	errChan, cancelFunc = runServer(t, runServerOpts{waitForSnapshot: true})
	cancelFunc()
	require.NoError(t, waitForShutdown(t, errChan))
	// we expect to see a deployment and a snapshot twice:
	// 1. the first pair is sent when the server starts
	// 2. the second pair is sent when the server shuts down
	for i := 0; i < 2; i++ {
		select {
		case <-snapshot:
		case <-time.After(testutil.WaitShort / 2):
			t.Fatalf("timed out waiting for snapshot")
		}
		select {
		case <-deployment:
		case <-time.After(testutil.WaitShort / 2):
			t.Fatalf("timed out waiting for deployment")
		}
	}

	errChan, cancelFunc = runServer(t, runServerOpts{telemetryDisabled: true, waitForTelemetryDisabledCheck: true})
	cancelFunc()
	require.NoError(t, waitForShutdown(t, errChan))

	// Since telemetry is disabled, we expect no deployment. We expect a snapshot
	// with the telemetry disabled item.
	require.Empty(t, deployment)
	select {
	case ss := <-snapshot:
		require.Len(t, ss.TelemetryItems, 1)
		require.Equal(t, string(telemetry.TelemetryItemKeyTelemetryEnabled), ss.TelemetryItems[0].Key)
		require.Equal(t, "false", ss.TelemetryItems[0].Value)
	case <-time.After(testutil.WaitShort / 2):
		t.Fatalf("timed out waiting for snapshot")
	}

	errChan, cancelFunc = runServer(t, runServerOpts{telemetryDisabled: true, waitForTelemetryDisabledCheck: true})
	cancelFunc()
	require.NoError(t, waitForShutdown(t, errChan))
	// Since telemetry is disabled and we've already sent a snapshot, we expect no
	// new deployments or snapshots.
	require.Empty(t, deployment)
	require.Empty(t, snapshot)
}

func mockTelemetryServer(t *testing.T) (*url.URL, chan *telemetry.Deployment, chan *telemetry.Snapshot) {
	t.Helper()
	deployment := make(chan *telemetry.Deployment, 64)
	snapshot := make(chan *telemetry.Snapshot, 64)
	r := chi.NewRouter()
	r.Post("/deployment", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		dd := &telemetry.Deployment{}
		err := json.NewDecoder(r.Body).Decode(dd)
		require.NoError(t, err)
		deployment <- dd
		// Ensure the header is sent only after deployment is sent
		w.WriteHeader(http.StatusAccepted)
	})
	r.Post("/snapshot", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		ss := &telemetry.Snapshot{}
		err := json.NewDecoder(r.Body).Decode(ss)
		require.NoError(t, err)
		snapshot <- ss
		// Ensure the header is sent only after snapshot is sent
		w.WriteHeader(http.StatusAccepted)
	})
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	return serverURL, deployment, snapshot
}
