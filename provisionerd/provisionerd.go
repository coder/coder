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

	"cdr.dev/slog"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/retry"
)

// Dialer represents the function to create a daemon client connection.
type Dialer func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)

// Provisioners maps provisioner ID to implementation.
type Provisioners map[string]sdkproto.DRPCProvisionerClient

// Options provides customizations to the behavior of a provisioner daemon.
type Options struct {
	Logger slog.Logger

	UpdateInterval time.Duration
	PollInterval   time.Duration
	Provisioners   Provisioners
	WorkDirectory  string
}

// New creates and starts a provisioner daemon.
func New(clientDialer Dialer, opts *Options) io.Closer {
	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.UpdateInterval == 0 {
		opts.UpdateInterval = 5 * time.Second
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	daemon := &provisionerDaemon{
		clientDialer: clientDialer,
		opts:         opts,

		closeCancel: ctxCancel,
		closed:      make(chan struct{}),

		jobRunning: make(chan struct{}),
	}
	// Start off with a closed channel so
	// isRunningJob() returns properly.
	close(daemon.jobRunning)
	go daemon.connect(ctx)
	return daemon
}

type provisionerDaemon struct {
	opts *Options

	clientDialer Dialer
	client       proto.DRPCProvisionerDaemonClient
	updateStream proto.DRPCProvisionerDaemon_UpdateJobClient

	// Locked when closing the daemon.
	closeMutex  sync.Mutex
	closeCancel context.CancelFunc
	closed      chan struct{}
	closeError  error

	// Locked when acquiring or canceling a job.
	jobMutex   sync.Mutex
	jobID      string
	jobRunning chan struct{}
	jobCancel  context.CancelFunc
}

// Connect establishes a connection to coderd.
func (p *provisionerDaemon) connect(ctx context.Context) {
	var err error
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		p.client, err = p.clientDialer(ctx)
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
		p.updateStream, err = p.client.UpdateJob(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if p.isClosed() {
				return
			}
			p.opts.Logger.Warn(context.Background(), "create update job stream", slog.Error(err))
			continue
		}
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
		select {
		case <-p.closed:
			return
		case <-p.updateStream.Context().Done():
			// We use the update stream to detect when the connection
			// has been interrupted. This works well, because logs need
			// to buffer if a job is running in the background.
			p.opts.Logger.Debug(context.Background(), "update stream ended", slog.Error(p.updateStream.Context().Err()))
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
			select {
			case <-p.closed:
				return
			case <-p.updateStream.Context().Done():
				return
			case <-ticker.C:
				p.acquireJob(ctx)
			}
		}
	}()
}

func (p *provisionerDaemon) isRunningJob() bool {
	select {
	case <-p.jobRunning:
		return false
	default:
		return true
	}
}

// Locks a job in the database, and runs it!
func (p *provisionerDaemon) acquireJob(ctx context.Context) {
	p.jobMutex.Lock()
	defer p.jobMutex.Unlock()
	if p.isClosed() {
		return
	}
	if p.isRunningJob() {
		p.opts.Logger.Debug(context.Background(), "skipping acquire; job is already running")
		return
	}
	var err error
	job, err := p.client.AcquireJob(ctx, &proto.Empty{})
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
		p.opts.Logger.Debug(context.Background(), "no jobs available")
		return
	}
	ctx, p.jobCancel = context.WithCancel(ctx)
	p.jobRunning = make(chan struct{})
	p.jobID = job.JobId

	p.opts.Logger.Info(context.Background(), "acquired job",
		slog.F("organization_name", job.OrganizationName),
		slog.F("project_name", job.ProjectName),
		slog.F("username", job.UserName),
		slog.F("provisioner", job.Provisioner),
		slog.F("id", job.JobId),
	)

	go p.runJob(ctx, job)
}

