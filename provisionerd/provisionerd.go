package provisionerd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/spf13/afero"
	"go.uber.org/atomic"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionerd/runner"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/retry"
)

// IsMissingParameterError returns whether the error message provided
// is a missing parameter error. This can indicate to consumers that
// they should check parameters.
func IsMissingParameterError(err string) bool {
	return strings.Contains(err, runner.MissingParameterErrorText)
}

// Dialer represents the function to create a daemon client connection.
type Dialer func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)

// Provisioners maps provisioner ID to implementation.
type Provisioners map[string]sdkproto.DRPCProvisionerClient

// Options provides customizations to the behavior of a provisioner daemon.
type Options struct {
	Filesystem afero.Fs
	Logger     slog.Logger

	ForceCancelInterval time.Duration
	UpdateInterval      time.Duration
	PollInterval        time.Duration
	Provisioners        Provisioners
	WorkDirectory       string
}

// New creates and starts a provisioner daemon.
func New(clientDialer Dialer, opts *Options) *Server {
	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.UpdateInterval == 0 {
		opts.UpdateInterval = 5 * time.Second
	}
	if opts.ForceCancelInterval == 0 {
		opts.ForceCancelInterval = time.Minute
	}
	if opts.Filesystem == nil {
		opts.Filesystem = afero.NewOsFs()
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	daemon := &Server{
		clientDialer: clientDialer,
		opts:         opts,

		closeContext: ctx,
		closeCancel:  ctxCancel,

		shutdown: make(chan struct{}),
	}

	go daemon.connect(ctx)
	return daemon
}

type Server struct {
	opts *Options

	clientDialer Dialer
	clientValue  atomic.Value

	// Locked when closing the daemon, shutting down, or starting a new job.
	mutex        sync.Mutex
	closeContext context.Context
	closeCancel  context.CancelFunc
	closeError   error
	shutdown     chan struct{}
	activeJob    *runner.Runner
}

// Connect establishes a connection to coderd.
func (p *Server) connect(ctx context.Context) {
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		client, err := p.clientDialer(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if p.isClosed() {
				return
			}
			p.opts.Logger.Warn(context.Background(), "failed to dial", slog.Error(err))
			continue
		}
		p.clientValue.Store(client)
		p.opts.Logger.Debug(context.Background(), "connected")
		break
	}
	select {
	case <-ctx.Done():
		return
	default:
	}

	go func() {
		if p.isClosed() {
			return
		}
		client, ok := p.client()
		if !ok {
			return
		}
		select {
		case <-p.closeContext.Done():
			return
		case <-client.DRPCConn().Closed():
			// We use the update stream to detect when the connection
			// has been interrupted. This works well, because logs need
			// to buffer if a job is running in the background.
			p.opts.Logger.Debug(context.Background(), "client stream ended")
			p.connect(ctx)
		}
	}()

	go func() {
		if p.isClosed() {
			return
		}
		ticker := time.NewTicker(p.opts.PollInterval)
		defer ticker.Stop()
		for {
			client, ok := p.client()
			if !ok {
				return
			}
			select {
			case <-p.closeContext.Done():
				return
			case <-client.DRPCConn().Closed():
				return
			case <-ticker.C:
				p.acquireJob(ctx)
			}
		}
	}()
}

func (p *Server) client() (proto.DRPCProvisionerDaemonClient, bool) {
	rawClient := p.clientValue.Load()
	if rawClient == nil {
		return nil, false
	}
	client, ok := rawClient.(proto.DRPCProvisionerDaemonClient)
	return client, ok
}

// isRunningJob returns true if a job is running.  Caller must hold the mutex.
func (p *Server) isRunningJob() bool {
	if p.activeJob == nil {
		return false
	}
	select {
	case <-p.activeJob.Done():
		return false
	default:
		return true
	}
}

