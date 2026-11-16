// Package agentrelay lets what runs in a workspace reach the control plane
// through the workspace's daemon.
//
// The daemon serves a local address inside the workspace and forwards every
// request it gets there to the control plane. On the way it attaches a lease:
// a short-lived token that only this daemon can obtain. Where the control
// plane enforces the relay, it accepts the session token of the workspace
// only together with such a lease, so a session token that leaves the
// workspace does not work.
//
// The relay adds nothing else to a request. The caller authenticates with
// its own credential, as it would against the control plane directly.
package agentrelay

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/workspacerelay"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/quartz"
)

const (
	// DefaultListenAddress is where the relay listens: on the loopback
	// interface, on a port the system picks.
	DefaultListenAddress = "127.0.0.1:0"

	// retryAfterError is how long the relay waits after a failed attempt
	// to obtain a lease.
	retryAfterError = 5 * time.Second
	// retryWhileHeld is how long it waits while another instance of the
	// daemon holds the relay. That instance's lease lapses within a
	// minute of its last renewal.
	retryWhileHeld = 15 * time.Second
	// retryWhileOff is how long it waits while the control plane has the
	// relay turned off.
	retryWhileOff = time.Minute
	// ensureTimeout bounds the first attempt, which the daemon waits for
	// before it starts anything in the workspace.
	ensureTimeout = 5 * time.Second
)

// Options configures a Relay.
type Options struct {
	Logger slog.Logger
	Clock  quartz.Clock
	// ControlPlane is where requests are forwarded.
	ControlPlane *url.URL
	// Transport is how the control plane is reached. Nil uses the default
	// transport.
	Transport http.RoundTripper
	// Lease obtains or renews this daemon instance's lease.
	Lease func(ctx context.Context, req agentsdk.WorkspaceRelayLeaseRequest) (agentsdk.WorkspaceRelayLease, error)
	// Release gives the relay up when the daemon stops. Optional.
	Release func(ctx context.Context, req agentsdk.WorkspaceRelayLeaseRequest) error
	// ListenAddress is where the relay listens inside the workspace.
	// Empty uses DefaultListenAddress.
	ListenAddress string
}

// Relay forwards requests from inside the workspace to the control plane,
// and vouches for them with its lease.
type Relay struct {
	opts Options
	// privateKey is what makes this instance of the daemon the holder of
	// the relay. It exists in memory only.
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	proxy      *httputil.ReverseProxy

	// attempt serializes attempts to obtain a lease.
	attempt sync.Mutex

	mu       sync.Mutex
	lease    agentsdk.WorkspaceRelayLease
	hasLease bool
	listener net.Listener
	server   *http.Server
	url      string
	closed   bool
}

// New makes a relay. It does nothing until Run or Ensure is called.
func New(opts Options) (*Relay, error) {
	if opts.ControlPlane == nil {
		return nil, xerrors.New("the relay needs the URL of the control plane")
	}
	if opts.Lease == nil {
		return nil, xerrors.New("the relay needs a way to obtain a lease")
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.ListenAddress == "" {
		opts.ListenAddress = DefaultListenAddress
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("generate the relay's key: %w", err)
	}
	r := &Relay{
		opts:       opts,
		privateKey: privateKey,
		publicKey:  publicKey,
	}
	r.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(opts.ControlPlane)
			// Only the daemon vouches. A caller cannot bring a lease.
			pr.Out.Header.Del(workspacerelay.LeaseHeader)
			if lease, ok := r.currentLease(); ok {
				pr.Out.Header.Set(workspacerelay.LeaseHeader, lease)
			}
		},
		Transport: opts.Transport,
		// Responses are passed on as they arrive: the API streams.
		FlushInterval: -1,
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			opts.Logger.Warn(req.Context(), "relay a request to the control plane", slog.F("path", req.URL.Path), slog.Error(err))
			rw.WriteHeader(http.StatusBadGateway)
		},
	}
	return r, nil
}

// URL is the address what runs in the workspace uses instead of the control
// plane's. It is empty until the control plane has answered that the relay
// is on, which it never does while the relay is turned off there.
func (r *Relay) URL() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.url
}

// currentLease returns the lease to attach, when the relay holds one that
// has not run out.
func (r *Relay) currentLease() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasLease || !r.opts.Clock.Now().Before(r.lease.ExpiresAt) {
		return "", false
	}
	return r.lease.Lease, true
}

// Ensure makes one attempt to obtain a lease, unless the relay already
// serves. The daemon calls it before it starts anything in the workspace, so
// that what it starts is given the relay's address.
func (r *Relay) Ensure(ctx context.Context) {
	if r.URL() != "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, ensureTimeout)
	defer cancel()
	_ = r.obtainLease(ctx)
}

