package provisionerd

import (
	"context"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/provisionerd/proto"
	provisionersdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/retry"
)

// Dialer returns a gRPC client to communicate with.
// The provisioner daemon handles intermittent connection failures
// for upgrades to coderd.
type Dialer func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)

// Provisioners maps provisioner ID to implementation.
type Provisioners map[string]provisionersdkproto.DRPCProvisionerClient

type Options struct {
	AcquireInterval time.Duration
	Logger          slog.Logger
}

func New(apiDialer Dialer, provisioners Provisioners, opts *Options) *API {
	if opts.AcquireInterval == 0 {
		opts.AcquireInterval = 5 * time.Second
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	api := &API{
		dialer:       apiDialer,
		provisioners: provisioners,
		opts:         opts,

		closeContext:       ctx,
		closeContextCancel: ctxCancel,
		closed:             make(chan struct{}),
	}
	go api.connect()
	return api
}

type API struct {
	provisioners Provisioners
	opts         *Options

	dialer       Dialer
	connectMutex sync.Mutex
	client       proto.DRPCProvisionerDaemonClient
	updateStream proto.DRPCProvisionerDaemon_UpdateJobClient

	closeContext       context.Context
	closeContextCancel context.CancelFunc

	closed     chan struct{}
	closeMutex sync.Mutex
	closeError error

	activeJob *proto.AcquiredJob
	logQueue  []proto.Log
}

// connect establishes a connection
func (a *API) connect() {
	a.connectMutex.Lock()
	defer a.connectMutex.Unlock()

	var err error
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(a.closeContext); {
		a.client, err = a.dialer(a.closeContext)
		if err != nil {
			// Warn
			a.opts.Logger.Warn(context.Background(), "failed to dial", slog.Error(err))
			continue
		}
		a.updateStream, err = a.client.UpdateJob(a.closeContext)
		if err != nil {
			a.opts.Logger.Warn(context.Background(), "create update job stream", slog.Error(err))
			continue
		}
		a.opts.Logger.Debug(context.Background(), "connected")
		break
	}

	go func() {
		if a.isClosed() {
			return
		}
		select {
		case <-a.closed:
			return
		case <-a.updateStream.Context().Done():
			// We use the update stream to detect when the connection
			// has been interrupted. This works well, because logs need
			// to buffer if a job is running in the background.
			a.opts.Logger.Debug(context.Background(), "update stream ended", slog.Error(a.updateStream.Context().Err()))
			a.connect()
		}
	}()

	go func() {
		if a.isClosed() {
			return
		}
		ticker := time.NewTicker(a.opts.AcquireInterval)
		defer ticker.Stop()
		for {
			select {
			case <-a.closed:
				return
			case <-a.updateStream.Context().Done():
				return
			case <-ticker.C:
				if a.activeJob != nil {
					a.opts.Logger.Debug(context.Background(), "skipping acquire; job is already running")
					continue
				}
				a.acquireJob()
			}
		}
	}()
}

func (a *API) acquireJob() {
	a.opts.Logger.Debug(context.Background(), "acquiring new job")
	var err error
	a.activeJob, err = a.client.AcquireJob(a.closeContext, &proto.Empty{})
	if err != nil {
		a.opts.Logger.Error(context.Background(), "acquire job", slog.Error(err))
		return
	}
	a.opts.Logger.Info(context.Background(), "acquired job",
		slog.F("organization_name", a.activeJob.OrganizationName),
		slog.F("project_name", a.activeJob.ProjectName),
		slog.F("username", a.activeJob.UserName),
		slog.F("provisioner", a.activeJob.Provisioner),
	)
	// Work!
}

// isClosed returns whether the API is closed or not.
func (a *API) isClosed() bool {
	select {
	case <-a.closed:
		return true
	default:
		return false
	}
}

// Close ends the provisioner. It will mark any active jobs as canceled.
func (a *API) Close() error {
	return a.closeWithError(nil)
}

// closeWithError closes the provisioner; subsequent reads/writes will return the error err.
func (a *API) closeWithError(err error) error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return a.closeError
	}

	a.opts.Logger.Debug(context.Background(), "closing server with error", slog.Error(err))
	a.closeError = err
	close(a.closed)
	a.closeContextCancel()

	if a.updateStream != nil {
		_ = a.client.DRPCConn().Close()
		_ = a.updateStream.Close()
	}

	return err
}
