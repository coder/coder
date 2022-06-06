package cli_test

import (
	"context"
	"fmt"
	"io"
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
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	agentConn, err := client.DialWorkspaceAgent(context.Background(), resources[0].Agents[0].ID, nil)
	require.NoError(t, err)
	defer agentConn.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})
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
	t.Cleanup(func() {
		_ = listener.Close()
	})

	sshConfigFile, coderConfigFile := sshConfigFileNames(t)

	tcpAddr, valid := listener.Addr().(*net.TCPAddr)
	require.True(t, valid)
	cmd, root := clitest.New(t, "config-ssh",
		"--ssh-option", "HostName "+tcpAddr.IP.String(),
		"--ssh-option", "Port "+strconv.Itoa(tcpAddr.Port),
		"--ssh-config-file", sshConfigFile,
		"--test.ssh-coder-config-file", coderConfigFile,
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
	// Set HOME because coder config is included from ~/.ssh/coder.
	sshCmd.Env = append(sshCmd.Env, fmt.Sprintf("HOME=%s", home))
	sshCmd.Stderr = os.Stderr
	data, err := sshCmd.Output()
	require.NoError(t, err)
	require.Equal(t, "test", strings.TrimSpace(string(data)))
}

func TestConfigSSH_FileWriteAndOptionsFlow(t *testing.T) {
	t.Parallel()

	type writeConfig struct {
		ssh   string
		coder string
	}
	type wantConfig struct {
		ssh          string
		coder        string
		coderPartial bool
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
			name: "Config files are created",
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
			wantConfig: wantConfig{
				ssh: strings.Join([]string{
					"Include coder",
					"",
				}, "\n"),
				coder:        "# This file is managed by coder. DO NOT EDIT.",
				coderPartial: true,
			},
		},
		{
			name: "Include is written to top of ssh config",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"# This is a host",
					"Host test",
					"  HostName test",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: strings.Join([]string{
					"Include coder",
					"",
					"# This is a host",
					"Host test",
					"  HostName test",
				}, "\n"),
			},
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Include below Host is invalid, move it to the top",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"Host test",
					"  HostName test",
					"",
					"Include coder",
					"",
					"",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: strings.Join([]string{
					"Include coder",
					"",
					"Host test",
					"  HostName test",
					"",
					// Only "Include coder" with accompanying
					// newline is removed.
					"",
					"",
				}, "\n"),
			},
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "SSH Config does not need modification",
			writeConfig: writeConfig{
				ssh: strings.Join([]string{
					"Include something/other",
					"Include coder",
					"",
					"# This is a host",
					"Host test",
					"  HostName test",
				}, "\n"),
			},
			wantConfig: wantConfig{
				ssh: strings.Join([]string{
					"Include something/other",
					"Include coder",
					"",
					"# This is a host",
					"Host test",
					"  HostName test",
				}, "\n"),
			},
			matches: []match{
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "When options differ, selecting yes overwrites previous options",
			writeConfig: writeConfig{
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
				coder: strings.Join([]string{
					"# This file is managed by coder. DO NOT EDIT.",
					"#",
					"# You should not hand-edit this file, all changes will be lost when running",
					"# \"coder config-ssh\".",
					"#",
					"# Last config-ssh options:",
					"#",
				}, "\n"),
				coderPartial: true,
			},
			matches: []match{
				{match: "Use new options?", write: "yes"},
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "When options differ, selecting no preserves previous options",
			writeConfig: writeConfig{
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
				coderPartial: true,
			},
			matches: []match{
				{match: "Use new options?", write: "no"},
				{match: "Continue?", write: "yes"},
			},
		},
		{
			name: "Do not overwrite unknown coder config",
			writeConfig: writeConfig{
				coder: strings.Join([]string{
					"We're no strangers to love",
					"You know the rules and so do I (do I)",
				}, "\n"),
			},
			wantConfig: wantConfig{
				coder: strings.Join([]string{
					"We're no strangers to love",
					"You know the rules and so do I (do I)",
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
				"--test.default-ssh-config-file", sshConfigName,
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
			if tt.wantConfig.coder != "" {
				got := sshConfigFileRead(t, coderConfigName)
				if tt.wantConfig.coderPartial {
					assert.Contains(t, got, tt.wantConfig.coder)
				} else {
					assert.Equal(t, tt.wantConfig.coder, got)
				}
			}
		})
	}
}
