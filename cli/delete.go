package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

// nolint
func (r *RootCmd) deleteWorkspace() *clibase.Cmd {
	var orphan bool
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "delete <workspace>",
		Short:       "Delete a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			sinceLastUsed := time.Since(workspace.LastUsedAt)
			cliui.Infof(inv.Stderr, "%v was last used %.0f days ago", workspace.FullName(), sinceLastUsed.Hours()/24)

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm delete workspace?",
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			var state []byte
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

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"\n%s has been deleted at %s!\n", cliui.Keyword(workspace.FullName()),
				cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        "orphan",
			Description: "Delete a workspace without deleting its resources. This can delete a workspace in a broken state, but may also lead to unaccounted cloud resources.",

			Value: clibase.BoolOf(&orphan),
		},
		cliui.SkipPromptOption(),
	}
	return cmd
}
