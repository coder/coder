package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) organizationSettings(orgContext *OrganizationContext) *serpent.Command {
	cmd := &serpent.Command{
		Use:     "settings",
		Short:   "Manage organization settings.",
		Aliases: []string{"setting"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Hidden: true,
		Children: []*serpent.Command{
			r.printOrganizationSetting(orgContext),
		},
	}
	return cmd
}

func (r *RootCmd) printOrganizationSetting(orgContext *OrganizationContext) *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "show <groupsync | rolesync>",
		Short: "Outputs specified organization setting.",
		Long: FormatExamples(
			Example{
				Description: "Output group sync settings.",
				Command:     "coder organization settings show groupsync",
			},
		),
		Options: []serpent.Option{},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			var setting any
			switch strings.ToLower(inv.Args[0]) {
			case "groupsync", "group-sync":
				setting, err = client.GroupIDPSyncSettings(ctx, org.ID.String())
			case "rolesync", "role-sync":
				// TODO: Implement role sync settings.
				return fmt.Errorf("role sync settings are not implemented")
			default:
				_, _ = fmt.Fprintln(inv.Stderr, "Valid organization settings are: 'groupsync', 'rolesync'")
				return fmt.Errorf("unknown organization setting %s", inv.Args[0])
			}

			if err != nil {
				return fmt.Errorf("failed to get organization setting %s: %w", inv.Args[0], err)
			}

			settingJSON, err := json.Marshal(setting)
			if err != nil {
				return fmt.Errorf("failed to marshal organization setting %s: %w", inv.Args[0], err)
			}

			var dst bytes.Buffer
			err = json.Indent(&dst, settingJSON, "", "\t")
			if err != nil {
				return fmt.Errorf("failed to indent organization setting as json %s: %w", inv.Args[0], err)
			}

			_, err = fmt.Fprintln(inv.Stdout, dst.String())
			return err
		},
	}
	return cmd
}