func (p *provisionerDaemon) runJob(ctx context.Context, job *proto.AcquiredJob) {
	go func() {
		ticker := time.NewTicker(p.opts.UpdateInterval)
		defer ticker.Stop()
		select {
		case <-p.closed:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := p.updateStream.Send(&proto.JobUpdate{
				JobId: job.JobId,
			})
			if err != nil {
				go p.cancelActiveJobf("send periodic update: %s", err)
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
		go p.cancelActiveJobf("provisioner %q not registered", job.Provisioner)
		return
	}

	err := os.MkdirAll(p.opts.WorkDirectory, 0700)
	if err != nil {
		go p.cancelActiveJobf("create work directory %q: %s", p.opts.WorkDirectory, err)
		return
	}

	p.opts.Logger.Info(ctx, "unpacking project source archive", slog.F("size_bytes", len(job.ProjectSourceArchive)))
	reader := tar.NewReader(bytes.NewBuffer(job.ProjectSourceArchive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			go p.cancelActiveJobf("read project source archive: %s", err)
			return
		}
		// #nosec
		path := filepath.Join(p.opts.WorkDirectory, header.Name)
		if !strings.HasPrefix(path, filepath.Clean(p.opts.WorkDirectory)) {
			go p.cancelActiveJobf("tar attempts to target relative upper directory")
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
				go p.cancelActiveJobf("mkdir %q: %s", path, err)
				return
			}
			p.opts.Logger.Debug(context.Background(), "extracted directory", slog.F("path", path))
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
			if err != nil {
				go p.cancelActiveJobf("create file %q (mode %s): %s", path, mode, err)
				return
			}
			// Max file size of 10MB.
			size, err := io.CopyN(file, reader, (1<<20)*10)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				go p.cancelActiveJobf("copy file %q: %s", path, err)
				return
			}
			err = file.Close()
			if err != nil {
				go p.cancelActiveJobf("close file %q: %s", path, err)
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
	case *proto.AcquiredJob_ProjectImport_:
		p.opts.Logger.Debug(context.Background(), "acquired job is project import",
			slog.F("project_version_name", jobType.ProjectImport.ProjectVersionName),
		)

		p.runProjectImport(ctx, provisioner, job)
	case *proto.AcquiredJob_WorkspaceProvision_:
		p.opts.Logger.Debug(context.Background(), "acquired job is workspace provision",
			slog.F("workspace_name", jobType.WorkspaceProvision.WorkspaceName),
			slog.F("state_length", len(jobType.WorkspaceProvision.State)),
			slog.F("parameters", jobType.WorkspaceProvision.ParameterValues),
		)

		p.runWorkspaceProvision(ctx, provisioner, job)
	default:
		go p.cancelActiveJobf("unknown job type %q; ensure your provisioner daemon is up-to-date", reflect.TypeOf(job.Type).String())
		return
	}

	// Ensure the job is still running to output.
	// It's possible the job was canceled.
	if p.isRunningJob() {
		p.opts.Logger.Info(context.Background(), "completed job", slog.F("id", job.JobId))
	}
}

func (p *provisionerDaemon) runProjectImport(ctx context.Context, provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob) {
	stream, err := provisioner.Parse(ctx, &sdkproto.Parse_Request{
		Directory: p.opts.WorkDirectory,
	})
	if err != nil {
		go p.cancelActiveJobf("parse source: %s", err)
		return
	}
	defer stream.Close()
	for {
		msg, err := stream.Recv()
		if err != nil {
			go p.cancelActiveJobf("recv parse source: %s", err)
			return
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Parse_Response_Log:
			p.opts.Logger.Debug(context.Background(), "parse job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
				slog.F("project_version_id", job.GetProjectImport().ProjectVersionId),
			)

			err = p.updateStream.Send(&proto.JobUpdate{
				JobId: job.JobId,
				ProjectImportLogs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
				}},
			})
			if err != nil {
				go p.cancelActiveJobf("update job: %s", err)
				return
			}
		case *sdkproto.Parse_Response_Complete:
			p.opts.Logger.Info(context.Background(), "parse job complete",
				slog.F("parameter_schemas", msgType.Complete.ParameterSchemas))

			_, err = p.client.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.JobId,
				Type: &proto.CompletedJob_ProjectImport_{
					ProjectImport: &proto.CompletedJob_ProjectImport{
						ParameterSchemas: msgType.Complete.ParameterSchemas,
					},
				},
			})
			if err != nil {
				go p.cancelActiveJobf("complete job: %s", err)
				return
			}
			// Return so we stop looping!
			return
		default:
			go p.cancelActiveJobf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String())
			return
		}
	}
}

