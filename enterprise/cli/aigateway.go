package cli

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) aiGateway() *serpent.Command {
	return &serpent.Command{
		Use:   "ai-gateway",
		Short: "Manage AI Gateway",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.aiGatewayKeys(),
		},
	}
}

func (r *RootCmd) aiGatewayKeys() *serpent.Command {
	return &serpent.Command{
		Use:   "keys",
		Short: "Manage AI Gateway keys",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.aiGatewayKeysCreate(),
			r.aiGatewayKeysDelete(),
			r.aiGatewayKeysList(),
		},
	}
}

func (r *RootCmd) aiGatewayKeysCreate() *serpent.Command {
	return &serpent.Command{
		Use:   "create <name>",
		Short: "Create an AI Gateway key",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			res, err := client.CreateAIGatewayKey(inv.Context(), codersdk.CreateAIGatewayKeyRequest{
				Name: inv.Args[0],
			})
			if err != nil {
				return xerrors.Errorf("create AI Gateway key %q: %w", inv.Args[0], err)
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"Successfully created AI Gateway key %s (ID: %s, Prefix: %s).\nSave this authentication token, it will not be shown again.\n\n%s\n",
				cliui.Keyword(res.Name),
				res.ID,
				res.KeyPrefix,
				cliui.Keyword(res.Key),
			)
			return nil
		},
	}
}

func (r *RootCmd) aiGatewayKeysList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.AIGatewayKey{}, []string{"id", "name", "key prefix", "last heartbeat at", "created at"}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List AI Gateway keys",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			keys, err := client.ListAIGatewayKeys(inv.Context())
			if err != nil {
				return xerrors.Errorf("list AI Gateway keys: %w", err)
			}

			out, err := formatter.Format(inv.Context(), keys)
			if err != nil {
				return xerrors.Errorf("format AI Gateway keys: %w", err)
			}
			if out == "" {
				cliui.Info(inv.Stderr, "No AI Gateway keys found.")
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) aiGatewayKeysDelete() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "delete <name|id>",
		Aliases: []string{"rm"},
		Short:   "Delete an AI Gateway key",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			key, err := aiGatewayKeyByNameOrID(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Are you sure you want to delete AI Gateway key %s (ID: %s, Prefix: %s)?", cliui.Keyword(key.Name), key.ID, key.KeyPrefix),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			err = client.DeleteAIGatewayKey(inv.Context(), key.ID)
			if err != nil {
				return xerrors.Errorf("delete AI Gateway key %q: %w", key.Name, err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully deleted AI Gateway key %s (ID: %s, Prefix: %s).\n", cliui.Keyword(key.Name), key.ID, key.KeyPrefix)
			return nil
		},
	}

	return cmd
}

// aiGatewayKeyByNameOrID resolves an AI Gateway key from a name or ID. Names
// take priority over IDs.
func aiGatewayKeyByNameOrID(ctx context.Context, client *codersdk.Client, nameOrID string) (codersdk.AIGatewayKey, error) {
	keys, err := client.ListAIGatewayKeys(ctx)
	if err != nil {
		return codersdk.AIGatewayKey{}, xerrors.Errorf("list AI Gateway keys: %w", err)
	}

	for _, key := range keys {
		if key.Name == nameOrID {
			return key, nil
		}
	}

	if id, err := uuid.Parse(nameOrID); err == nil {
		for _, key := range keys {
			if key.ID == id {
				return key, nil
			}
		}
	}

	return codersdk.AIGatewayKey{}, xerrors.Errorf("AI Gateway key %q not found", nameOrID)
}
