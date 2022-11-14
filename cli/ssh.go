package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
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
		identityAgent  string
		wsPollInterval time.Duration
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
			})
			if err != nil {
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
					return xerrors.Errorf("forward agent failed: %w", err)
				}
				err = gosshagent.RequestAgentForwarding(sshSession)
				if err != nil {
					return xerrors.Errorf("request agent forwarding failed: %w", err)
				}
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
				// If the connection drops unexpectedly, we get an ExitMissingError but no other
				// error details, so try to at least give the user a better message
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
	cliflag.StringVarP(cmd.Flags(), &identityAgent, "identity-agent", "", "CODER_SSH_IDENTITY_AGENT", "", "Specifies which identity agent to use (overrides $SSH_AUTH_SOCK), forward agent must also be enabled")
	cliflag.DurationVarP(cmd.Flags(), &wsPollInterval, "workspace-poll-interval", "", "CODER_WORKSPACE_POLL_INTERVAL", workspacePollInterval, "Specifies how often to poll for workspace automated shutdown.")
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
			Owner: codersdk.Me,
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
