package dashboard

import (
	"context"
	"errors"
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
	if cfg.WaitLoaded == nil {
		cfg.WaitLoaded = waitForWorkspacesPageLoaded
	}
	if cfg.ActionFunc == nil {
		cfg.ActionFunc = clickRandomElement
	}
	if cfg.Screenshot == nil {
		cfg.Screenshot = Screenshot
	}
	if cfg.RandIntn == nil {
		cfg.RandIntn = rand.Intn
	}
	return &Runner{
		client:  client,
		cfg:     cfg,
		metrics: metrics,
	}
}

func (r *Runner) Run(ctx context.Context, _ string, _ io.Writer) error {
	err := r.runUntilDeadlineExceeded(ctx)
	// If the context deadline exceeded, don't return an error.
	// This just means the test finished.
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (r *Runner) runUntilDeadlineExceeded(ctx context.Context) error {
	if r.client == nil {
		return xerrors.Errorf("client is nil")
	}
	me, err := r.client.User(ctx, codersdk.Me)
	if err != nil {
		return xerrors.Errorf("get scaletest user: %w", err)
	}
	//nolint:gocritic
	r.cfg.Logger.Info(ctx, "running as user", slog.F("username", me.Username))
	if len(me.OrganizationIDs) == 0 {
		return xerrors.Errorf("user has no organizations")
	}

	cdpCtx, cdpCancel, err := initChromeDPCtx(ctx, r.cfg.Logger, r.client.URL, r.client.SessionToken(), r.cfg.Headless)
	if err != nil {
		return xerrors.Errorf("init chromedp ctx: %w", err)
	}
	defer cdpCancel()
	t := time.NewTicker(1) // First one should be immediate
	defer t.Stop()
	r.cfg.Logger.Info(ctx, "waiting for workspaces page to load")
	loadWorkspacePageDeadline := time.Now().Add(r.cfg.Interval)
	if err := r.cfg.WaitLoaded(cdpCtx, loadWorkspacePageDeadline); err != nil {
		return xerrors.Errorf("wait for workspaces page to load: %w", err)
	}
	for {
		select {
		case <-cdpCtx.Done():
			return nil
		case <-t.C:
			var offset time.Duration
			if r.cfg.Jitter > 0 {
				offset = time.Duration(r.cfg.RandIntn(int(2*r.cfg.Jitter)) - int(r.cfg.Jitter))
			}
			wait := r.cfg.Interval + offset
			actionCompleteByDeadline := time.Now().Add(wait)
			t.Reset(wait)
			l, act, err := r.cfg.ActionFunc(cdpCtx, r.cfg.Logger, r.cfg.RandIntn, actionCompleteByDeadline)
			if err != nil {
				r.cfg.Logger.Error(ctx, "calling ActionFunc", slog.Error(err))
				sPath, sErr := r.cfg.Screenshot(cdpCtx, me.Username)
				if sErr != nil {
					r.cfg.Logger.Error(ctx, "screenshot failed", slog.Error(sErr))
				}
				r.cfg.Logger.Info(ctx, "screenshot saved", slog.F("path", sPath))
				continue
			}
			start := time.Now()
			err = act(cdpCtx)
			elapsed := time.Since(start)
			r.metrics.ObserveDuration(string(l), elapsed)
			if err != nil {
				r.metrics.IncErrors(string(l))
				//nolint:gocritic
				r.cfg.Logger.Error(ctx, "action failed", slog.F("label", l), slog.Error(err))
				sPath, sErr := r.cfg.Screenshot(cdpCtx, me.Username+"-"+string(l))
				if sErr != nil {
					r.cfg.Logger.Error(ctx, "screenshot failed", slog.Error(sErr))
				}
				r.cfg.Logger.Info(ctx, "screenshot saved", slog.F("path", sPath))
			} else {
				//nolint:gocritic
				r.cfg.Logger.Info(ctx, "action success", slog.F("label", l))
			}
		}
	}
}

func (*Runner) Cleanup(_ context.Context, _ string) error {
	return nil
}
