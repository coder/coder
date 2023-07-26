package dashboard

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/scaletest/harness"
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
	me, err := r.client.User(ctx, codersdk.Me)
	if err != nil {
		return err
	}
	if len(me.OrganizationIDs) == 0 {
		return xerrors.Errorf("user has no organizations")
	}

	c := &cache{}
	if err := c.fill(ctx, r.client); err != nil {
		return err
	}

	p := &Params{
		client: r.client,
		me:     me,
		c:      c,
	}
	rolls := make(chan int)
	go func() {
		t := time.NewTicker(r.randWait())
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rolls <- rand.Intn(r.cfg.RollTable.max() + 1) // nolint:gosec
				t.Reset(r.randWait())
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case n := <-rolls:
			act := r.cfg.RollTable.choose(n)
			go r.do(ctx, act, p)
		}
	}
}

func (*Runner) Cleanup(_ context.Context, _ string) error {
	return nil
}

func (r *Runner) do(ctx context.Context, act RollTableEntry, p *Params) {
	select {
	case <-ctx.Done():
		r.cfg.Logger.Info(ctx, "context done, stopping")
		return
	default:
		var errored bool
		cancelCtx, cancel := context.WithTimeout(ctx, r.cfg.MaxWait)
		defer cancel()
		start := time.Now()
		err := act.Fn(cancelCtx, p)
		cancel()
		elapsed := time.Since(start)
		if err != nil {
			errored = true
			r.cfg.Logger.Error( //nolint:gocritic
				ctx, "action failed",
				slog.Error(err),
				slog.F("action", act.Label),
				slog.F("elapsed", elapsed),
			)
		} else {
			r.cfg.Logger.Info(ctx, "completed successfully",
				slog.F("action", act.Label),
				slog.F("elapsed", elapsed),
			)
		}
		codeLabel := "200"
		if apiErr, ok := codersdk.AsError(err); ok {
			codeLabel = fmt.Sprintf("%d", apiErr.StatusCode())
		} else if xerrors.Is(err, context.Canceled) {
			codeLabel = "timeout"
		}
		r.metrics.ObserveDuration(act.Label, elapsed)
		r.metrics.IncStatuses(act.Label, codeLabel)
		if errored {
			r.metrics.IncErrors(act.Label)
		}
	}
}

func (r *Runner) randWait() time.Duration {
	// nolint:gosec // This is not for cryptographic purposes. Chill, gosec. Chill.
	wait := time.Duration(rand.Intn(int(r.cfg.MaxWait) - int(r.cfg.MinWait)))
	return r.cfg.MinWait + wait
}
