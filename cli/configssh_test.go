package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func sshConfigFileNames(t *testing.T) (sshConfig string, coderConfig string) {
	t.Helper()
	tmpdir := t.TempDir()
	dotssh := filepath.Join(tmpdir, ".ssh")
	err := os.Mkdir(dotssh, 0o700)
	require.NoError(t, err)
	n1 := filepath.Join(dotssh, "config")
	n2 := filepath.Join(dotssh, "coder")
	return n1, n2
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

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionDryRun: []*proto.Provision_Response{{
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
		Provision: []*proto.Provision_Response{{
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
	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
		Logger: slogtest.Make(t, nil),
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	agentConn, err := client.DialWorkspaceAgent(context.Background(), resources[0].Agents[0].ID, nil)
	require.NoError(t, err)
	defer agentConn.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			ssh, err := agentConn.SSH()
			assert.NoError(t, err)
			go io.Copy(conn, ssh)
			go io.Copy(ssh, conn)
		}
	}()

	sshConfigFile, _ := sshConfigFileNames(t)

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
		ssh   string
		coder string
	}
	type wantConfig struct {
		ssh       string
		coderKept bool
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

		// Tests for deprecated split coder config.
		{
			name: "Do not overwrite unknown coder config",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					baseHeader,
					"",
				}, "\n"),
				coder: strings.Join([]string{
					"We're no strangers to love",
					"You know the rules and so do I (do I)",
				}, "\n"),
			},
			wantConfig: wantConfig{
				coderKept: true,
			},
		},
		{
			name: "Transfer options from coder to ssh config",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"Include coder",
					"",
				}, "\n"),
				coder: strings.Join([]string{
					"# This file is managed by coder. DO NOT EDIT.",
					"#",
					"# You should not hand-edit this file, all changes will be lost when running",
					"# \"coder config-ssh\".",
					"#",
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
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
			matches: []match{
				{match: "Use new options?", write: "no"},
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Allow overwriting previous options from coder config",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"Include coder",
					"",
				}, "\n"),
				coder: strings.Join([]string{
					"# This file is managed by coder. DO NOT EDIT.",
					"#",
					"# You should not hand-edit this file, all changes will be lost when running",
					"# \"coder config-ssh\".",
					"#",
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
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
			name: "Allow overwriting previous options from coder config when they differ",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"Include coder",
					"",
				}, "\n"),
				coder: strings.Join([]string{
					"# This file is managed by coder. DO NOT EDIT.",
					"#",
					"# You should not hand-edit this file, all changes will be lost when running",
					"# \"coder config-ssh\".",
					"#",
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=yes",
					"#",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: strings.Join([]string{
					headerStart,
					"# Last config-ssh options:",
					"# :ssh-option=ForwardAgent=no",
					"#",
					headerEnd,
					"",
				}, "\n"),
			},
			args: []string{"--ssh-option", "ForwardAgent=no"},
			matches: []match{
				{match: "Use new options?", write: "yes"},
				{match: "Continue?", write: "yes"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
				user      = coderdtest.CreateFirstUser(t, client)
				version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
				_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
				project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
				workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
				_         = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			)

			// Prepare ssh config files.
			sshConfigName, coderConfigName := sshConfigFileNames(t)
			if tt.writeConfig.ssh != "" {
				sshConfigFileCreate(t, sshConfigName, strings.NewReader(tt.writeConfig.ssh))
			}
			if tt.writeConfig.coder != "" {
				sshConfigFileCreate(t, coderConfigName, strings.NewReader(tt.writeConfig.coder))
			}

			args := []string{
				"config-ssh",
				"--ssh-config-file", sshConfigName,
				"--test.ssh-coder-config-file", coderConfigName,
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
			if !tt.wantConfig.coderKept {
				_, err := os.ReadFile(coderConfigName)
				assert.ErrorIs(t, err, fs.ErrNotExist)
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

			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user := coderdtest.CreateFirstUser(t, client)
			// authToken := uuid.NewString()
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse:           echo.ParseComplete,
				ProvisionDryRun: provisionResponse,
				Provision:       provisionResponse,
			})
			coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
			coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

			sshConfigFile, _ := sshConfigFileNames(t)

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
