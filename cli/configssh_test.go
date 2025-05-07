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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
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
	pty := ptytest.New(t)
	inv.Stdin = pty.Input()
	inv.Stdout = pty.Output()

	waiter := clitest.StartWithWaiter(t, inv)

	matches := []struct {
		match, write string
	}{
		{match: "Continue?", write: "yes"},
	}
	for _, m := range matches {
		pty.ExpectMatch(m.match)
		pty.WriteLine(m.write)
	}

	waiter.RequireSuccess()

	fileContents, err := os.ReadFile(sshConfigFile)
	require.NoError(t, err, "read ssh config file")
	require.Contains(t, string(fileContents), expectedKey, "ssh config file contains expected key")
	require.NotContains(t, string(fileContents), removeKey, "ssh config file should not have removed key")

	home := filepath.Dir(filepath.Dir(sshConfigFile))
	// #nosec
	sshCmd := exec.Command("ssh", "-F", sshConfigFile, hostname+r.Workspace.Name, "echo", "test")
	pty = ptytest.New(t)
	// Set HOME because coder config is included from ~/.ssh/coder.
	sshCmd.Env = append(sshCmd.Env, fmt.Sprintf("HOME=%s", home))
	inv.Stderr = pty.Output()
	data, err := sshCmd.Output()
	require.NoError(t, err)
	require.Equal(t, "test", strings.TrimSpace(string(data)))

	_ = listener.Close()
	<-copyDone
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

	// Check that the directory has proper permissions (0700)
	sshDirInfo, err := os.Stat(sshDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(os.ModePerm), sshDirInfo.Mode().Perm(), "directory should have 0700 permissions")
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
		regexMatch string
	}
	type match struct {
		match, write string
	}
	tests := []struct {
		name        string
		args        []string
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
					}, "\n")},
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
					}, "\n")},
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

			pty := ptytest.New(t)
			pty.Attach(inv)
			done := tGo(t, func() {
				err := inv.Run()
				if !tt.wantErr {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			})

			for _, m := range tt.matches {
				pty.ExpectMatch(m.match)
				pty.WriteLine(m.write)
			}

			<-done

			if len(tt.wantConfig.ssh) != 0 || tt.wantConfig.regexMatch != "" {
				got := sshConfigFileRead(t, sshConfigName)
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
			}
		})
	}
}
