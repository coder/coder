package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	aiagentapi "github.com/coder/agentapi-sdk-go"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskLogs() *serpent.Command {
	var follow bool

	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]codersdk.TaskLogEntry{},
			[]string{
				"type",
				"content",
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "logs <task>",
		Short: "Show a task's logs",
		Long: FormatExamples(
			Example{
				Description: "Show logs for a given task.",
				Command:     "coder exp task logs task1",
			},
			Example{
				Description: "Follow logs in real-time.",
				Command:     "coder exp task logs task1 --follow",
			}),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			var (
				ctx    = inv.Context()
				task   = inv.Args[0]
				taskID uuid.UUID
			)

			if id, err := uuid.Parse(task); err == nil {
				taskID = id
			} else {
				ws, err := namedWorkspace(ctx, client, task)
				if err != nil {
					return xerrors.Errorf("resolve task %q: %w", task, err)
				}

				taskID = ws.ID
			}

			if follow {
				return r.followTaskLogs(inv, client, taskID)
			}

			exp := codersdk.NewExperimentalClient(client)
			logs, err := exp.TaskLogs(ctx, codersdk.Me, taskID)
			if err != nil {
				return xerrors.Errorf("get task logs: %w", err)
			}

			out, err := formatter.Format(ctx, logs.Logs)
			if err != nil {
				return xerrors.Errorf("format task logs: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
		Options: serpent.OptionSet{
			{
				Flag:          "follow",
				FlagShorthand: "f",
				Description:   "Follow logs in real-time, similar to 'tail -f'.",
				Value:         serpent.BoolOf(&follow),
			},
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// TODO(cian): this will need to be updated when the task data model is updated.
func (*RootCmd) followTaskLogs(inv *serpent.Invocation, client *codersdk.Client, taskID uuid.UUID) error {
	ctx := inv.Context()
	workspace, err := client.Workspace(ctx, taskID)
	if err != nil {
		return xerrors.Errorf("fetch workspace: %w", err)
	}
	if workspace.LatestBuild.ID == uuid.Nil {
		return xerrors.Errorf("workspace has no builds")
	}
	build := workspace.LatestBuild
	if build.HasAITask == nil || !*build.HasAITask {
		return xerrors.Errorf("workspace is not configured as an AI task")
	}

	if build.AITaskSidebarAppID == nil || *build.AITaskSidebarAppID == uuid.Nil {
		return xerrors.Errorf("task is not configured with a sidebar app")
	}

	sidebarAppID := *build.AITaskSidebarAppID
	var agentID uuid.UUID
	var sidebarApp *codersdk.WorkspaceApp

	for _, res := range build.Resources {
		for _, agent := range res.Agents {
			for _, app := range agent.Apps {
				if app.ID == sidebarAppID {
					agentID = agent.ID
					sidebarApp = &app
					break
				}
			}
			if sidebarApp != nil {
				break
			}
		}
		if sidebarApp != nil {
			break
		}
	}

	if sidebarApp == nil {
		return xerrors.Errorf("task sidebar app not found in latest build")
	}
	switch sidebarApp.Health {
	case codersdk.WorkspaceAppHealthInitializing:
		return xerrors.Errorf("task sidebar app is initializing, try again shortly")
	case codersdk.WorkspaceAppHealthUnhealthy:
		return xerrors.Errorf("task sidebar app is unhealthy")
	}
	if sidebarApp.URL == "" {
		return xerrors.Errorf("task sidebar app URL is not configured")
	}
	parsedURL, err := url.Parse(sidebarApp.URL)
	if err != nil {
		return xerrors.Errorf("parse task app URL: %w", err)
	}
	if parsedURL.Scheme != "http" {
		return xerrors.Errorf("only http scheme is supported for direct agent-dial")
	}

	cliui.Infof(inv.Stderr, "Connecting to task workspace agent...")
	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()

	agentConn, err := workspacesdk.New(client).DialAgent(dialCtx, agentID, &workspacesdk.DialAgentOptions{
		Logger: inv.Logger,
	})
	if err != nil {
		return xerrors.Errorf("dial workspace agent: %w", err)
	}
	defer agentConn.Close()

	// Wait for the connection to be reachable
	agentConn.AwaitReachable(ctx)

	// Create HTTP client that dials through the agent
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return agentConn.DialContext(ctx, network, addr)
			},
		},
	}

	agentAPIClient, err := aiagentapi.NewClient(parsedURL.String(), aiagentapi.WithHTTPClient(httpClient))
	if err != nil {
		return xerrors.Errorf("create agent API client: %w", err)
	}

	cliui.Infof(inv.Stderr, "Subscribing to log events...")
	eventsCh, errCh, err := agentAPIClient.SubscribeEvents(ctx)
	if err != nil {
		return xerrors.Errorf("subscribe to events: %w", err)
	}

	messagesResp, err := agentAPIClient.GetMessages(ctx)
	if err != nil {
		return xerrors.Errorf("get existing messages: %w", err)
	}
	for _, msg := range messagesResp.Messages {
		_, _ = fmt.Fprintln(inv.Stdout, msg.Content)
	}

	lastSeenID := int64(-1)
	if len(messagesResp.Messages) > 0 {
		lastSeenID = messagesResp.Messages[len(messagesResp.Messages)-1].Id
	}

	cliui.Infof(inv.Stderr, "Following logs (Ctrl+C to stop)...")

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if err != nil {
				return xerrors.Errorf("error streaming events: %w", err)
			}
			return nil
		case event := <-eventsCh:
			switch msgEvent := event.(type) {
			case aiagentapi.EventMessageUpdate:
				if msgEvent.Id <= lastSeenID {
					continue
				}
				lastSeenID = msgEvent.Id
				// Prefix user messages
				if msgEvent.Role == aiagentapi.RoleUser {
					_, _ = fmt.Fprintf(inv.Stdout, "\t>")
				}
				_, _ = fmt.Fprintln(inv.Stdout, msgEvent.Message)
			default: // ignore all other events
			}
		}
	}
}
