package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/xerrors"

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
			r.updateOrganizationSetting(orgContext),
		},
	}
	return cmd
}

func (r *RootCmd) updateOrganizationSetting(orgContext *OrganizationContext) *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "set <groupsync | rolesync>",
		Short: "Update specified organization setting.",
		Long: FormatExamples(
			Example{
				Description: "Update group sync settings.",
				Command:     "coder organization settings set groupsync < input.json",
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

			// Read in the json
			inputData, err := io.ReadAll(inv.Stdin)
			if err != nil {
				return xerrors.Errorf("reading stdin: %w", err)
			}

			var setting any
			switch strings.ToLower(inv.Args[0]) {
			case "groupsync", "group-sync":
				var req codersdk.GroupSyncSettings
				err = json.Unmarshal(inputData, &req)
				if err != nil {
					return xerrors.Errorf("unmarshalling group sync settings: %w", err)
				}
				setting, err = client.PatchGroupIDPSyncSettings(ctx, org.ID.String(), req)
			case "rolesync", "role-sync":
				var req codersdk.RoleSyncSettings
				err = json.Unmarshal(inputData, &req)
				if err != nil {
					return xerrors.Errorf("unmarshalling role sync settings: %w", err)
				}
				setting, err = client.PatchRoleIDPSyncSettings(ctx, org.ID.String(), req)
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
				setting, err = client.RoleIDPSyncSettings(ctx, org.ID.String())
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
