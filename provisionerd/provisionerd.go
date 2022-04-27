package provisionerd

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"go.uber.org/atomic"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/retry"
)

const (
	missingParameterErrorText = "missing parameter"
)

// IsMissingParameterError returns whether the error message provided
// is a missing parameter error. This can indicate to consumers that
// they should check parameters.
func IsMissingParameterError(err string) bool {
	return strings.Contains(err, missingParameterErrorText)
}

// Dialer represents the function to create a daemon client connection.
type Dialer func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)

// Provisioners maps provisioner ID to implementation.
type Provisioners map[string]sdkproto.DRPCProvisionerClient

// Options provides customizations to the behavior of a provisioner daemon.
type Options struct {
	Logger slog.Logger

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
	ctx, ctxCancel := context.WithCancel(context.Background())
	daemon := &Server{
		clientDialer: clientDialer,
		opts:         opts,

		closeContext: ctx,
		closeCancel:  ctxCancel,

		shutdown: make(chan struct{}),

		jobRunning: make(chan struct{}),
		jobFailed:  *atomic.NewBool(true),
	}
	// Start off with a closed channel so
	// isRunningJob() returns properly.
	close(daemon.jobRunning)
	go daemon.connect(ctx)
	return daemon
}

type Server struct {
	opts *Options

	clientDialer Dialer
	clientValue  atomic.Value

	// Locked when closing the daemon.
	closeMutex   sync.Mutex
	closeContext context.Context
	closeCancel  context.CancelFunc
	closeError   error

	shutdownMutex sync.Mutex
	shutdown      chan struct{}

	// Locked when acquiring or failing a job.
	jobMutex        sync.Mutex
	jobID           string
	jobRunningMutex sync.Mutex
	jobRunning      chan struct{}
	jobFailed       atomic.Bool
	jobCancel       context.CancelFunc
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

func (p *Server) isRunningJob() bool {
	select {
	case <-p.jobRunning:
		return false
	default:
		return true
	}
}

// Locks a job in the database, and runs it!
func (p *Server) acquireJob(ctx context.Context) {
	p.jobMutex.Lock()
	defer p.jobMutex.Unlock()
	if p.isClosed() {
		return
	}
	if p.isRunningJob() {
		p.opts.Logger.Debug(context.Background(), "skipping acquire; job is already running")
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
	ctx, p.jobCancel = context.WithCancel(ctx)
	p.jobRunningMutex.Lock()
	p.jobRunning = make(chan struct{})
	p.jobRunningMutex.Unlock()
	p.jobFailed.Store(false)
	p.jobID = job.JobId

	p.opts.Logger.Info(context.Background(), "acquired job",
		slog.F("initiator_username", job.UserName),
		slog.F("provisioner", job.Provisioner),
		slog.F("id", job.JobId),
	)

	go p.runJob(ctx, job)
}

func (p *Server) runJob(ctx context.Context, job *proto.AcquiredJob) {
	shutdown, shutdownCancel := context.WithCancel(ctx)
	defer shutdownCancel()

	complete, completeCancel := context.WithCancel(ctx)
	defer completeCancel()
	go func() {
		ticker := time.NewTicker(p.opts.UpdateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-p.closeContext.Done():
				return
			case <-ctx.Done():
				return
			case <-complete.Done():
				return
			case <-p.shutdown:
				p.opts.Logger.Info(ctx, "attempting graceful cancelation")
				shutdownCancel()
				return
			case <-ticker.C:
			}
			client, ok := p.client()
			if !ok {
				continue
			}
			resp, err := client.UpdateJob(ctx, &proto.UpdateJobRequest{
				JobId: job.JobId,
			})
			if errors.Is(err, yamux.ErrSessionShutdown) || errors.Is(err, io.EOF) {
				continue
			}
			if err != nil {
				p.failActiveJobf("send periodic update: %s", err)
				return
			}
			if !resp.Canceled {
				continue
			}
			p.opts.Logger.Info(ctx, "attempting graceful cancelation")
			shutdownCancel()
			// Hard-cancel the job after a minute of pending cancelation.
			timer := time.NewTimer(p.opts.ForceCancelInterval)
			select {
			case <-timer.C:
				p.failActiveJobf("cancelation timed out")
				return
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
	defer func() {
		// Cleanup the work directory after execution.
		for attempt := 0; attempt < 5; attempt++ {
			err := os.RemoveAll(p.opts.WorkDirectory)
			if err != nil {
				// On Windows, open files cannot be removed.
				// When the provisioner daemon is shutting down,
				// it may take a few milliseconds for processes to exit.
				// See: https://github.com/golang/go/issues/50510
				p.opts.Logger.Debug(ctx, "failed to clean work directory; trying again", slog.Error(err))
				time.Sleep(250 * time.Millisecond)
				continue
			}
			p.opts.Logger.Debug(ctx, "cleaned up work directory", slog.Error(err))
			break
		}

		close(p.jobRunning)
	}()
	// It's safe to cast this ProvisionerType. This data is coming directly from coderd.
	provisioner, hasProvisioner := p.opts.Provisioners[job.Provisioner]
	if !hasProvisioner {
		p.failActiveJobf("provisioner %q not registered", job.Provisioner)
		return
	}

	err := os.MkdirAll(p.opts.WorkDirectory, 0700)
	if err != nil {
		p.failActiveJobf("create work directory %q: %s", p.opts.WorkDirectory, err)
		return
	}

	client, ok := p.client()
	if !ok {
		p.failActiveJobf("client disconnected")
		return
	}
	_, err = client.UpdateJob(ctx, &proto.UpdateJobRequest{
		JobId: job.GetJobId(),
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Setting up",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		p.failActiveJobf("write log: %s", err)
		return
	}

	p.opts.Logger.Info(ctx, "unpacking template source archive", slog.F("size_bytes", len(job.TemplateSourceArchive)))
	reader := tar.NewReader(bytes.NewBuffer(job.TemplateSourceArchive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			p.failActiveJobf("read template source archive: %s", err)
			return
		}
		// #nosec
		path := filepath.Join(p.opts.WorkDirectory, header.Name)
		if !strings.HasPrefix(path, filepath.Clean(p.opts.WorkDirectory)) {
			p.failActiveJobf("tar attempts to target relative upper directory")
			return
		}
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0600
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, mode)
			if err != nil {
				p.failActiveJobf("mkdir %q: %s", path, err)
				return
			}
			p.opts.Logger.Debug(context.Background(), "extracted directory", slog.F("path", path))
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
			if err != nil {
				p.failActiveJobf("create file %q (mode %s): %s", path, mode, err)
				return
			}
			// Max file size of 10MB.
			size, err := io.CopyN(file, reader, (1<<20)*10)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				p.failActiveJobf("copy file %q: %s", path, err)
				return
			}
			err = file.Close()
			if err != nil {
				p.failActiveJobf("close file %q: %s", path, err)
				return
			}
			p.opts.Logger.Debug(context.Background(), "extracted file",
				slog.F("size_bytes", size),
				slog.F("path", path),
				slog.F("mode", mode),
			)
		}
	}

	switch jobType := job.Type.(type) {
	case *proto.AcquiredJob_TemplateImport_:
		p.opts.Logger.Debug(context.Background(), "acquired job is template import")

		p.runTemplateImport(ctx, shutdown, provisioner, job)
	case *proto.AcquiredJob_WorkspaceBuild_:
		p.opts.Logger.Debug(context.Background(), "acquired job is workspace provision",
			slog.F("workspace_name", jobType.WorkspaceBuild.WorkspaceName),
			slog.F("state_length", len(jobType.WorkspaceBuild.State)),
			slog.F("parameters", jobType.WorkspaceBuild.ParameterValues),
		)

		p.runWorkspaceBuild(ctx, shutdown, provisioner, job)
	default:
		p.failActiveJobf("unknown job type %q; ensure your provisioner daemon is up-to-date", reflect.TypeOf(job.Type).String())
		return
	}

	client, ok = p.client()
	if !ok {
		return
	}
	// Ensure the job is still running to output.
	// It's possible the job has failed.
	if p.isRunningJob() {
		_, err = client.UpdateJob(ctx, &proto.UpdateJobRequest{
			JobId: job.GetJobId(),
			Logs: []*proto.Log{{
				Source:    proto.LogSource_PROVISIONER_DAEMON,
				Level:     sdkproto.LogLevel_INFO,
				Stage:     "Cleaning Up",
				CreatedAt: time.Now().UTC().UnixMilli(),
			}},
		})
		if err != nil {
			p.failActiveJobf("write log: %s", err)
			return
		}

		p.opts.Logger.Info(context.Background(), "completed job", slog.F("id", job.JobId))
	}
}

func (p *Server) runTemplateImport(ctx, shutdown context.Context, provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob) {
	client, ok := p.client()
	if !ok {
		p.failActiveJobf("client disconnected")
		return
	}
	_, err := client.UpdateJob(ctx, &proto.UpdateJobRequest{
		JobId: job.GetJobId(),
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Parse parameters",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		p.failActiveJobf("write log: %s", err)
		return
	}

	parameterSchemas, err := p.runTemplateImportParse(ctx, provisioner, job)
	if err != nil {
		p.failActiveJobf("run parse: %s", err)
		return
	}

	updateResponse, err := client.UpdateJob(ctx, &proto.UpdateJobRequest{
		JobId:            job.JobId,
		ParameterSchemas: parameterSchemas,
	})
	if err != nil {
		p.failActiveJobf("update job: %s", err)
		return
	}

	valueByName := map[string]*sdkproto.ParameterValue{}
	for _, parameterValue := range updateResponse.ParameterValues {
		valueByName[parameterValue.Name] = parameterValue
	}
	for _, parameterSchema := range parameterSchemas {
		_, ok := valueByName[parameterSchema.Name]
		if !ok {
			p.failActiveJobf("%s: %s", missingParameterErrorText, parameterSchema.Name)
			return
		}
	}

	_, err = client.UpdateJob(ctx, &proto.UpdateJobRequest{
		JobId: job.GetJobId(),
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Detecting persistent resources",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		p.failActiveJobf("write log: %s", err)
		return
	}
	startResources, err := p.runTemplateImportProvision(ctx, shutdown, provisioner, job, updateResponse.ParameterValues, &sdkproto.Provision_Metadata{
		CoderUrl:            job.GetTemplateImport().Metadata.CoderUrl,
		WorkspaceTransition: sdkproto.WorkspaceTransition_START,
	})
	if err != nil {
		p.failActiveJobf("template import provision for start: %s", err)
		return
	}
	_, err = client.UpdateJob(ctx, &proto.UpdateJobRequest{
		JobId: job.GetJobId(),
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Detecting ephemeral resources",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		p.failActiveJobf("write log: %s", err)
		return
	}
	stopResources, err := p.runTemplateImportProvision(ctx, shutdown, provisioner, job, updateResponse.ParameterValues, &sdkproto.Provision_Metadata{
		CoderUrl:            job.GetTemplateImport().Metadata.CoderUrl,
		WorkspaceTransition: sdkproto.WorkspaceTransition_STOP,
	})
	if err != nil {
		p.failActiveJobf("template import provision for start: %s", err)
		return
	}

	p.completeJob(&proto.CompletedJob{
		JobId: job.JobId,
		Type: &proto.CompletedJob_TemplateImport_{
			TemplateImport: &proto.CompletedJob_TemplateImport{
				StartResources: startResources,
				StopResources:  stopResources,
			},
		},
	})
}

// Parses parameter schemas from source.
func (p *Server) runTemplateImportParse(ctx context.Context, provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob) ([]*sdkproto.ParameterSchema, error) {
	client, ok := p.client()
	if !ok {
		return nil, xerrors.New("client disconnected")
	}
	stream, err := provisioner.Parse(ctx, &sdkproto.Parse_Request{
		Directory: p.opts.WorkDirectory,
	})
	if err != nil {
		return nil, xerrors.Errorf("parse source: %w", err)
	}
	defer stream.Close()
	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil, xerrors.Errorf("recv parse source: %w", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Parse_Response_Log:
			p.opts.Logger.Debug(context.Background(), "parse job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)

			_, err = client.UpdateJob(ctx, &proto.UpdateJobRequest{
				JobId: job.JobId,
				Logs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
				}},
			})
			if err != nil {
				return nil, xerrors.Errorf("update job: %w", err)
			}
		case *sdkproto.Parse_Response_Complete:
			p.opts.Logger.Info(context.Background(), "parse complete",
				slog.F("parameter_schemas", msgType.Complete.ParameterSchemas))

			return msgType.Complete.ParameterSchemas, nil
		default:
			return nil, xerrors.Errorf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String())
		}
	}
}

// Performs a dry-run provision when importing a template.
// This is used to detect resources that would be provisioned
// for a workspace in various states.
func (p *Server) runTemplateImportProvision(ctx, shutdown context.Context, provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob, values []*sdkproto.ParameterValue, metadata *sdkproto.Provision_Metadata) ([]*sdkproto.Resource, error) {
	stream, err := provisioner.Provision(ctx)
	if err != nil {
		return nil, xerrors.Errorf("provision: %w", err)
	}
	defer stream.Close()
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-shutdown.Done():
			_ = stream.Send(&sdkproto.Provision_Request{
				Type: &sdkproto.Provision_Request_Cancel{
					Cancel: &sdkproto.Provision_Cancel{},
				},
			})
		}
	}()
	err = stream.Send(&sdkproto.Provision_Request{
		Type: &sdkproto.Provision_Request_Start{
			Start: &sdkproto.Provision_Start{
				Directory:       p.opts.WorkDirectory,
				ParameterValues: values,
				DryRun:          true,
				Metadata:        metadata,
			},
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("start provision: %w", err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil, xerrors.Errorf("recv import provision: %w", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Provision_Response_Log:
			p.opts.Logger.Debug(context.Background(), "template import provision job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)
			client, ok := p.client()
			if !ok {
				continue
			}
			_, err = client.UpdateJob(ctx, &proto.UpdateJobRequest{
				JobId: job.JobId,
				Logs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
				}},
			})
			if err != nil {
				return nil, xerrors.Errorf("send job update: %w", err)
			}
		case *sdkproto.Provision_Response_Complete:
			p.opts.Logger.Info(context.Background(), "parse dry-run provision successful",
				slog.F("resource_count", len(msgType.Complete.Resources)),
				slog.F("resources", msgType.Complete.Resources),
				slog.F("state_length", len(msgType.Complete.State)),
			)

			return msgType.Complete.Resources, nil
		default:
			return nil, xerrors.Errorf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String())
		}
	}
}

func (p *Server) runWorkspaceBuild(ctx, shutdown context.Context, provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob) {
	var stage string
	switch job.GetWorkspaceBuild().Metadata.WorkspaceTransition {
	case sdkproto.WorkspaceTransition_START:
		stage = "Starting workspace"
	case sdkproto.WorkspaceTransition_STOP:
		stage = "Stopping workspace"
	case sdkproto.WorkspaceTransition_DESTROY:
		stage = "Destroying workspace"
	}

	client, ok := p.client()
	if !ok {
		p.failActiveJobf("client disconnected")
		return
	}
	_, err := client.UpdateJob(ctx, &proto.UpdateJobRequest{
		JobId: job.GetJobId(),
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     stage,
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		p.failActiveJobf("write log: %s", err)
		return
	}

	stream, err := provisioner.Provision(ctx)
	if err != nil {
		p.failActiveJobf("provision: %s", err)
		return
	}
	defer stream.Close()
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-shutdown.Done():
			_ = stream.Send(&sdkproto.Provision_Request{
				Type: &sdkproto.Provision_Request_Cancel{
					Cancel: &sdkproto.Provision_Cancel{},
				},
			})
		}
	}()
	err = stream.Send(&sdkproto.Provision_Request{
		Type: &sdkproto.Provision_Request_Start{
			Start: &sdkproto.Provision_Start{
				Directory:       p.opts.WorkDirectory,
				ParameterValues: job.GetWorkspaceBuild().ParameterValues,
				Metadata:        job.GetWorkspaceBuild().Metadata,
				State:           job.GetWorkspaceBuild().State,
			},
		},
	})
	if err != nil {
		p.failActiveJobf("start provision: %s", err)
		return
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			p.failActiveJobf("recv workspace provision: %s", err)
			return
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Provision_Response_Log:
			p.opts.Logger.Debug(context.Background(), "workspace provision job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
				slog.F("workspace_build_id", job.GetWorkspaceBuild().WorkspaceBuildId),
			)

			_, err = client.UpdateJob(ctx, &proto.UpdateJobRequest{
				JobId: job.JobId,
				Logs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
				}},
			})
			if err != nil {
				p.failActiveJobf("send job update: %s", err)
				return
			}
		case *sdkproto.Provision_Response_Complete:
			if msgType.Complete.Error != "" {
				p.opts.Logger.Info(context.Background(), "provision failed; updating state",
					slog.F("state_length", len(msgType.Complete.State)),
				)

				p.failActiveJob(&proto.FailedJob{
					Error: msgType.Complete.Error,
					Type: &proto.FailedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{
							State: msgType.Complete.State,
						},
					},
				})
				return
			}

			p.completeJob(&proto.CompletedJob{
				JobId: job.JobId,
				Type: &proto.CompletedJob_WorkspaceBuild_{
					WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
						State:     msgType.Complete.State,
						Resources: msgType.Complete.Resources,
					},
				},
			})
			p.opts.Logger.Info(context.Background(), "provision successful; marked job as complete",
				slog.F("resource_count", len(msgType.Complete.Resources)),
				slog.F("resources", msgType.Complete.Resources),
				slog.F("state_length", len(msgType.Complete.State)),
			)
			// Stop looping!
			return
		default:
			p.failActiveJobf("invalid message type %T received from provisioner", msg.Type)
			return
		}
	}
}

