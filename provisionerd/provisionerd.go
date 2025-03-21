package provisionerd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/valyala/fasthttp/fasthttputil"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionerd/runner"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/retry"
)

// Dialer represents the function to create a daemon client connection.
type Dialer func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)

// ConnectResponse is the response returned asynchronously from Connector.Connect
// containing either the Provisioner Client or an Error.  The Job is also returned
// unaltered to disambiguate responses if the respCh is shared among multiple jobs
type ConnectResponse struct {
	Job    *proto.AcquiredJob
	Client sdkproto.DRPCProvisionerClient
	Error  error
}

// Connector allows the provisioner daemon to Connect to a provisioner
// for the given job.
type Connector interface {
	// Connect to the correct provisioner for the given job. The response is
	// delivered asynchronously over the respCh.  If the provided context expires,
	// the Connector may stop waiting for the provisioner and return an error
	// response.
	Connect(ctx context.Context, job *proto.AcquiredJob, respCh chan<- ConnectResponse)
}

// Options provides customizations to the behavior of a provisioner daemon.
type Options struct {
	Logger         slog.Logger
	TracerProvider trace.TracerProvider
	Metrics        *Metrics

	ExternalProvisioner bool
	ForceCancelInterval time.Duration
	UpdateInterval      time.Duration
	LogBufferInterval   time.Duration
	Connector           Connector
	InitConnectionCh    chan struct{} // only to be used in tests
}

// New creates and starts a provisioner daemon.
func New(clientDialer Dialer, opts *Options) *Server {
	if opts == nil {
		opts = &Options{}
	}
	if opts.UpdateInterval == 0 {
		opts.UpdateInterval = 5 * time.Second
	}
	if opts.ForceCancelInterval == 0 {
		opts.ForceCancelInterval = 10 * time.Minute
	}
	if opts.LogBufferInterval == 0 {
		opts.LogBufferInterval = 250 * time.Millisecond
	}
	if opts.TracerProvider == nil {
		opts.TracerProvider = trace.NewNoopTracerProvider()
	}
	if opts.Metrics == nil {
		reg := prometheus.NewRegistry()
		mets := NewMetrics(reg)
		opts.Metrics = &mets
	}
	if opts.InitConnectionCh == nil {
		opts.InitConnectionCh = make(chan struct{})
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	daemon := &Server{
		opts:   opts,
		tracer: opts.TracerProvider.Tracer(tracing.TracerName),

		clientDialer: clientDialer,
		clientCh:     make(chan proto.DRPCProvisionerDaemonClient),

		closeContext:        ctx,
		closeCancel:         ctxCancel,
		closedCh:            make(chan struct{}),
		shuttingDownCh:      make(chan struct{}),
		acquireDoneCh:       make(chan struct{}),
		initConnectionCh:    opts.InitConnectionCh,
		externalProvisioner: opts.ExternalProvisioner,
	}

	daemon.wg.Add(2)
	go daemon.connect()
	go daemon.acquireLoop()
	return daemon
}

type Server struct {
	opts   *Options
	tracer trace.Tracer

	clientDialer Dialer
	clientCh     chan proto.DRPCProvisionerDaemonClient

	wg sync.WaitGroup

	// initConnectionCh will receive when the daemon connects to coderd for the
	// first time.
	initConnectionCh   chan struct{}
	initConnectionOnce sync.Once

	// mutex protects all subsequent fields
	mutex sync.Mutex
	// closeContext is canceled when we start closing.
	closeContext context.Context
	closeCancel  context.CancelFunc
	// closeError stores the error when closing to return to subsequent callers
	closeError error
	// closingB is set to true when we start closing
	closingB bool
	// closedCh will receive when we complete closing
	closedCh chan struct{}
	// shuttingDownB is set to true when we start graceful shutdown
	shuttingDownB bool
	// shuttingDownCh will receive when we start graceful shutdown
	shuttingDownCh chan struct{}
	// acquireDoneCh will receive when the acquireLoop exits
	acquireDoneCh       chan struct{}
	activeJob           *runner.Runner
	externalProvisioner bool
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
			NumDaemons: auto.NewGauge(prometheus.GaugeOpts{
				Namespace: "coderd",
				Subsystem: "provisionerd",
				Name:      "num_daemons",
				Help:      "The number of provisioner daemons.",
			}),
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
			WorkspaceBuilds: auto.NewCounterVec(prometheus.CounterOpts{
				Namespace: "coderd",
				Subsystem: "", // Explicitly empty to make this a top-level metric.
				Name:      "workspace_builds_total",
				Help:      "The number of workspaces started, updated, or deleted.",
			}, []string{"workspace_owner", "workspace_name", "template_name", "template_version", "workspace_transition", "status"}),
			WorkspaceBuildTimings: auto.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "coderd",
				Subsystem: "provisionerd",
				Name:      "workspace_build_timings_seconds",
				Help:      "The time taken for a workspace to build.",
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
			}, []string{"template_name", "template_version", "workspace_transition", "status"}),
		},
	}
}

