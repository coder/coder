package workspaceupdates

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/websocket"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
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
	workspaces map[string]*workspace
}

type workspace struct {
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
		workspaces: make(map[string]*workspace),
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

	r.createUserRunner = createusers.NewRunner(r.client, r.cfg.User)
	newUserAndToken, err := r.createUserRunner.RunReturningUser(ctx, id, logs)
	if err != nil {
		return xerrors.Errorf("create user: %w", err)
	}
	newUser := newUserAndToken.User
	newUserClient := codersdk.New(r.client.URL,
		codersdk.WithSessionToken(newUserAndToken.SessionToken),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies())

	logger.Info(ctx, fmt.Sprintf("user %q created", newUser.Username), slog.F("id", newUser.ID.String()))

	dialCtx, cancel := context.WithTimeout(ctx, r.cfg.DialTimeout)
	defer cancel()

	logger.Info(ctx, "connecting to workspace updates stream")
	clients, err := r.dialTailnet(dialCtx, newUserClient, newUser, logger)
	if err != nil {
		return xerrors.Errorf("tailnet dial failed: %w", err)
	}
	defer clients.Closer.Close()
	logger.Info(ctx, "connected to workspace updates stream")

	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	completionCh := make(chan error, 1)
	go func() {
		completionCh <- r.watchWorkspaceUpdates(watchCtx, clients, newUser, logger)
	}()

	reachedBarrier = true
	r.cfg.DialBarrier.Done()
	r.cfg.DialBarrier.Wait()

	r.workspacebuildRunners = make([]*workspacebuild.Runner, 0, r.cfg.WorkspaceCount)
	for i := range r.cfg.WorkspaceCount {
		workspaceName, err := loadtestutil.GenerateWorkspaceName(id)
		if err != nil {
			return xerrors.Errorf("generate random name for workspace: %w", err)
		}
		workspaceBuildConfig := r.cfg.Workspace
		workspaceBuildConfig.OrganizationID = r.cfg.User.OrganizationID
		workspaceBuildConfig.UserID = newUser.ID.String()
		workspaceBuildConfig.Request.Name = workspaceName
		// We'll watch for completion ourselves via the tailnet workspace
		// updates stream.
		workspaceBuildConfig.NoWaitForAgents = true
		workspaceBuildConfig.NoWaitForBuild = true

		runner := workspacebuild.NewRunner(newUserClient, workspaceBuildConfig)
		r.workspacebuildRunners = append(r.workspacebuildRunners, runner)

		logger.Info(ctx, fmt.Sprintf("creating workspace %d/%d", i+1, r.cfg.WorkspaceCount))

		// Record build start time before running the workspace build
		r.workspaces[workspaceName] = &workspace{
			buildStartTime: time.Now(),
		}
		_, err = runner.RunReturningWorkspace(ctx, fmt.Sprintf("%s-%d", id, i), logs)
		if err != nil {
			return xerrors.Errorf("create workspace %d: %w", i, err)
		}
	}

	logger.Info(ctx, fmt.Sprintf("waiting up to %v for workspace updates to complete...", r.cfg.WorkspaceUpdatesTimeout))

	waitUpdatesCtx, cancel := context.WithTimeout(ctx, r.cfg.WorkspaceUpdatesTimeout)
	defer cancel()

	select {
	case err := <-completionCh:
		if err != nil {
			return xerrors.Errorf("workspace updates streaming failed: %w", err)
		}
		logger.Info(ctx, "workspace updates streaming completed successfully")
		return nil
	case <-waitUpdatesCtx.Done():
		cancelWatch()
		clients.Closer.Close()
		<-completionCh // ensure watch goroutine exits
		if waitUpdatesCtx.Err() == context.DeadlineExceeded {
			return xerrors.Errorf("timeout waiting for workspace updates after %v", r.cfg.WorkspaceUpdatesTimeout)
		}
		return waitUpdatesCtx.Err()
	}
}

func (r *Runner) dialTailnet(ctx context.Context, client *codersdk.Client, user codersdk.User, logger slog.Logger) (*tailnet.ControlProtocolClients, error) {
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
func (r *Runner) watchWorkspaceUpdates(ctx context.Context, clients *tailnet.ControlProtocolClients, user codersdk.User, logger slog.Logger) error {
	expectedWorkspaces := r.cfg.WorkspaceCount
	// workspace name to time the update was seen
	seenWorkspaces := make(map[string]time.Time)

	logger.Info(ctx, fmt.Sprintf("waiting for %d workspaces and their agents", expectedWorkspaces))
	for {
		select {
		case <-ctx.Done():
			logger.Error(ctx, "context canceled while waiting for workspace updates", slog.Error(ctx.Err()))
			r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "context_done")
			return ctx.Err()
		default:
		}

		update, err := clients.WorkspaceUpdates.Recv()
		if err != nil {
			logger.Error(ctx, "workspace updates stream error", slog.Error(err))
			r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "recv")
			return xerrors.Errorf("receive workspace update: %w", err)
		}
		recvTime := time.Now()

		for _, ws := range update.UpsertedWorkspaces {
			seenWorkspaces[ws.Name] = recvTime
		}

		if len(seenWorkspaces) == int(expectedWorkspaces) {
			for wsName, seenTime := range seenWorkspaces {
				// We only receive workspace updates for those that we built.
				// If we received a workspace update for a workspace we didn't build,
				// we're risking racing with the code that writes workspace
				// build start times to this map.
				ws, ok := r.workspaces[wsName]
				if !ok {
					logger.Error(ctx, "received update for unexpected workspace", slog.F("workspace", wsName), slog.F("seen_workspaces", seenWorkspaces))
					r.cfg.Metrics.AddError(user.Username, r.cfg.WorkspaceCount, "unexpected_workspace")
					return xerrors.Errorf("received update for unexpected workspace %q", wsName)
				}
				ws.updateLatency = seenTime.Sub(ws.buildStartTime)
				r.cfg.Metrics.RecordCompletion(ws.updateLatency, user.Username, r.cfg.WorkspaceCount, wsName)
			}
			logger.Info(ctx, fmt.Sprintf("updates received for all %d workspaces and agents", expectedWorkspaces))
			return nil
		}
	}
}

const (
	WorkspaceUpdatesLatencyMetric = "workspace_updates_latency_seconds"
)

func (r *Runner) GetMetrics() map[string]any {
	latencyMap := make(map[string]float64)
	for wsName, ws := range r.workspaces {
		latencyMap[wsName] = ws.updateLatency.Seconds()
	}
	return map[string]any{
		WorkspaceUpdatesLatencyMetric: latencyMap,
	}
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	for i, runner := range r.workspacebuildRunners {
		if runner != nil {
			_, _ = fmt.Fprintf(logs, "Cleaning up workspace %d/%d...\n", i+1, len(r.workspacebuildRunners))
			if err := runner.Cleanup(ctx, fmt.Sprintf("%s-%d", id, i), logs); err != nil {
				return xerrors.Errorf("cleanup workspace %d: %w", i, err)
			}
		}
	}

	if r.createUserRunner != nil {
		_, _ = fmt.Fprintln(logs, "Cleaning up user...")
		if err := r.createUserRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup user: %w", err)
		}
	}

	return nil
}