func (p *Server) completeJob(job *proto.CompletedJob) {
	for retrier := retry.New(25*time.Millisecond, 5*time.Second); retrier.Wait(p.closeContext); {
		client, ok := p.client()
		if !ok {
			continue
		}
		// Complete job may need to be async if we disconnected...
		// When we reconnect we can flush any of these cached values.
		_, err := client.CompleteJob(p.closeContext, job)
		if xerrors.Is(err, yamux.ErrSessionShutdown) || xerrors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			p.opts.Logger.Warn(p.closeContext, "failed to complete job", slog.Error(err))
			p.failActiveJobf(err.Error())
			return
		}
		break
	}
}

func (p *Server) failActiveJobf(format string, args ...interface{}) {
	p.failActiveJob(&proto.FailedJob{
		Error: fmt.Sprintf(format, args...),
	})
}

func (p *Server) failActiveJob(failedJob *proto.FailedJob) {
	p.jobMutex.Lock()
	defer p.jobMutex.Unlock()
	if !p.isRunningJob() {
		if p.isClosed() {
			return
		}
		p.opts.Logger.Info(context.Background(), "skipping job fail; none running", slog.F("error_message", failedJob.Error))
		return
	}
	if p.jobFailed.Load() {
		p.opts.Logger.Warn(context.Background(), "job has already been marked as failed", slog.F("error_messsage", failedJob.Error))
		return
	}
	p.jobFailed.Store(true)
	p.jobCancel()
	p.opts.Logger.Info(context.Background(), "failing running job",
		slog.F("error_message", failedJob.Error),
		slog.F("job_id", p.jobID),
	)
	failedJob.JobId = p.jobID
	for retrier := retry.New(25*time.Millisecond, 5*time.Second); retrier.Wait(p.closeContext); {
		client, ok := p.client()
		if !ok {
			continue
		}
		_, err := client.FailJob(p.closeContext, failedJob)
		if xerrors.Is(err, yamux.ErrSessionShutdown) || xerrors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			if p.isClosed() {
				return
			}
			p.opts.Logger.Warn(context.Background(), "failed to notify of error; job is no longer running", slog.Error(err))
			return
		}
		p.opts.Logger.Debug(context.Background(), "marked running job as failed")
		return
	}
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
	p.shutdownMutex.Lock()
	defer p.shutdownMutex.Unlock()
	if !p.isRunningJob() {
		return nil
	}
	p.opts.Logger.Info(ctx, "attempting graceful shutdown")
	close(p.shutdown)
	select {
	case <-ctx.Done():
		p.opts.Logger.Warn(ctx, "graceful shutdown failed", slog.Error(ctx.Err()))
		return ctx.Err()
	case <-p.jobRunning:
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
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.isClosed() {
		return p.closeError
	}
	p.closeError = err

	errMsg := "provisioner daemon was shutdown gracefully"
	if err != nil {
		errMsg = err.Error()
	}
	p.failActiveJobf(errMsg)
	p.jobRunningMutex.Lock()
	<-p.jobRunning
	p.jobRunningMutex.Unlock()
	p.closeCancel()

	p.opts.Logger.Debug(context.Background(), "closing server with error", slog.Error(err))

	return err
}
