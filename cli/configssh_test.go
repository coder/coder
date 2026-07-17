package cli_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func sshConfigFileName(t *testing.T) (sshConfig string) {
	t.Helper()
	tmpdir := t.TempDir()
	dotssh := filepath.Join(tmpdir, ".ssh")
	err := os.Mkdir(dotssh, 0o700)
	require.NoError(t, err)
	n := filepath.Join(dotssh, "config")
	return n
}

func sshConfigFileCreate(t *testing.T, name string, data io.Reader) {
	t.Helper()
	t.Logf("Writing %s", name)
	f, err := os.Create(name)
	require.NoError(t, err)
	n, err := io.Copy(f, data)
	t.Logf("Wrote %d", n)
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)
}

func sshConfigFileRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

func TestConfigSSH(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("See coder/internal#117")
	}

	logger := testutil.Logger(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	const hostname = "test-coder."
	const expectedKey = "ConnectionAttempts"
	const removeKey = "ConnectTimeout"
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		ConfigSSH: codersdk.SSHConfigResponse{
			HostnamePrefix: hostname,
			SSHConfigOptions: map[string]string{
				// Something we can test for
				expectedKey: "3",
				removeKey:   "",
			},
		},
	})
	owner := coderdtest.CreateFirstUser(t, client)
	member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        memberUser.ID,
	}).WithAgent().Do()
	_ = agenttest.New(t, client.URL, r.AgentToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)
	agentConn, err := workspacesdk.New(client).
		DialAgent(context.Background(), resources[0].Agents[0].ID, nil)
	require.NoError(t, err)
	defer agentConn.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		var wg sync.WaitGroup
		for {
			conn, err := listener.Accept()
			if err != nil {
				break
			}
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			ssh, err := agentConn.SSH(ctx)
			cancel()
			assert.NoError(t, err)
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, _ = io.Copy(conn, ssh)
			}()
			go func() {
				defer wg.Done()
				_, _ = io.Copy(ssh, conn)
			}()
		}
		wg.Wait()
	}()

	sshConfigFile := sshConfigFileName(t)

	tcpAddr, valid := listener.Addr().(*net.TCPAddr)
	require.True(t, valid)
	inv, root := clitest.New(t, "config-ssh",
		"--ssh-option", "HostName "+tcpAddr.IP.String(),
		"--ssh-option", "Port "+strconv.Itoa(tcpAddr.Port),
		"--ssh-config-file", sshConfigFile,
		"--skip-proxy-command")
	clitest.SetupConfig(t, member, root)
	stdout := expecter.NewAttachedToInvocation(t, inv)
	stdin := testutil.NewWriterAttachedToInvocation(t, logger.Named("stdin"), inv)

	waiter := clitest.StartWithWaiter(t, inv)

	matches := []struct {
		match, write string
	}{
		{match: "Continue?", write: "yes"},
	}
	for _, m := range matches {
		stdout.ExpectMatch(ctx, m.match)
		stdin.WriteLine(m.write)
	}

	waiter.RequireSuccess()

	fileContents, err := os.ReadFile(sshConfigFile)
	require.NoError(t, err, "read ssh config file")
	require.Contains(t, string(fileContents), expectedKey, "ssh config file contains expected key")
	require.NotContains(t, string(fileContents), removeKey, "ssh config file should not have removed key")

	home := filepath.Dir(filepath.Dir(sshConfigFile))
	// #nosec
	sshCmd := exec.Command("ssh", "-F", sshConfigFile, hostname+r.Workspace.Name, "echo", "test")
	// Set HOME because coder config is included from ~/.ssh/coder.
	sshCmd.Env = append(sshCmd.Env, fmt.Sprintf("HOME=%s", home))
	data, err := sshCmd.Output()
	require.NoError(t, err)
	require.Equal(t, "test", strings.TrimSpace(string(data)))

	_ = listener.Close()
	<-copyDone
}

