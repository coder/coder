package cli

import (
	"fmt"
	"os"

	"github.com/pion/webrtc/v3"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
)

func workspaceSSH() *cobra.Command {
	return &cobra.Command{
		Use: "ssh",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByName(cmd.Context(), "", args[0])
			if err != nil {
				return err
			}
			build, err := client.WorkspaceBuildLatest(cmd.Context(), workspace.ID)
			if err != nil {
				return err
			}
			resources, err := client.WorkspaceResourcesByBuild(cmd.Context(), build.ID)
			if err != nil {
				return err
			}
			for _, resource := range resources {
				fmt.Printf("Got resource: %+v\n", resource)
				if resource.Agent == nil {
					continue
				}

				dialed, err := client.DialWorkspaceAgent(cmd.Context(), resource.ID)
				if err != nil {
					return err
				}
				stream, err := dialed.NegotiateConnection(cmd.Context())
				if err != nil {
					return err
				}
				conn, err := peerbroker.Dial(stream, []webrtc.ICEServer{{
					URLs: []string{"stun:stun.l.google.com:19302"},
				}}, &peer.ConnOptions{})
				if err != nil {
					return err
				}
				client, err := agent.DialSSHClient(conn)
				if err != nil {
					return err
				}

				session, err := client.NewSession()
				if err != nil {
					return err
				}
				// Set raw
				term.MakeRaw(int(os.Stdin.Fd()))
				err = session.RequestPty("xterm-256color", 128, 128, ssh.TerminalModes{
					ssh.OCRNL: 1,
				})
				if err != nil {
					return err
				}
				session.Stdin = os.Stdin
				session.Stdout = os.Stdout
				session.Stderr = os.Stderr
				err = session.Shell()
				if err != nil {
					return err
				}
				err = session.Wait()
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
}
