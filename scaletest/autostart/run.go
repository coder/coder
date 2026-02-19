package autostart

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner     *createusers.Runner
	workspacebuildRunner *workspacebuild.Runner
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	reachedBarrier := false
	defer func() {
		if !reachedBarrier {
			r.cfg.SetupBarrier.Done()
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

	//nolint:gocritic // short log is fine
	logger.Info(ctx, "user created", slog.F("username", newUser.Username), slog.F("user_id", newUser.ID.String()))

	workspaceBuildConfig := r.cfg.Workspace
	workspaceBuildConfig.OrganizationID = r.cfg.User.OrganizationID
	workspaceBuildConfig.UserID = newUser.ID.String()
	// We'll wait for the build ourselves to avoid multiple API requests.
	workspaceBuildConfig.NoWaitForBuild = true
	workspaceBuildConfig.NoWaitForAgents = true

	r.workspacebuildRunner = workspacebuild.NewRunner(newUserClient, workspaceBuildConfig)
	workspace, err := r.workspacebuildRunner.RunReturningWorkspace(ctx, id, logs)
	if err != nil {
		return xerrors.Errorf("create workspace: %w", err)
	}

	// Use the pre-provided build updates channel for this workspace.
	buildUpdates := r.cfg.BuildUpdates

	// Wait for the initial workspace build to complete.
	createWorkspaceCtx, cancel := context.WithTimeout(ctx, r.cfg.WorkspaceJobTimeout)
	defer cancel()

	logger.Info(ctx, "waiting for initial workspace build", slog.F("workspace_name", workspace.Name), slog.F("workspace_id", workspace.ID.String()))
	err = waitForBuild(createWorkspaceCtx, logger, buildUpdates, codersdk.WorkspaceTransitionStart)
	if err != nil {
		return xerrors.Errorf("wait for initial workspace build (workspace=%s, id=%s): %w", workspace.Name, workspace.ID, err)
	}

	logger.Info(ctx, "workspace started successfully", slog.F("workspace_name", workspace.Name))

	// Stop the workspace.
	logger.Info(ctx, "stopping workspace", slog.F("workspace_name", workspace.Name))

	_, err = newUserClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionStop,
	})
	if err != nil {
		return xerrors.Errorf("create stop build: %w", err)
	}

	// Wait for the stop build to complete.
	stopBuildCtx, cancel := context.WithTimeout(ctx, r.cfg.WorkspaceJobTimeout)
	defer cancel()

	err = waitForBuild(stopBuildCtx, logger, buildUpdates, codersdk.WorkspaceTransitionStop)
	if err != nil {
		return xerrors.Errorf("wait for stop build: %w", err)
	}

	logger.Info(ctx, "workspace stopped successfully", slog.F("workspace_name", workspace.Name))

	logger.Info(ctx, "waiting for all runners to reach barrier")
	reachedBarrier = true
	r.cfg.SetupBarrier.Done()
	r.cfg.SetupBarrier.Wait()
	logger.Info(ctx, "all runners reached barrier, proceeding with autostart schedule")

	// Schedule the workspace to autostart.
	testStartTime := time.Now().UTC()
	autostartTime := testStartTime.Add(r.cfg.AutostartDelay).Round(time.Minute)
	schedule := fmt.Sprintf("CRON_TZ=UTC %d %d * * *", autostartTime.Minute(), autostartTime.Hour())

	logger.Info(ctx, "setting autostart schedule for workspace", slog.F("workspace_name", workspace.Name), slog.F("schedule", schedule))

	err = newUserClient.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
		Schedule: &schedule,
	})
	if err != nil {
		return xerrors.Errorf("update workspace autostart: %w", err)
	}

	logger.Info(ctx, "autostart schedule configured successfully",
		slog.F("workspace_name", workspace.Name),
		slog.F("schedule", schedule),
		slog.F("autostart_time", autostartTime),
		slog.F("time_until_autostart", time.Until(autostartTime).Round(time.Second)))

	// Wait for the autostart build to complete. The build won't start until
	// the scheduled time, so we use AutostartBuildTimeout which should account
	// for: time until scheduled start + queueing time + build execution time.
	autostartBuildCtx, cancel := context.WithTimeout(ctx, r.cfg.AutostartBuildTimeout)
	defer cancel()

	logger.Info(ctx, "waiting for autostart build to trigger and complete",
		slog.F("workspace_name", workspace.Name),
		slog.F("timeout", r.cfg.AutostartBuildTimeout))

	err = waitForBuild(autostartBuildCtx, logger, buildUpdates, codersdk.WorkspaceTransitionStart)
	if err != nil {
		return xerrors.Errorf("wait for autostart build: %w", err)
	}

	logger.Info(ctx, "autostart build completed successfully", slog.F("workspace_name", workspace.Name))

	return nil
}

// waitForBuild waits for a build with the given transition to reach a
// terminal state. It returns nil on success, or an error if the build
// fails, is canceled, or the context expires. If an unexpected transition
// is received, it returns an error immediately.
func waitForBuild(ctx context.Context, logger slog.Logger, updates <-chan codersdk.WorkspaceBuildUpdate, transition codersdk.WorkspaceTransition) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				return xerrors.New("build updates channel closed")
			}
			logger.Debug(ctx, "received build update",
				slog.F("transition", update.Transition),
				slog.F("job_status", update.JobStatus),
				slog.F("build_number", update.BuildNumber))

			if update.Transition != string(transition) {
				return xerrors.Errorf("unexpected transition: expected %s, got %s (build_number=%d)", transition, update.Transition, update.BuildNumber)
			}
			switch codersdk.ProvisionerJobStatus(update.JobStatus) {
			case codersdk.ProvisionerJobSucceeded:
				return nil
			case codersdk.ProvisionerJobFailed:
				return xerrors.Errorf("workspace build failed (transition=%s, build_number=%d)", update.Transition, update.BuildNumber)
			case codersdk.ProvisionerJobCanceled:
				return xerrors.Errorf("workspace build canceled (transition=%s, build_number=%d)", update.Transition, update.BuildNumber)
			default:
				// Intermediate states (pending, running, canceling)
				// are expected; keep waiting.
			}
		}
	}
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.workspacebuildRunner != nil {
		_, _ = fmt.Fprintln(logs, "Cleaning up workspace...")
		if err := r.workspacebuildRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup workspace: %w", err)
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
