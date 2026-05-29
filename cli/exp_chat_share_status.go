package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) chatShareStatusCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "status <chat-id>",
		Short: "List all users and groups the given chat is shared with.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			chatID, err := parseChatShareID(inv.Args[0])
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			acl, err := codersdk.NewExperimentalClient(client).GetChatACL(inv.Context(), chatID)
			if err != nil {
				return xerrors.Errorf("unable to fetch ACL for chat: %w", err)
			}
			out, err := chatACLToTable(inv.Context(), &acl)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
}
