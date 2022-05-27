package cli

import (
	"context"
	"fmt"
	"io"
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
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

var workspacePollInterval = time.Minute
var autostopNotifyCountdown = []time.Duration{30 * time.Minute}

func ssh() *cobra.Command {
	var (
		stdio          bool
		shuffle        bool
		forwardAgent   bool
		wsPollInterval time.Duration
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "ssh <workspace>",
		Short:       "SSH into a workspace",
		Args:        cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
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

			workspace, agent, err := getWorkspaceAndAgent(cmd, client, organization.ID, codersdk.Me, args[0], shuffle)
			if err != nil {
				return err
			}

			// OpenSSH passes stderr directly to the calling TTY.
			// This is required in "stdio" mode so a connecting indicator can be displayed.
			err = cliui.Agent(cmd.Context(), cmd.ErrOrStderr(), cliui.AgentOptions{
				WorkspaceName: workspace.Name,
				Fetch: func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
					return client.WorkspaceAgent(ctx, agent.ID)
				},
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}

			conn, err := client.DialWorkspaceAgent(cmd.Context(), agent.ID, nil)
			if err != nil {
				return err
			}
			defer conn.Close()

			stopPolling := tryPollWorkspaceAutostop(cmd.Context(), client, workspace)
			defer stopPolling()

			if stdio {
				rawSSH, err := conn.SSH()
				if err != nil {
					return err
				}
				go func() {
					_, _ = io.Copy(cmd.OutOrStdout(), rawSSH)
				}()
				_, _ = io.Copy(rawSSH, cmd.InOrStdin())
				return nil
			}
			sshClient, err := conn.SSHClient()
			if err != nil {
				return err
			}

			sshSession, err := sshClient.NewSession()
			if err != nil {
				return err
			}

			if forwardAgent && os.Getenv("SSH_AUTH_SOCK") != "" {
				err = gosshagent.ForwardToRemote(sshClient, os.Getenv("SSH_AUTH_SOCK"))
				if err != nil {
					return xerrors.Errorf("forward agent failed: %w", err)
				}
				err = gosshagent.RequestAgentForwarding(sshSession)
				if err != nil {
					return xerrors.Errorf("request agent forwarding failed: %w", err)
				}
			}

			stdoutFile, valid := cmd.OutOrStdout().(*os.File)
			if valid && isatty.IsTerminal(stdoutFile.Fd()) {
				state, err := term.MakeRaw(int(os.Stdin.Fd()))
				if err != nil {
					return err
				}
				defer func() {
					_ = term.Restore(int(os.Stdin.Fd()), state)
				}()

				windowChange := listenWindowSize(cmd.Context())
				go func() {
					for {
						select {
						case <-cmd.Context().Done():
							return
						case <-windowChange:
						}
						width, height, _ := term.GetSize(int(stdoutFile.Fd()))
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
			sshSession.Stderr = cmd.OutOrStdout()

			err = sshSession.Shell()
			if err != nil {
				return err
			}

			err = sshSession.Wait()
			if err != nil {
				return err
			}

			return nil
		},
	}
	cliflag.BoolVarP(cmd.Flags(), &stdio, "stdio", "", "CODER_SSH_STDIO", false, "Specifies whether to emit SSH output over stdin/stdout.")
	cliflag.BoolVarP(cmd.Flags(), &shuffle, "shuffle", "", "CODER_SSH_SHUFFLE", false, "Specifies whether to choose a random workspace")
	_ = cmd.Flags().MarkHidden("shuffle")
	cliflag.BoolVarP(cmd.Flags(), &forwardAgent, "forward-agent", "A", "CODER_SSH_FORWARD_AGENT", false, "Specifies whether to forward the SSH agent specified in $SSH_AUTH_SOCK")
	cliflag.DurationVarP(cmd.Flags(), &wsPollInterval, "workspace-poll-interval", "", "CODER_WORKSPACE_POLL_INTERVAL", workspacePollInterval, "Specifies how often to poll for workspace automated shutdown.")

	return cmd
}

// getWorkspaceAgent returns the workspace and agent selected using either the
// `<workspace>[.<agent>]` syntax via `in` or picks a random workspace and agent
// if `shuffle` is true.
func getWorkspaceAndAgent(cmd *cobra.Command, client *codersdk.Client, orgID uuid.UUID, userID string, in string, shuffle bool) (codersdk.Workspace, codersdk.WorkspaceAgent, error) { //nolint:revive
	ctx := cmd.Context()

	var (
		workspace      codersdk.Workspace
		workspaceParts = strings.Split(in, ".")
		err            error
	)
	if shuffle {
		workspaces, err := client.WorkspacesByOwner(cmd.Context(), orgID, userID)
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
		if len(workspaces) == 0 {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("no workspaces to shuffle")
		}

		workspace, err = cryptorand.Element(workspaces)
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	} else {
		workspace, err = client.WorkspaceByOwnerAndName(cmd.Context(), orgID, userID, workspaceParts[0])
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	}

	if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("workspace must be in start transition to ssh")
	}
	if workspace.LatestBuild.Job.CompletedAt == nil {
		err := cliui.WorkspaceBuild(ctx, cmd.ErrOrStderr(), client, workspace.LatestBuild.ID, workspace.CreatedAt)
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	}
	if workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionDelete {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q is being deleted", workspace.Name)
	}

	resources, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
	if err != nil {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("fetch workspace resources: %w", err)
	}

	agents := make([]codersdk.WorkspaceAgent, 0)
	for _, resource := range resources {
		agents = append(agents, resource.Agents...)
	}
	if len(agents) == 0 {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q has no agents", workspace.Name)
	}
	var agent codersdk.WorkspaceAgent
	if len(workspaceParts) >= 2 {
		for _, otherAgent := range agents {
			if otherAgent.Name != workspaceParts[1] {
				continue
			}
			agent = otherAgent
			break
		}
		if agent.ID == uuid.Nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("agent not found by name %q", workspaceParts[1])
		}
	}
	if agent.ID == uuid.Nil {
		if len(agents) > 1 {
			if !shuffle {
				return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("you must specify the name of an agent")
			}
			agent, err = cryptorand.Element(agents)
			if err != nil {
				return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
			}
		} else {
			agent = agents[0]
		}
	}

	return workspace, agent, nil
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

		if ws.TTL == nil || *ws.TTL == 0 {
			return time.Time{}, nil
		}

		deadline = ws.LatestBuild.UpdatedAt.Add(*ws.TTL)
		callback = func() {
			ttl := deadline.Sub(now)
			var title, body string
			if ttl > time.Minute {
				title = fmt.Sprintf(`Workspace %s stopping in %.0f mins`, ws.Name, ttl.Minutes())
				body = fmt.Sprintf(
					`Your Coder workspace %s is scheduled to stop at %s.`,
					ws.Name,
					deadline.Format(time.Kitchen),
				)
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
