package cli
import (
	"errors"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
func (r *RootCmd) organizationSettings(orgContext *OrganizationContext) *serpent.Command {
	settings := []organizationSetting{
		{
			Name:    "group-sync",
			Aliases: []string{"groupsync"},
			Short:   "Group sync settings to sync groups from an IdP.",
			Patch: func(ctx context.Context, cli *codersdk.Client, org uuid.UUID, input json.RawMessage) (any, error) {
				var req codersdk.GroupSyncSettings
				err := json.Unmarshal(input, &req)
				if err != nil {
					return nil, fmt.Errorf("unmarshalling group sync settings: %w", err)
				}
				return cli.PatchGroupIDPSyncSettings(ctx, org.String(), req)
			},
			Fetch: func(ctx context.Context, cli *codersdk.Client, org uuid.UUID) (any, error) {
				return cli.GroupIDPSyncSettings(ctx, org.String())
			},
		},
		{
			Name:    "role-sync",
			Aliases: []string{"rolesync"},
			Short:   "Role sync settings to sync organization roles from an IdP.",
			Patch: func(ctx context.Context, cli *codersdk.Client, org uuid.UUID, input json.RawMessage) (any, error) {
				var req codersdk.RoleSyncSettings
				err := json.Unmarshal(input, &req)
				if err != nil {
					return nil, fmt.Errorf("unmarshalling role sync settings: %w", err)
				}
				return cli.PatchRoleIDPSyncSettings(ctx, org.String(), req)
			},
			Fetch: func(ctx context.Context, cli *codersdk.Client, org uuid.UUID) (any, error) {
				return cli.RoleIDPSyncSettings(ctx, org.String())
			},
		},
		{
			Name:              "organization-sync",
			Aliases:           []string{"organizationsync", "org-sync", "orgsync"},
			Short:             "Organization sync settings to sync organization memberships from an IdP.",
			DisableOrgContext: true,
			Patch: func(ctx context.Context, cli *codersdk.Client, _ uuid.UUID, input json.RawMessage) (any, error) {
				var req codersdk.OrganizationSyncSettings
				err := json.Unmarshal(input, &req)
				if err != nil {
					return nil, fmt.Errorf("unmarshalling organization sync settings: %w", err)
				}
				return cli.PatchOrganizationIDPSyncSettings(ctx, req)
			},
			Fetch: func(ctx context.Context, cli *codersdk.Client, _ uuid.UUID) (any, error) {
				return cli.OrganizationIDPSyncSettings(ctx)
			},
		},
	}
	cmd := &serpent.Command{
		Use:     "settings",
		Short:   "Manage organization settings.",
		Aliases: []string{"setting"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.printOrganizationSetting(orgContext, settings),
			r.setOrganizationSettings(orgContext, settings),
		},
	}
	return cmd
}
type organizationSetting struct {
	Name    string
	Aliases []string
	Short   string
	// DisableOrgContext is kinda a kludge. It tells the command constructor
	// to not require an organization context. This is used for the organization
	// sync settings which are not tied to a specific organization.
	// It feels excessive to build a more elaborate solution for this one-off.
	DisableOrgContext bool
	Patch             func(ctx context.Context, cli *codersdk.Client, org uuid.UUID, input json.RawMessage) (any, error)
	Fetch             func(ctx context.Context, cli *codersdk.Client, org uuid.UUID) (any, error)
}
func (r *RootCmd) setOrganizationSettings(orgContext *OrganizationContext, settings []organizationSetting) *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "set",
		Short: "Update specified organization setting.",
		Long: FormatExamples(
			Example{
				Description: "Update group sync settings.",
				Command:     "coder organization settings set groupsync < input.json",
			},
		),
		Options: []serpent.Option{},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
	}
	for _, set := range settings {
		set := set
		patch := set.Patch
		cmd.Children = append(cmd.Children, &serpent.Command{
			Use:     set.Name,
			Aliases: set.Aliases,
			Short:   set.Short,
			Options: []serpent.Option{},
			Middleware: serpent.Chain(
				serpent.RequireNArgs(0),
				r.InitClient(client),
			),
			Handler: func(inv *serpent.Invocation) error {
				ctx := inv.Context()
				var org codersdk.Organization
				var err error
				if !set.DisableOrgContext {
					org, err = orgContext.Selected(inv, client)
					if err != nil {
						return err
					}
				}
				// Read in the json
				inputData, err := io.ReadAll(inv.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}
				output, err := patch(ctx, client, org.ID, inputData)
				if err != nil {
					return fmt.Errorf("patching %q: %w", set.Name, err)
				}
				settingJSON, err := json.Marshal(output)
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
		})
	}
	return cmd
}
func (r *RootCmd) printOrganizationSetting(orgContext *OrganizationContext, settings []organizationSetting) *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "show",
		Short: "Outputs specified organization setting.",
		Long: FormatExamples(
			Example{
				Description: "Output group sync settings.",
				Command:     "coder organization settings show groupsync",
			},
		),
		Options: []serpent.Option{},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
	}
	for _, set := range settings {
		set := set
		fetch := set.Fetch
		cmd.Children = append(cmd.Children, &serpent.Command{
			Use:     set.Name,
			Aliases: set.Aliases,
			Short:   set.Short,
			Options: []serpent.Option{},
			Middleware: serpent.Chain(
				serpent.RequireNArgs(0),
				r.InitClient(client),
			),
			Handler: func(inv *serpent.Invocation) error {
				ctx := inv.Context()
				var org codersdk.Organization
				var err error
				if !set.DisableOrgContext {
					org, err = orgContext.Selected(inv, client)
					if err != nil {
						return err
					}
				}
				output, err := fetch(ctx, client, org.ID)
				if err != nil {
					return fmt.Errorf("patching %q: %w", set.Name, err)
				}
				settingJSON, err := json.Marshal(output)
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
		})
	}
	return cmd
}