// Connect establishes a connection to coderd.
func (p *Server) connect() {
	defer p.opts.Logger.Debug(p.closeContext, "connect loop exited")
	defer p.wg.Done()
	logConnect := p.opts.Logger.Debug
	if p.externalProvisioner {
		logConnect = p.opts.Logger.Info
	}
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
connectLoop:
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(p.closeContext); {
		// It's possible for the provisioner daemon to be shut down
		// before the wait is complete!
		if p.isClosed() {
			return
		}
		p.opts.Logger.Debug(p.closeContext, "dialing coderd")
		client, err := p.clientDialer(p.closeContext)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			var sdkErr *codersdk.Error
			// If something is wrong with our auth, stop trying to connect.
			if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusForbidden {
				p.opts.Logger.Error(p.closeContext, "not authorized to dial coderd", slog.Error(err))
				return
			}
			if p.isClosed() {
				return
			}
			p.opts.Logger.Warn(p.closeContext, "coderd client failed to dial", slog.Error(err))
			continue
		}
		// This log is useful to verify that an external provisioner daemon is
		// successfully connecting to coderd. It doesn't add much value if the
		// daemon is built-in, so we only log it on the info level if p.externalProvisioner
		// is true. This log message is mentioned in the docs:
		// https://github.com/coder/coder/blob/5bd86cb1c06561d1d3e90ce689da220467e525c0/docs/admin/provisioners.md#L346
		logConnect(p.closeContext, "successfully connected to coderd")
		retrier.Reset()
		p.initConnectionOnce.Do(func() {
			close(p.initConnectionCh)
		})

		// serve the client until we are closed or it disconnects
		for {
			select {
			case <-p.closeContext.Done():
				client.DRPCConn().Close()
				return
			case <-client.DRPCConn().Closed():
				logConnect(p.closeContext, "connection to coderd closed")
				continue connectLoop
			case p.clientCh <- client:
				continue
			}
		}
	}
}

func (p *Server) client() (proto.DRPCProvisionerDaemonClient, bool) {
	select {
	case <-p.closeContext.Done():
		return nil, false
	case <-p.shuttingDownCh:
		// Shutting down should return a nil client and unblock
		return nil, false
	case client := <-p.clientCh:
		return client, true
	}
}

func (p *Server) acquireLoop() {
	defer p.opts.Logger.Debug(p.closeContext, "acquire loop exited")
	defer p.wg.Done()
	defer func() { close(p.acquireDoneCh) }()
	ctx := p.closeContext
	for {
		if p.acquireExit() {
			return
		}
		client, ok := p.client()
		if !ok {
			p.opts.Logger.Debug(ctx, "shut down before client (re) connected")
			return
		}
		p.acquireAndRunOne(client)
	}
}

// acquireExit returns true if the acquire loop should exit
func (p *Server) acquireExit() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.closingB {
		p.opts.Logger.Debug(p.closeContext, "exiting acquire; provisionerd is closing")
		return true
	}
	if p.shuttingDownB {
		p.opts.Logger.Debug(p.closeContext, "exiting acquire; provisionerd is shutting down")
		return true
	}
	return false
}

