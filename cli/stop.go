package cli

import (
	"fmt"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) stop() *serpent.Command {
	var (
		bflags        buildFlags
		logDir        string
		logBufferSize int64
	)
	cmd := &serpent.Command{
		Annotations: serpent.Annotations(workspaceCommand).Mark(annotationClientSessionID, ""),
		Use:         "stop <workspace>",
		Short:       "Stop a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			logDirOption(&logDir, "CODER_LOG_DIR"),
			logBufferSizeOption(&logBufferSize),
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) (retErr error) {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()
			logger, closeLog, err := r.newSessionLogger(inv, "stop", logDir, logBufferSize)
			if err != nil {
				return err
			}
			defer closeLog()
			client.SetLogger(logger)
			// Logging the terminal error at Error flushes the buffered debug
			// history so the detail leading up to a failure is written to the
			// log file.
			defer func() {
				if retErr != nil {
					logger.Error(ctx, "command exit", slog.Error(retErr))
				}
			}()

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

			build, err := stopWorkspace(inv, client, workspace, bflags)
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

func stopWorkspace(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, bflags buildFlags) (codersdk.WorkspaceBuild, error) {
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
		Transition: codersdk.WorkspaceTransitionStop,
	}
	if bflags.provisionerLogDebug {
		wbr.LogLevel = codersdk.ProvisionerLogLevelDebug
	}
	return client.CreateWorkspaceBuild(inv.Context(), workspace.ID, wbr)
}
