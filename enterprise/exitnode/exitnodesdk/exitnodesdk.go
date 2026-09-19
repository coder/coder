// Package exitnodesdk is the client an exit node uses to talk to coderd. It
// authenticates every request with the Coder-Exit-Node-Token header and
// exposes the small set of routes under /api/v2/exitnodes/me that an exit
// node needs: registration, flow reporting, and the tailnet coordinator.
package exitnodesdk

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
	"github.com/coder/websocket"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
)

const (
	registerPath   = "/api/v2/exitnodes/me/register"
	deregisterPath = "/api/v2/exitnodes/me/deregister"
	coordinatePath = "/api/v2/exitnodes/me/coordinate"
	flowsPath      = "/api/v2/exitnodes/me/flows"
)

// Client is an HTTP client for the subset of Coder API routes that exit nodes
// need. The exit node token is attached to every request and every websocket
// dial via the SessionTokenProvider, so it never has to be handled (or
// logged) by callers.
type Client struct {
	SDKClient *codersdk.Client
}

// New creates an exit node client for the provided primary coderd URL. The
// token has the form "<exit node ID>:<secret>" and is sent in the
// Coder-Exit-Node-Token header.
func New(serverURL *url.URL, token string) *Client {
	sdkClient := codersdk.New(serverURL)
	sdkClient.SessionTokenProvider = codersdk.FixedSessionTokenProvider{
		SessionToken:       token,
		SessionTokenHeader: codersdk.ExitNodeTokenHeader,
	}
	return &Client{SDKClient: sdkClient}
}

// Register announces the exit node replica to coderd and returns its tailnet
// configuration. Both 200 and 201 are accepted.
func (c *Client) Register(ctx context.Context, req codersdk.RegisterExitNodeRequest) (codersdk.RegisterExitNodeResponse, error) {
	var resp codersdk.RegisterExitNodeResponse
	res, err := c.SDKClient.Request(ctx, http.MethodPost, registerPath, req)
	if err != nil {
		return resp, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return resp, codersdk.ReadBodyAsError(res)
	}
	if err := codersdk.ReadBodyAsJSON(res, &resp); err != nil {
		return resp, xerrors.Errorf("decode response: %w", err)
	}
	return resp, nil
}

