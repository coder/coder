package bridge

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/quartz"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner *createusers.Runner

	clock quartz.Clock
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
		clock:  quartz.NewReal(),
	}
}

func (r *Runner) WithClock(clock quartz.Clock) *Runner {
	r.clock = clock
	return r
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	r.createUserRunner = createusers.NewRunner(r.client, r.cfg.User)
	newUserAndToken, err := r.createUserRunner.RunReturningUser(ctx, id, logs)
	if err != nil {
		r.cfg.Metrics.AddError("create_user")
		return xerrors.Errorf("create user: %w", err)
	}
	newUser := newUserAndToken.User
	newUserClient := codersdk.New(r.client.URL,
		codersdk.WithSessionToken(newUserAndToken.SessionToken),
		codersdk.WithLogger(logger),
		codersdk.WithLogBodies())

	logger.Info(ctx, "runner user created", slog.F("username", newUser.Username), slog.F("user_id", newUser.ID.String()))

	logger.Info(ctx, "bridge runner is ready")

	_ = newUserClient
	// TODO: Implement bridge load generation logic here

	return nil
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.createUserRunner != nil {
		_, _ = fmt.Fprintln(logs, "Cleaning up user...")
		if err := r.createUserRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup user: %w", err)
		}
	}

	return nil
}

func (r *Runner) GetMetrics() map[string]any {
	// TODO: Return actual metrics when bridge load generation is implemented
	return map[string]any{}
}
