package cli

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/gitauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/retry"
	"github.com/coder/serpent"
)

// gitAskpass is used by the Coder agent to automatically authenticate
// with Git providers based on a hostname.
func (r *RootCmd) gitAskpass() *serpent.Command {
	return &serpent.Command{
		Use:    "gitaskpass",
		Hidden: true,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			user, host, err := gitauth.ParseAskpass(inv.Args[0])
			if err != nil {
				return xerrors.Errorf("parse host: %w", err)
			}

			client, err := r.createAgentClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			token, err := client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				Match: host,
			})
			if err != nil {
				var apiError *codersdk.Error
				if errors.As(err, &apiError) && apiError.StatusCode() == http.StatusNotFound {
					// This prevents the "Run 'coder --help' for usage"
					// message from occurring.
					lines := []string{apiError.Message}
					if apiError.Detail != "" {
						lines = append(lines, apiError.Detail)
					}
					cliui.Warn(inv.Stderr, "Coder was unable to handle this git request. The default git behavior will be used instead.",
						lines...,
					)
					return cliui.ErrCanceled
				}
				return xerrors.Errorf("get git token: %w", err)
			}
			if token.URL != "" {
				if err := openURL(inv, token.URL); err == nil {
					cliui.Infof(inv.Stderr, "Your browser has been opened to authenticate with Git:\n%s", token.URL)
				} else {
					cliui.Infof(inv.Stderr, "Open the following URL to authenticate with Git:\n%s", token.URL)
				}

				for r := retry.New(250*time.Millisecond, 10*time.Second); r.Wait(ctx); {
					token, err = client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
						Match:  host,
						Listen: true,
					})
					if err != nil {
						continue
					}
					cliui.Infof(inv.Stderr, "You've been authenticated with Git!")
					break
				}
			}

			if token.Password != "" {
				if user == "" {
					_, _ = fmt.Fprintln(inv.Stdout, token.Username)
				} else {
					_, _ = fmt.Fprintln(inv.Stdout, token.Password)
				}
			} else {
				_, _ = fmt.Fprintln(inv.Stdout, token.Username)
			}

			return nil
		},
	}
}
