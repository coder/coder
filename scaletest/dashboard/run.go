package dashboard

import (
	"context"
	"io"
	"math/rand"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
)

type Runner struct {
	client  *codersdk.Client
	cfg     Config
	metrics Metrics
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

func NewRunner(client *codersdk.Client, metrics Metrics, cfg Config) *Runner {
	client.Trace = cfg.Trace
	return &Runner{
		client:  client,
		cfg:     cfg,
		metrics: metrics,
	}
}

func (r *Runner) Run(ctx context.Context, _ string, _ io.Writer) error {
	if r.client == nil {
		return xerrors.Errorf("client is nil")
	}
	me, err := r.client.User(ctx, codersdk.Me)
	if err != nil {
		return xerrors.Errorf("get scaletest user: %w", err)
	}
	r.cfg.Logger.Info(ctx, "running as user", slog.F("username", me.Username))
	if len(me.OrganizationIDs) == 0 {
		return xerrors.Errorf("user has no organizations")
	}

	cdpCtx, cdpCancel, err := initChromeDPCtx(ctx, r.client.URL, r.client.SessionToken(), r.cfg.Headless)
	if err != nil {
		return xerrors.Errorf("init chromedp ctx: %w", err)
	}
	defer cdpCancel()
	t := time.NewTicker(1) // First one should be immediate
	defer t.Stop()
	for {
		select {
		case <-cdpCtx.Done():
			return nil
		case <-t.C:
			t.Reset(r.randWait())
			l, act, err := r.cfg.ActionFunc(cdpCtx)
			if err != nil {
				r.cfg.Logger.Error(ctx, "calling ActionFunc", slog.Error(err))
				continue
			}
			start := time.Now()
			err = act(cdpCtx)
			elapsed := time.Since(start)
			r.metrics.ObserveDuration(string(l), elapsed)
			if err != nil {
				r.metrics.IncErrors(string(l))
				r.cfg.Logger.Error(ctx, "action failed", slog.F("label", l), slog.Error(err))
			} else {
				r.cfg.Logger.Info(ctx, "action success", slog.F("label", l))
			}
		}
	}
}

func (*Runner) Cleanup(_ context.Context, _ string) error {
	return nil
}

func (r *Runner) randWait() time.Duration {
	// nolint:gosec // This is not for cryptographic purposes. Chill, gosec. Chill.
	var wait time.Duration
	if r.cfg.MaxWait > r.cfg.MinWait {
		wait = time.Duration(rand.Intn(int(r.cfg.MaxWait) - int(r.cfg.MinWait)))
	}

	return r.cfg.MinWait + wait
}
