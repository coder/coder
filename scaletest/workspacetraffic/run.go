package workspacetraffic
import (
	"errors"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"
	"github.com/google/uuid"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/websocket"
)
type Runner struct {
	client    *codersdk.Client
	webClient *codersdk.Client
	cfg       Config
}
var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)
// func NewRunner(client *codersdk.Client, cfg Config, metrics *Metrics) *Runner {
func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	webClient := client
	if cfg.WebClient != nil {
		webClient = cfg.WebClient
	}
	return &Runner{
		client:    client,
		webClient: webClient,
		cfg:       cfg,
	}
}
func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) (err error) {
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
		echo                = r.cfg.Echo
	)
	logger = logger.With(slog.F("agent_id", agentID))
	logger.Debug(ctx, "config",
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
	logger.Debug(ctx, "connect to workspace agent")
	output := "/dev/stdout"
	if !echo {
		output = "/dev/null"
	}
	command := fmt.Sprintf("dd if=/dev/stdin of=%s bs=%d status=none", output, bytesPerTick)
	var conn *countReadWriteCloser
	switch {
	case r.cfg.App.Name != "":
		logger.Info(ctx, "sending traffic to workspace app", slog.F("app", r.cfg.App.Name))
		conn, err = appClientConn(ctx, r.webClient, r.cfg.App.URL)
		if err != nil {
			logger.Error(ctx, "connect to workspace app", slog.Error(err))
			return fmt.Errorf("connect to workspace app: %w", err)
		}
	case r.cfg.SSH:
		logger.Info(ctx, "connecting to workspace agent", slog.F("method", "ssh"))
		// If echo is enabled, disable PTY to avoid double echo and
		// reduce CPU usage.
		requestPTY := !r.cfg.Echo
		conn, err = connectSSH(ctx, r.client, agentID, command, requestPTY)
		if err != nil {
			logger.Error(ctx, "connect to workspace agent via ssh", slog.Error(err))
			return fmt.Errorf("connect to workspace via ssh: %w", err)
		}
	default:
		logger.Info(ctx, "connecting to workspace agent", slog.F("method", "reconnectingpty"))
		conn, err = connectRPTY(ctx, r.webClient, agentID, reconnect, command)
		if err != nil {
			logger.Error(ctx, "connect to workspace agent via reconnectingpty", slog.Error(err))
			return fmt.Errorf("connect to workspace via reconnectingpty: %w", err)
		}
	}
	var closeErr error
	closeOnce := sync.Once{}
	closeConn := func() error {
		closeOnce.Do(func() {
			closeErr = conn.Close()
			if closeErr != nil {
				logger.Error(ctx, "close agent connection", slog.Error(closeErr))
			}
		})
		return closeErr
	}
	defer func() {
		if err2 := closeConn(); err2 != nil {
			// Allow close error to fail the test.
			if err == nil {
				err = err2
			}
		}
	}()
	conn.readMetrics = r.cfg.ReadMetrics
	conn.writeMetrics = r.cfg.WriteMetrics
	// Create a ticker for sending data to the conn.
	tick := time.NewTicker(tickInterval)
	defer tick.Stop()
	// Now we begin writing random data to the conn.
	rch := make(chan error, 1)
	wch := make(chan error, 1)
	// Read until connection is closed.
	go func() {
		logger.Debug(ctx, "reading from agent")
		rch <- drain(conn)
		logger.Debug(ctx, "done reading from agent")
		close(rch)
	}()
	// Write random data to the conn every tick.
	go func() {
		logger.Debug(ctx, "writing to agent")
		wch <- writeRandomData(conn, bytesPerTick, tick.C)
		logger.Debug(ctx, "done writing to agent")
		close(wch)
	}()
	var waitCloseTimeoutCh <-chan struct{}
	deadlineCtxCh := deadlineCtx.Done()
	wchRef, rchRef := wch, rch
	for {
		if wchRef == nil && rchRef == nil {
			return nil
		}
		select {
		case <-waitCloseTimeoutCh:
			logger.Warn(ctx, "timed out waiting for read/write to complete",
				slog.F("write_done", wchRef == nil),
				slog.F("read_done", rchRef == nil),
			)
			return fmt.Errorf("timed out waiting for read/write to complete: %w", ctx.Err())
		case <-deadlineCtxCh:
			go func() {
				_ = closeConn()
			}()
			deadlineCtxCh = nil // Only trigger once.
			// Wait at most closeTimeout for the connection to close cleanly.
			waitCtx, cancel := context.WithTimeout(context.Background(), waitCloseTimeout)
			defer cancel() //nolint:revive // Only called once.
			waitCloseTimeoutCh = waitCtx.Done()
		case err = <-wchRef:
			if err != nil {
				return fmt.Errorf("write to agent: %w", err)
			}
			wchRef = nil
		case err = <-rchRef:
			if err != nil {
				return fmt.Errorf("read from agent: %w", err)
			}
			rchRef = nil
		}
	}
}
// Cleanup does nothing, successfully.
func (*Runner) Cleanup(context.Context, string, io.Writer) error {
	return nil
}
// drain drains from src until it returns io.EOF or ctx times out.
func drain(src io.Reader) error {
	if _, err := io.Copy(io.Discard, src); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, io.ErrClosedPipe) {
			return nil
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		if errors.As(err, &websocket.CloseError{}) {
			return nil
		}
		return err
	}
	return nil
}
// Allowed characters for random strings, exclude most of the 0x00 - 0x1F range.
var allowedChars = []byte("\t !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}")
func writeRandomData(dst io.Writer, size int64, tick <-chan time.Time) error {
	var b bytes.Buffer
	p := make([]byte, size-1)
	for range tick {
		b.Reset()
		p := mustRandom(p)
		for _, c := range p {
			_, _ = b.WriteRune(rune(allowedChars[c%byte(len(allowedChars))]))
		}
		_, _ = b.WriteString("\n")
		if _, err := b.WriteTo(dst); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, io.ErrClosedPipe) {
				return nil
			}
			if errors.Is(err, context.Canceled) {
				return nil
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			if errors.As(err, &websocket.CloseError{}) {
				return nil
			}
			return err
		}
	}
	return nil
}
// mustRandom writes pseudo random bytes to p and panics if it fails.
func mustRandom(p []byte) []byte {
	n, err := rand.Read(p) //nolint:gosec // We want pseudorandomness here to avoid entropy issues.
	if err != nil {
		panic(err)
	}
	return p[:n]
}
