package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
	"github.com/coder/retry"
)

// gitAskpass is used by the Coder agent to automatically authenticate
// with Git providers based on a hostname.
func gitAskpass() *cobra.Command {
	return &cobra.Command{
		Use:    "gitaskpass",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			ctx, stop := signal.NotifyContext(ctx, InterruptSignals...)
			defer stop()

			user, host, err := gitauth.ParseAskpass(args[0])
			if err != nil {
				return xerrors.Errorf("parse host: %w", err)
			}

			client, err := createAgentClient(cmd)
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			token, err := client.GitAuth(ctx, host, false)
			if err != nil {
				var apiError *codersdk.Error
				if errors.As(err, &apiError) && apiError.StatusCode() == http.StatusNotFound {
					// This prevents the "Run 'coder --help' for usage"
					// message from occurring.
					cmd.Printf("%s\n", apiError.Message)
					return cliui.Canceled
				}
				return xerrors.Errorf("get git token: %w", err)
			}
			if token.URL != "" {
				if err := openURL(cmd, token.URL); err == nil {
					cmd.Printf("Your browser has been opened to authenticate with Git:\n\n\t%s\n\n", token.URL)
				} else {
					cmd.Printf("Open the following URL to authenticate with Git:\n\n\t%s\n\n", token.URL)
				}

				for r := retry.New(250*time.Millisecond, 10*time.Second); r.Wait(ctx); {
					token, err = client.GitAuth(ctx, host, true)
					if err != nil {
						continue
					}
					cmd.Printf("You've been authenticated with Git!\n")
					break
				}
			}

			if token.Password != "" {
				if user == "" {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), token.Username)
				} else {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), token.Password)
				}
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), token.Username)
			}

			return nil
		},
	}
}
