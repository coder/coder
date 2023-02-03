package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
	gosshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/autobuild/notify"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

var (
	workspacePollInterval   = time.Minute
	autostopNotifyCountdown = []time.Duration{30 * time.Minute}
)

func ssh() *cobra.Command {
	var (
		stdio          bool
		shuffle        bool
		forwardAgent   bool
		forwardGPG     bool
		identityAgent  string
		wsPollInterval time.Duration
		noWait         bool
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "ssh <workspace>",
		Short:       "Start a shell into a workspace",
		Args:        cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			if shuffle {
				err := cobra.ExactArgs(0)(cmd, args)
				if err != nil {
					return err
				}
			} else {
				err := cobra.MinimumNArgs(1)(cmd, args)
				if err != nil {
					return err
				}
			}

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, cmd, client, codersdk.Me, args[0], shuffle)
			if err != nil {
				return err
			}

			updateWorkspaceBanner, outdated := verifyWorkspaceOutdated(client, workspace)
			if outdated && isTTYErr(cmd) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), updateWorkspaceBanner)
			}

			// OpenSSH passes stderr directly to the calling TTY.
			// This is required in "stdio" mode so a connecting indicator can be displayed.
			err = cliui.Agent(ctx, cmd.ErrOrStderr(), cliui.AgentOptions{
				WorkspaceName: workspace.Name,
				Fetch: func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
					return client.WorkspaceAgent(ctx, workspaceAgent.ID)
				},
				NoWait: noWait,
			})
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					return cliui.Canceled
				}
				if xerrors.Is(err, cliui.AgentStartError) {
					return xerrors.New("Agent startup script exited with non-zero status, use --no-wait to login anyway.")
				}
				return xerrors.Errorf("await agent: %w", err)
			}

			conn, err := client.DialWorkspaceAgent(ctx, workspaceAgent.ID, &codersdk.DialWorkspaceAgentOptions{})
			if err != nil {
				return err
			}
			defer conn.Close()
			conn.AwaitReachable(ctx)
			stopPolling := tryPollWorkspaceAutostop(ctx, client, workspace)
			defer stopPolling()

			if stdio {
				rawSSH, err := conn.SSH(ctx)
				if err != nil {
					return err
				}
				defer rawSSH.Close()

				go func() {
					_, _ = io.Copy(cmd.OutOrStdout(), rawSSH)
				}()
				_, _ = io.Copy(rawSSH, cmd.InOrStdin())
				return nil
			}

			sshClient, err := conn.SSHClient(ctx)
			if err != nil {
				return err
			}
			defer sshClient.Close()

			sshSession, err := sshClient.NewSession()
			if err != nil {
				return err
			}
			defer sshSession.Close()

			// Ensure context cancellation is propagated to the
			// SSH session, e.g. to cancel `Wait()` at the end.
			go func() {
				<-ctx.Done()
				_ = sshSession.Close()
			}()

			if identityAgent == "" {
				identityAgent = os.Getenv("SSH_AUTH_SOCK")
			}
			if forwardAgent && identityAgent != "" {
				err = gosshagent.ForwardToRemote(sshClient, identityAgent)
				if err != nil {
					return xerrors.Errorf("forward agent: %w", err)
				}
				err = gosshagent.RequestAgentForwarding(sshSession)
				if err != nil {
					return xerrors.Errorf("request agent forwarding failed: %w", err)
				}
			}

			if forwardGPG {
				if workspaceAgent.OperatingSystem == "windows" {
					return xerrors.New("GPG forwarding is not supported for Windows workspaces")
				}

				err = uploadGPGKeys(ctx, sshClient)
				if err != nil {
					return xerrors.Errorf("upload GPG public keys and ownertrust to workspace: %w", err)
				}
				closer, err := forwardGPGAgent(ctx, cmd.ErrOrStderr(), sshClient)
				if err != nil {
					return xerrors.Errorf("forward GPG socket: %w", err)
				}
				defer closer.Close()
			}

			stdoutFile, validOut := cmd.OutOrStdout().(*os.File)
			stdinFile, validIn := cmd.InOrStdin().(*os.File)
			if validOut && validIn && isatty.IsTerminal(stdoutFile.Fd()) {
				state, err := term.MakeRaw(int(stdinFile.Fd()))
				if err != nil {
					return err
				}
				defer func() {
					_ = term.Restore(int(stdinFile.Fd()), state)
				}()

				windowChange := listenWindowSize(ctx)
				go func() {
					for {
						select {
						case <-ctx.Done():
							return
						case <-windowChange:
						}
						width, height, err := term.GetSize(int(stdoutFile.Fd()))
						if err != nil {
							continue
						}
						_ = sshSession.WindowChange(height, width)
					}
				}()
			}

			err = sshSession.RequestPty("xterm-256color", 128, 128, gossh.TerminalModes{})
			if err != nil {
				return err
			}

			sshSession.Stdin = cmd.InOrStdin()
			sshSession.Stdout = cmd.OutOrStdout()
			sshSession.Stderr = cmd.ErrOrStderr()

			err = sshSession.Shell()
			if err != nil {
				return err
			}

			// Put cancel at the top of the defer stack to initiate
			// shutdown of services.
			defer cancel()

			if validOut {
				// Set initial window size.
				width, height, err := term.GetSize(int(stdoutFile.Fd()))
				if err == nil {
					_ = sshSession.WindowChange(height, width)
				}
			}

			err = sshSession.Wait()
			if err != nil {
				// If the connection drops unexpectedly, we get an
				// ExitMissingError but no other error details, so try to at
				// least give the user a better message
				if errors.Is(err, &gossh.ExitMissingError{}) {
					return xerrors.New("SSH connection ended unexpectedly")
				}
				return err
			}

			return nil
		},
	}
	cliflag.BoolVarP(cmd.Flags(), &stdio, "stdio", "", "CODER_SSH_STDIO", false, "Specifies whether to emit SSH output over stdin/stdout.")
	cliflag.BoolVarP(cmd.Flags(), &shuffle, "shuffle", "", "CODER_SSH_SHUFFLE", false, "Specifies whether to choose a random workspace")
	_ = cmd.Flags().MarkHidden("shuffle")
	cliflag.BoolVarP(cmd.Flags(), &forwardAgent, "forward-agent", "A", "CODER_SSH_FORWARD_AGENT", false, "Specifies whether to forward the SSH agent specified in $SSH_AUTH_SOCK")
	cliflag.BoolVarP(cmd.Flags(), &forwardGPG, "forward-gpg", "G", "CODER_SSH_FORWARD_GPG", false, "Specifies whether to forward the GPG agent. Unsupported on Windows workspaces, but supports all clients. Requires gnupg (gpg, gpgconf) on both the client and workspace. The GPG agent must already be running locally and will not be started for you. If a GPG agent is already running in the workspace, it will be attempted to be killed.")
	cliflag.StringVarP(cmd.Flags(), &identityAgent, "identity-agent", "", "CODER_SSH_IDENTITY_AGENT", "", "Specifies which identity agent to use (overrides $SSH_AUTH_SOCK), forward agent must also be enabled")
	cliflag.DurationVarP(cmd.Flags(), &wsPollInterval, "workspace-poll-interval", "", "CODER_WORKSPACE_POLL_INTERVAL", workspacePollInterval, "Specifies how often to poll for workspace automated shutdown.")
	cliflag.BoolVarP(cmd.Flags(), &noWait, "no-wait", "", "CODER_SSH_NO_WAIT", false, "Specifies whether to wait for a workspace to become ready before logging in (only applicable when the login before ready option has not been enabled). Note that the workspace agent may still be in the process of executing the startup script and the workspace may be in an incomplete state.")
	return cmd
}

