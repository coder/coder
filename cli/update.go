package cli

import (
	"fmt"
	"io"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) update() *serpent.Command {
	var (
		parameterFlags      workspaceParameterFlags
		bflags              buildFlags
		templateVersion     string
		stopTemplateVersion string
	)
	cmd := &serpent.Command{
		Annotations: serpent.Annotations(workspaceCommand).Mark(annotationClientSessionID, "").Mark(annotationFlightRecorder, ""),
		Use:         "update <workspace>",
		Short:       "Will update and start a given workspace if it is out of date. If the workspace is already running, it will be stopped first.",
		Long:        "Use --always-prompt to change the parameter values of the workspace.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
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

			workspace, err := client.ResolveWorkspace(inv.Context(), inv.Args[0])
			if err != nil {
				return err
			}
			versionID, err := resolveTemplateVersionID(inv.Context(), client, workspace.TemplateID, templateVersion)
			if err != nil {
				return err
			}
			stopVersionID, err := resolveTemplateVersionID(inv.Context(), client, workspace.TemplateID, stopTemplateVersion)
			if err != nil {
				return err
			}
			hasStartWork := templateVersion != "" || workspace.Outdated || parameterFlags.promptRichParameters || parameterFlags.promptEphemeralParameters || len(parameterFlags.ephemeralParameters) > 0
			// The stop version only applies when update stops the workspace first.
			willStop := workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionStart
			if stopTemplateVersion != "" && !willStop {
				latest := workspace.LatestBuild
				if latest.Transition != codersdk.WorkspaceTransitionStop || latest.Status != codersdk.WorkspaceStatusStopped {
					return xerrors.Errorf("--stop-template-version only applies when update stops a started workspace; the latest build is a %s (%s). Use 'coder stop --template-version %s' to stop with that version", latest.Transition, latest.Status, stopTemplateVersion)
				}
				if !hasStartWork {
					return xerrors.New("workspace is already stopped and up-to-date; --stop-template-version has nothing to stop")
				}
				cliui.Warn(inv.Stderr, "Workspace is already stopped, ignoring --stop-template-version.")
				stopTemplateVersion = ""
			}
			if stopTemplateVersion == "" && !hasStartWork {
				_, _ = fmt.Fprintf(inv.Stdout, "Workspace is up-to-date.\n")
				return nil
			}

			stoppedForUpdate := false
			// #17840: If the workspace is already running, we will stop it before
			// updating. Simply performing a new start transition may not work if the
			// template specifies ignore_changes.
			if willStop {
				if workspace.LatestBuild.Status == codersdk.WorkspaceStatusRunning {
					targetVersion := "the active template version"
					if templateVersion != "" {
						targetVersion = fmt.Sprintf("template version %q", templateVersion)
					}
					_, err = cliui.Prompt(inv, cliui.PromptOptions{
						Text:      fmt.Sprintf("Updating your workspace will start the workspace on %s. This can delete non-persistent data. Continue?", targetVersion),
						IsConfirm: true,
					})
					if err != nil {
						return err
					}
				}

				build, err := stopWorkspace(inv, client, workspace, bflags, stopVersionID)
				if err != nil {
					return xerrors.Errorf("stop workspace: %w", err)
				}
				// Wait for the stop to complete.
				if err := cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID); err != nil {
					return xerrors.Errorf("wait for stop: %w", err)
				}
				stoppedForUpdate = true
			}

			build, err := startWorkspace(inv, client, workspace, parameterFlags, bflags, WorkspaceUpdate, versionID)
			if err != nil {
				if stoppedForUpdate {
					return xerrors.Errorf("start workspace after stopping (workspace is stopped; run 'coder start' to retry): %w", err)
				}
				return xerrors.Errorf("start workspace: %w", err)
			}

			logs, closer, err := client.WorkspaceBuildLogsAfter(inv.Context(), build.ID, 0)
			if err != nil {
				return err
			}
			defer closer.Close()
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				_, _ = fmt.Fprintf(inv.Stdout, "Output: %s\n", log.Output)
			}
			if err := cliui.WorkspaceBuild(inv.Context(), io.Discard, client, build.ID); err != nil {
				if stoppedForUpdate {
					return xerrors.Errorf("wait for start after stopping (workspace did not restart successfully; run 'coder start' to retry): %w", err)
				}
				return xerrors.Errorf("wait for start workspace: %w", err)
			}
			return nil
		},
	}

	cmd.Options = append(cmd.Options, serpent.Option{
		Flag:        "template-version",
		Description: "Start with a named version of the workspace template. Defaults to the active version.",
		Value:       serpent.StringOf(&templateVersion),
	}, serpent.Option{
		Flag:        "stop-template-version",
		Description: "Stop with a named version of the workspace template. Only applies when the workspace is started. Defaults to the workspace's current version.",
		Value:       serpent.StringOf(&stopTemplateVersion),
	})
	cmd.Options = append(cmd.Options, parameterFlags.allOptions()...)
	cmd.Options = append(cmd.Options, bflags.cliOptions()...)
	return cmd
}