func (p *Server) acquireAndRunOne(client proto.DRPCProvisionerDaemonClient) {
	ctx := p.closeContext
	p.opts.Logger.Debug(ctx, "start of acquireAndRunOne")
	job, err := p.acquireGraceful(client)
	p.opts.Logger.Debug(ctx, "graceful acquire done", slog.F("job_id", job.GetJobId()), slog.Error(err))
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, yamux.ErrSessionShutdown) ||
			errors.Is(err, fasthttputil.ErrInmemoryListenerClosed) {
			return
		}

		p.opts.Logger.Warn(ctx, "provisionerd was unable to acquire job", slog.Error(err))
		return
	}
	if job.JobId == "" {
		p.opts.Logger.Debug(ctx, "acquire job successfully canceled")
		return
	}

	if len(job.TraceMetadata) > 0 {
		ctx = tracing.MetadataToContext(ctx, job.TraceMetadata)
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

	fields := []any{
		slog.F("initiator_username", job.UserName),
		slog.F("provisioner", job.Provisioner),
		slog.F("job_id", job.JobId),
	}

	if build := job.GetWorkspaceBuild(); build != nil {
		fields = append(fields,
			slog.F("workspace_transition", build.Metadata.WorkspaceTransition.String()),
			slog.F("workspace_owner", build.Metadata.WorkspaceOwner),
			slog.F("template_name", build.Metadata.TemplateName),
			slog.F("template_version", build.Metadata.TemplateVersion),
			slog.F("workspace_build_id", build.WorkspaceBuildId),
			slog.F("workspace_id", build.Metadata.WorkspaceId),
			slog.F("workspace_name", build.WorkspaceName),
			slog.F("is_prebuild", build.Metadata.IsPrebuild),
		)

		span.SetAttributes(
			attribute.String("workspace_build_id", build.WorkspaceBuildId),
			attribute.String("workspace_id", build.Metadata.WorkspaceId),
			attribute.String("workspace_name", build.WorkspaceName),
			attribute.String("workspace_owner_id", build.Metadata.WorkspaceOwnerId),
			attribute.String("workspace_owner", build.Metadata.WorkspaceOwner),
			attribute.String("workspace_transition", build.Metadata.WorkspaceTransition.String()),
			attribute.Bool("is_prebuild", build.Metadata.IsPrebuild),
		)
	}

	p.opts.Logger.Debug(ctx, "acquired job", fields...)

	respCh := make(chan ConnectResponse)
	p.opts.Connector.Connect(ctx, job, respCh)
	resp := <-respCh
	if resp.Error != nil {
		err := p.FailJob(ctx, &proto.FailedJob{
			JobId: job.JobId,
			Error: fmt.Sprintf("failed to connect to provisioner: %s", resp.Error),
		})
		if err != nil {
			p.opts.Logger.Error(ctx, "provisioner job failed", slog.F("job_id", job.JobId), slog.Error(err))
		}
		return
	}

	p.mutex.Lock()
	p.activeJob = runner.New(
		ctx,
		job,
		runner.Options{
			Updater:             p,
			QuotaCommitter:      p,
			Logger:              p.opts.Logger.Named("runner"),
			Provisioner:         resp.Client,
			UpdateInterval:      p.opts.UpdateInterval,
			ForceCancelInterval: p.opts.ForceCancelInterval,
			LogDebounceInterval: p.opts.LogBufferInterval,
			Tracer:              p.tracer,
			Metrics:             p.opts.Metrics.Runner,
		},
	)
	p.mutex.Unlock()
	p.activeJob.Run()
	p.mutex.Lock()
	p.activeJob = nil
	p.mutex.Unlock()
}

// acquireGraceful attempts to acquire a job from the server, handling canceling the acquisition if we gracefully shut
// down.
func (p *Server) acquireGraceful(client proto.DRPCProvisionerDaemonClient) (*proto.AcquiredJob, error) {
	stream, err := client.AcquireJobWithCancel(p.closeContext)
	if err != nil {
		return nil, err
	}
	acquireDone := make(chan struct{})
	go func() {
		select {
		case <-p.closeContext.Done():
			return
		case <-p.shuttingDownCh:
			p.opts.Logger.Debug(p.closeContext, "sending acquire job cancel")
			err := stream.Send(&proto.CancelAcquire{})
			if err != nil {
				p.opts.Logger.Warn(p.closeContext, "failed to gracefully cancel acquire job")
			}
			return
		case <-acquireDone:
			return
		}
	}()
	job, err := stream.Recv()
	close(acquireDone)
	return job, err
}

