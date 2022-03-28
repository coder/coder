package cli

import (
	"context"
	"net/url"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
	"github.com/coder/retry"
)

func workspaceAgent() *cobra.Command {
	var (
		rawURL string
		auth   string
		token  string
	)
	cmd := &cobra.Command{
		Use: "agent",
		// This command isn't useful to manually execute.
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rawURL == "" {
				return xerrors.New("CODER_URL must be set")
			}
			coderURL, err := url.Parse(rawURL)
			if err != nil {
				return xerrors.Errorf("parse %q: %w", rawURL, err)
			}
			logger := slog.Make(sloghuman.Sink(cmd.OutOrStdout())).Leveled(slog.LevelDebug)
			client := codersdk.New(coderURL)
			switch auth {
			case "token":
				if token == "" {
					return xerrors.Errorf("CODER_TOKEN must be set for token auth")
				}
				client.SessionToken = token
			case "google-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var gcpClient *metadata.Client
				gcpClientRaw := cmd.Context().Value("gcp-client")
				if gcpClientRaw != nil {
					gcpClient, _ = gcpClientRaw.(*metadata.Client)
				}

				ctx, cancelFunc := context.WithTimeout(cmd.Context(), 30*time.Second)
				defer cancelFunc()
				for retry.New(100*time.Millisecond, 5*time.Second).Wait(ctx) {
					var response codersdk.WorkspaceAgentAuthenticateResponse

					response, err = client.AuthWorkspaceGoogleInstanceIdentity(ctx, "", gcpClient)
					if err != nil {
						logger.Warn(ctx, "authenticate workspace with Google Instance Identity", slog.Error(err))
						continue
					}
					client.SessionToken = response.SessionToken
					logger.Info(ctx, "authenticated with Google Instance Identity")
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

	cliflag.StringVarP(cmd.Flags(), &auth, "auth", "", "CODER_AUTH", "token", "Specify the authentication type to use for the agent")
	cliflag.StringVarP(cmd.Flags(), &rawURL, "url", "", "CODER_URL", "", "Specify the URL to access Coder")
	cliflag.StringVarP(cmd.Flags(), &auth, "token", "", "CODER_TOKEN", "", "Specifies the authentication token to access Coder")

	return cmd
}
