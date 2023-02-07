package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
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

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "example",
						}},
					}},
				},
			},
		}},
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "example",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
						}},
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent"),
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	agentConn, err := client.DialWorkspaceAgent(context.Background(), resources[0].Agents[0].ID, nil)
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
	cmd, root := clitest.New(t, "config-ssh",
		"--ssh-option", "HostName "+tcpAddr.IP.String(),
		"--ssh-option", "Port "+strconv.Itoa(tcpAddr.Port),
		"--ssh-config-file", sshConfigFile,
		"--skip-proxy-command")
	clitest.SetupConfig(t, client, root)
	doneChan := make(chan struct{})
	pty := ptytest.New(t)
	cmd.SetIn(pty.Input())
	cmd.SetOut(pty.Output())
	go func() {
		defer close(doneChan)
		err := cmd.Execute()
		assert.NoError(t, err)
	}()

	matches := []struct {
		match, write string
	}{
		{match: "Continue?", write: "yes"},
	}
	for _, m := range matches {
		pty.ExpectMatch(m.match)
		pty.WriteLine(m.write)
	}

	<-doneChan

	home := filepath.Dir(filepath.Dir(sshConfigFile))
	// #nosec
	sshCmd := exec.Command("ssh", "-F", sshConfigFile, "coder."+workspace.Name, "echo", "test")
	pty = ptytest.New(t)
	// Set HOME because coder config is included from ~/.ssh/coder.
	sshCmd.Env = append(sshCmd.Env, fmt.Sprintf("HOME=%s", home))
	sshCmd.Stderr = pty.Output()
	data, err := sshCmd.Output()
	require.NoError(t, err)
	require.Equal(t, "test", strings.TrimSpace(string(data)))

	_ = listener.Close()
	<-copyDone
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
		ssh string
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
	}{
		{
			name: "Config file is created",
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
			wantConfig: wantConfig{
				ssh: strings.Join([]string{
					baseHeader,
					"",
				}, "\n"),
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
				ssh: strings.Join([]string{
					"Host myhost",
					"	HostName myhost",
					baseHeader,
					"",
				}, "\n"),
			},
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Section is not moved on re-run",
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
				ssh: strings.Join([]string{
					"Host myhost",
					"	HostName myhost",
					"",
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
					"Host otherhost",
					"	HostName otherhost",
					"",
				}, "\n"),
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
				ssh: strings.Join([]string{
					baseHeader,
					"",
				}, "\n"),
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
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
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
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
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
				ssh: strings.Join([]string{
					baseHeader,
					"",
				}, "\n"),
			},
			matches: []match{
				{match: "Use new options?", write: "yes"},
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "No prompt on no changes",
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
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
			},
			args: []string{"--ssh-option", "ForwardAgent=yes"},
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
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
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
				ssh: strings.Join([]string{
					// Last options overwritten.
					baseHeader,
					"",
				}, "\n"),
			},
			args: []string{"--yes"},
		},
		{
			name: "Do not prompt for new options when prev opts flag is set",
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
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
					headerEnd,
					"",
				}, "\n"),
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
				ssh: strings.Join([]string{
					baseHeader,
					"",
				}, "\n"),
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
				_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			)

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
			cmd, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)

			pty := ptytest.New(t)
			cmd.SetIn(pty.Input())
			cmd.SetOut(pty.Output())
			done := tGo(t, func() {
				err := cmd.Execute()
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

			if tt.wantConfig.ssh != "" {
				got := sshConfigFileRead(t, sshConfigName)
				assert.Equal(t, tt.wantConfig.ssh, got)
			}
		})
	}
}

func TestConfigSSH_Hostnames(t *testing.T) {
	t.Parallel()

	type resourceSpec struct {
		name   string
		agents []string
	}
	tests := []struct {
		name      string
		resources []resourceSpec
		expected  []string
	}{
		{
			name: "one resource with one agent",
			resources: []resourceSpec{
				{name: "foo", agents: []string{"agent1"}},
			},
			expected: []string{"coder.@", "coder.@.agent1"},
		},
		{
			name: "one resource with two agents",
			resources: []resourceSpec{
				{name: "foo", agents: []string{"agent1", "agent2"}},
			},
			expected: []string{"coder.@.agent1", "coder.@.agent2"},
		},
		{
			name: "two resources with one agent",
			resources: []resourceSpec{
				{name: "foo", agents: []string{"agent1"}},
				{name: "bar"},
			},
			expected: []string{"coder.@", "coder.@.agent1"},
		},
		{
			name: "two resources with two agents",
			resources: []resourceSpec{
				{name: "foo", agents: []string{"agent1"}},
				{name: "bar", agents: []string{"agent2"}},
			},
			expected: []string{"coder.@.agent1", "coder.@.agent2"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var resources []*proto.Resource
			for _, resourceSpec := range tt.resources {
				resource := &proto.Resource{
					Name: resourceSpec.name,
					Type: "aws_instance",
				}
				for _, agentName := range resourceSpec.agents {
					resource.Agents = append(resource.Agents, &proto.Agent{
						Id:   uuid.NewString(),
						Name: agentName,
					})
				}
				resources = append(resources, resource)
			}

			provisionResponse := []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: resources,
					},
				},
			}}

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			// authToken := uuid.NewString()
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionPlan:  provisionResponse,
				ProvisionApply: provisionResponse,
			})
			coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

			sshConfigFile := sshConfigFileName(t)

			cmd, root := clitest.New(t, "config-ssh", "--ssh-config-file", sshConfigFile)
			clitest.SetupConfig(t, client, root)
			doneChan := make(chan struct{})
			pty := ptytest.New(t)
			cmd.SetIn(pty.Input())
			cmd.SetOut(pty.Output())
			go func() {
				defer close(doneChan)
				err := cmd.Execute()
				assert.NoError(t, err)
			}()

			matches := []struct {
				match, write string
			}{
				{match: "Continue?", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatch(m.match)
				pty.WriteLine(m.write)
			}

			<-doneChan

			var expectedHosts []string
			for _, hostnamePattern := range tt.expected {
				hostname := strings.ReplaceAll(hostnamePattern, "@", workspace.Name)
				expectedHosts = append(expectedHosts, hostname)
			}

			hosts := sshConfigFileParseHosts(t, sshConfigFile)
			require.ElementsMatch(t, expectedHosts, hosts)
		})
	}
}

// sshConfigFileParseHosts reads a file in the format of .ssh/config and extracts
// the hostnames that are listed in "Host" directives.
func sshConfigFileParseHosts(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)

	var result []string
	lineScanner := bufio.NewScanner(bytes.NewBuffer(b))
	for lineScanner.Scan() {
		line := lineScanner.Text()
		line = strings.TrimSpace(line)

		tokenScanner := bufio.NewScanner(bytes.NewBufferString(line))
		tokenScanner.Split(bufio.ScanWords)
		ok := tokenScanner.Scan()
		if ok && tokenScanner.Text() == "Host" {
			for tokenScanner.Scan() {
				result = append(result, tokenScanner.Text())
			}
		}
	}

	return result
}
