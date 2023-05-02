package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

type trafficGenOutput struct {
	DurationSeconds float64 `json:"duration_s"`
	SentBytes       int64   `json:"sent_bytes"`
	RcvdBytes       int64   `json:"rcvd_bytes"`
}

func (o trafficGenOutput) String() string {
	return fmt.Sprintf("Duration: %.2fs\n", o.DurationSeconds) +
		fmt.Sprintf("Sent:     %dB\n", o.SentBytes) +
		fmt.Sprintf("Rcvd:     %dB", o.RcvdBytes)
}

func (r *RootCmd) trafficGen() *clibase.Cmd {
	var (
		duration  time.Duration
		formatter = cliui.NewOutputFormatter(
			cliui.TextFormat(),
			cliui.JSONFormat(),
		)
		bps    int64
		client = new(codersdk.Client)
	)

	cmd := &clibase.Cmd{
		Use:    "trafficgen",
		Hidden: true,
		Short:  "Generate traffic to a Coder workspace",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(1, 2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			var (
				agentName    string
				tickInterval = 100 * time.Millisecond
			)
			ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			var agentID uuid.UUID
			for _, res := range ws.LatestBuild.Resources {
				if len(res.Agents) == 0 {
					continue
				}
				if agentName != "" && agentName != res.Agents[0].Name {
					continue
				}
				agentID = res.Agents[0].ID
			}

			if agentID == uuid.Nil {
				return xerrors.Errorf("no agent found for workspace %s", ws.Name)
			}

			// Setup our workspace agent connection.
			reconnect := uuid.New()
			conn, err := client.WorkspaceAgentReconnectingPTY(inv.Context(), codersdk.WorkspaceAgentReconnectingPTYOpts{
				AgentID:   agentID,
				Reconnect: reconnect,
				Height:    65535,
				Width:     65535,
				Command:   "/bin/sh",
			})
			if err != nil {
				return xerrors.Errorf("connect to workspace: %w", err)
			}

			defer func() {
				_ = conn.Close()
			}()

			// Wrap the conn in a countReadWriter so we can monitor bytes sent/rcvd.
			crw := countReadWriter{ReadWriter: conn}

			// Set a deadline for stopping the text.
			start := time.Now()
			deadlineCtx, cancel := context.WithDeadline(inv.Context(), start.Add(duration))
			defer cancel()

			// Create a ticker for sending data to the PTY.
			tick := time.NewTicker(tickInterval)
			defer tick.Stop()

			// Now we begin writing random data to the pty.
			writeSize := int(bps / 10)
			rch := make(chan error)
			wch := make(chan error)

			// Read forever in the background.
			go func() {
				rch <- readContext(deadlineCtx, &crw, writeSize*2)
				conn.Close()
				close(rch)
			}()

			// Write random data to the PTY every tick.
			go func() {
				wch <- writeRandomData(deadlineCtx, &crw, writeSize, tick.C)
				close(wch)
			}()

			// Wait for both our reads and writes to be finished.
			if wErr := <-wch; wErr != nil {
				return xerrors.Errorf("write to pty: %w", wErr)
			}
			if rErr := <-rch; rErr != nil {
				return xerrors.Errorf("read from pty: %w", rErr)
			}

			duration := time.Since(start)

			results := trafficGenOutput{
				DurationSeconds: duration.Seconds(),
				SentBytes:       crw.BytesWritten(),
				RcvdBytes:       crw.BytesRead(),
			}

			out, err := formatter.Format(inv.Context(), results)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	cmd.Options = []clibase.Option{
		{
			Flag:        "duration",
			Env:         "CODER_TRAFFICGEN_DURATION",
			Default:     "10s",
			Description: "How long to generate traffic for.",
			Value:       clibase.DurationOf(&duration),
		},
		{
			Flag:        "bps",
			Env:         "CODER_TRAFFICGEN_BPS",
			Default:     "1024",
			Description: "How much traffic to generate in bytes per second.",
			Value:       clibase.Int64Of(&bps),
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func readContext(ctx context.Context, src io.Reader, bufSize int) error {
	buf := make([]byte, bufSize)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if ctx.Err() != nil {
				return nil
			}
			_, err := src.Read(buf)
			if err != nil {
				if xerrors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	}
}

func writeRandomData(ctx context.Context, dst io.Writer, size int, tick <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
			payload := "#" + mustRandStr(size-1)
			data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
				Data: payload,
			})
			if err != nil {
				return err
			}
			if _, err := copyContext(ctx, dst, data); err != nil {
				return err
			}
		}
	}
}

// copyContext copies from src to dst until ctx is canceled.
func copyContext(ctx context.Context, dst io.Writer, src []byte) (int, error) {
	var count int
	for {
		select {
		case <-ctx.Done():
			return count, nil
		default:
			if ctx.Err() != nil {
				return count, nil
			}
			n, err := dst.Write(src)
			if err != nil {
				if xerrors.Is(err, io.EOF) {
					// On an EOF, assume that all of src was consumed.
					return len(src), nil
				}
				return count, err
			}
			count += n
			if n == len(src) {
				return count, nil
			}
			// Not all of src was consumed. Update src and retry.
			src = src[n:]
		}
	}
}

type countReadWriter struct {
	io.ReadWriter
	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
}

func (w *countReadWriter) Read(p []byte) (int, error) {
	n, err := w.ReadWriter.Read(p)
	if err == nil {
		w.bytesRead.Add(int64(n))
	}
	return n, err
}

func (w *countReadWriter) Write(p []byte) (int, error) {
	n, err := w.ReadWriter.Write(p)
	if err == nil {
		w.bytesWritten.Add(int64(n))
	}
	return n, err
}

func (w *countReadWriter) BytesRead() int64 {
	return w.bytesRead.Load()
}

func (w *countReadWriter) BytesWritten() int64 {
	return w.bytesWritten.Load()
}

func mustRandStr(len int) string {
	randStr, err := cryptorand.String(len)
	if err != nil {
		panic(err)
	}
	return randStr
}
