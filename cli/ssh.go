package cli

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func ssh() *cobra.Command {
	var (
		stdio bool
	)
	cmd := &cobra.Command{
		Use:  "ssh <workspace>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			workspace, agent, err := getWorkspaceAndAgent(cmd.Context(), client, organization.ID, codersdk.Me, args[0])
			if err != nil {
				return err
			}
			if workspace.LatestBuild.Transition != database.WorkspaceTransitionStart {
				return xerrors.New("workspace must be in start transition to ssh")
			}
			if workspace.LatestBuild.Job.CompletedAt == nil {
				err = cliui.WorkspaceBuild(cmd.Context(), cmd.ErrOrStderr(), client, workspace.LatestBuild.ID, workspace.CreatedAt)
				if err != nil {
					return err
				}
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

	return cmd
}

func getWorkspaceAndAgent(ctx context.Context, client *codersdk.Client, orgID uuid.UUID, userID uuid.UUID, in string) (codersdk.Workspace, codersdk.WorkspaceAgent, error) {
	workspaceParts := strings.Split(in, ".")
	workspace, err := client.WorkspaceByOwnerAndName(ctx, orgID, userID, workspaceParts[0])
	if err != nil {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("get workspace %q: %w", workspaceParts[0], err)
	}

	if workspace.LatestBuild.Transition == database.WorkspaceTransitionDelete {
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

	var agent *codersdk.WorkspaceAgent
	if len(workspaceParts) >= 2 {
		for _, otherAgent := range agents {
			if otherAgent.Name != workspaceParts[1] {
				continue
			}
			agent = &otherAgent
			break
		}

		if agent == nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("agent not found by name %q", workspaceParts[1])
		}
	}

	if agent == nil {
		if len(agents) > 1 {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("you must specify the name of an agent")
		}
		agent = &agents[0]
	}

	return workspace, *agent, nil
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
