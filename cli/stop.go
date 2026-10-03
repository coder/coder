package cli

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) stop() *serpent.Command {
	var bflags buildFlags
	var templateVersion string
	cmd := &serpent.Command{
		Annotations: serpent.Annotations(workspaceCommand).Mark(annotationClientSessionID, "").Mark(annotationFlightRecorder, ""),
		Use:         "stop <workspace>",
		Short:       "Stop a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Flag:        "template-version",
				Description: "Stop with a named version of the workspace template. Defaults to the workspace's current version.",
				Value:       serpent.StringOf(&templateVersion),
			},
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			// The invocation logger records debug detail and emits it to stderr
			// only if the command fails (see flightRecorderMiddleware).
			client.SetLogger(inv.Logger)

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm stop workspace?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			workspace, err := client.ResolveWorkspace(inv.Context(), inv.Args[0])
			if err != nil {
				return err
			}

			versionID, err := resolveWorkspaceTemplateVersion(inv.Context(), client, workspace, templateVersion)
			if err != nil {
				return err
			}
			build, err := stopWorkspace(inv, client, workspace, bflags, versionID)
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"\nThe %s workspace has been stopped at %s!\n",
				cliui.Keyword(workspace.Name),
				cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	cmd.Options = append(cmd.Options, bflags.cliOptions()...)

	return cmd
}

func stopWorkspace(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, bflags buildFlags, templateVersionID uuid.UUID) (codersdk.WorkspaceBuild, error) {
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
			return codersdk.WorkspaceBuild{}, err
		}
	}
	wbr := codersdk.CreateWorkspaceBuildRequest{
		Transition:        codersdk.WorkspaceTransitionStop,
		TemplateVersionID: templateVersionID,
	}
	if bflags.provisionerLogDebug {
		wbr.LogLevel = codersdk.ProvisionerLogLevelDebug
	}
	return client.CreateWorkspaceBuild(inv.Context(), workspace.ID, wbr)
}
