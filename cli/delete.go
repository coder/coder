package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// nolint
func (r *RootCmd) deleteWorkspace() *serpent.Command {
	var (
		orphan bool
		prov   buildFlags
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "delete <workspace>",
		Short:       "Delete a workspace",
		Long: FormatExamples(
			Example{
				Description: "Delete a workspace for another user (if you have permission)",
				Command:     "coder delete <username>/<workspace_name>",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
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
			req := codersdk.CreateWorkspaceBuildRequest{
				Transition:       codersdk.WorkspaceTransitionDelete,
				ProvisionerState: state,
				Orphan:           orphan,
			}
			if prov.provisionerLogDebug {
				req.LogLevel = codersdk.ProvisionerLogLevelDebug
			}
			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, req)
			if err != nil {
				return err
			}
			cliutil.WarnMatchedProvisioners(inv.Stdout, build.MatchedProvisioners, build.Job)

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
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "orphan",
			Description: "Delete a workspace without deleting its resources. This can delete a workspace in a broken state, but may also lead to unaccounted cloud resources.",

			Value: serpent.BoolOf(&orphan),
		},
		cliui.SkipPromptOption(),
	}
	cmd.Options = append(cmd.Options, prov.cliOptions()...)
	return cmd
}
