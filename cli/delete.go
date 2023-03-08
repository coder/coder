package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

// nolint
func (r *RootCmd) deleteWorkspace() *clibase.Cmd {
	var orphan bool
	var client *codersdk.Client
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "delete <workspace>",
		Short:       "Delete a workspace",
		Aliases:     []string{"rm"},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.useClient(client),
		),
		Handler: func(inv *clibase.Invokation) error {
			_, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm delete workspace?",
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			var state []byte

			if orphan {
				cliui.Warn(
					inv.Stderr,
					"Orphaning workspace requires template edit permission",
				)
			}

			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition:       codersdk.WorkspaceTransitionDelete,
				ProvisionerState: state,
				Orphan:           orphan,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout, "\nThe %s workspace has been deleted at %s!\n", cliui.Styles.Keyword.Render(workspace.Name), cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&orphan, "orphan", false,
		`Delete a workspace without deleting its resources. This can delete a
workspace in a broken state, but may also lead to unaccounted cloud resources.`,
	)
	cliui.SkipPromptOption(inv)
	return cmd
}
