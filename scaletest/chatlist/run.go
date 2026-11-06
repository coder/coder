package chatlist

import (
	"context"
	"io"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

// Runner issues a fixed number of chat list requests, mirroring the agents
// page loading its first page of chats.
type Runner struct {
	client *codersdk.Client
	cfg    Config

	completed     int
	lastPageSize  int
	totalDuration time.Duration
	maxDuration   time.Duration
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Collectable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{client: client, cfg: cfg}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug).Named(id)
	r.client.SetLogger(logger)

	span.SetAttributes(
		attribute.Int("chat_list.list_requests", r.cfg.ListRequests),
		attribute.Int("chat_list.page_size", r.cfg.PageSize),
	)
	defer func() {
		span.SetAttributes(attribute.Int("chat_list.completed", r.completed))
	}()

	for i := 0; i < r.cfg.ListRequests; i++ {
		start := time.Now()
		chats, err := r.client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{Limit: r.cfg.PageSize},
		})
		elapsed := time.Since(start)
		if err != nil {
			r.cfg.Metrics.ListErrorsTotal.Inc()
			return xerrors.Errorf("list chats request %d: %w", i, err)
		}
		r.cfg.Metrics.ListLatencySeconds.Observe(elapsed.Seconds())
		r.completed++
		r.lastPageSize = len(chats)
		r.totalDuration += elapsed
		r.maxDuration = max(r.maxDuration, elapsed)
	}

	logger.Info(ctx, "chat list requests completed",
		slog.F("requests", r.completed),
		slog.F("chats_per_page", r.lastPageSize),
		slog.F("mean", r.totalDuration/time.Duration(r.completed)),
		slog.F("max", r.maxDuration),
	)
	return nil
}

func (r *Runner) GetMetrics() map[string]any {
	var mean time.Duration
	if r.completed > 0 {
		mean = r.totalDuration / time.Duration(r.completed)
	}
	return map[string]any{
		"list_requests":          r.cfg.ListRequests,
		"page_size":              r.cfg.PageSize,
		"completed":              r.completed,
		"chats_per_page":         r.lastPageSize,
		"mean_latency_seconds":   mean.Seconds(),
		"max_latency_seconds":    r.maxDuration.Seconds(),
		"total_duration_seconds": r.totalDuration.Seconds(),
	}
}
