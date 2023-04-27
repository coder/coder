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
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

func (r *RootCmd) trafficGen() *clibase.Cmd {
	var (
		duration time.Duration
		bps      int64
		client   = new(codersdk.Client)
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
			var agentName string
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
			start := time.Now()
			ctx, cancel := context.WithDeadline(inv.Context(), start.Add(duration))
			defer cancel()
			crw := countReadWriter{ReadWriter: conn}
			// First, write a comment to the pty so we don't execute anything.
			data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
				Data: "#",
			})
			if err != nil {
				return xerrors.Errorf("serialize request: %w", err)
			}
			_, err = crw.Write(data)
			if err != nil {
				return xerrors.Errorf("write comment to pty: %w", err)
			}
			// Now we begin writing random data to the pty.
			writeSize := int(bps / 10)
			rch := make(chan error)
			wch := make(chan error)
			go func() {
				rch <- readForever(ctx, &crw)
				close(rch)
			}()
			go func() {
				wch <- writeRandomData(ctx, &crw, writeSize, 100*time.Millisecond)
				close(wch)
			}()

			if wErr := <-wch; wErr != nil {
				return xerrors.Errorf("write to pty: %w", wErr)
			}
			if rErr := <-rch; rErr != nil {
				return xerrors.Errorf("read from pty: %w", rErr)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Test results:\n")
			_, _ = fmt.Fprintf(inv.Stdout, "Took:     %.2fs\n", time.Since(start).Seconds())
			_, _ = fmt.Fprintf(inv.Stdout, "Sent:     %d bytes\n", crw.BytesWritten())
			_, _ = fmt.Fprintf(inv.Stdout, "Rcvd:     %d bytes\n", crw.BytesRead())
			return nil
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

	return cmd
}

func readForever(ctx context.Context, src io.Reader) error {
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, err := src.Read(buf)
			if err != nil && err != io.EOF {
				return err
			}
		}
	}
}

func writeRandomData(ctx context.Context, dst io.Writer, size int, period time.Duration) error {
	tick := time.NewTicker(period)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			randStr, err := cryptorand.String(size)
			if err != nil {
				return err
			}
			data, err := json.Marshal(codersdk.ReconnectingPTYRequest{
				Data: randStr,
			})
			if err != nil {
				return err
			}
			err = copyContext(ctx, dst, data)
			if err != nil {
				return err
			}
		}
	}
}

func copyContext(ctx context.Context, dst io.Writer, src []byte) error {
	for idx := range src {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, err := dst.Write(src[idx : idx+1])
			if err != nil {
				if xerrors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	}
	return nil
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
