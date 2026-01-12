package cli_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

//nolint:tparallel,paralleltest
func TestCommandHelp(t *testing.T) {
	// Test with AGPL commands
	getCmds := func(t *testing.T) *serpent.Command {
		// Must return a fresh instance of cmds each time.

		t.Helper()
		var root cli.RootCmd
		rootCmd, err := root.Command(root.AGPL())
		require.NoError(t, err)

		return rootCmd
	}
	clitest.TestCommandHelp(t, getCmds, append(clitest.DefaultCases(),
		clitest.CommandHelpCase{
			Name: "coder agent --help",
			Cmd:  []string{"agent", "--help"},
		},
		clitest.CommandHelpCase{
			Name: "coder list --output json",
			Cmd:  []string{"list", "--output", "json"},
		},
		clitest.CommandHelpCase{
			Name: "coder users list --output json",
			Cmd:  []string{"users", "list", "--output", "json"},
		},
		clitest.CommandHelpCase{
			Name: "coder users list",
			Cmd:  []string{"users", "list"},
		},
		clitest.CommandHelpCase{
			Name: "coder provisioner list",
			Cmd:  []string{"provisioner", "list"},
		},
		clitest.CommandHelpCase{
			Name: "coder provisioner list --output json",
			Cmd:  []string{"provisioner", "list", "--output", "json"},
		},
		clitest.CommandHelpCase{
			Name: "coder provisioner jobs list",
			Cmd:  []string{"provisioner", "jobs", "list"},
		},
		clitest.CommandHelpCase{
			Name: "coder provisioner jobs list --output json",
			Cmd:  []string{"provisioner", "jobs", "list", "--output", "json"},
		},
		// TODO (SasSwart): Remove these once the sync commands are promoted out of experimental.
		clitest.CommandHelpCase{
			Name: "coder exp sync --help",
			Cmd:  []string{"exp", "sync", "--help"},
		},
		clitest.CommandHelpCase{
			Name: "coder exp sync ping --help",
			Cmd:  []string{"exp", "sync", "ping", "--help"},
		},
		clitest.CommandHelpCase{
			Name: "coder exp sync start --help",
			Cmd:  []string{"exp", "sync", "start", "--help"},
		},
		clitest.CommandHelpCase{
			Name: "coder exp sync want --help",
			Cmd:  []string{"exp", "sync", "want", "--help"},
		},
		clitest.CommandHelpCase{
			Name: "coder exp sync complete --help",
			Cmd:  []string{"exp", "sync", "complete", "--help"},
		},
		clitest.CommandHelpCase{
			Name: "coder exp sync status --help",
			Cmd:  []string{"exp", "sync", "status", "--help"},
		},
	))
}

func TestRoot(t *testing.T) {
	t.Parallel()
	t.Run("MissingRootCommand", func(t *testing.T) {
		t.Parallel()

		out := new(bytes.Buffer)

		inv, _ := clitest.New(t, "idontexist")
		inv.Stdout = out

		err := inv.Run()
		assert.ErrorContains(t, err,
			`unrecognized subcommand "idontexist"`)
		require.Empty(t, out.String())
	})

	t.Run("MissingSubcommand", func(t *testing.T) {
		t.Parallel()

		out := new(bytes.Buffer)

		inv, _ := clitest.New(t, "server", "idontexist")
		inv.Stdout = out

		err := inv.Run()
		// subcommand error only when command has subcommands
		assert.ErrorContains(t, err,
			`unrecognized subcommand "idontexist"`)
		require.Empty(t, out.String())
	})

	t.Run("BadSubcommandArgs", func(t *testing.T) {
		t.Parallel()

		out := new(bytes.Buffer)

		inv, _ := clitest.New(t, "list", "idontexist")
		inv.Stdout = out

		err := inv.Run()
		assert.ErrorContains(t, err,
			`wanted no args but got 1 [idontexist]`)
		require.Empty(t, out.String())
	})

	t.Run("Version", func(t *testing.T) {
		t.Parallel()

		buf := new(bytes.Buffer)
		inv, _ := clitest.New(t, "version")
		inv.Stdout = buf
		err := inv.Run()
		require.NoError(t, err)

		output := buf.String()
		require.Contains(t, output, buildinfo.Version(), "has version")
		require.Contains(t, output, buildinfo.ExternalURL(), "has url")
	})

	t.Run("Header", func(t *testing.T) {
		t.Parallel()

		var url string
		var called int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&called, 1)
			assert.Equal(t, "wow", r.Header.Get("X-Testing"))
			assert.Equal(t, "Dean was Here!", r.Header.Get("Cool-Header"))
			assert.Equal(t, "very-wow-"+url, r.Header.Get("X-Process-Testing"))
			assert.Equal(t, "more-wow", r.Header.Get("X-Process-Testing2"))
			w.WriteHeader(http.StatusGone)
		}))
		defer srv.Close()
		url = srv.URL
		buf := new(bytes.Buffer)
		coderURLEnv := "$CODER_URL"
		if runtime.GOOS == "windows" {
			coderURLEnv = "%CODER_URL%"
		}
		inv, _ := clitest.New(t,
			"--no-feature-warning",
			"--no-version-warning",
			"--header", "X-Testing=wow",
			"--header", "Cool-Header=Dean was Here!",
			"--header-command", "printf X-Process-Testing=very-wow-"+coderURLEnv+"'\\r\\n'X-Process-Testing2=more-wow",
			"login", srv.URL,
		)
		inv.Stdout = buf

		err := inv.Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "unexpected status code 410")
		require.EqualValues(t, 1, atomic.LoadInt64(&called), "called exactly once")
	})
}