// Locks a job in the database, and runs it!
func (p *Server) acquireJob(ctx context.Context) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.isClosed() {
		return
	}
	if p.isRunningJob() {
		return
	}
	if p.isShutdown() {
		p.opts.Logger.Debug(context.Background(), "skipping acquire; provisionerd is shutting down...")
		return
	}
	var err error
	client, ok := p.client()
	if !ok {
		return
	}
	job, err := client.AcquireJob(ctx, &proto.Empty{})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, yamux.ErrSessionShutdown) {
			return
		}
		p.opts.Logger.Warn(context.Background(), "acquire job", slog.Error(err))
		return
	}
	if job.JobId == "" {
		return
	}
	p.opts.Logger.Info(context.Background(), "acquired job",
		slog.F("initiator_username", job.UserName),
		slog.F("provisioner", job.Provisioner),
		slog.F("job_id", job.JobId),
	)

	provisioner, ok := p.opts.Provisioners[job.Provisioner]
	if !ok {
		err := p.FailJob(ctx, &proto.FailedJob{
			JobId: job.JobId,
			Error: fmt.Sprintf("no provisioner %s", job.Provisioner),
		})
		if err != nil {
			p.opts.Logger.Error(context.Background(), "failed to call FailJob",
				slog.F("job_id", job.JobId), slog.Error(err))
		}
		return
	}
	p.activeJob = runner.NewRunner(job, p, p.opts.Logger, p.opts.Filesystem, p.opts.WorkDirectory, provisioner,
		p.opts.UpdateInterval, p.opts.ForceCancelInterval)
	go p.activeJob.Run()
}

func retryable(err error) bool {
	return xerrors.Is(err, yamux.ErrSessionShutdown) || xerrors.Is(err, io.EOF) ||
		// annoyingly, dRPC sometimes returns context.Canceled if the transport was closed, even if the context for
		// the RPC *is not canceled*.  Retrying is fine if the RPC context is not canceled.
		xerrors.Is(err, context.Canceled)
}

// clientDoWithRetries runs the function f with a client, and retries with backoff until either the error returned
// is not retryable() or the context expires.
func (p *Server) clientDoWithRetries(
	ctx context.Context, f func(context.Context, proto.DRPCProvisionerDaemonClient) (any, error)) (
	any, error) {
	for retrier := retry.New(25*time.Millisecond, 5*time.Second); retrier.Wait(ctx); {
		client, ok := p.client()
		if !ok {
			continue
		}
		resp, err := f(ctx, client)
		if retryable(err) {
			continue
		}
		return resp, err
	}
	return nil, ctx.Err()
}

func (p *Server) UpdateJob(ctx context.Context, in *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	out, err := p.clientDoWithRetries(ctx, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (any, error) {
		return client.UpdateJob(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	// nolint: forcetypeassert
	return out.(*proto.UpdateJobResponse), nil
}

func (p *Server) FailJob(ctx context.Context, in *proto.FailedJob) error {
	_, err := p.clientDoWithRetries(ctx, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (any, error) {
		return client.FailJob(ctx, in)
	})
	return err
}

func (p *Server) CompleteJob(ctx context.Context, in *proto.CompletedJob) error {
	_, err := p.clientDoWithRetries(ctx, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (any, error) {
		return client.CompleteJob(ctx, in)
	})
	return err
}

// isClosed returns whether the API is closed or not.
func (p *Server) isClosed() bool {
	select {
	case <-p.closeContext.Done():
		return true
	default:
		return false
	}
}

// isShutdown returns whether the API is shutdown or not.
func (p *Server) isShutdown() bool {
	select {
	case <-p.shutdown:
		return true
	default:
		return false
	}
}

// Shutdown triggers a graceful exit of each registered provisioner.
// It exits when an active job stops.
func (p *Server) Shutdown(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if !p.isRunningJob() {
		return nil
	}
	p.opts.Logger.Info(ctx, "attempting graceful shutdown")
	close(p.shutdown)
	if p.activeJob == nil {
		return nil
	}
	// wait for active job
	p.activeJob.Cancel()
	select {
	case <-ctx.Done():
		p.opts.Logger.Warn(ctx, "graceful shutdown failed", slog.Error(ctx.Err()))
		return ctx.Err()
	case <-p.activeJob.Done():
		p.opts.Logger.Info(ctx, "gracefully shutdown")
		return nil
	}
}

// Close ends the provisioner. It will mark any running jobs as failed.
func (p *Server) Close() error {
	return p.closeWithError(nil)
}

// closeWithError closes the provisioner; subsequent reads/writes will return the error err.
func (p *Server) closeWithError(err error) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.isClosed() {
		return p.closeError
	}
	p.closeError = err

	errMsg := "provisioner daemon was shutdown gracefully"
	if err != nil {
		errMsg = err.Error()
	}
	if p.activeJob != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		failErr := p.activeJob.Fail(ctx, &proto.FailedJob{Error: errMsg})
		if failErr != nil {
			p.activeJob.ForceStop()
		}
		if err == nil {
			err = failErr
		}
	}

	p.closeCancel()

	p.opts.Logger.Debug(context.Background(), "closing server with error", slog.Error(err))

	return err
}