// Deregister marks an exit node replica as stopped.
func (c *Client) Deregister(ctx context.Context, req codersdk.DeregisterExitNodeRequest) error {
	res, err := c.SDKClient.Request(ctx, http.MethodPost, deregisterPath, req)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

// ReportFlows sends a batch of flow reports to coderd.
func (c *Client) ReportFlows(ctx context.Context, req codersdk.ReportExitNodeFlowsRequest) error {
	res, err := c.SDKClient.Request(ctx, http.MethodPost, flowsPath, req)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusAccepted {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

// TailnetDialer returns a ControlProtocolDialer that connects to the exit node
// coordinator as replicaID. The websocket upgrade carries the exit node token
// header via the SDK client's SessionTokenProvider.
func (c *Client) TailnetDialer(replicaID uuid.UUID) (tailnet.ControlProtocolDialer, error) {
	coordinateURL, err := c.SDKClient.URL.Parse(coordinatePath)
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	query := coordinateURL.Query()
	query.Set(codersdk.ExitNodeCoordinateReplicaIDParam, replicaID.String())
	coordinateURL.RawQuery = query.Encode()
	wsOptions := &websocket.DialOptions{HTTPClient: c.SDKClient.HTTPClient}
	c.SDKClient.SessionTokenProvider.SetDialOption(wsOptions)
	return workspacesdk.NewWebsocketDialer(c.SDKClient.Logger().Named("tailnet_dialer"), coordinateURL, wsOptions), nil
}

// RegisterLoopOpts configures RegisterLoop.
type RegisterLoopOpts struct {
	Logger  slog.Logger
	Request codersdk.RegisterExitNodeRequest

	// Interval between re-registrations. Defaults to 5 seconds. The initial
	// registration is not delayed.
	Interval time.Duration
	// MaxFailureCount is how many consecutive failed re-registrations are
	// tolerated before FailureFn is invoked. Defaults to 10.
	MaxFailureCount int
	// AttemptTimeout bounds one registration or deregistration request.
	// Defaults to 10 seconds.
	AttemptTimeout time.Duration

	// MutateFn is invoked before every registration request. It may refresh
	// fields whose values can change while the process is running.
	MutateFn func(req *codersdk.RegisterExitNodeRequest)
	// CallbackFn is invoked with every successful re-registration response
	// after the first. Returning an error stops the loop and forwards the error
	// to FailureFn.
	CallbackFn func(res codersdk.RegisterExitNodeResponse) error
	// FailureFn is invoked once when the loop terminates for a registration or
	// callback failure.
	FailureFn func(err error)

	// Clock is for testing only.
	Clock quartz.Clock
}

// RegisterLoop keeps an exit node registered with coderd until Close is called.
type RegisterLoop struct {
	client         *Client
	logger         slog.Logger
	replicaID      uuid.UUID
	attemptTimeout time.Duration
	cancel         context.CancelFunc
	done           chan struct{}
	deregisterOnce sync.Once
}

// RegisterLoop performs the initial registration synchronously and then keeps
// re-registering in the background until Close is called. The first response
// is returned to the caller; later responses are delivered to CallbackFn.
func (c *Client) RegisterLoop(ctx context.Context, opts RegisterLoopOpts) (*RegisterLoop, codersdk.RegisterExitNodeResponse, error) {
	opts.Interval = cmp.Or(opts.Interval, 5*time.Second)
	opts.MaxFailureCount = cmp.Or(opts.MaxFailureCount, 10)
	opts.AttemptTimeout = cmp.Or(opts.AttemptTimeout, 10*time.Second)
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	register := func(ctx context.Context) (codersdk.RegisterExitNodeResponse, error) {
		if opts.MutateFn != nil {
			opts.MutateFn(&opts.Request)
		}
		ctx, cancel := context.WithTimeout(ctx, opts.AttemptTimeout)
		defer cancel()
		res, err := c.Register(ctx, opts.Request)
		if err != nil {
			return res, xerrors.Errorf("register exit node replica %s: %w", opts.Request.ReplicaID, err)
		}
		return res, nil
	}

	first, err := register(ctx)
	if err != nil {
		return nil, codersdk.RegisterExitNodeResponse{}, xerrors.Errorf("initial registration: %w", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	loop := &RegisterLoop{
		client:         c,
		logger:         opts.Logger,
		replicaID:      opts.Request.ReplicaID,
		attemptTimeout: opts.AttemptTimeout,
		cancel:         cancel,
		done:           make(chan struct{}),
	}
	fail := func(err error) {
		loop.deregister(err)
		if opts.FailureFn != nil {
			opts.FailureFn(err)
		}
	}
	go func() {
		defer close(loop.done)
		ticker := opts.Clock.NewTicker(opts.Interval, "exitnodesdk", "register")
		defer ticker.Stop()
		failedAttempts := 0
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
			}
			res, err := register(loopCtx)
			if err != nil {
				if loopCtx.Err() != nil {
					return
				}
				if isPermanentRegistrationError(err) {
					fail(xerrors.Errorf("permanent registration failure: %w", err))
					return
				}
				failedAttempts++
				opts.Logger.Warn(context.Background(), "failed to re-register exit node with coderd",
					slog.F("failed_attempts", failedAttempts), slog.Error(err))
				if failedAttempts > opts.MaxFailureCount {
					fail(xerrors.Errorf("exceeded re-registration failure count of %d: last error: %w", opts.MaxFailureCount, err))
					return
				}
				continue
			}
			failedAttempts = 0
			if opts.CallbackFn != nil {
				if err := opts.CallbackFn(res); err != nil {
					fail(xerrors.Errorf("registration callback: %w", err))
					return
				}
			}
		}
	}()
	return loop, first, nil
}

func isPermanentRegistrationError(err error) bool {
	var sdkErr *codersdk.Error
	return errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusBadRequest
}

func (l *RegisterLoop) deregister(rootErr error) {
	l.deregisterOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), l.attemptTimeout)
		defer cancel()
		err := l.client.Deregister(ctx, codersdk.DeregisterExitNodeRequest{ReplicaID: l.replicaID})
		if err == nil {
			return
		}
		fields := []slog.Field{slog.Error(err)}
		if rootErr != nil {
			fields = append(fields, slog.F("root_error", rootErr.Error()))
		}
		l.logger.Warn(context.Background(), "failed to deregister exit node replica", fields...)
	})
}

// Close stops the loop, waits for it to exit, and deregisters the replica.
func (l *RegisterLoop) Close() {
	l.cancel()
	<-l.done
	l.deregister(nil)
}
