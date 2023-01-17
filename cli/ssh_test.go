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
	"os"
	"os/exec"
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

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func setupWorkspaceForAgent(t *testing.T, mutate func([]*proto.Agent) []*proto.Agent) (*codersdk.Client, codersdk.Workspace, string) {
	t.Helper()
	if mutate == nil {
		mutate = func(a []*proto.Agent) []*proto.Agent {
			return a
		}
	}
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "dev",
						Type: "google_compute_instance",
						Agents: mutate([]*proto.Agent{{
							Id: uuid.NewString(),
							Auth: &proto.Agent_Token{
								Token: agentToken,
							},
						}}),
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	workspace, err := client.Workspace(context.Background(), workspace.ID)
	require.NoError(t, err)

	return client, workspace, agentToken
}

func TestSSH(t *testing.T) {
	t.Parallel()
	t.Run("ImmediateExit", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		cmd, root := clitest.New(t, "ssh", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetErr(pty.Output())
		cmd.SetOut(pty.Output())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := cmd.ExecuteContext(ctx)
			assert.NoError(t, err)
		})
		pty.ExpectMatch("Waiting")

		agentClient := codersdk.New(client.URL)
		agentClient.SetSessionToken(agentToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			Logger: slogtest.Make(t, nil).Named("agent"),
		})
		defer func() {
			_ = agentCloser.Close()
		}()

		// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
		pty.WriteLine("exit")
		<-cmdDone
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
		cmd, root := clitest.New(t, "ssh", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetErr(pty.Output())
		cmd.SetOut(pty.Output())

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		cmdDone := tGo(t, func() {
			err := cmd.ExecuteContext(ctx)
			assert.ErrorIs(t, err, context.Canceled)
		})
		pty.ExpectMatch(wantURL)
		cancel()
		<-cmdDone
	})
	t.Run("Stdio", func(t *testing.T) {
		t.Parallel()
		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)
		_, _ = tGoContext(t, func(ctx context.Context) {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect!
			agentClient := codersdk.New(client.URL)
			agentClient.SetSessionToken(agentToken)
			agentCloser := agent.New(agent.Options{
				Client: agentClient,
				Logger: slogtest.Make(t, nil).Named("agent"),
			})
			<-ctx.Done()
			_ = agentCloser.Close()
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

		cmd, root := clitest.New(t, "ssh", "--stdio", workspace.Name)
		clitest.SetupConfig(t, client, root)
		cmd.SetIn(clientOutput)
		cmd.SetOut(serverInput)
		cmd.SetErr(io.Discard)
		cmdDone := tGo(t, func() {
			err := cmd.ExecuteContext(ctx)
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
	t.Run("ForwardAgent", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Test not supported on windows")
		}

		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)

		agentClient := codersdk.New(client.URL)
		agentClient.SetSessionToken(agentToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			Logger: slogtest.Make(t, nil).Named("agent"),
		})
		defer agentCloser.Close()

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

		cmd, root := clitest.New(t,
			"ssh",
			workspace.Name,
			"--forward-agent",
			"--identity-agent", agentSock, // Overrides $SSH_AUTH_SOCK.
		)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		cmd.SetErr(pty.Output())
		cmdDone := tGo(t, func() {
			err := cmd.ExecuteContext(ctx)
			assert.NoError(t, err, "ssh command failed")
		})

		// Ensure that SSH_AUTH_SOCK is set.
		// Linux: /tmp/auth-agent3167016167/listener.sock
		// macOS: /var/folders/ng/m1q0wft14hj0t3rtjxrdnzsr0000gn/T/auth-agent3245553419/listener.sock
		pty.WriteLine("env")
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

	//nolint:paralleltest // This test uses t.Setenv.
	t.Run("ForwardGPG", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			// While GPG forwarding from a Windows client works, we currently do
			// not support forwarding to a Windows workspace. Our tests use the
			// same platform for the "client" and "workspace" as they run in the
			// same process.
			t.Skip("Test not supported on windows")
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
		agentCmd := exec.CommandContext(ctx, gpgAgentPath, "--no-detach", "--extra-socket", extraSocketPath)
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

		client, workspace, agentToken := setupWorkspaceForAgent(t, nil)

		agentClient := codersdk.New(client.URL)
		agentClient.SetSessionToken(agentToken)
		agentCloser := agent.New(agent.Options{
			Client: agentClient,
			EnvironmentVariables: map[string]string{
				"GNUPGHOME": gnupgHomeWorkspace,
			},
			Logger: slogtest.Make(t, nil).Named("agent"),
		})
		defer agentCloser.Close()

		cmd, root := clitest.New(t,
			"ssh",
			workspace.Name,
			"--forward-gpg",
		)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		cmd.SetErr(pty.Output())
		cmdDone := tGo(t, func() {
			err := cmd.ExecuteContext(ctx)
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
		_ = pty.Peek(ctx, 1)

		pty.WriteLine("echo hello 'world'")
		pty.ExpectMatch("hello world")

		// Check the GNUPGHOME was correctly inherited via shell.
		pty.WriteLine("env && echo env-''-command-done")
		match := pty.ExpectMatch("env--command-done")
		require.Contains(t, match, "GNUPGHOME="+gnupgHomeWorkspace, match)

		// Get the agent extra socket path in the "workspace" via shell.
		pty.WriteLine("gpgconf --list-dir agent-socket && echo gpgconf-''-agentsocket-command-done")
		pty.ExpectMatch(workspaceAgentSocketPath)
		pty.ExpectMatch("gpgconf--agentsocket-command-done")

		// List the keys in the "workspace".
		pty.WriteLine("gpg --list-keys && echo gpg-''-listkeys-command-done")
		listKeysOutput := pty.ExpectMatch("gpg--listkeys-command-done")
		require.Contains(t, listKeysOutput, "[ultimate] Coder Test <test@coder.com>")
		require.Contains(t, listKeysOutput, "[ultimate] Dean Sheather (work key) <dean@coder.com>")

		// Try to sign something. This demonstrates that the forwarding is
		// working as expected, since the workspace doesn't have access to the
		// private key directly and must use the forwarded agent.
		pty.WriteLine("echo 'hello world' | gpg --clearsign && echo gpg-''-sign-command-done")
		pty.ExpectMatch("BEGIN PGP SIGNED MESSAGE")
		pty.ExpectMatch("Hash:")
		pty.ExpectMatch("hello world")
		pty.ExpectMatch("gpg--sign-command-done")

		// And we're done.
		pty.WriteLine("exit")
		<-cmdDone
	})
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
