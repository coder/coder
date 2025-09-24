package coderconnect

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/websocket"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/syncmap"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner      *createusers.Runner
	workspacebuildRunners []*workspacebuild.Runner

	// workspace name to workspace
	workspaces *syncmap.Map[string, *workspace]
}

type workspace struct {
	workspaceID    uuid.UUID
	buildStartTime time.Time
	updateLatency  time.Duration
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:     client,
		cfg:        cfg,
		workspaces: syncmap.New[string, *workspace](),
	}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	reachedBarrier := false
	defer func() {
		if !reachedBarrier {
			r.cfg.DialBarrier.Done()
		}
	}()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	var (
		client = r.client
		user   codersdk.User
		err    error
	)
	if r.cfg.User.SessionToken != "" {
		user, err = client.User(ctx, "me")
		if err != nil {
			return xerrors.Errorf("get user with session token: %w", err)
		}
	} else {
		createUserConfig := createusers.Config{
			OrganizationID: r.cfg.User.OrganizationID,
			Username:       r.cfg.User.Username,
			Email:          r.cfg.User.Email,
		}
		if err := createUserConfig.Validate(); err != nil {
			return xerrors.Errorf("validate create user config: %w", err)
		}
		r.createUserRunner = createusers.NewRunner(r.client, createUserConfig)
		newUser, err := r.createUserRunner.RunReturningUser(ctx, id, logs)
		if err != nil {
			return xerrors.Errorf("create user: %w", err)
		}
		user = newUser.User
		client = codersdk.New(r.client.URL)
		client.SetSessionToken(newUser.SessionToken)
		client.SetLogger(logger)
		client.SetLogBodies(true)
	}

	_, _ = fmt.Fprintf(logs, "Using user %q (id: %s)\n", user.Username, user.ID)

	dialCtx, cancel := context.WithTimeout(ctx, r.cfg.DialTimeout)
	defer cancel()

	_, _ = fmt.Fprintf(logs, "Connecting to workspace updates stream for user %s\n", user.Username)
	clients, err := r.dialCoderConnect(dialCtx, client, user, logger)
	if err != nil {
		return xerrors.Errorf("coder connect dial failed: %w", err)
	}
	_, _ = fmt.Fprintf(logs, "Successfully connected to workspace updates stream\n")

	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	completionCh := make(chan error, 1)
	go func() {
		completionCh <- r.watchWorkspaceUpdates(watchCtx, clients, user, logs)
	}()

	reachedBarrier = true
	r.cfg.DialBarrier.Done()
	r.cfg.DialBarrier.Wait()

	workspaceRunners := make([]*workspacebuild.Runner, 0, r.cfg.WorkspaceCount)
	for i := range r.cfg.WorkspaceCount {
		workspaceName, err := loadtestutil.GenerateWorkspaceName(id)
		if err != nil {
			return xerrors.Errorf("generate random name for workspace: %w", err)
		}
		workspaceBuildConfig := r.cfg.Workspace
		workspaceBuildConfig.OrganizationID = r.cfg.User.OrganizationID
		workspaceBuildConfig.UserID = user.ID.String()
		workspaceBuildConfig.Request.Name = workspaceName

		runner := workspacebuild.NewRunner(client, workspaceBuildConfig)
		workspaceRunners = append(workspaceRunners, runner)

		_, _ = fmt.Fprintf(logs, "Creating workspace %d/%d...\n", i+1, r.cfg.WorkspaceCount)

		// Record build start time before running the workspace build
		r.workspaces.Store(workspaceName, &workspace{
			buildStartTime: time.Now(),
		})
		err = runner.Run(ctx, fmt.Sprintf("%s-%d", id, i), logs)
		if err != nil {
			return xerrors.Errorf("create workspace %d: %w", i, err)
		}
	}

	r.workspacebuildRunners = workspaceRunners

	_, _ = fmt.Fprintf(logs, "Waiting up to %v for workspace updates to complete...\n", r.cfg.WorkspaceUpdatesTimeout)

	waitUpdatesCtx, cancel := context.WithTimeout(ctx, r.cfg.WorkspaceUpdatesTimeout)
	defer cancel()

	select {
	case err := <-completionCh:
		if err != nil {
			return xerrors.Errorf("workspace updates streaming failed: %w", err)
		}
		_, _ = fmt.Fprintf(logs, "Workspace updates streaming completed successfully\n")
		return nil
	case <-waitUpdatesCtx.Done():
		if waitUpdatesCtx.Err() == context.DeadlineExceeded {
			return xerrors.Errorf("timeout waiting for workspace updates after %v", r.cfg.WorkspaceUpdatesTimeout)
		}
		return waitUpdatesCtx.Err()
	}
}

