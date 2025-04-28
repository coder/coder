package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) provisionerKeys() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "keys",
		Short: "Manage provisioner keys",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Aliases: []string{"key"},
		Children: []*serpent.Command{
			r.provisionerKeysCreate(),
			r.provisionerKeysList(),
			r.provisionerKeysDelete(),
		},
	}

	return cmd
}

func (r *RootCmd) provisionerKeysCreate() *serpent.Command {
	var (
		orgContext = agpl.NewOrganizationContext()
		rawTags    []string
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "create <name>",
		Short: "Create a new provisioner key",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			tags, err := agpl.ParseProvisionerTags(rawTags)
			if err != nil {
				return err
			}

			res, err := client.CreateProvisionerKey(ctx, org.ID, codersdk.CreateProvisionerKeyRequest{
				Name: inv.Args[0],
				Tags: tags,
			})
			if err != nil {
				return xerrors.Errorf("create provisioner key: %w", err)
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"Successfully created provisioner key %s! Save this authentication token, it will not be shown again.\n\n%s\n",
				pretty.Sprint(cliui.DefaultStyles.Keyword, strings.ToLower(inv.Args[0])),
				pretty.Sprint(cliui.DefaultStyles.Keyword, res.Key),
			)

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "tag",
			FlagShorthand: "t",
			Env:           "CODER_PROVISIONERD_TAGS",
			Description:   "Tags to filter provisioner jobs by.",
			Value:         serpent.StringArrayOf(&rawTags),
		},
	}
	orgContext.AttachOptions(cmd)

	return cmd
}

func (r *RootCmd) provisionerKeysList() *serpent.Command {
	var (
		orgContext = agpl.NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]codersdk.ProvisionerKey{}, []string{"created at", "name", "tags"}),
			cliui.JSONFormat(),
		)
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List provisioner keys in an organization",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			keys, err := client.ListProvisionerKeys(ctx, org.ID)
			if err != nil {
				return xerrors.Errorf("list provisioner keys: %w", err)
			}

			if len(keys) == 0 {
				_, _ = fmt.Fprintln(inv.Stdout, "No provisioner keys found")
				return nil
			}

			out, err := formatter.Format(inv.Context(), keys)
			if err != nil {
				return xerrors.Errorf("display provisioner keys: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)

			return nil
		},
	}

	orgContext.AttachOptions(cmd)
	formatter.AttachOptions(&cmd.Options)

	return cmd
}

func (r *RootCmd) provisionerKeysDelete() *serpent.Command {
	orgContext := agpl.NewOrganizationContext()

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "delete <name>",
		Short: "Delete a provisioner key",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Are you sure you want to delete provisioner key %s?", pretty.Sprint(cliui.DefaultStyles.Keyword, inv.Args[0])),
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			err = client.DeleteProvisionerKey(ctx, org.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("delete provisioner key: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully deleted provisioner key %s!\n", pretty.Sprint(cliui.DefaultStyles.Keyword, strings.ToLower(inv.Args[0])))

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		cliui.SkipPromptOption(),
	}
	orgContext.AttachOptions(cmd)

	return cmd
}
