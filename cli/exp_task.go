package cli

import (
	"context"
	"errors"
	"net/url"
	"time"

	agentapi "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "task",
		Short: "Interact with AI tasks.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.taskReportStatus(),
		},
	}
	return cmd
}

func (r *RootCmd) taskReportStatus() *serpent.Command {
	var (
		slug     string
		interval time.Duration
		llmURL   url.URL
	)
	cmd := &serpent.Command{
		Use:   "report-status",
		Short: "Report status of the currently running task to Coder.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			// This is meant to run in a workspace, so instead of a regular client we
			// need a workspace agent client to update the status in coderd.
			agentClient, err := r.createAgentClient()
			if err != nil {
				return err
			}

			// We also need an agentapi client to get the LLM agent's current status.
			llmClient, err := agentapi.NewClient(llmURL.String())
			if err != nil {
				return err
			}

			notifyCtx, notifyCancel := inv.SignalNotifyContext(ctx, StopSignals...)
			defer notifyCancel()

		outerLoop:
			for {
				res, err := llmClient.GetStatus(notifyCtx)
				if err != nil && !errors.Is(err, context.Canceled) {
					cliui.Warnf(inv.Stderr, "failed to fetch status: %s", err)
				} else {
					// Currently we only update the status, which leaves the last summary
					// (if any) untouched.  If we do want to update the summary here, we
					// will need to fetch the messages and generate one.
					status := codersdk.WorkspaceAppStatusStateWorking
					switch res.Status {
					case agentapi.StatusStable: // Stable == idle == done
						status = codersdk.WorkspaceAppStatusStateComplete
					case agentapi.StatusRunning: // Running == working
					}
					err = agentClient.PatchAppStatus(notifyCtx, agentsdk.PatchAppStatus{
						AppSlug: slug,
						State:   status,
					})
					if err != nil && !errors.Is(err, context.Canceled) {
						cliui.Warnf(inv.Stderr, "failed to update status: %s", err)
					}
				}

				timer := time.NewTimer(interval)
				select {
				case <-notifyCtx.Done():
					timer.Stop()
					break outerLoop
				case <-timer.C:
				}
			}

			return nil
		},
		Options: []serpent.Option{
			{
				Flag:        "app-slug",
				Description: "The app slug to use when reporting the status.",
				Env:         "CODER_MCP_APP_STATUS_SLUG",
				Required:    true,
				Value:       serpent.StringOf(&slug),
			},
			{
				Flag:        "agentapi-url",
				Description: "The URL of the LLM agent API.",
				Env:         "CODER_AGENTAPI_URL",
				Required:    true,
				Value:       serpent.URLOf(&llmURL),
			},
			{
				Flag:        "interval",
				Description: "The interval on which to poll for the status.",
				Env:         "CODER_APP_STATUS_INTERVAL",
				Default:     "30s",
				Value:       serpent.DurationOf(&interval),
			},
		},
	}
	return cmd
}
