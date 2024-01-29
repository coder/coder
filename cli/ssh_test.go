package cli_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	gosshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func setupWorkspaceForAgent(t *testing.T, mutations ...func([]*proto.Agent) []*proto.Agent) (*codersdk.Client, database.Workspace, string) {
	t.Helper()

	client, store := coderdtest.NewWithDatabase(t, nil)
	client.SetLogger(slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug))
	first := coderdtest.CreateFirstUser(t, client)
	userClient, user := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
	r := dbfake.WorkspaceBuild(t, store, database.Workspace{
		OrganizationID: first.OrganizationID,
		OwnerID:        user.ID,
	}).WithAgent(mutations...).Do()

	return userClient, r.Workspace, r.AgentToken
}

func TestSSH(t *testing.T) {
	t.Parallel()
	t.Run("ImmediateExit", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "ssh", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})
		pty.ExpectMatch("Waiting")

		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
		pty.WriteLine("exit")
		<-cmdDone
	})
	t.Run("StartStoppedWorkspace", func(t *testing.T) {
		t.Parallel()

		authToken := uuid.NewString()
		ownerClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, owner.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
		// Stop the workspace
		workspaceBuild := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspaceBuild.ID)

		// SSH to the workspace which should autostart it
		inv, root := clitest.New(t, "ssh", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		// When the agent connects, the workspace was started, and we should
		// have access to the shell.
		_ = agenttest.New(t, client.URL, authToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
		pty.WriteLine("exit")
		<-cmdDone
	})
	t.Run("RequireActiveVersion", func(t *testing.T) {
		t.Parallel()

		authToken := uuid.NewString()
		ownerClient := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleMember())

		echoResponses := &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		}

		version := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, echoResponses)
		coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, version.ID)
		template := coderdtest.CreateTemplate(t, ownerClient, owner.OrganizationID, version.ID)

		workspace := coderdtest.CreateWorkspace(t, client, owner.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
			cwr.AutomaticUpdates = codersdk.AutomaticUpdatesAlways
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// Stop the workspace
		workspaceBuild := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspaceBuild.ID)

		// Update template version
		version = coderdtest.UpdateTemplateVersion(t, ownerClient, owner.OrganizationID, echoResponses, template.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, version.ID)
		err := ownerClient.UpdateActiveTemplateVersion(context.Background(), template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: version.ID,
		})
		require.NoError(t, err)

		// SSH to the workspace which should auto-update and autostart it
		inv, root := clitest.New(t, "ssh", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		})

		// When the agent connects, the workspace was started, and we should
		// have access to the shell.
		_ = agenttest.New(t, client.URL, authToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
		pty.WriteLine("exit")
		<-cmdDone

		// Double-check if workspace's template version is up-to-date
		workspace, err = client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)
		assert.Equal(t, version.ID, workspace.TemplateActiveVersionID)
		assert.Equal(t, workspace.TemplateActiveVersionID, workspace.LatestBuild.TemplateVersionID)
		assert.False(t, workspace.Outdated)
	})

	t.Run("ShowTroubleshootingURLAfterTimeout", func(t *testing.T) {
		t.Parallel()

		wantURL := "https://example.com/troubleshoot"
		client, workspace, _ := setupWorkspaceForAgent(t, func(a []*proto.Agent) []*proto.Agent {
			// Unfortunately, one second is the lowest
			// we can go because 0 disables the feature.
			a[0].ConnectionTimeoutSeconds = 1
			a[0].TroubleshootingUrl = wantURL
			return a
		})
		inv, root := clitest.New(t, "ssh", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stderr = pty.Output()
		inv.Stdout = pty.Output()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.ErrorIs(t, err, cliui.Canceled)
		})
		pty.ExpectMatch(wantURL)
		cancel()
		<-cmdDone
	})

	t.Run("ExitOnStop", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("Windows doesn't seem to clean up the process, maybe #7100 will fix it")
		}

		store, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Pubsub: ps, Database: store})
		client.SetLogger(slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug))
		first := coderdtest.CreateFirstUser(t, client)
		userClient, user := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		r := dbfake.WorkspaceBuild(t, store, database.Workspace{
			OrganizationID: first.OrganizationID,
			OwnerID:        user.ID,
		}).WithAgent().Do()
		inv, root := clitest.New(t, "ssh", r.Workspace.Name)
		clitest.SetupConfig(t, userClient, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.Error(t, err)
		})
		pty.ExpectMatch("Waiting")

		_ = agenttest.New(t, client.URL, r.AgentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, r.Workspace.ID)

		// Ensure the agent is connected.
		pty.WriteLine("echo hell'o'")
		pty.ExpectMatchContext(ctx, "hello")

		_ = dbfake.WorkspaceBuild(t, store, r.Workspace).
			Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStop,
				BuildNumber: 2,
			}).
			Pubsub(ps).Do()
		t.Log("stopped workspace")

		select {
		case <-cmdDone:
		case <-ctx.Done():
			require.Fail(t, "command did not exit in time")
		}
	})

	t.Run("Stdio", func(t *testing.T) {
		t.Parallel()
		client, workspace, agentToken := setupWorkspaceForAgent(t)
		_, _ = tGoContext(t, func(ctx context.Context) {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect!
			_ = agenttest.New(t, client.URL, agentToken)
			<-ctx.Done()
		})

		clientOutput, clientInput := io.Pipe()
		serverOutput, serverInput := io.Pipe()
		defer func() {
			for _, c := range []io.Closer{clientOutput, clientInput, serverOutput, serverInput} {
				_ = c.Close()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t, "ssh", "--stdio", workspace.Name)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = clientOutput
		inv.Stdout = serverInput
		inv.Stderr = io.Discard

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
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
		defer conn.Close()

		sshClient := ssh.NewClient(conn, channels, requests)
		session, err := sshClient.NewSession()
		require.NoError(t, err)
		defer session.Close()

		command := "sh -c exit"
		if runtime.GOOS == "windows" {
			command = "cmd.exe /c exit"
		}
		err = session.Run(command)
		require.NoError(t, err)
		err = sshClient.Close()
		require.NoError(t, err)
		_ = clientOutput.Close()

		<-cmdDone
	})

	t.Run("Stdio_RemoteForward_Signal", func(t *testing.T) {
		t.Parallel()
		client, workspace, agentToken := setupWorkspaceForAgent(t)
		_, _ = tGoContext(t, func(ctx context.Context) {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect!
			_ = agenttest.New(t, client.URL, agentToken)
			<-ctx.Done()
		})

		clientOutput, clientInput := io.Pipe()
		serverOutput, serverInput := io.Pipe()
		defer func() {
			for _, c := range []io.Closer{clientOutput, clientInput, serverOutput, serverInput} {
				_ = c.Close()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t, "ssh", "--stdio", workspace.Name)
		fsn := clitest.NewFakeSignalNotifier(t)
		inv = inv.WithTestSignalNotifyContext(t, fsn.NotifyContext)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = clientOutput
		inv.Stdout = serverInput
		inv.Stderr = io.Discard

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
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
		defer conn.Close()

		sshClient := ssh.NewClient(conn, channels, requests)

		tmpdir := tempDirUnixSocket(t)

		remoteSock := path.Join(tmpdir, "remote.sock")
		_, err = sshClient.ListenUnix(remoteSock)
		require.NoError(t, err)

		fsn.Notify()
		<-cmdDone
		fsn.AssertStopped()
		require.Eventually(t, func() bool {
			_, err = os.Stat(remoteSock)
			return xerrors.Is(err, os.ErrNotExist)
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("Stdio_BrokenConn", func(t *testing.T) {
		t.Parallel()
		client, workspace, agentToken := setupWorkspaceForAgent(t)
		_, _ = tGoContext(t, func(ctx context.Context) {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect!
			_ = agenttest.New(t, client.URL, agentToken)
			<-ctx.Done()
		})

		clientOutput, clientInput := io.Pipe()
		serverOutput, serverInput := io.Pipe()
		defer func() {
			for _, c := range []io.Closer{clientOutput, clientInput, serverOutput, serverInput} {
				_ = c.Close()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t, "ssh", "--stdio", workspace.Name)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = clientOutput
		inv.Stdout = serverInput
		inv.Stderr = io.Discard

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
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
		defer conn.Close()

		sshClient := ssh.NewClient(conn, channels, requests)
		_ = serverOutput.Close()
		_ = clientInput.Close()
		select {
		case <-cmdDone:
			// OK
		case <-time.After(testutil.WaitShort):
			t.Error("timeout waiting for command to exit")
		}

		_ = sshClient.Close()
	})

	// Test that we handle OS signals properly while remote forwarding, and don't just leave the TCP
	// socket hanging.
	t.Run("RemoteForward_Unix_Signal", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("No unix sockets on windows")
		}
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		client, workspace, agentToken := setupWorkspaceForAgent(t)
		_, _ = tGoContext(t, func(ctx context.Context) {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect!
			_ = agenttest.New(t, client.URL, agentToken)
			<-ctx.Done()
		})

		tmpdir := tempDirUnixSocket(t)
		localSock := filepath.Join(tmpdir, "local.sock")
		l, err := net.Listen("unix", localSock)
		require.NoError(t, err)
		defer l.Close()
		remoteSock := path.Join(tmpdir, "remote.sock")
		for i := 0; i < 2; i++ {
			t.Logf("connect %d of 2", i+1)
			inv, root := clitest.New(t,
				"ssh",
				workspace.Name,
				"--remote-forward",
				remoteSock+":"+localSock,
			)
			fsn := clitest.NewFakeSignalNotifier(t)
			inv = inv.WithTestSignalNotifyContext(t, fsn.NotifyContext)
			inv.Stdout = io.Discard
			inv.Stderr = io.Discard

			clitest.SetupConfig(t, client, root)
			cmdDone := tGo(t, func() {
				err := inv.WithContext(ctx).Run()
				assert.Error(t, err)
			})

			// accept a single connection
			msgs := make(chan string, 1)
			go func() {
				conn, err := l.Accept()
				if !assert.NoError(t, err) {
					return
				}
				msg, err := io.ReadAll(conn)
				if !assert.NoError(t, err) {
					return
				}
				msgs <- string(msg)
			}()

			// Unfortunately, there is a race in crypto/ssh where it sends the request to forward
			// unix sockets before it is prepared to receive the response, meaning that even after
			// the socket exists on the file system, the client might not be ready to accept the
			// channel.
			//
			// https://cs.opensource.google/go/x/crypto/+/master:ssh/streamlocal.go;drc=2fc4c88bf43f0ea5ea305eae2b7af24b2cc93287;l=33
			//
			// To work around this, we attempt to send messages in a loop until one succeeds
			success := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				var (
					conn net.Conn
					err  error
				)
				for {
					time.Sleep(testutil.IntervalMedium)
					select {
					case <-ctx.Done():
						t.Error("timeout")
						return
					case <-success:
						return
					default:
						// Ok
					}
					conn, err = net.Dial("unix", remoteSock)
					if err != nil {
						t.Logf("dial error: %s", err)
						continue
					}
					_, err = conn.Write([]byte("test"))
					if err != nil {
						t.Logf("write error: %s", err)
					}
					err = conn.Close()
					if err != nil {
						t.Logf("close error: %s", err)
					}
				}
			}()

			msg := testutil.RequireRecvCtx(ctx, t, msgs)
			require.Equal(t, "test", msg)
			close(success)
			fsn.Notify()
			<-cmdDone
			fsn.AssertStopped()
			// wait for dial goroutine to complete
			_ = testutil.RequireRecvCtx(ctx, t, done)

			// wait for the remote socket to get cleaned up before retrying,
			// because cleaning up the socket happens asynchronously, and we
			// might connect to an old listener on the agent side.
			require.Eventually(t, func() bool {
				_, err = os.Stat(remoteSock)
				return xerrors.Is(err, os.ErrNotExist)
			}, testutil.WaitShort, testutil.IntervalFast)
		}
	})

	t.Run("StdioExitOnStop", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("Windows doesn't seem to clean up the process, maybe #7100 will fix it")
		}

		store, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Pubsub: ps, Database: store})
		client.SetLogger(slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug))
		first := coderdtest.CreateFirstUser(t, client)
		userClient, user := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		r := dbfake.WorkspaceBuild(t, store, database.Workspace{
			OrganizationID: first.OrganizationID,
			OwnerID:        user.ID,
		}).WithAgent().Do()

		_, _ = tGoContext(t, func(ctx context.Context) {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect.
			_ = agenttest.New(t, client.URL, r.AgentToken)
			<-ctx.Done()
		})

		clientOutput, clientInput := io.Pipe()
		serverOutput, serverInput := io.Pipe()
		defer func() {
			for _, c := range []io.Closer{clientOutput, clientInput, serverOutput, serverInput} {
				_ = c.Close()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t, "ssh", "--stdio", r.Workspace.Name)
		clitest.SetupConfig(t, userClient, root)
		inv.Stdin = clientOutput
		inv.Stdout = serverInput
		inv.Stderr = io.Discard

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
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
		defer conn.Close()

		sshClient := ssh.NewClient(conn, channels, requests)
		defer sshClient.Close()

		session, err := sshClient.NewSession()
		require.NoError(t, err)
		defer session.Close()

		err = session.Shell()
		require.NoError(t, err)

		_ = dbfake.WorkspaceBuild(t, store, r.Workspace).
			Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStop,
				BuildNumber: 2,
			}).
			Pubsub(ps).
			Do()
		t.Log("stopped workspace")

		select {
		case <-cmdDone:
		case <-ctx.Done():
			require.Fail(t, "command did not exit in time")
		}
	})

	t.Run("ForwardAgent", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Test not supported on windows")
		}

		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Generate private key.
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		kr := gosshagent.NewKeyring()
		kr.Add(gosshagent.AddedKey{
			PrivateKey: privateKey,
		})

		// Start up ssh agent listening on unix socket.
		tmpdir := tempDirUnixSocket(t)
		agentSock := filepath.Join(tmpdir, "agent.sock")
		l, err := net.Listen("unix", agentSock)
		require.NoError(t, err)
		defer l.Close()
		_ = tGo(t, func() {
			for {
				fd, err := l.Accept()
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						assert.NoError(t, err, "listener accept failed")
					}
					return
				}

				err = gosshagent.ServeAgent(kr, fd)
				if !errors.Is(err, io.EOF) {
					assert.NoError(t, err, "serve agent failed")
				}
				_ = fd.Close()
			}
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		inv, root := clitest.New(t,
			"ssh",
			workspace.Name,
			"--forward-agent",
			"--identity-agent", agentSock, // Overrides $SSH_AUTH_SOCK.
		)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stderr = pty.Output()
		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err, "ssh command failed")
		})

		// Wait for the prompt or any output really to indicate the command has
		// started and accepting input on stdin.
		_ = pty.Peek(ctx, 1)

		// Ensure that SSH_AUTH_SOCK is set.
		// Linux: /tmp/auth-agent3167016167/listener.sock
		// macOS: /var/folders/ng/m1q0wft14hj0t3rtjxrdnzsr0000gn/T/auth-agent3245553419/listener.sock
		pty.WriteLine(`env | grep SSH_AUTH_SOCK=`)
		pty.ExpectMatch("SSH_AUTH_SOCK=")
		// Ensure that ssh-add lists our key.
		pty.WriteLine("ssh-add -L")
		keys, err := kr.List()
		require.NoError(t, err, "list keys failed")
		pty.ExpectMatch(keys[0].String())

		// And we're done.
		pty.WriteLine("exit")
		<-cmdDone
	})

	t.Run("RemoteForward", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Test not supported on windows")
		}

		t.Parallel()

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello world"))
		}))
		defer httpServer.Close()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		inv, root := clitest.New(t,
			"ssh",
			workspace.Name,
			"--remote-forward",
			"8222:"+httpServer.Listener.Addr().String(),
		)
		clitest.SetupConfig(t, client, root)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			// fails because we cancel context to close
			assert.Error(t, err, "ssh command should fail")
		})

		require.Eventually(t, func() bool {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8222/", nil)
			if !assert.NoError(t, err) {
				// true exits the loop.
				return true
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Logf("HTTP GET http://localhost:8222/ %s", err)
				return false
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			assert.EqualValues(t, "hello world", body)
			return true
		}, testutil.WaitLong, testutil.IntervalFast)

		// And we're done.
		cancel()
		<-cmdDone
	})

	t.Run("RemoteForwardUnixSocket", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Test not supported on windows")
		}

		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		tmpdir := tempDirUnixSocket(t)
		localSock := filepath.Join(tmpdir, "local.sock")
		l, err := net.Listen("unix", localSock)
		require.NoError(t, err)
		defer l.Close()
		remoteSock := filepath.Join(tmpdir, "remote.sock")

		inv, root := clitest.New(t,
			"ssh",
			workspace.Name,
			"--remote-forward",
			fmt.Sprintf("%s:%s", remoteSock, localSock),
		)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stderr = pty.Output()
		cmdDone := tGo(t, func() {
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err, "ssh command failed")
		})

		// Wait for the prompt or any output really to indicate the command has
		// started and accepting input on stdin.
		_ = pty.Peek(ctx, 1)

		// Download the test page
		pty.WriteLine(fmt.Sprintf("ss -xl state listening src %s | wc -l", remoteSock))
		pty.ExpectMatch("2")

		// And we're done.
		pty.WriteLine("exit")
		<-cmdDone
	})

	// Test that we can forward a local unix socket to a remote unix socket and
	// that new SSH sessions take over the socket without closing active socket
	// connections.
	t.Run("RemoteForwardUnixSocketMultipleSessionsOverwrite", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Test not supported on windows")
		}

		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Wait super super long so this doesn't flake on -race test.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong*2)
		defer cancel()

		tmpdir := tempDirUnixSocket(t)

		localSock := filepath.Join(tmpdir, "local.sock")
		l, err := net.Listen("unix", localSock)
		require.NoError(t, err)
		defer l.Close()
		testutil.Go(t, func() {
			for {
				fd, err := l.Accept()
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						assert.NoError(t, err, "listener accept failed")
					}
					return
				}

				testutil.Go(t, func() {
					defer fd.Close()
					agentssh.Bicopy(ctx, fd, fd)
				})
			}
		})

		remoteSock := filepath.Join(tmpdir, "remote.sock")

		var done []func() error
		for i := 0; i < 2; i++ {
			id := fmt.Sprintf("ssh-%d", i)
			inv, root := clitest.New(t,
				"ssh",
				workspace.Name,
				"--remote-forward",
				fmt.Sprintf("%s:%s", remoteSock, localSock),
			)
			inv.Logger = inv.Logger.Named(id)
			clitest.SetupConfig(t, client, root)
			pty := ptytest.New(t).Attach(inv)
			inv.Stderr = pty.Output()
			cmdDone := tGo(t, func() {
				err := inv.WithContext(ctx).Run()
				assert.NoError(t, err, "ssh command failed: %s", id)
			})

			// Since something was output, it should be safe to write input.
			// This could show a prompt or "running startup scripts", so it's
			// not indicative of the SSH connection being ready.
			_ = pty.Peek(ctx, 1)

			// Ensure the SSH connection is ready by testing the shell
			// input/output.
			pty.WriteLine("echo ping' 'pong")
			pty.ExpectMatchContext(ctx, "ping pong")

			d := &net.Dialer{}
			fd, err := d.DialContext(ctx, "unix", remoteSock)
			require.NoError(t, err, id)

			// Ping / pong to ensure the socket is working.
			_, err = fd.Write([]byte("hello world"))
			require.NoError(t, err, id)

			buf := make([]byte, 11)
			_, err = fd.Read(buf)
			require.NoError(t, err, id)
			require.Equal(t, "hello world", string(buf), id)

			done = append(done, func() error {
				// Redo ping / pong to ensure that the socket
				// connections still work.
				_, err := fd.Write([]byte("hello world"))
				assert.NoError(t, err, id)

				buf := make([]byte, 11)
				_, err = fd.Read(buf)
				assert.NoError(t, err, id)
				assert.Equal(t, "hello world", string(buf), id)

				pty.WriteLine("exit")
				<-cmdDone
				return nil
			})
		}

		var eg errgroup.Group
		for _, d := range done {
			eg.Go(d)
		}
		err = eg.Wait()
		require.NoError(t, err)
	})

	// Test that we can remote forward multiple sockets, whether or not the
	// local sockets exists at the time of establishing xthe SSH connection.
	t.Run("RemoteForwardMultipleUnixSockets", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Test not supported on windows")
		}

		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Wait super long so this doesn't flake on -race test.
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()

		tmpdir := tempDirUnixSocket(t)

		type testSocket struct {
			local  string
			remote string
		}

		args := []string{"ssh", workspace.Name}
		var sockets []testSocket
		for i := 0; i < 2; i++ {
			localSock := filepath.Join(tmpdir, fmt.Sprintf("local-%d.sock", i))
			remoteSock := filepath.Join(tmpdir, fmt.Sprintf("remote-%d.sock", i))
			sockets = append(sockets, testSocket{
				local:  localSock,
				remote: remoteSock,
			})
			args = append(args, "--remote-forward", fmt.Sprintf("%s:%s", remoteSock, localSock))
		}

		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		inv.Stderr = pty.Output()

		w := clitest.StartWithWaiter(t, inv.WithContext(ctx))
		defer w.Wait() // We don't care about any exit error (exit code 255: SSH connection ended unexpectedly).

		// Since something was output, it should be safe to write input.
		// This could show a prompt or "running startup scripts", so it's
		// not indicative of the SSH connection being ready.
		_ = pty.Peek(ctx, 1)

		// Ensure the SSH connection is ready by testing the shell
		// input/output.
		pty.WriteLine("echo ping' 'pong")
		pty.ExpectMatchContext(ctx, "ping pong")

		for i, sock := range sockets {
			i := i
			// Start the listener on the "local machine".
			l, err := net.Listen("unix", sock.local)
			require.NoError(t, err)
			defer l.Close() //nolint:revive // Defer is fine in this loop, we only run it twice.
			testutil.Go(t, func() {
				for {
					fd, err := l.Accept()
					if err != nil {
						if !errors.Is(err, net.ErrClosed) {
							assert.NoError(t, err, "listener accept failed", i)
						}
						return
					}

					testutil.Go(t, func() {
						defer fd.Close()
						agentssh.Bicopy(ctx, fd, fd)
					})
				}
			})

			// Dial the forwarded socket on the "remote machine".
			d := &net.Dialer{}
			fd, err := d.DialContext(ctx, "unix", sock.remote)
			require.NoError(t, err, i)
			defer fd.Close() //nolint:revive // Defer is fine in this loop, we only run it twice.

			// Ping / pong to ensure the socket is working.
			_, err = fd.Write([]byte("hello world"))
			require.NoError(t, err, i)

			buf := make([]byte, 11)
			_, err = fd.Read(buf)
			require.NoError(t, err, i)
			require.Equal(t, "hello world", string(buf), i)
		}

		// And we're done.
		pty.WriteLine("exit")
	})

	t.Run("FileLogging", func(t *testing.T) {
		t.Parallel()

		logDir := t.TempDir()

		client, workspace, agentToken := setupWorkspaceForAgent(t)
		inv, root := clitest.New(t, "ssh", "-l", logDir, workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)

		pty.ExpectMatch("Waiting")

		agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
		pty.WriteLine("exit")
		w.RequireSuccess()

		ents, err := os.ReadDir(logDir)
		require.NoError(t, err)
		require.Len(t, ents, 1, "expected one file in logdir %s", logDir)
	})
}

