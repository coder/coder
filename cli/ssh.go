package cli

import (
	"context"

	"github.com/pion/webrtc/v3"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func ssh() *cobra.Command {
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
			if workspace.LatestBuild.Job.CompletedAt == nil {
				err = cliui.WorkspaceBuild(cmd, client, workspace.LatestBuild.ID, workspace.CreatedAt)
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
			err = cliui.Agent(cmd, cliui.AgentOptions{
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
			sshClient, err := conn.SSHClient()
			if err != nil {
				return err
			}

			sshSession, err := sshClient.NewSession()
			if err != nil {
				return err
			}

			err = sshSession.RequestPty("xterm-256color", 128, 128, gossh.TerminalModes{
				gossh.OCRNL: 1,
			})
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

	return cmd
}