func (p *provisionerDaemon) runWorkspaceProvision(ctx context.Context, provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob) {
	stream, err := provisioner.Provision(ctx, &sdkproto.Provision_Request{
		Directory:       p.opts.WorkDirectory,
		ParameterValues: job.GetWorkspaceProvision().ParameterValues,
		State:           job.GetWorkspaceProvision().State,
	})
	if err != nil {
		go p.cancelActiveJobf("provision: %s", err)
		return
	}
	defer stream.Close()

	for {
		msg, err := stream.Recv()
		if err != nil {
			go p.cancelActiveJobf("recv workspace provision: %s", err)
			return
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Provision_Response_Log:
			p.opts.Logger.Debug(context.Background(), "workspace provision job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
				slog.F("workspace_history_id", job.GetWorkspaceProvision().WorkspaceHistoryId),
			)

			err = p.updateStream.Send(&proto.JobUpdate{
				JobId: job.JobId,
				WorkspaceProvisionLogs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
				}},
			})
			if err != nil {
				go p.cancelActiveJobf("send job update: %s", err)
				return
			}
		case *sdkproto.Provision_Response_Complete:
			p.opts.Logger.Info(context.Background(), "provision successful; marking job as complete",
				slog.F("resource_count", len(msgType.Complete.Resources)),
				slog.F("resources", msgType.Complete.Resources),
				slog.F("state_length", len(msgType.Complete.State)),
			)

			// Complete job may need to be async if we disconnected...
			// When we reconnect we can flush any of these cached values.
			_, err = p.client.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.JobId,
				Type: &proto.CompletedJob_WorkspaceProvision_{
					WorkspaceProvision: &proto.CompletedJob_WorkspaceProvision{
						State:     msgType.Complete.State,
						Resources: msgType.Complete.Resources,
					},
				},
			})
			if err != nil {
				go p.cancelActiveJobf("complete job: %s", err)
				return
			}
			// Return so we stop looping!
			return
		default:
			go p.cancelActiveJobf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String())
			return
		}
	}
}

func (p *provisionerDaemon) cancelActiveJobf(format string, args ...interface{}) {
	p.jobMutex.Lock()
	defer p.jobMutex.Unlock()
	errMsg := fmt.Sprintf(format, args...)
	if !p.isRunningJob() {
		if p.isClosed() {
			// We don't want to log if we're already closed!
			return
		}
		p.opts.Logger.Warn(context.Background(), "skipping job cancel; none running", slog.F("error_message", errMsg))
		return
	}
	p.jobCancel()
	p.opts.Logger.Info(context.Background(), "canceling running job",
		slog.F("error_message", errMsg),
		slog.F("job_id", p.jobID),
	)
	_, err := p.client.CancelJob(context.Background(), &proto.CancelledJob{
		JobId: p.jobID,
		Error: fmt.Sprintf("provisioner daemon: %s", errMsg),
	})
	if err != nil {
		p.opts.Logger.Warn(context.Background(), "failed to notify of cancel; job is no longer running", slog.Error(err))
		return
	}
	<-p.jobRunning
	p.opts.Logger.Debug(context.Background(), "canceled running job")
}

// isClosed returns whether the API is closed or not.
func (p *provisionerDaemon) isClosed() bool {
	select {
	case <-p.closed:
		return true
	default:
		return false
	}
}

// Close ends the provisioner. It will mark any running jobs as canceled.
func (p *provisionerDaemon) Close() error {
	return p.closeWithError(nil)
}

// closeWithError closes the provisioner; subsequent reads/writes will return the error err.
func (p *provisionerDaemon) closeWithError(err error) error {
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.isClosed() {
		return p.closeError
	}
	p.closeError = err
	close(p.closed)

	errMsg := "provisioner daemon was shutdown gracefully"
	if err != nil {
		errMsg = err.Error()
	}
	p.cancelActiveJobf(errMsg)
	p.closeCancel()

	p.opts.Logger.Debug(context.Background(), "closing server with error", slog.Error(err))

	return err
}
