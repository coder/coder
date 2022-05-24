package cli_test

import (
	"context"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func setupWorkspaceForSSH(t *testing.T) (*codersdk.Client, codersdk.Workspace, string) {
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "dev",
						Type: "google_compute_instance",
						Agents: []*proto.Agent{{
							Id: uuid.NewString(),
							Auth: &proto.Agent_Token{
								Token: agentToken,
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

	return client, workspace, agentToken
}

func TestSSH(t *testing.T) {
	t.Parallel()
	t.Run("ImmediateExit", func(t *testing.T) {
		t.Parallel()
		client, workspace, agentToken := setupWorkspaceForSSH(t)
		cmd, root := clitest.New(t, "ssh", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetErr(pty.Output())
		cmd.SetOut(pty.Output())
		tGo(t, func() {
			err := cmd.Execute()
			assert.NoError(t, err)
		})
		pty.ExpectMatch("Waiting")
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
		agentClient := codersdk.New(client.URL)
		agentClient.SessionToken = agentToken
		agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
			Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		t.Cleanup(func() {
			_ = agentCloser.Close()
		})
		// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
		pty.WriteLine("exit")
	})
	t.Run("Stdio", func(t *testing.T) {
		t.Parallel()
		client, workspace, agentToken := setupWorkspaceForSSH(t)

		tGoContext(t, func(ctx context.Context) {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect!
			coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			agentClient := codersdk.New(client.URL)
			agentClient.SessionToken = agentToken
			agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
				Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
			})
			<-ctx.Done()
			_ = agentCloser.Close()
		})

		clientOutput, clientInput := io.Pipe()
		serverOutput, serverInput := io.Pipe()

		cmd, root := clitest.New(t, "ssh", "--stdio", workspace.Name)
		clitest.SetupConfig(t, client, root)
		cmd.SetIn(clientOutput)
		cmd.SetOut(serverInput)
		cmd.SetErr(io.Discard)
		tGo(t, func() {
			err := cmd.Execute()
			assert.NoError(t, err)
		})

		conn, channels, requests, err := ssh.NewClientConn(&stdioConn{
			Reader: serverOutput,
			Writer: clientInput,
		}, "", &ssh.ClientConfig{
			// #nosec
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		})
		require.NoError(t, err)
		sshClient := ssh.NewClient(conn, channels, requests)
		session, err := sshClient.NewSession()
		require.NoError(t, err)
		command := "sh -c exit"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c exit"
		}
		err = session.Run(command)
		require.NoError(t, err)
		err = sshClient.Close()
		require.NoError(t, err)
		_ = clientOutput.Close()
	})
}

// tGoContext runs fn in a goroutine passing a context that will be
// canceled on test completion and wait until fn has finished executing.
//
// NOTE(mafredri): This could be moved to a helper library.
func tGoContext(t *testing.T, fn func(context.Context)) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	go func() {
		fn(ctx)
		close(done)
	}()
}

// tGo runs fn in a goroutine and waits until fn has completed before
// test completion.
//
// NOTE(mafredri): This could be moved to a helper library.
func tGo(t *testing.T, fn func()) {
	t.Helper()

	done := make(chan struct{})
	t.Cleanup(func() {
		<-done
	})
	go func() {
		fn()
		close(done)
	}()
}

type stdioConn struct {
	io.Reader
	io.Writer
}

func (*stdioConn) Close() (err error) {
	return nil
}

func (*stdioConn) LocalAddr() net.Addr {
	return nil
}

func (*stdioConn) RemoteAddr() net.Addr {
	return nil
}

func (*stdioConn) SetDeadline(_ time.Time) error {
	return nil
}

func (*stdioConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (*stdioConn) SetWriteDeadline(_ time.Time) error {
	return nil
}
