package workspacetraffic

import (
	"context"
	"encoding/json"
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
)

type Runner struct {
	client *codersdk.Client
	cfg    Config
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

// func NewRunner(client *codersdk.Client, cfg Config, metrics *Metrics) *Runner {
func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	// Initialize our metrics eagerly. This is mainly so that we can test for the
	// presence of a zero-valued metric as opposed to the absence of a metric.
	r.cfg.ReadMetrics.AddError(0)
	r.cfg.ReadMetrics.AddTotal(0)
	r.cfg.ReadMetrics.ObserveLatency(0)
	r.cfg.WriteMetrics.AddError(0)
	r.cfg.WriteMetrics.AddTotal(0)
	r.cfg.WriteMetrics.ObserveLatency(0)

	var (
		agentID             = r.cfg.AgentID
		reconnect           = uuid.New()
		height       uint16 = 25
		width        uint16 = 80
		tickInterval        = r.cfg.TickInterval
		bytesPerTick        = r.cfg.BytesPerTick
	)

	logger.Debug(ctx, "config",
		slog.F("agent_id", agentID),
		slog.F("reconnecting_pty_id", reconnect),
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

	conn, err := connectPTY(ctx, r.client, agentID, reconnect)
	if err != nil {
		logger.Error(ctx, "connect to workspace agent", slog.F("agent_id", agentID), slog.Error(err))
		return xerrors.Errorf("connect to workspace: %w", err)
	}

	conn.readMetrics = r.cfg.ReadMetrics
	conn.writeMetrics = r.cfg.WriteMetrics

	go func() {
		<-deadlineCtx.Done()
		logger.Debug(ctx, "close agent connection", slog.F("agent_id", agentID))
		_ = conn.Close()
	}()

	// Create a ticker for sending data to the PTY.
	tick := time.NewTicker(tickInterval)
	defer tick.Stop()

	// Now we begin writing random data to the pty.
	rch := make(chan error, 1)
	wch := make(chan error, 1)

	go func() {
		<-deadlineCtx.Done()
		logger.Debug(ctx, "closing agent connection")
		_ = conn.Close()
	}()

	// Read forever in the background.
	go func() {
		logger.Debug(ctx, "reading from agent", slog.F("agent_id", agentID))
		rch <- drain(conn)
		logger.Debug(ctx, "done reading from agent", slog.F("agent_id", agentID))
		close(rch)
	}()

	// Write random data to the PTY every tick.
	go func() {
		logger.Debug(ctx, "writing to agent", slog.F("agent_id", agentID))
		wch <- writeRandomData(conn, bytesPerTick, tick.C)
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
