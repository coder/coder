package cli

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/vscodeipc"
	"github.com/coder/coder/codersdk"
)

// vscodeipcCmd spawns a local HTTP server on the provided port that listens to messages.
// It's made for use by the Coder VS Code extension. See: https://github.com/coder/vscode-coder
func vscodeipcCmd() *cobra.Command {
	var port uint16
	cmd := &cobra.Command{
		Use:    "vscodeipc <workspace-agent>",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			rawURL := os.Getenv("CODER_URL")
			if rawURL == "" {
				return xerrors.New("CODER_URL must be set!")
			}
			token := os.Getenv("CODER_TOKEN")
			if token == "" {
				return xerrors.New("CODER_TOKEN must be set!")
			}
			if port == 0 {
				return xerrors.Errorf("port must be specified!")
			}
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return xerrors.Errorf("listen: %w", err)
			}
			defer listener.Close()
			url, err := url.Parse(rawURL)
			if err != nil {
				return err
			}
			agentID, err := uuid.Parse(args[0])
			if err != nil {
				return err
			}
			client := codersdk.New(url)
			client.SetSessionToken(token)

			handler, closer, err := vscodeipc.New(cmd.Context(), client, agentID, nil)
			if err != nil {
				return err
			}
			defer closer.Close()
			server := http.Server{
				Handler: handler,
			}
			cmd.Printf("Ready\n")
			return server.Serve(listener)
		},
	}
	cmd.Flags().Uint16VarP(&port, "port", "p", 0, "The port to listen on!")
	return cmd
}