func retryable(err error) bool {
	return xerrors.Is(err, yamux.ErrSessionShutdown) || xerrors.Is(err, io.EOF) || xerrors.Is(err, fasthttputil.ErrInmemoryListenerClosed) ||
		// annoyingly, dRPC sometimes returns context.Canceled if the transport was closed, even if the context for
		// the RPC *is not canceled*.  Retrying is fine if the RPC context is not canceled.
		xerrors.Is(err, context.Canceled)
}

// clientDoWithRetries runs the function f with a client, and retries with
// backoff until either the error returned is not retryable() or the context
// expires.
func clientDoWithRetries[T any](ctx context.Context,
	getClient func() (proto.DRPCProvisionerDaemonClient, bool),
	f func(context.Context, proto.DRPCProvisionerDaemonClient) (T, error),
) (ret T, _ error) {
	for retrier := retry.New(25*time.Millisecond, 5*time.Second); retrier.Wait(ctx); {
		client, ok := getClient()
		if !ok {
			continue
		}
		resp, err := f(ctx, client)
		if retryable(err) {
			continue
		}
		return resp, err
	}
	return ret, ctx.Err()
}

func (p *Server) CommitQuota(ctx context.Context, in *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error) {
	out, err := clientDoWithRetries(ctx, p.client, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (*proto.CommitQuotaResponse, error) {
		return client.CommitQuota(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Server) UpdateJob(ctx context.Context, in *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	out, err := clientDoWithRetries(ctx, p.client, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (*proto.UpdateJobResponse, error) {
		return client.UpdateJob(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Server) FailJob(ctx context.Context, in *proto.FailedJob) error {
	_, err := clientDoWithRetries(ctx, p.client, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (*proto.Empty, error) {
		return client.FailJob(ctx, in)
	})
	return err
}

func (p *Server) CompleteJob(ctx context.Context, in *proto.CompletedJob) error {
	_, err := clientDoWithRetries(ctx, p.client, func(ctx context.Context, client proto.DRPCProvisionerDaemonClient) (*proto.Empty, error) {
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

// Shutdown gracefully exists with the option to cancel the active job.
// If false, it will wait for the job to complete.
//
//nolint:revive
func (p *Server) Shutdown(ctx context.Context, cancelActiveJob bool) error {
	p.mutex.Lock()
	p.opts.Logger.Info(ctx, "attempting graceful shutdown")
	if !p.shuttingDownB {
		close(p.shuttingDownCh)
		p.shuttingDownB = true
	}
	if cancelActiveJob && p.activeJob != nil {
		p.activeJob.Cancel()
	}
	p.mutex.Unlock()
	select {
	case <-ctx.Done():
		p.opts.Logger.Warn(ctx, "graceful shutdown failed", slog.Error(ctx.Err()))
		return ctx.Err()
	case <-p.acquireDoneCh:
		p.opts.Logger.Info(ctx, "gracefully shutdown")
		return nil
	}
}

// Close ends the provisioner. It will mark any running jobs as failed.
func (p *Server) Close() error {
	p.opts.Logger.Info(p.closeContext, "closing provisionerd")
	return p.closeWithError(nil)
}

// closeWithError closes the provisioner; subsequent reads/writes will return the error err.
func (p *Server) closeWithError(err error) error {
	p.mutex.Lock()
	var activeJob *runner.Runner
	first := false
	if !p.closingB {
		first = true
		p.closingB = true
		// only the first caller to close should attempt to fail the active job
		activeJob = p.activeJob
	}
	// don't hold the mutex while doing I/O.
	p.mutex.Unlock()
	if activeJob != nil {
		errMsg := "provisioner daemon was shutdown gracefully"
		if err != nil {
			errMsg = err.Error()
		}
		p.opts.Logger.Debug(p.closeContext, "failing active job because of close")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		failErr := activeJob.Fail(ctx, &proto.FailedJob{Error: errMsg})
		if failErr != nil {
			activeJob.ForceStop()
		}
		if err == nil {
			err = failErr
		}
	}

	if first {
		p.closeCancel()
		p.opts.Logger.Debug(context.Background(), "waiting for goroutines to exit")
		p.wg.Wait()
		p.opts.Logger.Debug(context.Background(), "closing server with error", slog.Error(err))
		p.closeError = err
		close(p.closedCh)
		return err
	}
	p.opts.Logger.Debug(p.closeContext, "waiting for first closer to complete")
	<-p.closedCh
	p.opts.Logger.Debug(p.closeContext, "first closer completed")
	return p.closeError
}
