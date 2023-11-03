package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) autoupdate() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "autoupdate <workspace> <always|never>",
		Short:       "Toggle auto-update policy for a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			policy := strings.ToLower(inv.Args[1])
			err := validateAutoUpdatePolicy(policy)
			if err != nil {
				return xerrors.Errorf("validate policy: %w", err)
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			err = client.UpdateWorkspaceAutomaticUpdates(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceAutomaticUpdatesRequest{
				AutomaticUpdates: codersdk.AutomaticUpdates(policy),
			})
			if err != nil {
				return xerrors.Errorf("update workspace automatic updates policy: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Updated workspace %q auto-update policy to %q\n", workspace.Name, policy)
			return nil
		},
	}

	cmd.Options = append(cmd.Options, cliui.SkipPromptOption())
	return cmd
}

func validateAutoUpdatePolicy(arg string) error {
	switch codersdk.AutomaticUpdates(arg) {
	case codersdk.AutomaticUpdatesAlways, codersdk.AutomaticUpdatesNever:
		return nil
	default:
		return xerrors.Errorf("invalid option %q must be either of %q or %q", arg, codersdk.AutomaticUpdatesAlways, codersdk.AutomaticUpdatesNever)
	}
}
