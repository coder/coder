package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
	orgContext := agpl.NewOrganizationContext()

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

			res, err := client.CreateProvisionerKey(ctx, org.ID, codersdk.CreateProvisionerKeyRequest{
				Name: inv.Args[0],
			})
			if err != nil {
				return xerrors.Errorf("create provisioner key: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully created provisioner key %s!\n\n%s\n", pretty.Sprint(cliui.DefaultStyles.Keyword, strings.ToLower(inv.Args[0])), pretty.Sprint(cliui.DefaultStyles.Keyword, res.Key))

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{}
	orgContext.AttachOptions(cmd)

	return cmd
}

type provisionerKeysTableRow struct {
	// For json output:
	Key codersdk.ProvisionerKey `table:"-"`

	// For table output:
	Name           string    `json:"-" table:"name,default_sort"`
	CreatedAt      time.Time `json:"-" table:"created_at"`
	OrganizationID uuid.UUID `json:"-" table:"organization_id"`
}

func provisionerKeysToRows(keys ...codersdk.ProvisionerKey) []provisionerKeysTableRow {
	rows := make([]provisionerKeysTableRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, provisionerKeysTableRow{
			Name:           key.Name,
			CreatedAt:      key.CreatedAt,
			OrganizationID: key.OrganizationID,
		})
	}

	return rows
}

func (r *RootCmd) provisionerKeysList() *serpent.Command {
	var (
		orgContext = agpl.NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]provisionerKeysTableRow{}, nil),
			cliui.JSONFormat(),
		)
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List provisioner keys",
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

			rows := provisionerKeysToRows(keys...)
			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return xerrors.Errorf("display provisioner keys: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{}
	orgContext.AttachOptions(cmd)

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
