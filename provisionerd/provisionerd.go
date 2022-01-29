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

	"cdr.dev/slog"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/retry"
)

// Dialer returns a gRPC client to communicate with.
// The provisioner daemon handles intermittent connection failures
// for upgrades to coderd.
type Dialer func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)

// Provisioners maps provisioner ID to implementation.
type Provisioners map[string]sdkproto.DRPCProvisionerClient

type Options struct {
	AcquireInterval time.Duration
	Logger          slog.Logger
	WorkDirectory   string
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

	activeJob      *proto.AcquiredJob
	activeJobMutex sync.Mutex
	logQueue       []proto.Log
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
	a.activeJobMutex.Lock()
	a.activeJob, err = a.client.AcquireJob(a.closeContext, &proto.Empty{})
	a.activeJobMutex.Unlock()
	if err != nil {
		a.opts.Logger.Error(context.Background(), "acquire job", slog.Error(err))
		return
	}
	if a.activeJob.JobId == "" {
		a.activeJob = nil
		a.opts.Logger.Info(context.Background(), "no jobs available")
		return
	}
	a.opts.Logger.Info(context.Background(), "acquired job",
		slog.F("organization_name", a.activeJob.OrganizationName),
		slog.F("project_name", a.activeJob.ProjectName),
		slog.F("username", a.activeJob.UserName),
		slog.F("provisioner", a.activeJob.Provisioner),
	)

	provisioner, hasProvisioner := a.provisioners[a.activeJob.Provisioner]
	if !hasProvisioner {
		a.cancelActiveJob(fmt.Sprintf("provisioner %q not registered", a.activeJob.Provisioner))
		return
	}
	defer func() {
		// Cleanup the work directory after execution.
		err = os.RemoveAll(a.opts.WorkDirectory)
		if err != nil {
			a.cancelActiveJob(fmt.Sprintf("remove all from %q directory: %s", a.opts.WorkDirectory, err))
			return
		}
		a.opts.Logger.Debug(context.Background(), "cleaned up work directory")
	}()

	err = os.MkdirAll(a.opts.WorkDirectory, 0600)
	if err != nil {
		a.cancelActiveJob(fmt.Sprintf("create work directory %q: %s", a.opts.WorkDirectory, err))
		return
	}

	a.opts.Logger.Debug(context.Background(), "unpacking project source archive", slog.F("size_bytes", len(a.activeJob.ProjectSourceArchive)))
	reader := tar.NewReader(bytes.NewBuffer(a.activeJob.ProjectSourceArchive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			a.cancelActiveJob(fmt.Sprintf("read project source archive: %s", err))
			return
		}
		// #nosec
		path := filepath.Join(a.opts.WorkDirectory, header.Name)
		if !strings.HasPrefix(path, filepath.Clean(a.opts.WorkDirectory)) {
			a.cancelActiveJob("tar attempts to target relative upper directory")
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
				a.cancelActiveJob(fmt.Sprintf("mkdir %q: %s", path, err))
				return
			}
			a.opts.Logger.Debug(context.Background(), "extracted directory", slog.F("path", path))
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
			if err != nil {
				a.cancelActiveJob(fmt.Sprintf("create file %q: %s", path, err))
				return
			}
			// Max file size of 10MB.
			size, err := io.CopyN(file, reader, (1<<20)*10)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				a.cancelActiveJob(fmt.Sprintf("copy file %q: %s", path, err))
				return
			}
			err = file.Close()
			if err != nil {
				a.cancelActiveJob(fmt.Sprintf("close file %q: %s", path, err))
				return
			}
			a.opts.Logger.Debug(context.Background(), "extracted file",
				slog.F("size_bytes", size),
				slog.F("path", path),
				slog.F("mode", mode),
			)
		}
	}

	switch jobType := a.activeJob.Type.(type) {
	case *proto.AcquiredJob_ProjectImport_:
		a.opts.Logger.Debug(context.Background(), "acquired job is project import",
			slog.F("project_history_name", jobType.ProjectImport.ProjectHistoryName),
		)

		a.runProjectImport(provisioner, jobType)
	case *proto.AcquiredJob_WorkspaceProvision_:
		a.opts.Logger.Debug(context.Background(), "acquired job is workspace provision",
			slog.F("workspace_name", jobType.WorkspaceProvision.WorkspaceName),
			slog.F("state_length", len(jobType.WorkspaceProvision.State)),
			slog.F("parameters", jobType.WorkspaceProvision.ParameterValues),
		)

		a.runWorkspaceProvision(provisioner, jobType)
	default:
		a.cancelActiveJob(fmt.Sprintf("unknown job type %q; ensure your provisioner daemon is up-to-date", reflect.TypeOf(a.activeJob.Type).String()))
		return
	}

	a.activeJob = nil
}

