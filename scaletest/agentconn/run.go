package agentconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
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
func (r *Runner) Run(ctx context.Context, _ string, w io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs := loadtestutil.NewSyncWriter(w)
	defer logs.Close()
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	_, _ = fmt.Fprintln(logs, "Opening connection to workspace agent")
	switch r.cfg.ConnectionMode {
	case ConnectionModeDirect:
		_, _ = fmt.Fprintln(logs, "\tUsing direct connection...")
	case ConnectionModeDerp:
		_, _ = fmt.Fprintln(logs, "\tUsing proxied DERP connection through coder server...")
	}

	conn, err := workspacesdk.New(r.client).
		DialAgent(ctx, r.cfg.AgentID, &workspacesdk.DialAgentOptions{
			Logger: logger.Named("agentconn"),
			// If the config requested DERP, then force DERP.
			BlockEndpoints: r.cfg.ConnectionMode == ConnectionModeDerp,
		})
	if err != nil {
		return xerrors.Errorf("dial workspace agent: %w", err)
	}
	defer conn.Close()

	err = waitForDisco(ctx, logs, conn)
	if err != nil {
		return xerrors.Errorf("wait for discovery connection: %w", err)
	}

	// Wait for a direct connection if requested.
	if r.cfg.ConnectionMode == ConnectionModeDirect {
		err = waitForDirectConnection(ctx, logs, conn)
		if err != nil {
			return xerrors.Errorf("wait for direct connection: %w", err)
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

	// HACK: even though the ping passed above, we still need to open a
	// connection to the agent to ensure it's ready to accept connections. Not
	// sure why this is the case but it seems to be necessary.
	err = verifyConnection(ctx, logs, conn)
	if err != nil {
		return xerrors.Errorf("verify connection: %w", err)
	}

	_, _ = fmt.Fprint(logs, "\nConnection verified.\n\n")

	// Make initial connections sequentially to ensure the services are
	// reachable before we start spawning a bunch of goroutines and tickers.
	err = performInitialConnections(ctx, logs, conn, r.cfg.Connections)
	if err != nil {
		return xerrors.Errorf("perform initial connections: %w", err)
	}

	if r.cfg.HoldDuration > 0 {
		err = holdConnection(ctx, logs, conn, time.Duration(r.cfg.HoldDuration), r.cfg.Connections)
		if err != nil {
			return xerrors.Errorf("hold connection: %w", err)
		}
	}

	err = conn.Close()
	if err != nil {
		return xerrors.Errorf("close connection: %w", err)
	}

	return nil
}

func waitForDisco(ctx context.Context, logs io.Writer, conn *workspacesdk.AgentConn) error {
	const pingAttempts = 10
	const pingDelay = 1 * time.Second

	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	for i := 0; i < pingAttempts; i++ {
		_, _ = fmt.Fprintf(logs, "\tDisco ping attempt %d/%d...\n", i+1, pingAttempts)
		pingCtx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)
		_, p2p, _, err := conn.Ping(pingCtx)
		cancel()
		if err == nil {
			_, _ = fmt.Fprintf(logs, "\tDisco ping succeeded after %d attempts, p2p = %v\n", i+1, p2p)
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

	return nil
}

func waitForDirectConnection(ctx context.Context, logs io.Writer, conn *workspacesdk.AgentConn) error {
	const directConnectionAttempts = 30
	const directConnectionDelay = 1 * time.Second

	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	for i := 0; i < directConnectionAttempts; i++ {
		_, _ = fmt.Fprintf(logs, "\tDirect connection check %d/%d...\n", i+1, directConnectionAttempts)
		status := conn.Status()

		var err error
		if len(status.Peers()) != 1 {
			_, _ = fmt.Fprintf(logs, "\t\tExpected 1 peer, found %d\n", len(status.Peers()))
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

	return nil
}

func verifyConnection(ctx context.Context, logs io.Writer, conn *workspacesdk.AgentConn) error {
	const verifyConnectionAttempts = 30
	const verifyConnectionDelay = 1 * time.Second

	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	client := agentHTTPClient(conn)
	for i := 0; i < verifyConnectionAttempts; i++ {
		_, _ = fmt.Fprintf(logs, "\tVerify connection attempt %d/%d...\n", i+1, verifyConnectionAttempts)
		verifyCtx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)

		u := &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("localhost", strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort)),
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

	return nil
}

func performInitialConnections(ctx context.Context, logs io.Writer, conn *workspacesdk.AgentConn, specs []Connection) error {
	if len(specs) == 0 {
		return nil
	}

	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	_, _ = fmt.Fprintln(logs, "Performing initial service connections...")
	client := agentHTTPClient(conn)
	for i, connSpec := range specs {
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

	return nil
}

func holdConnection(ctx context.Context, logs io.Writer, conn *workspacesdk.AgentConn, holdDur time.Duration, specs []Connection) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	eg, egCtx := errgroup.WithContext(ctx)
	client := agentHTTPClient(conn)
	if len(specs) > 0 {
		_, _ = fmt.Fprintln(logs, "\nStarting connection loops...")
	}
	for i, connSpec := range specs {
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
	_, _ = fmt.Fprintf(logs, "\nWaiting for %s...\n", holdDur)
	eg.Go(func() error {
		t := time.NewTicker(holdDur)
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

	err := eg.Wait()
	if err != nil && !errors.Is(err, holdDurationEndedError{}) {
		return xerrors.Errorf("run connections loop: %w", err)
	}

	return nil
}

func agentHTTPClient(conn *workspacesdk.AgentConn) *http.Client {
	return &http.Client{
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

				// Addr doesn't matter here, besides the port. DialContext will
				// automatically choose the right IP to dial.
				return conn.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", portUint))
			},
		},
	}
}
