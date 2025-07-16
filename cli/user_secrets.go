package cli

import (
	"fmt"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) secrets() *serpent.Command {
	return &serpent.Command{
		Use:   "secrets",
		Short: "Manage your user secrets",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.secretCreate(),
		},
	}
}

func (r *RootCmd) secretCreate() *serpent.Command {
	client := new(codersdk.Client)
	var value string
	var description string
	cmd := &serpent.Command{
		Use:   "create <name>",
		Short: "Create a new user secret",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			name := inv.Args[0]
			if value == "" {
				return fmt.Errorf("--value is required")
			}
			secret, err := client.CreateUserSecret(inv.Context(), codersdk.CreateUserSecretRequest{
				Name:        name,
				Value:       value,
				Description: description,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(inv.Stdout, "Created user secret %q (ID: %s)\n", secret.Name, secret.ID)
			return nil
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "value",
			Description: "Value of the secret (required).",
			Value:       serpent.StringOf(&value),
		},
		{
			Flag:        "description",
			Description: "Description of the secret.",
			Value:       serpent.StringOf(&description),
		},
	}
	return cmd
}
