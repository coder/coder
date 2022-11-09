package workspacebuild

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/loadtest/harness"
	"github.com/coder/coder/loadtest/loadtestutil"
)

type Runner struct {
	client      *codersdk.Client
	cfg         Config
	workspaceID uuid.UUID
}

var _ harness.Runnable = &Runner{}
var _ harness.Cleanable = &Runner{}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

// Run implements Runnable.
func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.Logger = logger
	r.client.LogBodies = true

	req := r.cfg.Request
	if req.Name == "" {
		randName, err := cryptorand.HexString(8)
		if err != nil {
			return xerrors.Errorf("generate random name for workspace: %w", err)
		}
		req.Name = "test-" + randName
	}

	workspace, err := r.client.CreateWorkspace(ctx, r.cfg.OrganizationID, r.cfg.UserID, req)
	if err != nil {
		return xerrors.Errorf("create workspace: %w", err)
	}
	r.workspaceID = workspace.ID

	err = waitForBuild(ctx, logs, r.client, workspace.LatestBuild.ID)
	if err != nil {
		return xerrors.Errorf("wait for build: %w", err)
	}

	_, _ = fmt.Fprintln(logs, "")
	err = waitForAgents(ctx, logs, r.client, workspace.ID)
	if err != nil {
		return xerrors.Errorf("wait for agent: %w", err)
	}

	return nil
}

// Cleanup implements Cleanable.
func (r *Runner) Cleanup(ctx context.Context, _ string) error {
	if r.workspaceID == uuid.Nil {
		return nil
	}
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	build, err := r.client.CreateWorkspaceBuild(ctx, r.workspaceID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
	})
	if err != nil {
		return xerrors.Errorf("delete workspace: %w", err)
	}

	// TODO: capture these logs
	logs := io.Discard
	err = waitForBuild(ctx, logs, r.client, build.ID)
	if err != nil {
		return xerrors.Errorf("wait for build: %w", err)
	}

	return nil
}

func waitForBuild(ctx context.Context, w io.Writer, client *codersdk.Client, buildID uuid.UUID) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	_, _ = fmt.Fprint(w, "Build is currently queued...")

	// Wait for build to start.
	for {
		build, err := client.WorkspaceBuild(ctx, buildID)
		if err != nil {
			return xerrors.Errorf("fetch build: %w", err)
		}

		if build.Job.Status != codersdk.ProvisionerJobPending {
			break
		}

		_, _ = fmt.Fprint(w, ".")
		time.Sleep(500 * time.Millisecond)
	}

	_, _ = fmt.Fprintln(w, "\nBuild started! Streaming logs below:")

	logs, closer, err := client.WorkspaceBuildLogsAfter(ctx, buildID, 0)
	if err != nil {
		return xerrors.Errorf("start streaming build logs: %w", err)
	}
	defer closer.Close()

	currentStage := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case log, ok := <-logs:
			if !ok {
				build, err := client.WorkspaceBuild(ctx, buildID)
				if err != nil {
					return xerrors.Errorf("fetch build: %w", err)
				}

				_, _ = fmt.Fprintln(w, "")
				switch build.Job.Status {
				case codersdk.ProvisionerJobSucceeded:
					_, _ = fmt.Fprintln(w, "\nBuild succeeded!")
					return nil
				case codersdk.ProvisionerJobFailed:
					_, _ = fmt.Fprintf(w, "\nBuild failed with error %q.\nSee logs above for more details.\n", build.Job.Error)
					return xerrors.Errorf("build failed with status %q: %s", build.Job.Status, build.Job.Error)
				case codersdk.ProvisionerJobCanceled:
					_, _ = fmt.Fprintln(w, "\nBuild canceled.")
					return xerrors.New("build canceled")
				default:
					_, _ = fmt.Fprintf(w, "\nLogs disconnected with unexpected job status %q and error %q.\n", build.Job.Status, build.Job.Error)
					return xerrors.Errorf("logs disconnected with unexpected job status %q and error %q", build.Job.Status, build.Job.Error)
				}
			}

			if log.Stage != currentStage {
				currentStage = log.Stage
				_, _ = fmt.Fprintf(w, "\n%s\n", currentStage)
			}

			level := "unknown"
			if log.Level != "" {
				level = string(log.Level)
			}
			_, _ = fmt.Fprintf(w, "\t%s:\t%s\n", level, log.Output)
		}
	}
}

func waitForAgents(ctx context.Context, w io.Writer, client *codersdk.Client, workspaceID uuid.UUID) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	_, _ = fmt.Fprint(w, "Waiting for agents to connect...\n\n")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		workspace, err := client.Workspace(ctx, workspaceID)
		if err != nil {
			return xerrors.Errorf("fetch workspace: %w", err)
		}

		ok := true
		for _, res := range workspace.LatestBuild.Resources {
			for _, agent := range res.Agents {
				if agent.Status != codersdk.WorkspaceAgentConnected {
					ok = false
				}

				_, _ = fmt.Fprintf(w, "\tAgent %q is %s\n", agent.Name, agent.Status)
			}
		}
		if ok {
			break
		}

		_, _ = fmt.Fprintln(w, "")
		time.Sleep(1 * time.Second)
	}

	_, _ = fmt.Fprint(w, "\nAgents connected!\n\n")
	return nil
}
