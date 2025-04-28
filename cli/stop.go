package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) stop() *serpent.Command {
	var bflags buildFlags
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "stop <workspace>",
		Short:       "Stop a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			_, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm stop workspace?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			if workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobPending {
				// cliutil.WarnMatchedProvisioners also checks if the job is pending
				// but we still want to avoid users spamming multiple builds that will
				// not be picked up.
				cliui.Warn(inv.Stderr, "The workspace is already stopping!")
				cliutil.WarnMatchedProvisioners(inv.Stderr, workspace.LatestBuild.MatchedProvisioners, workspace.LatestBuild.Job)
				if _, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Enqueue another stop?",
					IsConfirm: true,
					Default:   cliui.ConfirmNo,
				}); err != nil {
					return err
				}
			}

			wbr := codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
			}
			if bflags.provisionerLogDebug {
				wbr.LogLevel = codersdk.ProvisionerLogLevelDebug
			}
			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, wbr)
			if err != nil {
				return err
			}
			cliutil.WarnMatchedProvisioners(inv.Stderr, build.MatchedProvisioners, build.Job)

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"\nThe %s workspace has been stopped at %s!\n", cliui.Keyword(workspace.Name),

				cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	cmd.Options = append(cmd.Options, bflags.cliOptions()...)

	return cmd
}