// getWorkspaceAgent returns the workspace and agent selected using either the
// `<workspace>[.<agent>]` syntax via `in` or picks a random workspace and agent
// if `shuffle` is true.
func getWorkspaceAndAgent(ctx context.Context, cmd *cobra.Command, client *codersdk.Client, userID string, in string, shuffle bool) (codersdk.Workspace, codersdk.WorkspaceAgent, error) { //nolint:revive
	var (
		workspace      codersdk.Workspace
		workspaceParts = strings.Split(in, ".")
		err            error
	)
	if shuffle {
		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: userID,
		})
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
		if len(res.Workspaces) == 0 {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("no workspaces to shuffle")
		}

		workspace, err = cryptorand.Element(res.Workspaces)
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	} else {
		workspace, err = namedWorkspace(cmd, client, workspaceParts[0])
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	}

	if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("workspace must be in start transition to ssh")
	}
	if workspace.LatestBuild.Job.CompletedAt == nil {
		err := cliui.WorkspaceBuild(ctx, cmd.ErrOrStderr(), client, workspace.LatestBuild.ID)
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	}
	if workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionDelete {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q is being deleted", workspace.Name)
	}

	resources := workspace.LatestBuild.Resources

	agents := make([]codersdk.WorkspaceAgent, 0)
	for _, resource := range resources {
		agents = append(agents, resource.Agents...)
	}
	if len(agents) == 0 {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q has no agents", workspace.Name)
	}
	var workspaceAgent codersdk.WorkspaceAgent
	if len(workspaceParts) >= 2 {
		for _, otherAgent := range agents {
			if otherAgent.Name != workspaceParts[1] {
				continue
			}
			workspaceAgent = otherAgent
			break
		}
		if workspaceAgent.ID == uuid.Nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("agent not found by name %q", workspaceParts[1])
		}
	}
	if workspaceAgent.ID == uuid.Nil {
		if len(agents) > 1 {
			if !shuffle {
				return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("you must specify the name of an agent")
			}
			workspaceAgent, err = cryptorand.Element(agents)
			if err != nil {
				return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
			}
		} else {
			workspaceAgent = agents[0]
		}
	}

	return workspace, workspaceAgent, nil
}

