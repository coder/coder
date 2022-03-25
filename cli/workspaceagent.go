package cli

import (
	"context"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
	"github.com/coder/retry"
)

func workspaceAgent() *cobra.Command {
	return &cobra.Command{
		Use: "agent",
		// This command isn't useful to manually execute.
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			coderURLRaw, exists := os.LookupEnv("CODER_URL")
			if !exists {
				return xerrors.New("CODER_URL must be set")
			}
			coderURL, err := url.Parse(coderURLRaw)
			if err != nil {
				return xerrors.Errorf("parse %q: %w", coderURLRaw, err)
			}
			logger := slog.Make(sloghuman.Sink(cmd.OutOrStdout()))
			client := codersdk.New(coderURL)
			auth, exists := os.LookupEnv("CODER_AUTH")
			if !exists {
				auth = "token"
			}
			switch auth {
			case "token":
				sessionToken, exists := os.LookupEnv("CODER_TOKEN")
				if !exists {
					return xerrors.Errorf("CODER_TOKEN must be set for token auth")
				}
				client.SessionToken = sessionToken
			case "google-instance-identity":
				ctx, cancelFunc := context.WithTimeout(cmd.Context(), 30*time.Second)
				defer cancelFunc()
				for retry.New(100*time.Millisecond, 5*time.Second).Wait(ctx) {
					var response codersdk.WorkspaceAgentAuthenticateResponse
					response, err = client.AuthWorkspaceGoogleInstanceIdentity(cmd.Context(), "", nil)
					if err != nil {
						logger.Warn(ctx, "authenticate workspace with Google Instance Identity", slog.Error(err))
						continue
					}
					client.SessionToken = response.SessionToken
					break
				}
				if err != nil {
					return xerrors.Errorf("agent failed to authenticate in time: %w", err)
				}
			case "aws-instance-identity":
				return xerrors.Errorf("not implemented")
			case "azure-instance-identity":
				return xerrors.Errorf("not implemented")
			}
			closer := agent.New(client.ListenWorkspaceAgent, &peer.ConnOptions{
				Logger: logger,
			})
			<-cmd.Context().Done()
			return closer.Close()
		},
	}
}
