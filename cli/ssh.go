package cli

import (
	"io"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/pion/webrtc/v3"
	"github.com/spf13/cobra"
)

func ssh() *cobra.Command {
	return &cobra.Command{
		Use: "ssh",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			history, err := client.WorkspaceHistory(cmd.Context(), "", "kyle", "")
			if err != nil {
				return err
			}
			resources, err := client.WorkspaceProvisionJobResources(cmd.Context(), organization.Name, history.ProvisionJobID)
			if err != nil {
				return err
			}
			for _, resource := range resources {
				if resource.Agent == nil {
					continue
				}
				wagent, err := client.WorkspaceAgentConnect(cmd.Context(), organization.Name, history.ProvisionJobID, resource.ID)
				if err != nil {
					return err
				}
				stream, err := wagent.NegotiateConnection(cmd.Context())
				if err != nil {
					return err
				}
				conn, err := peerbroker.Dial(stream, []webrtc.ICEServer{{
					URLs: []string{"stun:stun.l.google.com:19302"},
				}}, &peer.ConnOptions{
					// Logger: slog.Make(sloghuman.Sink(cmd.OutOrStdout())).Leveled(slog.LevelDebug),
				})
				if err != nil {
					return err
				}
				sshConn, err := agent.DialSSH(conn)
				if err != nil {
					return err
				}
				go func() {
					_, _ = io.Copy(cmd.OutOrStdout(), sshConn)
				}()
				_, _ = io.Copy(sshConn, cmd.InOrStdin())
			}
			return nil
		},
	}
}
