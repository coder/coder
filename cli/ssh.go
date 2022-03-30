package cli

import (
	"context"
	"io"
	"net"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/pion/webrtc/v3"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"golang.org/x/crypto/ssh/terminal"
)

func ssh() *cobra.Command {
	var (
		stdio bool
	)
	cmd := &cobra.Command{
		Use: "ssh <workspace> [resource]",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := client.WorkspaceByName(cmd.Context(), "", args[0])
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
			if workspace.LatestBuild.Transition == database.WorkspaceTransitionDelete {
				return xerrors.New("workspace is deleting...")
			}
			resources, err := client.WorkspaceResourcesByBuild(cmd.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return err
			}
			resourceByAddress := make(map[string]codersdk.WorkspaceResource)
			for _, resource := range resources {
				if resource.Agent == nil {
					continue
				}
				resourceByAddress[resource.Address] = resource
			}
			var resourceAddress string
			if len(args) >= 2 {
				resourceAddress = args[1]
			} else {
				// No resource name was provided!
				if len(resourceByAddress) > 1 {
					// List available resources to connect into?
					return xerrors.Errorf("multiple agents")
				}
				for _, resource := range resourceByAddress {
					resourceAddress = resource.Address
					break
				}
			}
			resource, exists := resourceByAddress[resourceAddress]
			if !exists {
				resourceKeys := make([]string, 0)
				for resourceKey := range resourceByAddress {
					resourceKeys = append(resourceKeys, resourceKey)
				}
				return xerrors.Errorf("no sshable agent with address %q: %+v", resourceAddress, resourceKeys)
			}
			// OpenSSH passes stderr directly to the calling TTY.
			// This is required in "stdio" mode so a connecting indicator can be displayed.
			err = cliui.Agent(cmd.Context(), cmd.ErrOrStderr(), cliui.AgentOptions{
				WorkspaceName: workspace.Name,
				Fetch: func(ctx context.Context) (codersdk.WorkspaceResource, error) {
					return client.WorkspaceResource(ctx, resource.ID)
				},
			})
			if err != nil {
				return xerrors.Errorf("await agent: %w", err)
			}

			conn, err := client.DialWorkspaceAgent(cmd.Context(), resource.ID, []webrtc.ICEServer{{
				URLs: []string{"stun:stun.l.google.com:19302"},
			}}, nil)
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

			if isatty.IsTerminal(os.Stdout.Fd()) {
				state, err := terminal.MakeRaw(int(os.Stdin.Fd()))
				if err != nil {
					return err
				}
				defer func() {
					_ = terminal.Restore(int(os.Stdin.Fd()), state)
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
