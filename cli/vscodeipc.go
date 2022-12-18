package cli

import (
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/vscodeipc"
	"github.com/coder/coder/codersdk"
)

// vscodeipcCmd spawns a local HTTP server on the provided port that listens to messages.
// It's made for use by the Coder VS Code extension. See: https://github.com/coder/vscode-coder
func vscodeipcCmd() *cobra.Command {
	var (
		rawURL string
		token  string
		port   uint16
	)
	cmd := &cobra.Command{
		Use:          "vscodeipc <workspace-agent>",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		Hidden:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rawURL == "" {
				return xerrors.New("CODER_URL must be set!")
			}
			// token is validated in a header on each request to prevent
			// unauthenticated clients from connecting.
			if token == "" {
				return xerrors.New("CODER_TOKEN must be set!")
			}
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return xerrors.Errorf("listen: %w", err)
			}
			defer listener.Close()
			addr, ok := listener.Addr().(*net.TCPAddr)
			if !ok {
				return xerrors.Errorf("listener.Addr() is not a *net.TCPAddr: %T", listener.Addr())
			}
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
			// nolint:gosec
			server := http.Server{
				Handler: handler,
			}
			defer server.Close()
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", addr.String())
			errChan := make(chan error, 1)
			go func() {
				err := server.Serve(listener)
				errChan <- err
			}()
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case err := <-errChan:
				return err
			}
		},
	}
	cliflag.StringVarP(cmd.Flags(), &rawURL, "url", "u", "CODER_URL", "", "The URL of the Coder instance!")
	cliflag.StringVarP(cmd.Flags(), &token, "token", "t", "CODER_TOKEN", "", "The session token of the user!")
	cmd.Flags().Uint16VarP(&port, "port", "p", 0, "The port to listen on!")
	return cmd
}
