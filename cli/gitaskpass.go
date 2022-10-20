package cli

import (
	"fmt"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/retry"
)

func gitAskpass() *cobra.Command {
	return &cobra.Command{
		Use:    "gitaskpass",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ctx := cmd.Context()

			ctx, stop := signal.NotifyContext(ctx, interruptSignals...)
			defer stop()

			defer func() {
				if ctx.Err() != nil {
					err = ctx.Err()
				}
			}()

			user, host, err := gitauth.ParseAskpass(args[0])
			if err != nil {
				return xerrors.Errorf("parse host: %w", err)
			}

			client, err := createAgentClient(cmd)
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			token, err := client.WorkspaceAgentGitAuth(ctx, host, false)
			if err != nil {
				return xerrors.Errorf("get git token: %w", err)
			}
			if token.URL != "" {
				cmd.Printf("Visit the following URL to authenticate with Git:\n%s\n", token.URL)
				for r := retry.New(time.Second, 10*time.Second); r.Wait(ctx); {
					token, err = client.WorkspaceAgentGitAuth(ctx, host, true)
					if err != nil {
						continue
					}
					cmd.Printf("\nYou've been authenticated with Git!\n")
					break
				}
			}

			if token.Password != "" {
				if user == "" {
					fmt.Fprintln(cmd.OutOrStdout(), token.Username)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), token.Password)
				}
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), token.Username)
			}

			return nil
		},
	}
}
