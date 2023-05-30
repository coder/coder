package workspacetraffic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/scaletest/harness"
	"github.com/coder/coder/scaletest/loadtestutil"

	promtest "github.com/prometheus/client_golang/prometheus/testutil"
)

type Runner struct {
	client  *codersdk.Client
	cfg     Config
	metrics *Metrics
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config, metrics *Metrics) *Runner {
	return &Runner{
		client:  client,
		cfg:     cfg,
		metrics: metrics,
	}
}

func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.Logger = logger
	r.client.LogBodies = true

	// Initialize our metrics eagerly. This is mainly so that we can test for the
	// presence of a zero-valued metric as opposed to the absence of a metric.
	lvs := []string{r.cfg.WorkspaceOwner, r.cfg.WorkspaceName, r.cfg.AgentName}
	r.metrics.BytesReadTotal.WithLabelValues(lvs...).Add(0)
	r.metrics.BytesWrittenTotal.WithLabelValues(lvs...).Add(0)
	r.metrics.ReadErrorsTotal.WithLabelValues(lvs...).Add(0)
	r.metrics.WriteErrorsTotal.WithLabelValues(lvs...).Add(0)
	r.metrics.ReadLatencySeconds.WithLabelValues(lvs...).Observe(0)
	r.metrics.WriteLatencySeconds.WithLabelValues(lvs...).Observe(0)

	var (
		agentID             = r.cfg.AgentID
		reconnect           = uuid.New()
		height       uint16 = 25
		width        uint16 = 80
		tickInterval        = r.cfg.TickInterval
		bytesPerTick        = r.cfg.BytesPerTick
	)

	logger.Info(ctx, "config",
		slog.F("agent_id", agentID),
		slog.F("reconnect", reconnect),
		slog.F("height", height),
		slog.F("width", width),
		slog.F("tick_interval", tickInterval),
		slog.F("bytes_per_tick", bytesPerTick),
	)

	// Set a deadline for stopping the text.
	start := time.Now()
	deadlineCtx, cancel := context.WithDeadline(ctx, start.Add(r.cfg.Duration))
	defer cancel()
	logger.Debug(ctx, "connect to workspace agent", slog.F("agent_id", agentID))

	conn, err := r.client.WorkspaceAgentReconnectingPTY(ctx, codersdk.WorkspaceAgentReconnectingPTYOpts{
		AgentID:   agentID,
		Reconnect: reconnect,
		Height:    height,
		Width:     width,
		Command:   "/bin/sh",
	})
	if err != nil {
		logger.Error(ctx, "connect to workspace agent", slog.F("agent_id", agentID), slog.Error(err))
		return xerrors.Errorf("connect to workspace: %w", err)
	}

	go func() {
		<-deadlineCtx.Done()
		logger.Debug(ctx, "close agent connection", slog.F("agent_id", agentID))
		_ = conn.Close()
	}()

	// Wrap the conn in a countReadWriter so we can monitor bytes sent/rcvd.
	crw := countReadWriter{ReadWriter: conn, metrics: r.metrics, labels: lvs}

	// Create a ticker for sending data to the PTY.
	tick := time.NewTicker(tickInterval)
	defer tick.Stop()

	// Now we begin writing random data to the pty.
	rch := make(chan error, 1)
	wch := make(chan error, 1)

	go func() {
		<-deadlineCtx.Done()
		logger.Debug(ctx, "closing agent connection")
		conn.Close()
	}()

	// Read forever in the background.
	go func() {
		logger.Debug(ctx, "reading from agent", slog.F("agent_id", agentID))
		rch <- drain(&crw)
		logger.Debug(ctx, "done reading from agent", slog.F("agent_id", agentID))
		close(rch)
	}()

	// Write random data to the PTY every tick.
	go func() {
		logger.Debug(ctx, "writing to agent", slog.F("agent_id", agentID))
		wch <- writeRandomData(&crw, bytesPerTick, tick.C)
		logger.Debug(ctx, "done writing to agent", slog.F("agent_id", agentID))
		close(wch)
	}()

	// Write until the context is canceled.
	if wErr := <-wch; wErr != nil {
		return xerrors.Errorf("write to pty: %w", wErr)
	}
	if rErr := <-rch; rErr != nil {
		return xerrors.Errorf("read from pty: %w", rErr)
	}

	duration := time.Since(start)
	logger.Info(ctx, "Test Results",
		slog.F("duration", duration),
		slog.F("bytes_read_total", promtest.ToFloat64(r.metrics.BytesReadTotal)),
		slog.F("bytes_written_total", promtest.ToFloat64(r.metrics.BytesWrittenTotal)),
		slog.F("read_errors_total", promtest.ToFloat64(r.metrics.ReadErrorsTotal)),
		slog.F("write_errors_total", promtest.ToFloat64(r.metrics.WriteErrorsTotal)),
	)

	return nil
}

// Cleanup does nothing, successfully.
func (*Runner) Cleanup(context.Context, string) error {
	return nil
}

// drain drains from src until it returns io.EOF or ctx times out.
func drain(src io.Reader) error {
	if _, err := io.Copy(io.Discard, src); err != nil {
		if xerrors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		if xerrors.As(err, &websocket.CloseError{}) {
			return nil
		}
		return err
	}
	return nil
}

func writeRandomData(dst io.Writer, size int64, tick <-chan time.Time) error {
	var (
		enc    = json.NewEncoder(dst)
		ptyReq = codersdk.ReconnectingPTYRequest{}
	)
	for range tick {
		payload := "#" + mustRandStr(size-1)
		ptyReq.Data = payload
		if err := enc.Encode(ptyReq); err != nil {
			if xerrors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			if xerrors.As(err, &websocket.CloseError{}) {
				return nil
			}
			return err
		}
	}
	return nil
}

// countReadWriter wraps an io.ReadWriter and counts the number of bytes read and written.
type countReadWriter struct {
	io.ReadWriter
	metrics *Metrics
	labels  []string
}

func (w *countReadWriter) Read(p []byte) (int, error) {
	start := time.Now()
	n, err := w.ReadWriter.Read(p)
	if reportableErr(err) {
		w.metrics.ReadErrorsTotal.WithLabelValues(w.labels...).Inc()
	}
	w.metrics.ReadLatencySeconds.WithLabelValues(w.labels...).Observe(time.Since(start).Seconds())
	if n > 0 {
		w.metrics.BytesReadTotal.WithLabelValues(w.labels...).Add(float64(n))
	}
	return n, err
}

func (w *countReadWriter) Write(p []byte) (int, error) {
	start := time.Now()
	n, err := w.ReadWriter.Write(p)
	if reportableErr(err) {
		w.metrics.WriteErrorsTotal.WithLabelValues(w.labels...).Inc()
	}
	w.metrics.WriteLatencySeconds.WithLabelValues(w.labels...).Observe(time.Since(start).Seconds())
	if n > 0 {
		w.metrics.BytesWrittenTotal.WithLabelValues(w.labels...).Add(float64(n))
	}
	return n, err
}

func mustRandStr(l int64) string {
	if l < 1 {
		l = 1
	}
	randStr, err := cryptorand.String(int(l))
	if err != nil {
		panic(err)
	}
	return randStr
}

// some errors we want to report in metrics; others we want to ignore
// such as websocket.StatusNormalClosure or context.Canceled
func reportableErr(err error) bool {
	if err == nil {
		return false
	}
	if xerrors.Is(err, context.Canceled) {
		return false
	}
	var wsErr websocket.CloseError
	if errors.As(err, &wsErr) {
		return wsErr.Code != websocket.StatusNormalClosure
	}
	return false
}