// Attempt to poll workspace autostop. We write a per-workspace lockfile to
// avoid spamming the user with notifications in case of multiple instances
// of the CLI running simultaneously.
func tryPollWorkspaceAutostop(ctx context.Context, client *codersdk.Client, workspace codersdk.Workspace) (stop func()) {
	lock := flock.New(filepath.Join(os.TempDir(), "coder-autostop-notify-"+workspace.ID.String()))
	condition := notifyCondition(ctx, client, workspace.ID, lock)
	return notify.Notify(condition, workspacePollInterval, autostopNotifyCountdown...)
}

// Notify the user if the workspace is due to shutdown.
func notifyCondition(ctx context.Context, client *codersdk.Client, workspaceID uuid.UUID, lock *flock.Flock) notify.Condition {
	return func(now time.Time) (deadline time.Time, callback func()) {
		// Keep trying to regain the lock.
		locked, err := lock.TryLockContext(ctx, workspacePollInterval)
		if err != nil || !locked {
			return time.Time{}, nil
		}

		ws, err := client.Workspace(ctx, workspaceID)
		if err != nil {
			return time.Time{}, nil
		}

		if ptr.NilOrZero(ws.TTLMillis) {
			return time.Time{}, nil
		}

		deadline = ws.LatestBuild.Deadline.Time
		callback = func() {
			ttl := deadline.Sub(now)
			var title, body string
			if ttl > time.Minute {
				title = fmt.Sprintf(`Workspace %s stopping soon`, ws.Name)
				body = fmt.Sprintf(
					`Your Coder workspace %s is scheduled to stop in %.0f mins`, ws.Name, ttl.Minutes())
			} else {
				title = fmt.Sprintf("Workspace %s stopping!", ws.Name)
				body = fmt.Sprintf("Your Coder workspace %s is stopping any time now!", ws.Name)
			}
			// notify user with a native system notification (best effort)
			_ = beeep.Notify(title, body, "")
		}
		return deadline.Truncate(time.Minute), callback
	}
}

// Verify if the user workspace is outdated and prepare an actionable message for user.
func verifyWorkspaceOutdated(client *codersdk.Client, workspace codersdk.Workspace) (string, bool) {
	if !workspace.Outdated {
		return "", false // workspace is up-to-date
	}

	workspaceLink := buildWorkspaceLink(client.URL, workspace)
	return fmt.Sprintf("👋 Your workspace is outdated! Update it here: %s\n", workspaceLink), true
}

// Build the user workspace link which navigates to the Coder web UI.
func buildWorkspaceLink(serverURL *url.URL, workspace codersdk.Workspace) *url.URL {
	return serverURL.ResolveReference(&url.URL{Path: fmt.Sprintf("@%s/%s", workspace.OwnerName, workspace.Name)})
}

// runLocal runs a command on the local machine.
func runLocal(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin

	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr := new(exec.ExitError); errors.As(err, &exitErr) {
			stderr = exitErr.Stderr
		}

		return out, xerrors.Errorf(
			"`%s %s` failed: stderr: %s\n\nstdout: %s\n\n%w",
			name,
			strings.Join(args, " "),
			bytes.TrimSpace(stderr),
			bytes.TrimSpace(out),
			err,
		)
	}

	return out, nil
}

// runRemoteSSH runs a command on a remote machine/workspace via SSH.
func runRemoteSSH(sshClient *gossh.Client, stdin io.Reader, cmd string) ([]byte, error) {
	sess, err := sshClient.NewSession()
	if err != nil {
		return nil, xerrors.Errorf("create SSH session")
	}
	defer sess.Close()

	stderr := bytes.NewBuffer(nil)
	sess.Stdin = stdin
	// On fish, this was outputting to stderr instead of stdout.
	// The tests pass differently on different Linux machines,
	// so it's best we capture the output of both.
	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return out, xerrors.Errorf(
			"`%s` failed: stderr: %s\n\nstdout: %s:\n\n%w",
			cmd,
			bytes.TrimSpace(stderr.Bytes()),
			bytes.TrimSpace(out),
			err,
		)
	}

	return out, nil
}