func TestConfigSSH_RejectsUnsafeServerConfig(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("See coder/internal#117")
	}

	testCases := []struct {
		name      string
		configSSH codersdk.SSHConfigResponse
		wantErr   string
	}{
		{
			name:      "HostnameSuffix",
			configSSH: codersdk.SSHConfigResponse{HostnameSuffix: "coder\nHost *"},
			wantErr:   "workspace hostname suffix",
		},
		{
			name:      "HostnamePrefix",
			configSSH: codersdk.SSHConfigResponse{HostnamePrefix: "coder.\nHost *"},
			wantErr:   "workspace hostname prefix",
		},
		{
			name:      "HostnameSuffixGlob",
			configSSH: codersdk.SSHConfigResponse{HostnameSuffix: "*"},
			wantErr:   "glob",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const existingConfig = "Host safe\n\tHostName safe.example.com\n"
			client := coderdtest.New(t, &coderdtest.Options{
				ConfigSSH: tc.configSSH,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			sshConfigPath := sshConfigFileName(t)
			sshConfigFileCreate(t, sshConfigPath, strings.NewReader(existingConfig))

			inv, root := clitest.New(t,
				"config-ssh",
				"--ssh-config-file", sshConfigPath,
				"--yes",
			)
			clitest.SetupConfig(t, client, root)

			err := inv.Run()
			require.Error(t, err)
			require.ErrorContains(t, err, tc.wantErr)
			require.Equal(t, existingConfig, sshConfigFileRead(t, sshConfigPath))
		})
	}
}

func TestConfigSSH_MissingDirectory(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("See coder/internal#117")
	}

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	// Create a temporary directory but don't create .ssh subdirectory
	tmpdir := t.TempDir()
	sshConfigPath := filepath.Join(tmpdir, ".ssh", "config")

	// Run config-ssh with a non-existent .ssh directory
	args := []string{
		"config-ssh",
		"--ssh-config-file", sshConfigPath,
		"--yes", // Skip confirmation prompts
	}
	inv, root := clitest.New(t, args...)
	clitest.SetupConfig(t, client, root)

	err := inv.Run()
	require.NoError(t, err, "config-ssh should succeed with non-existent directory")

	// Verify that the .ssh directory was created
	sshDir := filepath.Dir(sshConfigPath)
	_, err = os.Stat(sshDir)
	require.NoError(t, err, ".ssh directory should exist")

	// Verify that the config file was created
	_, err = os.Stat(sshConfigPath)
	require.NoError(t, err, "config file should exist")

	// Check that the directory has proper permissions (rwx for owner, none for
	// group and everyone)
	sshDirInfo, err := os.Stat(sshDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), sshDirInfo.Mode().Perm(), "directory should have rwx------ permissions")
}

