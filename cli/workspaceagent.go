package cli

import (
	"net/url"
	"os"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
)

func workspaceAgent() *cobra.Command {
	return &cobra.Command{
		Use: "agent",
		// This command isn't useful for users, and seems
		// more likely to confuse.
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
			client := codersdk.New(coderURL)
			sessionToken, exists := os.LookupEnv("CODER_TOKEN")
			if !exists {
				// probe, err := cloud.New()
				// if err != nil {
				// 	return xerrors.Errorf("probe cloud: %w", err)
				// }
				// if !probe.Detected {
				// 	return xerrors.Errorf("no valid authentication method found; set \"CODER_TOKEN\"")
				// }
				// switch {
				// case probe.GCP():
				response, err := client.AuthWorkspaceGoogleInstanceIdentity(cmd.Context(), "", nil)
				if err != nil {
					return xerrors.Errorf("authenticate workspace with gcp: %w", err)
				}
				sessionToken = response.SessionToken
				// default:
				// 	return xerrors.Errorf("%q authentication not supported; set \"CODER_TOKEN\" instead", probe.Name)
				// }
			}
			client.SessionToken = sessionToken
			closer := agent.New(client.ListenWorkspaceAgent, &peer.ConnOptions{
				Logger: slog.Make(sloghuman.Sink(cmd.OutOrStdout())),
			})
			<-cmd.Context().Done()
			return closer.Close()
		},
	}
}
