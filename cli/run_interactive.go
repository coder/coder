package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/muesli/cancelreader"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func workloadTerminalSize(inv *serpent.Invocation) (uint16, uint16, error) {
	stdin, inOK := inv.Stdin.(*os.File)
	stdout, outOK := inv.Stdout.(*os.File)
	if !inOK || !outOK || !term.IsTerminal(int(stdin.Fd())) || !term.IsTerminal(int(stdout.Fd())) {
		return 0, 0, xerrors.New("--tty requires a local input and output terminal; omit --tty for piped input")
	}
	width, height, err := term.GetSize(int(stdout.Fd()))
	if err != nil {
		return 0, 0, xerrors.Errorf("read terminal dimensions: %w", err)
	}
	if width <= 0 || height <= 0 || width > 65535 || height > 65535 {
		return 0, 0, xerrors.New("terminal dimensions must be between 1 and 65535")
	}
	return uint16(width), uint16(height), nil
}

func watchInteractiveWorkload(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, org uuid.UUID, req codersdk.CreateWorkloadRequest, clock quartz.Clock, timings *workloadRunTimings) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	conn, err := client.DialWorkloadInput(ctx, org, req.ID)
	if err != nil {
		// A command can exit between admission and input attachment. Drain its
		// output instead of turning successful completion into an input error.
		checkCtx, stop := context.WithTimeout(ctx, workloadRequestTimeout)
		output, checkErr := client.WorkloadOutput(checkCtx, org, req.ID, 0)
		stop()
		if checkErr == nil && output.Workload.Terminal() {
			return watchWorkload(ctx, inv, client, org, req.ID, clock, timings)
		}
		return xerrors.Errorf("attach workload input (the workload has not been canceled): %w", err)
	}
	defer conn.CloseNow()

	if req.TTY {
		stdin := inv.Stdin.(*os.File)
		stdout := inv.Stdout.(*os.File)
		inState, err := pty.MakeInputRaw(stdin.Fd())
		if err != nil {
			return xerrors.Errorf("set input terminal mode: %w", err)
		}
		defer func() { _ = pty.RestoreTerminal(stdin.Fd(), inState) }()
		outState, err := pty.MakeOutputRaw(stdout.Fd())
		if err != nil {
			return xerrors.Errorf("set output terminal mode: %w", err)
		}
		defer func() { _ = pty.RestoreTerminal(stdout.Fd(), outState) }()
	}

	reader, err := cancelreader.NewReader(inv.Stdin)
	if err != nil {
		return xerrors.Errorf("read workload stdin: %w", err)
	}
	inputDone := make(chan struct{})
	defer func() {
		cancel(nil)
		if reader.Cancel() {
			<-inputDone
		}
		_ = reader.Close()
	}()

	send := func(message codersdk.WorkloadInput) error {
		writeCtx, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		return wsjson.Write(writeCtx, conn, message)
	}
	go func() {
		defer close(inputDone)
		buffer := make([]byte, 16<<10)
		for {
			n, readErr := reader.Read(buffer)
			if n > 0 {
				if err := send(codersdk.WorkloadInput{Type: "data", Data: buffer[:n]}); err != nil {
					cancel(xerrors.Errorf("write workload stdin; input was not replayed: %w", err))
					return
				}
			}
			if readErr == io.EOF {
				if err := send(codersdk.WorkloadInput{Type: "eof"}); err != nil {
					cancel(xerrors.Errorf("close workload stdin: %w", err))
				}
				return
			}
			if readErr != nil {
				if !errors.Is(readErr, cancelreader.ErrCanceled) {
					cancel(xerrors.Errorf("read workload stdin: %w", readErr))
				}
				return
			}
		}
	}()

	// Read concurrently to handle WebSocket control frames and detect loss of
	// the input channel. Replaying uncertain keystrokes is not safe.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				if ctx.Err() == nil && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					cancel(xerrors.New("workload input connection closed; the workload was not canceled and input was not replayed"))
				}
				return
			}
		}
	}()
	defer func() {
		cancel(nil)
		_ = conn.CloseNow()
		<-readDone
	}()

	if req.TTY {
		windowChanges := listenWindowSize(ctx)
		resizeDone := make(chan struct{})
		go func() {
			defer close(resizeDone)
			for {
				select {
				case <-ctx.Done():
					return
				case <-windowChanges:
					width, height, err := workloadTerminalSize(inv)
					if err != nil {
						continue
					}
					if err := send(codersdk.WorkloadInput{Type: "resize", Width: width, Height: height}); err != nil {
						cancel(xerrors.Errorf("resize workload terminal: %w", err))
						return
					}
				}
			}
		}()
		defer func() {
			cancel(nil)
			<-resizeDone
		}()
	}

	err = watchWorkload(ctx, inv, client, org, req.ID, clock, timings)
	if cause := context.Cause(ctx); errors.Is(err, context.Canceled) && cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	return err
}