func TestConfigSSH_FileWriteAndOptionsFlow(t *testing.T) {
	t.Parallel()

	headerStart := strings.Join([]string{
		"# ------------START-CODER-----------",
		"# This section is managed by coder. DO NOT EDIT.",
		"#",
		"# You should not hand-edit this section unless you are removing it, all",
		"# changes will be lost when running \"coder config-ssh\".",
		"#",
	}, "\n")
	headerEnd := "# ------------END-CODER------------"
	baseHeader := strings.Join([]string{
		headerStart,
		headerEnd,
	}, "\n")

	type writeConfig struct {
		ssh string
	}
	type wantConfig struct {
		ssh        []string
		notWant    []string
		regexMatch string
	}
	type match struct {
		match, write string
	}
	tests := []struct {
		name        string
		args        []string
		env         map[string]string
		matches     []match
		writeConfig writeConfig
		wantConfig  wantConfig
		wantErr     bool
		hasAgent    bool
	}{
		{
			name: "Config file is created",
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
			wantConfig: wantConfig{
				ssh: []string{
					headerStart,
					headerEnd,
				},
			},
		},
		{
			name: "Section is written after user content",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"Host myhost",
					"	HostName myhost",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{
					strings.Join([]string{
						"Host myhost",
						"	HostName myhost",
					}, "\n"),
					headerStart,
					headerEnd,
				},
			},
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Section is not moved on re-run with new options",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"Host myhost",
					"	HostName myhost",
					"",
					baseHeader,
					"",
					"Host otherhost",
					"	HostName otherhost",
					"",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{
					strings.Join([]string{
						"Host myhost",
						"	HostName myhost",
						"",
						headerStart,
						"# Last config-ssh options:",
						"# :ssh-option=ForwardAgent=yes",
						"#",
					}, "\n"),
					strings.Join([]string{
						headerEnd,
						"",
						"Host otherhost",
						"	HostName otherhost",
						"",
					}, "\n"),
				},
			},
			args: []string{
				"--ssh-option", "ForwardAgent=yes",
			},
			matches: []match{
				{match: "Use new options?", write: "yes"},
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Adds newline at EOF",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					baseHeader,
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{
					headerStart,
					strings.Join([]string{
						headerEnd,
						"",
					}, "\n"),
				},
			},
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Do not prompt for new options on first run",
			writeConfig: writeConfig{
				ssh: "",
			},
			wantConfig: wantConfig{
				ssh: []string{
					strings.Join([]string{
						headerStart,
						"# Last config-ssh options:",
						"# :ssh-option=ForwardAgent=yes",
						"#",
					}, "\n"),
					strings.Join([]string{
						headerEnd,
						"",
					}, "\n"),
				},
			},
			args: []string{"--ssh-option", "ForwardAgent=yes"},
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Prompt for new options when there are no previous options",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					baseHeader,
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{
					strings.Join([]string{
						headerStart,
						"# Last config-ssh options:",
						"# :ssh-option=ForwardAgent=yes",
						"#",
					}, "\n"),
					strings.Join([]string{
						headerEnd,
						"",
					}, "\n"),
				},
			},
			args: []string{"--ssh-option", "ForwardAgent=yes"},
			matches: []match{
				{match: "Use new options?", write: "yes"},
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Prompt for new options when there are previous options",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{
					headerStart,
					strings.Join([]string{
						headerEnd,
						"",
					}, "\n"),
				},
			},
			matches: []match{
				{match: "Use new options?", write: "yes"},
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "No changes when continue = no",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n")},
			},
			args: []string{"--ssh-option", "ForwardAgent=no"},
			matches: []match{
				{match: "Use new options?", write: "yes"},
				{match: "Continue?", write: "no"},
			},
		},
		{
			name: "Do not prompt when using --yes",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{
					headerStart,
					headerEnd,
				},
			},
			args: []string{"--yes"},
		},
		{
			name: "Serialize supported flags",
			wantConfig: wantConfig{
				ssh: []string{
					strings.Join([]string{
						headerStart,
						"# Last config-ssh options:",
						"# :wait=yes",
						"# :ssh-host-prefix=coder-test.",
						"# :hostname-suffix=coder-suffix",
						"# :header=X-Test-Header=foo",
						"# :header=X-Test-Header2=bar",
						"# :header-command=echo h1=v1 h2=\"v2\" h3='v3'",
						"#",
					}, "\n"),
					strings.Join([]string{
						headerEnd,
						"",
					}, "\n"),
				},
			},
			args: []string{
				"--yes",
				"--wait=yes",
				"--ssh-host-prefix", "coder-test.",
				"--hostname-suffix", "coder-suffix",
				"--header", "X-Test-Header=foo",
				"--header", "X-Test-Header2=bar",
				"--header-command", "echo h1=v1 h2=\"v2\" h3='v3'",
			},
		},
		{
			name: "Serialize no-wildcard flag",
			wantConfig: wantConfig{
				ssh: []string{
					strings.Join([]string{
						headerStart,
						"# Last config-ssh options:",
						"# :hostname-suffix=coder-suffix",
						"# :no-wildcard=true",
						"#",
					}, "\n"),
					strings.Join([]string{
						headerEnd,
						"",
					}, "\n"),
				},
			},
			args: []string{
				"--yes",
				"--hostname-suffix", "coder-suffix",
				"--no-wildcard",
			},
		},
		{
			name: "No wildcard generates per-workspace entries",
			args: []string{
				"--yes",
				"--hostname-suffix", "coder",
				"--no-wildcard",
			},
			hasAgent: true,
			wantConfig: wantConfig{
				ssh: []string{
					"# :hostname-suffix=coder",
					"# :no-wildcard=true",
				},
				regexMatch: `Host [a-z0-9_-]+\.coder`,
			},
		},
		{
			name: "Do not prompt for new options when prev opts flag is set",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :wait=no",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{
					strings.Join(
						[]string{
							headerStart,
							"# Last config-ssh options:",
							"# :wait=no",
							"# :ssh-option=ForwardAgent=yes",
							"#",
						}, "\n"),
					strings.Join([]string{
						headerEnd,
						"",
					}, "\n"),
				},
			},
			args: []string{
				"--use-previous-options",
				"--yes",
			},
		},
		{
			name: "Do not overwrite config when using --dry-run",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					baseHeader,
					"",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: []string{strings.Join([]string{
					baseHeader,
					"",
				}, "\n")},
			},
			args: []string{
				"--ssh-option", "ForwardAgent=yes",
				"--dry-run",
				"--yes",
			},
		},
		{
			name:    "Start/End out of order",
			matches: []match{
				// {match: "Continue?", write: "yes"},
			},
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"# Content before coder block",
					headerEnd,
					headerStart,
					"# Content after coder block",
				}, "\n"),
			},
			wantErr: true,
		},
		{
			name:    "Multiple sections",
			matches: []match{
				// {match: "Continue?", write: "yes"},
			},
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					headerStart,
					headerEnd,
					headerStart,
					headerEnd,
				}, "\n"),
			},
			wantErr: true,
		},
		{
			name: "Custom CLI Path",
			args: []string{
				"-y", "--coder-binary-path", "/foo/bar/coder",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				regexMatch: "ProxyCommand /foo/bar/coder",
			},
		},
		{
			name: "Header",
			args: []string{
				"--yes",
				"--header", "X-Test-Header=foo",
				"--header", "X-Test-Header2=bar",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				regexMatch: `ProxyCommand .* --header "X-Test-Header=foo" --header "X-Test-Header2=bar" ssh .* --ssh-host-prefix coder. %h`,
			},
		},
		{
			name: "Header command",
			args: []string{
				"--yes",
				"--header-command", "echo h1=v1",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				regexMatch: `ProxyCommand .* --header-command "echo h1=v1" ssh .* --ssh-host-prefix coder. %h`,
			},
		},
		{
			name: "Header command with double quotes",
			args: []string{
				"--yes",
				"--header-command", "echo h1=v1 h2=\"v2\"",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				regexMatch: `ProxyCommand .* --header-command "echo h1=v1 h2=\\\"v2\\\"" ssh .* --ssh-host-prefix coder. %h`,
			},
		},
		{
			name: "Header command with single quotes",
			args: []string{
				"--yes",
				"--header-command", "echo h1=v1 h2='v2'",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				regexMatch: `ProxyCommand .* --header-command "echo h1=v1 h2='v2'" ssh .* --ssh-host-prefix coder. %h`,
			},
		},
		{
			name: "Multiple remote forwards",
			args: []string{
				"--yes",
				"--ssh-option", "RemoteForward 2222 192.168.11.1:2222",
				"--ssh-option", "RemoteForward 2223 192.168.11.1:2223",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				regexMatch: "RemoteForward 2222 192.168.11.1:2222.*\n.*RemoteForward 2223 192.168.11.1:2223",
			},
		},
		{
			name: "Hostname Suffix",
			args: []string{
				"--yes",
				"--ssh-option", "Foo=bar",
				"--hostname-suffix", "testy",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				ssh: []string{
					"Host *.testy",
					"Foo=bar",
					"ConnectTimeout=0",
					"StrictHostKeyChecking=no",
					"UserKnownHostsFile=/dev/null",
					"LogLevel ERROR",
				},
				regexMatch: `Match host \*\.testy !exec ".* connect exists %h"\n\tProxyCommand .* ssh .* --hostname-suffix testy %h`,
			},
		},
		{
			name: "Hostname Prefix and Suffix",
			args: []string{
				"--yes",
				"--ssh-host-prefix", "presto.",
				"--hostname-suffix", "testy",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				ssh: []string{"Host presto.*", "Match host *.testy !exec"},
			},
		},
		{
			// Regression test for https://github.com/coder/internal/issues/1208:
			// an explicitly empty --ssh-host-prefix must not fall back to the
			// server's default prefix.
			name: "Explicit empty ssh-host-prefix omits legacy block",
			args: []string{
				"--yes",
				"--ssh-host-prefix", "",
			},
			wantErr: false,
			wantConfig: wantConfig{
				ssh: []string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-host-prefix=\n",
					headerEnd,
				},
				notWant: []string{"Host coder.*", "--ssh-host-prefix coder."},
			},
		},
		{
			// Same as above, but via the env var instead of the flag.
			name: "Explicit empty ssh-host-prefix env var omits legacy block",
			args: []string{"--yes"},
			env: map[string]string{
				"CODER_CONFIGSSH_SSH_HOST_PREFIX": "",
			},
			wantErr: false,
			wantConfig: wantConfig{
				ssh: []string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-host-prefix=\n",
					headerEnd,
				},
				notWant: []string{"Host coder.*", "--ssh-host-prefix coder."},
			},
		},
		{
			// An explicit empty prefix alongside an explicit suffix should
			// produce only the suffix block, not both.
			name: "Explicit empty ssh-host-prefix with hostname-suffix set",
			args: []string{
				"--yes",
				"--ssh-host-prefix", "",
				"--hostname-suffix", "testy",
			},
			wantErr:  false,
			hasAgent: true,
			wantConfig: wantConfig{
				ssh: []string{
					"# :ssh-host-prefix=\n",
					"# :hostname-suffix=testy\n",
					"Host *.testy",
				},
				notWant: []string{"Host coder.*", "--ssh-host-prefix coder."},
			},
		},
		{
			// Regression test: the "omit this block" choice must survive a
			// later --use-previous-options run that doesn't repeat the flag,
			// not just the invocation where the flag was passed.
			name: "use-previous-options preserves an explicitly empty prefix across runs",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-host-prefix=",
					"#",
					headerEnd,
					"",
				}, "\n"),
			},
			args: []string{
				"--use-previous-options",
				"--yes",
			},
			wantConfig: wantConfig{
				ssh: []string{
					"# :ssh-host-prefix=\n",
				},
				notWant: []string{"Host coder.*", "--ssh-host-prefix coder."},
			},
		},
		{
			// Regression test: --use-previous-options should still win over
			// this run's explicit empty flag, since that's what "use previous
			// options" means. The empty-prefix fix must not change this.
			name: "use-previous-options keeps prior prefix despite this run's explicit empty flag",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-host-prefix=coder-test.",
					"#",
					headerEnd,
					"",
				}, "\n"),
			},
			args: []string{
				"--use-previous-options",
				"--yes",
				"--ssh-host-prefix", "",
			},
			wantConfig: wantConfig{
				ssh: []string{
					"# :ssh-host-prefix=coder-test.",
					"Host coder-test.*",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitMedium)

			client, db := coderdtest.NewWithDatabase(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			if tt.hasAgent {
				_ = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
					OrganizationID: user.OrganizationID,
					OwnerID:        user.UserID,
				}).WithAgent().Do()
			}

			// Prepare ssh config files.
			sshConfigName := sshConfigFileName(t)
			if tt.writeConfig.ssh != "" {
				sshConfigFileCreate(t, sshConfigName, strings.NewReader(tt.writeConfig.ssh))
			}

			args := []string{
				"config-ssh",
				"--ssh-config-file", sshConfigName,
			}
			args = append(args, tt.args...)
			inv, root := clitest.New(t, args...)
			//nolint:gocritic // This has always ran with the admin user.
			clitest.SetupConfig(t, client, root)
			for k, v := range tt.env {
				inv.Environ.Set(k, v)
			}

			stdout := expecter.NewAttachedToInvocation(t, inv)
			stdin := testutil.NewWriterAttachedToInvocation(t, logger.Named("stdin"), inv)
			done := tGo(t, func() {
				err := inv.Run()
				if !tt.wantErr {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			})

			for _, m := range tt.matches {
				stdout.ExpectMatch(ctx, m.match)
				stdin.WriteLine(m.write)
			}

			<-done

			if len(tt.wantConfig.ssh) != 0 || tt.wantConfig.regexMatch != "" || len(tt.wantConfig.notWant) != 0 {
				full := sshConfigFileRead(t, sshConfigName)
				got := full
				// Require that the generated config has the expected snippets in order.
				for _, want := range tt.wantConfig.ssh {
					idx := strings.Index(got, want)
					if idx == -1 {
						require.Contains(t, got, want)
					}
					got = got[idx+len(want):]
				}
				if tt.wantConfig.regexMatch != "" {
					assert.Regexp(t, tt.wantConfig.regexMatch, got, "regex match")
				}
				for _, notWant := range tt.wantConfig.notWant {
					assert.NotContains(t, full, notWant, "unexpected snippet found")
				}
			}
		})
	}
}

