package cli

import (
	"context"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peerbroker"
	"github.com/spf13/cobra"
)

func workspaceAgent() *cobra.Command {
	return &cobra.Command{
		Use:    "agent",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			closer := agent.New(createAgentDialer(cmd, client), &agent.Options{
				Logger: slog.Make(sloghuman.Sink(cmd.OutOrStdout())),
			})
			<-cmd.Context().Done()
			return closer.Close()
		},
	}
}

func createAgentDialer(cmd *cobra.Command, client *codersdk.Client) agent.Dialer {
	return func(ctx context.Context) (*peerbroker.Listener, error) {

		return nil, nil
	}
}
