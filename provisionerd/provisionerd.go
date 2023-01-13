package provisionerd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/spf13/afero"
	"github.com/valyala/fasthttp/fasthttputil"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/cryptorand"
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
	Filesystem     afero.Fs
	Logger         slog.Logger
	TracerProvider trace.TracerProvider
	Metrics        *Metrics

	ForceCancelInterval time.Duration
	UpdateInterval      time.Duration
	LogBufferInterval   time.Duration
	JobPollInterval     time.Duration
	JobPollJitter       time.Duration
	JobPollDebounce     time.Duration
	Provisioners        Provisioners
	// WorkDirectory must not be used by multiple processes at once.
	WorkDirectory string
}

// New creates and starts a provisioner daemon.
func New(clientDialer Dialer, opts *Options) *Server {
	if opts == nil {
		opts = &Options{}
	}
	if opts.JobPollInterval == 0 {
		opts.JobPollInterval = 5 * time.Second
	}
	if opts.JobPollJitter == 0 {
		opts.JobPollJitter = time.Second
	}
	if opts.UpdateInterval == 0 {
		opts.UpdateInterval = 5 * time.Second
	}
	if opts.ForceCancelInterval == 0 {
		opts.ForceCancelInterval = 10 * time.Minute
	}
	if opts.LogBufferInterval == 0 {
		opts.LogBufferInterval = 50 * time.Millisecond
	}
	if opts.Filesystem == nil {
		opts.Filesystem = afero.NewOsFs()
	}
	if opts.TracerProvider == nil {
		opts.TracerProvider = trace.NewNoopTracerProvider()
	}
	if opts.Metrics == nil {
		reg := prometheus.NewRegistry()
		mets := NewMetrics(reg)
		opts.Metrics = &mets
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	daemon := &Server{
		opts:   opts,
		tracer: opts.TracerProvider.Tracer(tracing.TracerName),

		clientDialer: clientDialer,

		closeContext: ctx,
		closeCancel:  ctxCancel,

		shutdown: make(chan struct{}),
	}

	go daemon.connect(ctx)
	return daemon
}

type Server struct {
	opts   *Options
	tracer trace.Tracer

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

type Metrics struct {
	Runner runner.Metrics
}

func NewMetrics(reg prometheus.Registerer) Metrics {
	auto := promauto.With(reg)

	return Metrics{
		Runner: runner.Metrics{
			ConcurrentJobs: auto.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "coderd",
				Subsystem: "provisionerd",
				Name:      "jobs_current",
				Help:      "The number of currently running provisioner jobs.",
			}, []string{"provisioner"}),
			JobTimings: auto.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "coderd",
				Subsystem: "provisionerd",
				Name:      "job_timings_seconds",
				Help:      "The provisioner job time duration in seconds.",
				Buckets: []float64{
					1, // 1s
					10,
					30,
					60, // 1min
					60 * 5,
					60 * 10,
					60 * 30, // 30min
					60 * 60, // 1hr
				},
			}, []string{"provisioner", "status"}),
		},
	}
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
		timer := time.NewTimer(p.opts.JobPollInterval)
		defer timer.Stop()
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
			case <-timer.C:
				p.acquireJob(ctx)
				timer.Reset(p.nextInterval())
			}
		}
	}()
}

func (p *Server) nextInterval() time.Duration {
	r, err := cryptorand.Float64()
	if err != nil {
		panic("get random float:" + err.Error())
	}

	return p.opts.JobPollInterval + time.Duration(float64(p.opts.JobPollJitter)*r)
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

var (
	lastAcquire      time.Time
	lastAcquireMutex sync.RWMutex
)

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

	// This prevents loads of provisioner daemons from consistently sending
	// requests when no jobs are available.
	//
	// The debounce only occurs when no job is returned, so if loads of jobs are
	// added at once, they will start after at most this duration.
	lastAcquireMutex.RLock()
	if !lastAcquire.IsZero() && time.Since(lastAcquire) < p.opts.JobPollDebounce {
		lastAcquireMutex.RUnlock()
		return
	}
	lastAcquireMutex.RUnlock()

	var err error
	client, ok := p.client()
	if !ok {
		return
	}

	job, err := client.AcquireJob(ctx, &proto.Empty{})
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, yamux.ErrSessionShutdown) ||
			errors.Is(err, fasthttputil.ErrInmemoryListenerClosed) {
			return
		}

		p.opts.Logger.Warn(ctx, "acquire job", slog.Error(err))
		return
	}
	if job.JobId == "" {
		lastAcquireMutex.Lock()
		lastAcquire = time.Now()
		lastAcquireMutex.Unlock()
		return
	}

	ctx, span := p.tracer.Start(ctx, tracing.FuncName(), trace.WithAttributes(
		semconv.ServiceNameKey.String("coderd.provisionerd"),
		attribute.String("job_id", job.JobId),
		attribute.String("job_type", reflect.TypeOf(job.GetType()).Elem().Name()),
		attribute.Int64("job_created_at", job.CreatedAt),
		attribute.String("initiator_username", job.UserName),
		attribute.String("provisioner", job.Provisioner),
		attribute.Int("template_size_bytes", len(job.TemplateSourceArchive)),
	))
	defer span.End()

	if build := job.GetWorkspaceBuild(); build != nil {
		span.SetAttributes(
			attribute.String("workspace_build_id", build.WorkspaceBuildId),
			attribute.String("workspace_id", build.Metadata.WorkspaceId),
			attribute.String("workspace_name", build.WorkspaceName),
			attribute.String("workspace_owner_id", build.Metadata.WorkspaceOwnerId),
			attribute.String("workspace_owner", build.Metadata.WorkspaceOwner),
			attribute.String("workspace_transition", build.Metadata.WorkspaceTransition.String()),
		)
	}

	p.opts.Logger.Info(ctx, "acquired job",
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
			p.opts.Logger.Error(ctx, "fail job", slog.F("job_id", job.JobId), slog.Error(err))
		}
		return
	}

	p.activeJob = runner.New(
		ctx,
		job,
		runner.Options{
			Updater:             p,
			QuotaCommitter:      p,
			Logger:              p.opts.Logger,
			Filesystem:          p.opts.Filesystem,
			WorkDirectory:       p.opts.WorkDirectory,
			Provisioner:         provisioner,
			UpdateInterval:      p.opts.UpdateInterval,
			ForceCancelInterval: p.opts.ForceCancelInterval,
			LogDebounceInterval: p.opts.LogBufferInterval,
			Tracer:              p.tracer,
			Metrics:             p.opts.Metrics.Runner,
		},
	)

	go p.activeJob.Run()
}

func retryable(err error) bool {
	return xerrors.Is(err, yamux.ErrSessionShutdown) || xerrors.Is(err, io.EOF) || xerrors.Is(err, fasthttputil.ErrInmemoryListenerClosed) ||
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

func (p *Server) CommitQuota(ctx context.Context, in *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error) {
	out, err := p.clientDoWithRetries(ctx, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (any, error) {
		return client.CommitQuota(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	// nolint: forcetypeassert
	return out.(*proto.CommitQuotaResponse), nil
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
