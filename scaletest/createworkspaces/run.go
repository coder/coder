package createworkspaces

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/agentconn"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/reconnectingpty"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner     *createusers.Runner
	workspacebuildRunner *workspacebuild.Runner
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

// Run implements Runnable.
func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

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
		_, _ = fmt.Fprintln(logs, "Using existing user session token:")
		user, err = client.User(ctx, "me")
		if err != nil {
			return xerrors.Errorf("generate random password for user: %w", err)
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
	}

	_, _ = fmt.Fprintln(logs, "\nCreating workspace...")
	workspaceBuildConfig := r.cfg.Workspace
	workspaceBuildConfig.OrganizationID = r.cfg.User.OrganizationID
	workspaceBuildConfig.UserID = user.ID.String()
	r.workspacebuildRunner = workspacebuild.NewRunner(client, workspaceBuildConfig)
	slimWorkspace, err := r.workspacebuildRunner.RunReturningWorkspace(ctx, id, logs)
	if err != nil {
		return xerrors.Errorf("create workspace: %w", err)
	}
	workspace, err := client.Workspace(ctx, slimWorkspace.ID)
	if err != nil {
		return xerrors.Errorf("get full workspace info: %w", err)
	}

	if r.cfg.Workspace.NoWaitForAgents {
		return nil
	}

	// Find the first agent.
	var agent codersdk.WorkspaceAgent
resourceLoop:
	for _, res := range workspace.LatestBuild.Resources {
		for _, a := range res.Agents {
			agent = a
			break resourceLoop
		}
	}
	if agent.ID == uuid.Nil {
		return xerrors.Errorf("no agents found for workspace %q", workspace.ID.String())
	}

	eg, egCtx := errgroup.WithContext(ctx)
	if r.cfg.ReconnectingPTY != nil {
		eg.Go(func() error {
			reconnectingPTYConfig := *r.cfg.ReconnectingPTY
			reconnectingPTYConfig.AgentID = agent.ID

			reconnectingPTYRunner := reconnectingpty.NewRunner(client, reconnectingPTYConfig)
			err := reconnectingPTYRunner.Run(egCtx, id, logs)
			if err != nil {
				return xerrors.Errorf("run reconnecting pty: %w", err)
			}

			return nil
		})
	}
	if r.cfg.AgentConn != nil {
		eg.Go(func() error {
			agentConnConfig := *r.cfg.AgentConn
			agentConnConfig.AgentID = agent.ID

			agentConnRunner := agentconn.NewRunner(client, agentConnConfig)
			err := agentConnRunner.Run(egCtx, id, logs)
			if err != nil {
				return xerrors.Errorf("run agent connection: %w", err)
			}

			return nil
		})
	}

	err = eg.Wait()
	if err != nil {
		return xerrors.Errorf("run workspace connections in parallel: %w", err)
	}

	return nil
}

// Cleanup implements Cleanable.
func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.cfg.NoCleanup {
		_, _ = fmt.Fprintln(logs, "skipping cleanup")
		return nil
	}

	if r.workspacebuildRunner != nil {
		err := r.workspacebuildRunner.Cleanup(ctx, id, logs)
		if err != nil {
			return xerrors.Errorf("cleanup workspace: %w", err)
		}
	}

	if r.createUserRunner != nil {
		err := r.createUserRunner.Cleanup(ctx, id, logs)
		if err != nil {
			return xerrors.Errorf("cleanup user: %w", err)
		}
	}

	return nil
}
