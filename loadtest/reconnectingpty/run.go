package reconnectingpty

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/harness"
	"github.com/coder/coder/loadtest/loadtestutil"
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
	r.client.Logger = logger
	r.client.LogBodies = true

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

	conn, err := r.client.WorkspaceAgentReconnectingPTY(ctx, r.cfg.AgentID, id, width, height, r.cfg.Init.Command)
	if err != nil {
		return xerrors.Errorf("open reconnecting PTY: %w", err)
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
	matched, err := copyContext(copyCtx, copyOutput, conn, r.cfg.ExpectOutput)
	copyCancel()
	if r.cfg.ExpectTimeout {
		if err == nil {
			return xerrors.Errorf("expected timeout, but the command exited successfully")
		}
		if !xerrors.Is(err, context.DeadlineExceeded) {
			return xerrors.Errorf("expected timeout, but got a different error: %w", err)
		}
	} else if err != nil {
		return xerrors.Errorf("copy context: %w", err)
	}
	if !matched {
		return xerrors.Errorf("expected string %q not found in output", r.cfg.ExpectOutput)
	}

	return nil
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader, expectOutput string) (bool, error) {
	var (
		copyErr = make(chan error)
		matched = expectOutput == ""
	)
	go func() {
		defer close(copyErr)

		scanner := bufio.NewScanner(src)
		for scanner.Scan() {
			if expectOutput != "" && strings.Contains(scanner.Text(), expectOutput) {
				matched = true
			}

			_, err := dst.Write([]byte("\t" + scanner.Text() + "\n"))
			if err != nil {
				copyErr <- xerrors.Errorf("write to logs: %w", err)
				return
			}
		}
		if scanner.Err() != nil {
			copyErr <- xerrors.Errorf("read from reconnecting PTY: %w", scanner.Err())
			return
		}
	}()

	select {
	case <-ctx.Done():
		return matched, ctx.Err()
	case err := <-copyErr:
		return matched, err
	}
}
