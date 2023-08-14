package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
	"github.com/coder/retry"
)

// gitAskpass is used by the Coder agent to automatically authenticate
// with Git providers based on a hostname.
func (r *RootCmd) gitAskpass() *clibase.Cmd {
	return &clibase.Cmd{
		Use:    "gitaskpass",
		Hidden: true,
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()

			ctx, stop := signal.NotifyContext(ctx, InterruptSignals...)
			defer stop()

			user, host, err := gitauth.ParseAskpass(inv.Args[0])
			if err != nil {
				return xerrors.Errorf("parse host: %w", err)
			}

			client, err := r.createAgentClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			token, err := client.GitAuth(ctx, host, false)
			if err != nil {
				var apiError *codersdk.Error
				if errors.As(err, &apiError) && apiError.StatusCode() == http.StatusNotFound {
					// This prevents the "Run 'coder --help' for usage"
					// message from occurring.
					cliui.Errorf(inv.Stderr, "%s\n", apiError.Message)
					return cliui.Canceled
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
					token, err = client.GitAuth(ctx, host, true)
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
