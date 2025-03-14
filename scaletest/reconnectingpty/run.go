package reconnectingpty
import (
	"errors"
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"github.com/google/uuid"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)
type Runner struct {
	client *codersdk.Client
	cfg    Config
}
var _ harness.Runnable = &Runner{}
func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}
// Run implements Runnable.
func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)
	var (
		id     = r.cfg.Init.ID
		width  = r.cfg.Init.Width
		height = r.cfg.Init.Height
	)
	if id == uuid.Nil {
		id = uuid.New()
	}
	if width == 0 {
		width = DefaultWidth
	}
	if height == 0 {
		height = DefaultHeight
	}
	_, _ = fmt.Fprintln(logs, "Opening reconnecting PTY connection to agent via coderd...")
	_, _ = fmt.Fprintf(logs, "\tID:      %s\n", id.String())
	_, _ = fmt.Fprintf(logs, "\tWidth:   %d\n", width)
	_, _ = fmt.Fprintf(logs, "\tHeight:  %d\n", height)
	_, _ = fmt.Fprintf(logs, "\tCommand: %q\n\n", r.cfg.Init.Command)
	conn, err := workspacesdk.New(r.client).AgentReconnectingPTY(ctx, workspacesdk.WorkspaceAgentReconnectingPTYOpts{
		AgentID:   r.cfg.AgentID,
		Reconnect: id,
		Width:     width,
		Height:    height,
		Command:   r.cfg.Init.Command,
	})
	if err != nil {
		return fmt.Errorf("open reconnecting PTY: %w", err)
	}
	defer conn.Close()
	var (
		copyTimeout = r.cfg.Timeout
		copyOutput  = io.Discard
	)
	if copyTimeout == 0 {
		copyTimeout = DefaultTimeout
	}
	if r.cfg.LogOutput {
		_, _ = fmt.Fprintln(logs, "Output:")
		copyOutput = logs
	}
	copyCtx, copyCancel := context.WithTimeout(ctx, time.Duration(copyTimeout))
	defer copyCancel()
	matched, err := copyContext(copyCtx, copyOutput, conn, r.cfg.ExpectOutput)
	if r.cfg.ExpectTimeout {
		if err == nil {
			return fmt.Errorf("expected timeout, but the command exited successfully")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("expected timeout, but got a different error: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("copy context: %w", err)
	}
	if !matched {
		return fmt.Errorf("expected string %q not found in output", r.cfg.ExpectOutput)
	}
	return nil
}
func copyContext(ctx context.Context, dst io.Writer, src io.Reader, expectOutput string) (bool, error) {
	var (
		copyErr = make(chan error)
		matched = expectOutput == ""
	)
	// Guard goroutine for loop body to ensure reading `matched` is safe on
	// context cancellation and that `dst` won't be written to after we
	// return from this function.
	processing := make(chan struct{}, 1)
	processing <- struct{}{}
	go func() {
		defer close(processing)
		defer close(copyErr)
		scanner := bufio.NewScanner(src)
		for scanner.Scan() {
			select {
			case <-processing:
			default:
			}
			if ctx.Err() != nil {
				return
			}
			if expectOutput != "" && strings.Contains(scanner.Text(), expectOutput) {
				matched = true
			}
			_, err := dst.Write([]byte("\t" + scanner.Text() + "\n"))
			if err != nil {
				copyErr <- fmt.Errorf("write to logs: %w", err)
				return
			}
			processing <- struct{}{}
		}
		if scanner.Err() != nil && !errors.Is(scanner.Err(), io.EOF) {
			copyErr <- fmt.Errorf("read from reconnecting PTY: %w", scanner.Err())
			return
		}
	}()
	select {
	case <-ctx.Done():
		select {
		case <-processing:
		case <-copyErr:
		}
		return matched, ctx.Err()
	case err := <-copyErr:
		return matched, err
	}
}
