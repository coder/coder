package cli

import (
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func workspaceSSH() *cobra.Command {
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
			if resource.Agent.LastConnectedAt == nil {
				return xerrors.Errorf("agent hasn't connected yet")
			}

			conn, err := client.DialWorkspaceAgent(cmd.Context(), resource.ID, nil, nil)
			if err != nil {
				return err
			}
			sshClient, err := conn.SSHClient()
			if err != nil {
				return err
			}

			sshSession, err := sshClient.NewSession()
			if err != nil {
				return err
			}
			_, _ = term.MakeRaw(int(os.Stdin.Fd()))
			err = sshSession.RequestPty("xterm-256color", 128, 128, ssh.TerminalModes{
				ssh.OCRNL: 1,
			})
			if err != nil {
				return err
			}
			sshSession.Stdin = os.Stdin
			sshSession.Stdout = os.Stdout
			sshSession.Stderr = os.Stderr
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
