package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) Provisioners() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "provisioner",
		Short: "View and manage provisioner daemons and jobs",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Aliases: []string{"provisioners"},
		Children: []*serpent.Command{
			r.provisionerList(),
			r.provisionerJobs(),
		},
	}

	return cmd
}

func (r *RootCmd) provisionerList() *serpent.Command {
	type provisionerDaemonRow struct {
		codersdk.ProvisionerDaemonWithStatus `table:"provisioner_daemon,recursive_inline"`
		OrganizationName                     string `json:"organization_name" table:"organization"`
	}
	var (
		client     = new(codersdk.Client)
		orgContext = NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]provisionerDaemonRow{}, []string{"name", "organization", "status", "key name", "created at", "last seen at", "version", "tags"}),
			cliui.JSONFormat(),
		)
	)

	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List provisioner daemons in an organization",
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

			daemons, err := client.OrganizationProvisionerDaemons(ctx, org.ID, nil)
			if err != nil {
				return xerrors.Errorf("list provisioner daemons: %w", err)
			}

			if len(daemons) == 0 {
				_, _ = fmt.Fprintln(inv.Stdout, "No provisioner daemons found")
				return nil
			}

			var rows []provisionerDaemonRow
			for _, daemon := range daemons {
				rows = append(rows, provisionerDaemonRow{
					ProvisionerDaemonWithStatus: daemon,
					OrganizationName:            org.HumanName(),
				})
			}

			out, err := formatter.Format(ctx, rows)
			if err != nil {
				return xerrors.Errorf("display provisioner daemons: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)

			return nil
		},
	}

	orgContext.AttachOptions(cmd)
	formatter.AttachOptions(&cmd.Options)

	return cmd
}
