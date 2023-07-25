package dashboard

import (
	"context"
	"io"
	"math/rand"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/scaletest/harness"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	client.Trace = cfg.Trace
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

func (r *Runner) Run(ctx context.Context, _ string, _ io.Writer) error {
	_, err := r.client.User(ctx, codersdk.Me)
	if err != nil {
		return err
	}
	go r.do(ctx, "fetch workspaces", func(client *codersdk.Client) error {
		_, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
		return err
	})
	go r.do(ctx, "fetch users", func(client *codersdk.Client) error {
		_, err := client.Users(ctx, codersdk.UsersRequest{})
		return err
	})
	go r.do(ctx, "fetch templates", func(client *codersdk.Client) error {
		me, err := client.User(ctx, codersdk.Me)
		if err != nil {
			return err
		}
		_, err = client.TemplatesByOrganization(ctx, me.OrganizationIDs[0])
		return err
	})
	<-ctx.Done()
	return nil
}

func (*Runner) Cleanup(_ context.Context, _ string) error {
	return nil
}

func (r *Runner) do(ctx context.Context, label string, fn func(client *codersdk.Client) error) {
	t := time.NewTicker(r.randWait())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			r.cfg.Logger.Info(ctx, "context done, stopping")
			return
		case <-t.C:
			start := time.Now()
			err := fn(r.client)
			elapsed := time.Since(start)
			if err != nil {
				r.cfg.Logger.Error(
					ctx, "function returned error",
					slog.Error(err),
					slog.F("fn", label),
					slog.F("elapsed", elapsed),
				)
			} else {
				r.cfg.Logger.Info(ctx, "completed successfully",
					slog.F("fn", label),
					slog.F("elapsed", elapsed),
				)
			}
			t.Reset(r.randWait())
		}
	}
}

func (r *Runner) randWait() time.Duration {
	// nolint:gosec // This is not for cryptographic purposes. Chill, gosec. Chill.
	wait := time.Duration(rand.Intn(int(r.cfg.MaxWait) - int(r.cfg.MinWait)))
	return r.cfg.MinWait + wait
}