// TestDERPHeaders ensures that the client sends the global `--header`s and
// `--header-command` to the DERP server when connecting.
func TestDERPHeaders(t *testing.T) {
	t.Parallel()

	// Create a coderd API instance the hard way since we need to change the
	// handler to inject our custom /derp handler.
	dv := coderdtest.DeploymentValues(t)
	dv.DERP.Config.BlockDirect = true
	setHandler, cancelFunc, serverURL, newOptions := coderdtest.NewOptions(t, &coderdtest.Options{
		DeploymentValues: dv,
	})

	// We set the handler after server creation for the access URL.
	coderAPI := coderd.New(newOptions)
	setHandler(coderAPI.RootHandler)
	provisionerCloser := coderdtest.NewProvisionerDaemon(t, coderAPI)
	t.Cleanup(func() {
		_ = provisionerCloser.Close()
	})
	client := codersdk.New(serverURL)
	t.Cleanup(func() {
		cancelFunc()
		_ = provisionerCloser.Close()
		_ = coderAPI.Close()
		client.HTTPClient.CloseIdleConnections()
	})

	var (
		admin              = coderdtest.CreateFirstUser(t, client)
		member, memberUser = coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		workspace          = runAgent(t, client, memberUser.ID, newOptions.Database)
	)

	// Inject custom /derp handler so we can inspect the headers.
	var (
		expectedHeaders = map[string]string{
			"X-Test-Header":     "test-value",
			"Cool-Header":       "Dean was Here!",
			"X-Process-Testing": "very-wow",
		}
		derpCalled int64
	)
	setHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/derp") {
			ok := true
			for k, v := range expectedHeaders {
				if r.Header.Get(k) != v {
					ok = false
					break
				}
			}
			if ok {
				// Only increment if all the headers are set, because the agent
				// calls derp also.
				atomic.AddInt64(&derpCalled, 1)
			}
		}

		coderAPI.RootHandler.ServeHTTP(w, r)
	}))

	// Connect with the headers set as args.
	args := []string{
		"-v",
		"--no-feature-warning",
		"--no-version-warning",
		"ping", workspace.Name,
		"-n", "1",
		"--header-command", "printf X-Process-Testing=very-wow",
	}
	for k, v := range expectedHeaders {
		if k != "X-Process-Testing" {
			args = append(args, "--header", fmt.Sprintf("%s=%s", k, v))
		}
	}
	inv, root := clitest.New(t, args...)
	clitest.SetupConfig(t, member, root)
	pty := ptytest.New(t)
	inv.Stdin = pty.Input()
	inv.Stderr = pty.Output()
	inv.Stdout = pty.Output()

	ctx := testutil.Context(t, testutil.WaitLong)
	cmdDone := tGo(t, func() {
		err := inv.WithContext(ctx).Run()
		assert.NoError(t, err)
	})

	pty.ExpectMatch("pong from " + workspace.Name)
	<-cmdDone

	require.Greater(t, atomic.LoadInt64(&derpCalled), int64(0), "expected /derp to be called at least once")
}

func TestHandlersOK(t *testing.T) {
	t.Parallel()

	var root cli.RootCmd
	cmd, err := root.Command(root.CoreSubcommands())
	require.NoError(t, err)

	clitest.HandlersOK(t, cmd)
}

func TestCreateAgentClient_Token(t *testing.T) {
	t.Parallel()

	client := createAgentWithFlags(t,
		"--agent-token", "fake-token",
		"--agent-url", "http://coder.fake")
	require.Equal(t, "fake-token", client.GetSessionToken())
}