func (r *Runner) dialCoderConnect(ctx context.Context, client *codersdk.Client, user codersdk.User, logger slog.Logger) (*tailnet.ControlProtocolClients, error) {
	u, err := client.URL.Parse("/api/v2/tailnet")
	if err != nil {
		logger.Error(ctx, "failed to parse tailnet URL", slog.Error(err))
		r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "parse_url")
		return nil, xerrors.Errorf("parse tailnet URL: %w", err)
	}

	dialer := workspacesdk.NewWebsocketDialer(
		logger,
		u,
		&websocket.DialOptions{
			HTTPHeader: http.Header{
				"Coder-Session-Token": []string{client.SessionToken()},
			},
		},
		workspacesdk.WithWorkspaceUpdates(&tailnetproto.WorkspaceUpdatesRequest{
			WorkspaceOwnerId: tailnet.UUIDToByteSlice(user.ID),
		}),
	)

	clients, err := dialer.Dial(ctx, nil)
	if err != nil {
		logger.Error(ctx, "failed to dial workspace updates", slog.Error(err))
		r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "dial")
		return nil, xerrors.Errorf("dial workspace updates: %w", err)
	}

	return &clients, nil
}

// watchWorkspaceUpdates processes workspace updates and returns error or nil
// once all expected workspaces and agents are seen.
func (r *Runner) watchWorkspaceUpdates(ctx context.Context, clients *tailnet.ControlProtocolClients, user codersdk.User, logs io.Writer) error {
	defer clients.Closer.Close()

	expectedWorkspaces := r.cfg.WorkspaceCount
	seenWorkspaces := 0
	// Workspace ID to agent update arrival time.
	// At the end, we reconcile to see which took longer, and mark that as the
	// latency.
	agents := make(map[uuid.UUID]time.Time)

	_, _ = fmt.Fprintf(logs, "Waiting for %d workspaces and their agents\n", expectedWorkspaces)
	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintf(logs, "Context canceled while waiting for workspace updates: %v\n", ctx.Err())
			r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "context_done")
			return ctx.Err()
		default:
		}

		update, err := clients.WorkspaceUpdates.Recv()
		if err != nil {
			_, _ = fmt.Fprintf(logs, "Workspace updates stream error: %v\n", err)
			r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "recv")
			return xerrors.Errorf("receive workspace update: %w", err)
		}

		for _, ws := range update.UpsertedWorkspaces {
			wsID, err := uuid.FromBytes(ws.Id)
			if err != nil {
				_, _ = fmt.Fprintf(logs, "Invalid workspace ID in update: %v\n", err)
				r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "bad_workspace_id")
				continue
			}

			if tracking, ok := r.workspaces.Load(ws.GetName()); ok {
				if tracking.updateLatency == 0 {
					r.workspaces.Store(ws.GetName(), &workspace{
						workspaceID:    wsID,
						buildStartTime: tracking.buildStartTime,
						updateLatency:  time.Since(tracking.buildStartTime),
					})
					seenWorkspaces++
				}
			} else if !ok {
				return xerrors.Errorf("received update for unknown workspace %q (id: %s)", ws.GetName(), wsID)
			}
		}

		for _, agent := range update.UpsertedAgents {
			wsID, err := uuid.FromBytes(agent.WorkspaceId)
			if err != nil {
				_, _ = fmt.Fprintf(logs, "Invalid workspace ID in agent update: %v\n", err)
				r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "bad_agent_workspace_id")
				continue
			}

			if _, ok := agents[wsID]; !ok {
				agents[wsID] = time.Now()
			}
		}

		if seenWorkspaces == int(expectedWorkspaces) && len(agents) == int(expectedWorkspaces) {
			// For each workspace, record the latency from build start to
			// workspace update, or agent update, whichever is later.
			r.workspaces.Range(func(wsName string, ws *workspace) bool {
				if agentTime, ok := agents[ws.workspaceID]; ok {
					agentLatency := agentTime.Sub(ws.buildStartTime)
					if agentLatency > ws.updateLatency {
						// Update in-place, so GetMetrics is accurate.
						ws.updateLatency = agentLatency
					}
				} else {
					// Unreachable, recorded for debugging
					r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "missing_agent")
				}
				r.cfg.Metrics.RecordCompletion(ws.updateLatency, user.Username, r.cfg.WorkspaceCount, wsName)
				return true
			})
			_, _ = fmt.Fprintf(logs, "Updates received for all %d workspaces and agents\n", expectedWorkspaces)
			return nil
		}
	}
}

const (
	WorkspaceUpdatesLatencyMetric = "workspace_updates_latency_seconds"
	WorkspaceUpdatesErrorsTotal   = "workspace_updates_errors_total"
)

func (r *Runner) GetMetrics() map[string]any {
	latencyMap := make(map[string]float64)
	r.workspaces.Range(func(wsName string, ws *workspace) bool {
		latencyMap[wsName] = ws.updateLatency.Seconds()
		return true
	})
	return map[string]any{
		WorkspaceUpdatesErrorsTotal:   r.cfg.Metrics.numErrors.Load(),
		WorkspaceUpdatesLatencyMetric: latencyMap,
	}
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.cfg.NoCleanup {
		_, _ = fmt.Fprintln(logs, "skipping cleanup")
		return nil
	}

	for i, runner := range r.workspacebuildRunners {
		if runner != nil {
			_, _ = fmt.Fprintf(logs, "Cleaning up workspace %d/%d...\n", i+1, len(r.workspacebuildRunners))
			if err := runner.Cleanup(ctx, fmt.Sprintf("%s-%d", id, i), logs); err != nil {
				return xerrors.Errorf("cleanup workspace %d: %w", i, err)
			}
		}
	}

	if r.createUserRunner != nil {
		if err := r.createUserRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup user: %w", err)
		}
	}

	return nil
}
