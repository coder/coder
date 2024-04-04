package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) auth() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "auth <subcommand>",
		Short: "Manage information about internal authentication.",
		Children: []*serpent.Command{
			r.authStatus(),
			r.authToken(),
			r.login(),
		},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
	}
	return cmd
}

func (r *RootCmd) authToken() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "token",
		Short: "Show session token value and expiration time.",
		Middleware: serpent.Chain(
			r.InitClient(client),
			validateUserMW(client, r),
		),
		Handler: func(inv *serpent.Invocation) error {
			sessionID := strings.Split(client.SessionToken(), "-")[0]
			key, err := client.APIKeyByID(inv.Context(), codersdk.Me, sessionID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Your session token '%s' expires at %s.\n", client.SessionToken(), key.ExpiresAt)

			return nil
		},
	}
	return cmd
}

func (r *RootCmd) authStatus() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "status",
		Short: "Show user authentication status.",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			res, err := client.User(inv.Context(), codersdk.Me)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Hello there, %s! You're authenticated at %s.\n", pretty.Sprint(cliui.DefaultStyles.Keyword, res.Username), r.clientURL)
			return nil
		},
	}
	return cmd
}

func validateUserMW(client *codersdk.Client, _ *RootCmd) serpent.MiddlewareFunc {
	return func(next serpent.HandlerFunc) serpent.HandlerFunc {
		return func(inv *serpent.Invocation) error {
			_, err := client.User(inv.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("get user: %w", err)
			}

			return next(inv)
		}
	}
}