func TestConfigSSH_NoWildcard(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("See coder/internal#117")
	}

	ctx := testutil.Context(t, testutil.WaitMedium)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	// Create two workspaces with names in reverse lexical order so that we can
	// verify the SSH config entries are sorted by name, not by creation order.
	// ws1 sorts after ws2 alphabetically.
	ws1 := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		Name:           "ws-beta",
	}).WithAgent(func(a []*sdkproto.Agent) []*sdkproto.Agent {
		a[0].Name = "agent-beta"
		return a
	}).Do()
	ws2 := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		Name:           "ws-alpha",
	}).WithAgent(func(a []*sdkproto.Agent) []*sdkproto.Agent {
		a[0].Name = "agent-alpha"
		return a
	}).Do()

	sshConfigPath := sshConfigFileName(t)

	runConfigSSH := func() {
		inv, root := clitest.New(t,
			"config-ssh",
			"--ssh-config-file", sshConfigPath,
			"--hostname-suffix", "coder",
			"--no-wildcard",
			"--yes",
		)
		//nolint:gocritic // This has always ran with the admin user.
		clitest.SetupConfig(t, client, root)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	}

	// hostLines extracts lines beginning with "Host " from the SSH config.
	// ProxyCommand lines embed a per-invocation temp path and are excluded so
	// that two runs with different global-config dirs can still be compared.
	hostLines := func(s string) []string {
		var out []string
		for line := range strings.SplitSeq(s, "\n") {
			if strings.HasPrefix(line, "Host ") {
				out = append(out, line)
			}
		}
		return out
	}

	runConfigSSH()
	config := sshConfigFileRead(t, sshConfigPath)

	// The server always injects a "coder." hostname prefix in addition to the
	// user-supplied "--hostname-suffix coder" entries. With stable workspace
	// names we can assert the complete, ordered host-entry list exactly.
	// ws-alpha sorts before ws-beta even though ws-alpha was created second.
	wantHosts := []string{
		"Host coder." + ws2.Workspace.Name,      // coder.ws-alpha
		"Host coder." + ws1.Workspace.Name,      // coder.ws-beta
		"Host " + ws2.Workspace.Name + ".coder", // ws-alpha.coder
		"Host " + ws1.Workspace.Name + ".coder", // ws-beta.coder
	}
	require.Empty(t, cmp.Diff(wantHosts, hostLines(config)))

	// No wildcard entries must appear in the Coder section.
	require.NotContains(t, config, "Host *.coder")
	require.NotContains(t, config, "Host *.")

	// The no-wildcard option must be persisted in the header.
	require.Contains(t, config, "# :no-wildcard=true")

	// Running the command again must yield identical host entries, confirming
	// that the ordering is stable across runs.
	runConfigSSH()
	require.Empty(t, cmp.Diff(wantHosts, hostLines(sshConfigFileRead(t, sshConfigPath))))
}