// Run keeps the lease fresh until ctx is done.
func (r *Relay) Run(ctx context.Context) {
	for {
		wait := r.obtainLease(ctx)
		timer := r.opts.Clock.NewTimer(wait, "agentrelay", "renew")
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// signedRequest proves that this instance of the daemon is asking.
func (r *Relay) signedRequest() agentsdk.WorkspaceRelayLeaseRequest {
	now := r.opts.Clock.Now()
	return agentsdk.WorkspaceRelayLeaseRequest{
		PublicKey: r.publicKey,
		Timestamp: now,
		Signature: ed25519.Sign(r.privateKey, workspacerelay.SigningInput(now)),
	}
}

// obtainLease asks the control plane for a lease and returns how long to
// wait before asking again.
func (r *Relay) obtainLease(ctx context.Context) time.Duration {
	r.attempt.Lock()
	defer r.attempt.Unlock()

	lease, err := r.opts.Lease(ctx, r.signedRequest())
	if err != nil {
		var sdkErr *codersdk.Error
		switch {
		case ctx.Err() != nil:
			return retryAfterError
		case errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound:
			// The relay is off, or the control plane predates it. Requests
			// that still come to the local address are forwarded as they
			// are.
			r.opts.Logger.Debug(ctx, "the control plane has the workspace relay turned off")
			return retryWhileOff
		case errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusConflict:
			r.opts.Logger.Warn(ctx, "another instance of this workspace's daemon holds the relay; waiting for its lease to lapse")
			// The relay is on, so serve already. What the daemon starts
			// now is given the relay's address, and its requests are
			// vouched for as soon as there is a lease. A process keeps
			// the address it was started with.
			if err := r.serve(ctx); err != nil {
				r.opts.Logger.Error(ctx, "serve the workspace relay", slog.Error(err))
			}
			return retryWhileHeld
		default:
			r.opts.Logger.Warn(ctx, "obtain a workspace relay lease", slog.Error(err))
			return retryAfterError
		}
	}

	r.mu.Lock()
	first := !r.hasLease
	changed := r.hasLease && r.lease.Generation != lease.Generation
	r.lease = lease
	r.hasLease = true
	r.mu.Unlock()
	if first || changed {
		r.opts.Logger.Info(ctx, "obtained a workspace relay lease",
			slog.F("generation", lease.Generation), slog.F("enforced", lease.Enforced))
	}
	if err := r.serve(ctx); err != nil {
		r.opts.Logger.Error(ctx, "serve the workspace relay", slog.Error(err))
		return retryAfterError
	}

	// Renew well before the lease runs out, so that one failed attempt
	// does not interrupt what runs in the workspace.
	wait := lease.ExpiresAt.Sub(r.opts.Clock.Now()) / 3
	if wait < time.Second {
		wait = time.Second
	}
	return wait
}

// serve starts the local listener once. From then on the relay has an
// address, which it keeps.
func (r *Relay) serve(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listener != nil || r.closed {
		return nil
	}
	listener, err := net.Listen("tcp", r.opts.ListenAddress)
	if err != nil {
		return xerrors.Errorf("listen on %s: %w", r.opts.ListenAddress, err)
	}
	r.listener = listener
	r.url = "http://" + listener.Addr().String()
	r.server = &http.Server{
		Handler:           r.proxy,
		ReadHeaderTimeout: time.Minute,
	}
	server := r.server
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.opts.Logger.Error(context.Background(), "workspace relay stopped serving", slog.Error(err))
		}
	}()
	r.opts.Logger.Info(ctx, "workspace relay is serving", slog.F("url", r.url))
	return nil
}

// ServeHTTP relays one request. The relay serves by itself once it holds a
// lease. This is for tests.
func (r *Relay) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	r.proxy.ServeHTTP(rw, req)
}

// Close stops serving and gives the relay up, so that the next instance of
// the daemon does not have to wait for the lease to lapse.
func (r *Relay) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	server := r.server
	held := r.hasLease
	r.hasLease = false
	r.mu.Unlock()

	if held && r.opts.Release != nil {
		ctx, cancel := context.WithTimeout(context.Background(), ensureTimeout)
		defer cancel()
		if err := r.opts.Release(ctx, r.signedRequest()); err != nil {
			r.opts.Logger.Debug(ctx, "release the workspace relay", slog.Error(err))
		}
	}
	if server != nil {
		return server.Close()
	}
	return nil
}