//nolint:paralleltest // This test uses t.Setenv, parent test MUST NOT be parallel.
func TestSSH_ForwardGPG(t *testing.T) {
	if runtime.GOOS == "windows" {
		// While GPG forwarding from a Windows client works, we currently do
		// not support forwarding to a Windows workspace. Our tests use the
		// same platform for the "client" and "workspace" as they run in the
		// same process.
		t.Skip("Test not supported on windows")
	}
	if testing.Short() {
		t.SkipNow()
	}

	// This key is for dean@coder.com.
	const randPublicKeyFingerprint = "7BDFBA0CC7F5A96537C806C427BC6335EB5117F1"
	const randPublicKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----

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
p7KeSZdlk47pMBGOfnvEmoQ=
=OxHv
-----END PGP PUBLIC KEY BLOCK-----`

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	gpgPath, err := exec.LookPath("gpg")
	if err != nil {
		t.Skip("gpg not found")
	}
	gpgConfPath, err := exec.LookPath("gpgconf")
	if err != nil {
		t.Skip("gpgconf not found")
	}
	gpgAgentPath, err := exec.LookPath("gpg-agent")
	if err != nil {
		t.Skip("gpg-agent not found")
	}

	// Setup GPG home directory on the "client".
	gnupgHomeClient := tempDirUnixSocket(t)
	t.Setenv("GNUPGHOME", gnupgHomeClient)

	// Get the agent extra socket path.
	var (
		stdout = bytes.NewBuffer(nil)
		stderr = bytes.NewBuffer(nil)
	)
	c := exec.CommandContext(ctx, gpgConfPath, "--list-dir", "agent-extra-socket")
	c.Stdout = stdout
	c.Stderr = stderr
	err = c.Run()
	require.NoError(t, err, "get extra socket path failed: %s", stderr.String())
	extraSocketPath := strings.TrimSpace(stdout.String())

	// Generate private key non-interactively.
	genKeyScript := `
Key-Type: 1
Key-Length: 2048
Subkey-Type: 1
Subkey-Length: 2048
Name-Real: Coder Test
Name-Email: test@coder.com
Expire-Date: 0
%no-protection
`
	c = exec.CommandContext(ctx, gpgPath, "--batch", "--gen-key")
	c.Stdin = strings.NewReader(genKeyScript)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "generate key failed: %s", out)

	// Import a random public key.
	stdin := strings.NewReader(randPublicKey + "\n")
	c = exec.CommandContext(ctx, gpgPath, "--import", "-")
	c.Stdin = stdin
	out, err = c.CombinedOutput()
	require.NoError(t, err, "import key failed: %s", out)

	// Set ultimate trust on imported key.
	stdin = strings.NewReader(randPublicKeyFingerprint + ":6:\n")
	c = exec.CommandContext(ctx, gpgPath, "--import-ownertrust")
	c.Stdin = stdin
	out, err = c.CombinedOutput()
	require.NoError(t, err, "import ownertrust failed: %s", out)

	// Start the GPG agent.
	agentCmd := pty.CommandContext(ctx, gpgAgentPath, "--no-detach", "--extra-socket", extraSocketPath)
	agentCmd.Env = append(agentCmd.Env, "GNUPGHOME="+gnupgHomeClient)
	agentPTY, agentProc, err := pty.Start(agentCmd, pty.WithPTYOption(pty.WithGPGTTY()))
	require.NoError(t, err, "launch agent failed")
	defer func() {
		_ = agentProc.Kill()
		_ = agentPTY.Close()
	}()

	// Get the agent socket path in the "workspace".
	gnupgHomeWorkspace := tempDirUnixSocket(t)

	stdout = bytes.NewBuffer(nil)
	stderr = bytes.NewBuffer(nil)
	c = exec.CommandContext(ctx, gpgConfPath, "--list-dir", "agent-socket")
	c.Env = append(c.Env, "GNUPGHOME="+gnupgHomeWorkspace)
	c.Stdout = stdout
	c.Stderr = stderr
	err = c.Run()
	require.NoError(t, err, "get agent socket path in workspace failed: %s", stderr.String())
	workspaceAgentSocketPath := strings.TrimSpace(stdout.String())
	require.NotEqual(t, extraSocketPath, workspaceAgentSocketPath, "socket path should be different")

	client, workspace, agentToken := setupWorkspaceForAgent(t)

	_ = agenttest.New(t, client.URL, agentToken, func(o *agent.Options) {
		o.EnvironmentVariables = map[string]string{
			"GNUPGHOME": gnupgHomeWorkspace,
		}
	})
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	inv, root := clitest.New(t,
		"ssh",
		workspace.Name,
		"--forward-gpg",
	)
	clitest.SetupConfig(t, client, root)
	tpty := ptytest.New(t)
	inv.Stdin = tpty.Input()
	inv.Stdout = tpty.Output()
	inv.Stderr = tpty.Output()
	cmdDone := tGo(t, func() {
		err := inv.WithContext(ctx).Run()
		assert.NoError(t, err, "ssh command failed")
	})
	// Prevent the test from hanging if the asserts below kill the test
	// early. This will cause the command to exit with an error, which will
	// let the t.Cleanup'd `<-done` inside of `tGo` exit and not hang.
	// Without this, the test will hang forever on failure, preventing the
	// real error from being printed.
	t.Cleanup(cancel)

	// Wait for the prompt or any output really to indicate the command has
	// started and accepting input on stdin.
	_ = tpty.Peek(ctx, 1)

	tpty.WriteLine("echo hello 'world'")
	tpty.ExpectMatch("hello world")

	// Check the GNUPGHOME was correctly inherited via shell.
	tpty.WriteLine("env && echo env-''-command-done")
	match := tpty.ExpectMatch("env--command-done")
	require.Contains(t, match, "GNUPGHOME="+gnupgHomeWorkspace, match)

	// Get the agent extra socket path in the "workspace" via shell.
	tpty.WriteLine("gpgconf --list-dir agent-socket && echo gpgconf-''-agentsocket-command-done")
	tpty.ExpectMatch(workspaceAgentSocketPath)
	tpty.ExpectMatch("gpgconf--agentsocket-command-done")

	// List the keys in the "workspace".
	tpty.WriteLine("gpg --list-keys && echo gpg-''-listkeys-command-done")
	listKeysOutput := tpty.ExpectMatch("gpg--listkeys-command-done")
	require.Contains(t, listKeysOutput, "[ultimate] Coder Test <test@coder.com>")
	require.Contains(t, listKeysOutput, "[ultimate] Dean Sheather (work key) <dean@coder.com>")

	// Try to sign something. This demonstrates that the forwarding is
	// working as expected, since the workspace doesn't have access to the
	// private key directly and must use the forwarded agent.
	tpty.WriteLine("echo 'hello world' | gpg --clearsign && echo gpg-''-sign-command-done")
	tpty.ExpectMatch("BEGIN PGP SIGNED MESSAGE")
	tpty.ExpectMatch("Hash:")
	tpty.ExpectMatch("hello world")
	tpty.ExpectMatch("gpg--sign-command-done")

	// And we're done.
	tpty.WriteLine("exit")
	<-cmdDone
}

// tGoContext runs fn in a goroutine passing a context that will be
// canceled on test completion and wait until fn has finished executing.
// Done and cancel are returned for optionally waiting until completion
// or early cancellation.
//
// NOTE(mafredri): This could be moved to a helper library.
func tGoContext(t *testing.T, fn func(context.Context)) (done <-chan struct{}, cancel context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	doneC := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	go func() {
		fn(ctx)
		close(doneC)
	}()

	return doneC, cancel
}

// tGo runs fn in a goroutine and waits until fn has completed before
// test completion. Done is returned for optionally waiting for fn to
// exit.
//
// NOTE(mafredri): This could be moved to a helper library.
func tGo(t *testing.T, fn func()) (done <-chan struct{}) {
	t.Helper()

	doneC := make(chan struct{})
	t.Cleanup(func() {
		<-doneC
	})
	go func() {
		fn()
		close(doneC)
	}()

	return doneC
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

// tempDirUnixSocket returns a temporary directory that can safely hold unix
// sockets (probably).
//
// During tests on darwin we hit the max path length limit for unix sockets
// pretty easily in the default location, so this function uses /tmp instead to
// get shorter paths.
func tempDirUnixSocket(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		testName := strings.ReplaceAll(t.Name(), "/", "_")
		dir, err := os.MkdirTemp("/tmp", fmt.Sprintf("coder-test-%s-", testName))
		require.NoError(t, err, "create temp dir for gpg test")

		t.Cleanup(func() {
			err := os.RemoveAll(dir)
			assert.NoError(t, err, "remove temp dir", dir)
		})
		return dir
	}

	return t.TempDir()
}