func uploadGPGKeys(ctx context.Context, sshClient *gossh.Client) error {
	// Check if the agent is running in the workspace already.
	//
	// Note: we don't support windows in the workspace for GPG forwarding so
	//       using shell commands is fine.
	//
	// Note: we sleep after killing the agent because it doesn't always die
	//       immediately.
	agentSocketBytes, err := runRemoteSSH(sshClient, nil, `sh -c '
set -eux
agent_socket=$(gpgconf --list-dir agent-socket)
echo "$agent_socket"
if [ -S "$agent_socket" ]; then
  echo "agent socket exists, attempting to kill it" >&2
  gpgconf --kill gpg-agent
  rm -f "$agent_socket"
  sleep 1
fi

test ! -S "$agent_socket"
'`)
	agentSocket := strings.TrimSpace(string(agentSocketBytes))
	if err != nil {
		return xerrors.Errorf("check if agent socket is running (check if %q exists): %w", agentSocket, err)
	}
	if agentSocket == "" {
		return xerrors.Errorf("agent socket path is empty, check the output of `gpgconf --list-dir agent-socket`")
	}

	// Read the user's public keys and ownertrust from GPG.
	pubKeyExport, err := runLocal(ctx, nil, "gpg", "--armor", "--export")
	if err != nil {
		return xerrors.Errorf("export local public keys from GPG: %w", err)
	}
	ownerTrustExport, err := runLocal(ctx, nil, "gpg", "--export-ownertrust")
	if err != nil {
		return xerrors.Errorf("export local ownertrust from GPG: %w", err)
	}

	// Import the public keys and ownertrust into the workspace.
	_, err = runRemoteSSH(sshClient, bytes.NewReader(pubKeyExport), "gpg --import")
	if err != nil {
		return xerrors.Errorf("import public keys into workspace: %w", err)
	}
	_, err = runRemoteSSH(sshClient, bytes.NewReader(ownerTrustExport), "gpg --import-ownertrust")
	if err != nil {
		return xerrors.Errorf("import ownertrust into workspace: %w", err)
	}

	// Kill the agent in the workspace if it was started by one of the above
	// commands.
	_, err = runRemoteSSH(sshClient, nil, fmt.Sprintf("gpgconf --kill gpg-agent && rm -f %q", agentSocket))
	if err != nil {
		return xerrors.Errorf("kill existing agent in workspace: %w", err)
	}

	return nil
}

func localGPGExtraSocket(ctx context.Context) (string, error) {
	localSocket, err := runLocal(ctx, nil, "gpgconf", "--list-dir", "agent-extra-socket")
	if err != nil {
		return "", xerrors.Errorf("get local GPG agent socket: %w", err)
	}

	return string(bytes.TrimSpace(localSocket)), nil
}

func remoteGPGAgentSocket(sshClient *gossh.Client) (string, error) {
	remoteSocket, err := runRemoteSSH(sshClient, nil, "gpgconf --list-dir agent-socket")
	if err != nil {
		return "", xerrors.Errorf("get remote GPG agent socket: %w", err)
	}

	return string(bytes.TrimSpace(remoteSocket)), nil
}

// cookieAddr is a special net.Addr accepted by sshForward() which includes a
// cookie which is written to the connection before forwarding.
type cookieAddr struct {
	net.Addr
	cookie []byte
}

// sshForwardRemote starts forwarding connections from a remote listener to a
// local address via SSH in a goroutine.
//
// Accepts a `cookieAddr` as the local address.
func sshForwardRemote(ctx context.Context, stderr io.Writer, sshClient *gossh.Client, localAddr, remoteAddr net.Addr) (io.Closer, error) {
	listener, err := sshClient.Listen(remoteAddr.Network(), remoteAddr.String())
	if err != nil {
		return nil, xerrors.Errorf("listen on remote SSH address %s: %w", remoteAddr.String(), err)
	}

	go func() {
		for {
			remoteConn, err := listener.Accept()
			if err != nil {
				if ctx.Err() == nil {
					_, _ = fmt.Fprintf(stderr, "Accept SSH listener connection: %+v\n", err)
				}
				return
			}

			go func() {
				defer remoteConn.Close()

				localConn, err := net.Dial(localAddr.Network(), localAddr.String())
				if err != nil {
					_, _ = fmt.Fprintf(stderr, "Dial local address %s: %+v\n", localAddr.String(), err)
					return
				}
				defer localConn.Close()

				if c, ok := localAddr.(cookieAddr); ok {
					_, err = localConn.Write(c.cookie)
					if err != nil {
						_, _ = fmt.Fprintf(stderr, "Write cookie to local connection: %+v\n", err)
						return
					}
				}

				agent.Bicopy(ctx, localConn, remoteConn)
			}()
		}
	}()

	return listener, nil
}