func TestCreateAgentClient_Google(t *testing.T) {
	t.Parallel()

	client := createAgentWithFlags(t,
		"--auth", "google-instance-identity",
		"--agent-url", "http://coder.fake")
	provider, ok := client.RefreshableSessionTokenProvider.(*agentsdk.InstanceIdentitySessionTokenProvider)
	require.True(t, ok)
	require.NotNil(t, provider.TokenExchanger)
	require.IsType(t, &agentsdk.GoogleSessionTokenExchanger{}, provider.TokenExchanger)
}

func TestCreateAgentClient_AWS(t *testing.T) {
	t.Parallel()

	client := createAgentWithFlags(t,
		"--auth", "aws-instance-identity",
		"--agent-url", "http://coder.fake")
	provider, ok := client.RefreshableSessionTokenProvider.(*agentsdk.InstanceIdentitySessionTokenProvider)
	require.True(t, ok)
	require.NotNil(t, provider.TokenExchanger)
	require.IsType(t, &agentsdk.AWSSessionTokenExchanger{}, provider.TokenExchanger)
}

func TestCreateAgentClient_Azure(t *testing.T) {
	t.Parallel()

	client := createAgentWithFlags(t,
		"--auth", "azure-instance-identity",
		"--agent-url", "http://coder.fake")
	provider, ok := client.RefreshableSessionTokenProvider.(*agentsdk.InstanceIdentitySessionTokenProvider)
	require.True(t, ok)
	require.NotNil(t, provider.TokenExchanger)
	require.IsType(t, &agentsdk.AzureSessionTokenExchanger{}, provider.TokenExchanger)
}

func createAgentWithFlags(t *testing.T, flags ...string) *agentsdk.Client {
	t.Helper()
	r := &cli.RootCmd{}
	var client *agentsdk.Client
	subCmd := agentClientCommand(&client)
	cmd, err := r.Command([]*serpent.Command{subCmd})
	require.NoError(t, err)
	inv, _ := clitest.NewWithCommand(t, cmd,
		append([]string{"agent-client"}, flags...)...)
	err = inv.Run()
	require.NoError(t, err)
	require.NotNil(t, client)
	return client
}

// agentClientCommand creates a subcommand that creates an agent client and stores it in the provided clientRef. Used to
// test the properties of the client with various root command flags.
func agentClientCommand(clientRef **agentsdk.Client) *serpent.Command {
	agentAuth := &cli.AgentAuth{}
	cmd := &serpent.Command{
		Use:   "agent-client",
		Short: `Creates and agent client for testing.`,
		Handler: func(inv *serpent.Invocation) error {
			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}
			*clientRef = client
			return nil
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

func TestWrapTransportWithUserAgentHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                    string
		cmdArgs                 []string
		cmdEnv                  map[string]string
		expectedUserAgentHeader string
	}{
		{
			name:                    "top-level command",
			cmdArgs:                 []string{"login"},
			expectedUserAgentHeader: fmt.Sprintf("coder-cli/%s (%s/%s; coder login)", buildinfo.Version(), runtime.GOOS, runtime.GOARCH),
		},
		{
			name:                    "nested commands",
			cmdArgs:                 []string{"templates", "list"},
			expectedUserAgentHeader: fmt.Sprintf("coder-cli/%s (%s/%s; coder templates list)", buildinfo.Version(), runtime.GOOS, runtime.GOARCH),
		},
		{
			name:                    "does not include positional args, flags, or env",
			cmdArgs:                 []string{"templates", "push", "my-template", "-d", "/path/to/template", "--yes", "--var", "myvar=myvalue"},
			cmdEnv:                  map[string]string{"SECRET_KEY": "secret_value"},
			expectedUserAgentHeader: fmt.Sprintf("coder-cli/%s (%s/%s; coder templates push)", buildinfo.Version(), runtime.GOOS, runtime.GOARCH),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ch := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case ch <- r.Header.Get("User-Agent"):
				default: // already sent
				}
			}))
			t.Cleanup(srv.Close)

			args := append([]string{}, tc.cmdArgs...)
			inv, _ := clitest.New(t, args...)
			inv.Environ.Set("CODER_URL", srv.URL)
			for k, v := range tc.cmdEnv {
				inv.Environ.Set(k, v)
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			_ = inv.WithContext(ctx).Run() // Ignore error as we only care about headers.

			actual := testutil.RequireReceive(ctx, t, ch)
			require.Equal(t, tc.expectedUserAgentHeader, actual, "User-Agent should match expected format exactly")
		})
	}
}
