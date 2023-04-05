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

	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/scaletest/agentconn"
	"github.com/coder/coder/scaletest/harness"
	"github.com/coder/coder/scaletest/loadtestutil"
	"github.com/coder/coder/scaletest/reconnectingpty"
	"github.com/coder/coder/scaletest/workspacebuild"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	userID               uuid.UUID
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
	r.client.Logger = logger
	r.client.LogBodies = true

	_, _ = fmt.Fprintln(logs, "Generating user password...")
	password, err := cryptorand.String(16)
	if err != nil {
		return xerrors.Errorf("generate random password for user: %w", err)
	}

	_, _ = fmt.Fprintln(logs, "Creating user:")
	_, _ = fmt.Fprintf(logs, "\tOrg ID:   %s\n", r.cfg.User.OrganizationID.String())
	_, _ = fmt.Fprintf(logs, "\tUsername: %s\n", r.cfg.User.Username)
	_, _ = fmt.Fprintf(logs, "\tEmail:    %s\n", r.cfg.User.Email)
	_, _ = fmt.Fprintf(logs, "\tPassword: ****************\n")
	user, err := r.client.CreateUser(ctx, codersdk.CreateUserRequest{
		OrganizationID: r.cfg.User.OrganizationID,
		Username:       r.cfg.User.Username,
		Email:          r.cfg.User.Email,
		Password:       password,
	})
	if err != nil {
		return xerrors.Errorf("create user: %w", err)
	}
	r.userID = user.ID

	_, _ = fmt.Fprintln(logs, "\nLogging in as new user...")
	userClient := codersdk.New(r.client.URL)
	loginRes, err := userClient.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    r.cfg.User.Email,
		Password: password,
	})
	if err != nil {
		return xerrors.Errorf("login as new user: %w", err)
	}
	userClient.SetSessionToken(loginRes.SessionToken)

	_, _ = fmt.Fprintln(logs, "\nCreating workspace...")
	workspaceBuildConfig := r.cfg.Workspace
	workspaceBuildConfig.OrganizationID = r.cfg.User.OrganizationID
	workspaceBuildConfig.UserID = user.ID.String()
	r.workspacebuildRunner = workspacebuild.NewRunner(userClient, workspaceBuildConfig)
	err = r.workspacebuildRunner.Run(ctx, id, logs)
	if err != nil {
		return xerrors.Errorf("create workspace: %w", err)
	}

	if r.cfg.Workspace.NoWaitForAgents {
		return nil
	}

	// Get the workspace.
	workspaceID, err := r.workspacebuildRunner.WorkspaceID()
	if err != nil {
		return xerrors.Errorf("get workspace ID: %w", err)
	}
	workspace, err := userClient.Workspace(ctx, workspaceID)
	if err != nil {
		return xerrors.Errorf("get workspace %q: %w", workspaceID.String(), err)
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
		return xerrors.Errorf("no agents found for workspace %q", workspaceID.String())
	}

	eg, egCtx := errgroup.WithContext(ctx)
	if r.cfg.ReconnectingPTY != nil {
		eg.Go(func() error {
			reconnectingPTYConfig := *r.cfg.ReconnectingPTY
			reconnectingPTYConfig.AgentID = agent.ID

			reconnectingPTYRunner := reconnectingpty.NewRunner(userClient, reconnectingPTYConfig)
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

			agentConnRunner := agentconn.NewRunner(userClient, agentConnConfig)
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
func (r *Runner) Cleanup(ctx context.Context, id string) error {
	if r.cfg.NoCleanup {
		return nil
	}

	if r.workspacebuildRunner != nil {
		err := r.workspacebuildRunner.Cleanup(ctx, id)
		if err != nil {
			return xerrors.Errorf("cleanup workspace: %w", err)
		}
	}

	if r.userID != uuid.Nil {
		err := r.client.DeleteUser(ctx, r.userID)
		if err != nil {
			return xerrors.Errorf("delete user: %w", err)
		}
	}

	return nil
}
