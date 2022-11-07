package agentconn

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/harness"
)

const defaultRequestTimeout = 5 * time.Second

type holdDurationEndedError struct{}

func (holdDurationEndedError) Error() string {
	return "hold duration ended"
}

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
	logs = syncWriter{
		mut: &sync.Mutex{},
		w:   logs,
	}
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)

	_, _ = fmt.Fprintln(logs, "Opening connection to workspace agent")
	switch r.cfg.ConnectionMode {
	case ConnectionModeDirect:
		_, _ = fmt.Fprintln(logs, "\tUsing direct connection...")
	case ConnectionModeDerp:
		_, _ = fmt.Fprintln(logs, "\tUsing proxied DERP connection through coder server...")
	}

	conn, err := r.client.DialWorkspaceAgent(ctx, r.cfg.AgentID, &codersdk.DialWorkspaceAgentOptions{
		Logger: logger.Named("agentconn"),
		// If the config requested DERP, then force DERP.
		BlockEndpoints: r.cfg.ConnectionMode == ConnectionModeDerp,
	})
	if err != nil {
		return xerrors.Errorf("dial workspace agent: %w", err)
	}
	defer conn.Close()

	// Wait for the disco connection to be established.
	const pingAttempts = 10
	const pingDelay = 1 * time.Second
	for i := 0; i < pingAttempts; i++ {
		_, _ = fmt.Fprintf(logs, "\tDisco ping attempt %d/%d...\n", i+1, pingAttempts)
		pingCtx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)
		_, err := conn.Ping(pingCtx)
		cancel()
		if err == nil {
			break
		}
		if i == pingAttempts-1 {
			return xerrors.Errorf("ping workspace agent: %w", err)
		}

		select {
		case <-ctx.Done():
			return xerrors.Errorf("wait for connection to be established: %w", ctx.Err())
		// We use time.After here since it's a very short duration so leaking a
		// timer is fine.
		case <-time.After(pingDelay):
		}
	}

	// Wait for a direct connection if requested.
	if r.cfg.ConnectionMode == ConnectionModeDirect {
		const directConnectionAttempts = 30
		const directConnectionDelay = 1 * time.Second
		for i := 0; i < directConnectionAttempts; i++ {
			_, _ = fmt.Fprintf(logs, "\tDirect connection check %d/%d...\n", i+1, directConnectionAttempts)
			status := conn.Status()

			var err error
			if len(status.Peers()) != 1 {
				_, _ = fmt.Fprintf(logs, "\t\tExpected 1 peer, found %d", len(status.Peers()))
				err = xerrors.Errorf("expected 1 peer, got %d", len(status.Peers()))
			} else {
				peer := status.Peer[status.Peers()[0]]
				_, _ = fmt.Fprintf(logs, "\t\tCurAddr: %s\n", peer.CurAddr)
				_, _ = fmt.Fprintf(logs, "\t\tRelay: %s\n", peer.Relay)
				if peer.Relay != "" && peer.CurAddr == "" {
					err = xerrors.Errorf("peer is connected via DERP, not direct")
				}
			}
			if err == nil {
				break
			}
			if i == directConnectionAttempts-1 {
				return xerrors.Errorf("wait for direct connection to agent: %w", err)
			}

			select {
			case <-ctx.Done():
				return xerrors.Errorf("wait for direct connection to agent: %w", ctx.Err())
			// We use time.After here since it's a very short duration so
			// leaking a timer is fine.
			case <-time.After(directConnectionDelay):
			}
		}
	}

	// Ensure DERP for completeness.
	if r.cfg.ConnectionMode == ConnectionModeDerp {
		status := conn.Status()
		if len(status.Peers()) != 1 {
			return xerrors.Errorf("check connection mode: expected 1 peer, got %d", len(status.Peers()))
		}
		peer := status.Peer[status.Peers()[0]]
		if peer.Relay == "" || peer.CurAddr != "" {
			return xerrors.Errorf("check connection mode: peer is connected directly, not via DERP")
		}
	}

	_, _ = fmt.Fprint(logs, "\nConnection established.\n\n")

	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, xerrors.Errorf("split host port %q: %w", addr, err)
				}

				portUint, err := strconv.ParseUint(port, 10, 16)
				if err != nil {
					return nil, xerrors.Errorf("parse port %q: %w", port, err)
				}
				return conn.DialContextTCP(ctx, netip.AddrPortFrom(codersdk.TailnetIP, uint16(portUint)))
			},
		},
	}

	// HACK: even though the ping passed above, we still need to open a
	// connection to the agent to ensure it's ready to accept connections. Not
	// sure why this is the case but it seems to be necessary.
	const verifyConnectionAttempts = 30
	const verifyConnectionDelay = 1 * time.Second
	for i := 0; i < verifyConnectionAttempts; i++ {
		_, _ = fmt.Fprintf(logs, "\tVerify connection attempt %d/%d...\n", i+1, verifyConnectionAttempts)
		verifyCtx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)

		u := &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("localhost", strconv.Itoa(codersdk.TailnetStatisticsPort)),
			Path:   "/",
		}
		req, err := http.NewRequestWithContext(verifyCtx, http.MethodGet, u.String(), nil)
		if err != nil {
			cancel()
			return xerrors.Errorf("new verify connection request to %q: %w", u.String(), err)
		}
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if i == verifyConnectionAttempts-1 {
			return xerrors.Errorf("verify connection: %w", err)
		}

		select {
		case <-ctx.Done():
			return xerrors.Errorf("verify connection: %w", ctx.Err())
		case <-time.After(verifyConnectionDelay):
		}
	}

	_, _ = fmt.Fprint(logs, "\nConnection verified.\n\n")

	// Make initial connections sequentially to ensure the services are
	// reachable before we start spawning a bunch of goroutines and tickers.
	if len(r.cfg.Connections) > 0 {
		_, _ = fmt.Fprintln(logs, "Performing initial service connections...")
	}
	for i, connSpec := range r.cfg.Connections {
		_, _ = fmt.Fprintf(logs, "\t%d. %s\n", i, connSpec.URL)

		timeout := defaultRequestTimeout
		if connSpec.Timeout > 0 {
			timeout = time.Duration(connSpec.Timeout)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, connSpec.URL, nil)
		if err != nil {
			cancel()
			return xerrors.Errorf("create request: %w", err)
		}

		res, err := client.Do(req)
		cancel()
		if err != nil {
			_, _ = fmt.Fprintf(logs, "\t\tFailed: %+v\n", err)
			return xerrors.Errorf("make initial connection to conn spec %d %q: %w", i, connSpec.URL, err)
		}
		_ = res.Body.Close()

		_, _ = fmt.Fprintln(logs, "\t\tOK")
	}

	if r.cfg.HoldDuration > 0 {
		eg, egCtx := errgroup.WithContext(ctx)

		if len(r.cfg.Connections) > 0 {
			_, _ = fmt.Fprintln(logs, "\nStarting connection loops...")
		}
		for i, connSpec := range r.cfg.Connections {
			i, connSpec := i, connSpec
			if connSpec.Interval <= 0 {
				continue
			}

			eg.Go(func() error {
				t := time.NewTicker(time.Duration(connSpec.Interval))
				defer t.Stop()

				timeout := defaultRequestTimeout
				if connSpec.Timeout > 0 {
					timeout = time.Duration(connSpec.Timeout)
				}

				for {
					select {
					case <-egCtx.Done():
						return egCtx.Err()
					case <-t.C:
						ctx, cancel := context.WithTimeout(ctx, timeout)
						req, err := http.NewRequestWithContext(ctx, http.MethodGet, connSpec.URL, nil)
						if err != nil {
							cancel()
							return xerrors.Errorf("create request: %w", err)
						}

						res, err := client.Do(req)
						cancel()
						if err != nil {
							_, _ = fmt.Fprintf(logs, "\tERR: %s (%d): %+v\n", connSpec.URL, i, err)
							return xerrors.Errorf("make connection to conn spec %d %q: %w", i, connSpec.URL, err)
						}
						res.Body.Close()

						_, _ = fmt.Fprintf(logs, "\tOK: %s (%d)\n", connSpec.URL, i)
						t.Reset(time.Duration(connSpec.Interval))
					}
				}
			})
		}

		// Wait for the hold duration to end. We use a fake error to signal that
		// the hold duration has ended.
		_, _ = fmt.Fprintf(logs, "\nWaiting for %s...\n", time.Duration(r.cfg.HoldDuration))
		eg.Go(func() error {
			t := time.NewTicker(time.Duration(r.cfg.HoldDuration))
			defer t.Stop()

			select {
			case <-egCtx.Done():
				return egCtx.Err()
			case <-t.C:
				// Returning an error here will cause the errgroup context to
				// be canceled, which is what we want. This fake error is
				// ignored below.
				return holdDurationEndedError{}
			}
		})

		err = eg.Wait()
		if err != nil && !xerrors.Is(err, holdDurationEndedError{}) {
			return xerrors.Errorf("run connections loop: %w", err)
		}
	}

	err = conn.Close()
	if err != nil {
		return xerrors.Errorf("close connection: %w", err)
	}

	return nil
}

// syncWriter wraps an io.Writer in a sync.Mutex.
type syncWriter struct {
	mut *sync.Mutex
	w   io.Writer
}

// Write implements io.Writer.
func (sw syncWriter) Write(p []byte) (n int, err error) {
	sw.mut.Lock()
	defer sw.mut.Unlock()
	return sw.w.Write(p)
}