func (a *API) runProjectImport(provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob_ProjectImport_) {
	stream, err := provisioner.Parse(a.closeContext, &sdkproto.Parse_Request{
		Directory: a.opts.WorkDirectory,
	})
	if err != nil {
		a.cancelActiveJob(fmt.Sprintf("parse source: %s", err))
		return
	}
	defer stream.Close()
	for {
		msg, err := stream.Recv()
		if err != nil {
			a.cancelActiveJob(fmt.Sprintf("recv parse source: %s", err))
			return
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Parse_Response_Log:
			a.opts.Logger.Debug(context.Background(), "parse job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("text", msgType.Log.Text),
				slog.F("project_history_id", job.ProjectImport.ProjectHistoryId),
			)

			a.logQueue = append(a.logQueue, proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UTC().UnixMilli(),
				Text:      msgType.Log.Text,
				Type: &proto.Log_ProjectImport_{
					ProjectImport: &proto.Log_ProjectImport{
						ProjectHistoryId: job.ProjectImport.ProjectHistoryId,
					},
				},
			})
		case *sdkproto.Parse_Response_Complete:
			_, err = a.client.CompleteJob(a.closeContext, &proto.CompletedJob{
				JobId: a.activeJob.JobId,
				Type: &proto.CompletedJob_ProjectImport_{
					ProjectImport: &proto.CompletedJob_ProjectImport{
						ParameterSchemas: msgType.Complete.ParameterSchemas,
					},
				},
			})
			if err != nil {
				a.cancelActiveJob(fmt.Sprintf("complete job: %s", err))
				return
			}
			// Return so we stop looping!
			return
		default:
			a.cancelActiveJob(fmt.Sprintf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String()))
			return
		}
	}
}

func (a *API) runWorkspaceProvision(provisioner sdkproto.DRPCProvisionerClient, job *proto.AcquiredJob_WorkspaceProvision_) {
	stream, err := provisioner.Provision(a.closeContext, &sdkproto.Provision_Request{
		Directory:       a.opts.WorkDirectory,
		ParameterValues: job.WorkspaceProvision.ParameterValues,
		State:           job.WorkspaceProvision.State,
	})
	if err != nil {
		a.cancelActiveJob(fmt.Sprintf("provision: %s", err))
		return
	}
	defer stream.Close()

	for {
		msg, err := stream.Recv()
		if err != nil {
			a.cancelActiveJob(fmt.Sprintf("recv workspace provision: %s", err))
			return
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Provision_Response_Log:
			a.opts.Logger.Debug(context.Background(), "provision job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("text", msgType.Log.Text),
				slog.F("workspace_history_id", job.WorkspaceProvision.WorkspaceHistoryId),
			)

			a.logQueue = append(a.logQueue, proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UTC().UnixMilli(),
				Text:      msgType.Log.Text,
				Type: &proto.Log_WorkspaceProvision_{
					WorkspaceProvision: &proto.Log_WorkspaceProvision{
						WorkspaceHistoryId: job.WorkspaceProvision.WorkspaceHistoryId,
					},
				},
			})
		case *sdkproto.Provision_Response_Complete:
			a.opts.Logger.Debug(context.Background(), "provision successful; marking job as complete",
				slog.F("resource_count", len(msgType.Complete.Resources)),
				slog.F("resources", msgType.Complete.Resources),
				slog.F("state_length", len(msgType.Complete.State)),
			)

			// Complete job may need to be async if we disconnected...
			// When we reconnect we can flush any of these cached values.
			_, err = a.client.CompleteJob(a.closeContext, &proto.CompletedJob{
				JobId: a.activeJob.JobId,
				Type: &proto.CompletedJob_WorkspaceProvision_{
					WorkspaceProvision: &proto.CompletedJob_WorkspaceProvision{
						State:     msgType.Complete.State,
						Resources: msgType.Complete.Resources,
					},
				},
			})
			if err != nil {
				a.cancelActiveJob(fmt.Sprintf("complete job: %s", err))
				return
			}
			// Return so we stop looping!
			return
		default:
			a.cancelActiveJob(fmt.Sprintf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String()))
			return
		}
	}
}

func (a *API) cancelActiveJob(errMsg string) {
	a.activeJobMutex.Lock()
	defer a.activeJobMutex.Unlock()

	if a.client == nil {
		a.activeJob = nil
		return
	}
	if a.activeJob == nil {
		return
	}

	a.opts.Logger.Info(context.Background(), "canceling active job",
		slog.F("error_message", errMsg),
		slog.F("job_id", a.activeJob.JobId),
	)
	_, err := a.client.CancelJob(a.closeContext, &proto.CancelledJob{
		JobId: a.activeJob.JobId,
		Error: fmt.Sprintf("provisioner daemon: %s", errMsg),
	})
	if err != nil {
		a.opts.Logger.Error(context.Background(), "couldn't cancel job", slog.Error(err))
	}
	a.opts.Logger.Debug(context.Background(), "canceled active job")
	a.activeJob = nil
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

	if a.activeJob != nil {
		errMsg := "provisioner daemon was shutdown gracefully"
		if err != nil {
			errMsg = err.Error()
		}
		a.cancelActiveJob(errMsg)
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
