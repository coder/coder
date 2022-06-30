package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

const (
	MissingParameterErrorText = "missing parameter"
)

type Runner struct {
	job                 *proto.AcquiredJob
	sender              JobUpdater
	logger              slog.Logger
	filesystem          afero.Fs
	workDirectory       string
	provisioner         sdkproto.DRPCProvisionerClient
	updateInterval      time.Duration
	forceCancelInterval time.Duration

	// closed when the Runner is finished sending any updates/failed/complete.
	done chan any
	// active as long as we are not canceled
	gracefulContext context.Context
	cancelFunc      context.CancelFunc
	// active as long as we haven't been force stopped
	forceStopContext context.Context
	forceStopFunc    context.CancelFunc

	// mutex controls access to all the following variables.
	mutex *sync.Mutex
	// used to wait for the failedJob or completedJob to be populated
	cond         *sync.Cond
	failedJob    *proto.FailedJob
	completedJob *proto.CompletedJob
	// setting this false signals that no more messages about this job should be sent.  Usually this means that a
	// terminal message like FailedJob or CompletedJob has been sent, but if we are force canceled, we may set this
	// false and not send one.
	okToSend bool
}

type JobUpdater interface {
	UpdateJob(ctx context.Context, in *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error)
	FailJob(ctx context.Context, in *proto.FailedJob) error
	CompleteJob(ctx context.Context, in *proto.CompletedJob) error
}

func NewRunner(
	job *proto.AcquiredJob,
	updater JobUpdater,
	logger slog.Logger,
	filesystem afero.Fs,
	workDirectory string,
	provisioner sdkproto.DRPCProvisionerClient,
	updateInterval time.Duration,
	forceCancelInterval time.Duration) *Runner {
	m := new(sync.Mutex)

	// we need to create our contexts here in case a call to Cancel() comes immediately.
	logCtx := slog.With(context.Background(), slog.F("job_id", job.JobId))
	forceStopContext, forceStopFunc := context.WithCancel(logCtx)
	gracefulContext, cancelFunc := context.WithCancel(forceStopContext)

	return &Runner{
		job:                 job,
		sender:              updater,
		logger:              logger,
		filesystem:          filesystem,
		workDirectory:       workDirectory,
		provisioner:         provisioner,
		updateInterval:      updateInterval,
		forceCancelInterval: forceCancelInterval,
		mutex:               m,
		cond:                sync.NewCond(m),
		done:                make(chan any),
		okToSend:            true,
		forceStopContext:    forceStopContext,
		forceStopFunc:       forceStopFunc,
		gracefulContext:     gracefulContext,
		cancelFunc:          cancelFunc,
	}
}

func (r *Runner) Start() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	defer r.forceStopFunc()

	// the idea here is to run two goroutines to work on the job: run and heartbeat, then use the `r.cond` to wait until
	// the job is either complete or failed.  This function then sends the complete or failed message --- the exception
	// to this is if something calls Fail() on the Runner; either something external, like the server getting closed,
	// or the heartbeat goroutine timing out after attempting to gracefully cancel.  If something calls Fail(), then
	// the failure is sent on that goroutine on the context passed into Fail(), and it marks okToSend false to signal
	// us here that this function should not also send a terminal message.
	go r.run()
	go r.heartbeat()
	for r.failedJob == nil && r.completedJob == nil {
		r.cond.Wait()
	}
	if !r.okToSend {
		// nothing else to do.
		return
	}
	if r.failedJob != nil {
		r.logger.Debug(r.forceStopContext, "sending FailedJob")
		err := r.sender.FailJob(r.forceStopContext, r.failedJob)
		if err != nil {
			r.logger.Error(r.forceStopContext, "send FailJob", slog.Error(err))
		}
		r.logger.Info(r.forceStopContext, "sent FailedJob")
	} else {
		r.logger.Debug(r.forceStopContext, "sending CompletedJob")
		err := r.sender.CompleteJob(r.forceStopContext, r.completedJob)
		if err != nil {
			r.logger.Error(r.forceStopContext, "send CompletedJob", slog.Error(err))
		}
		r.logger.Info(r.forceStopContext, "sent CompletedJob")
	}
	close(r.done)
	r.okToSend = false
}

// Cancel initiates a Cancel on the job, but allows it to keep running to do so gracefully.  Read from Done() to
// be notified when the job completes.
func (r *Runner) Cancel() {
	r.cancelFunc()
}

func (r *Runner) Done() <-chan any {
	return r.done
}

// Fail immediately halts updates and, if the job is not complete sends FailJob to the coder server.  Running goroutines
// are canceled but complete asynchronously (although they are prevented from further updating the job to the coder
// server).  The provided context sets how long to keep trying to send the FailJob.
func (r *Runner) Fail(ctx context.Context, f *proto.FailedJob) error {
	f.JobId = r.job.JobId
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if !r.okToSend {
		return nil // already done
	}
	r.Cancel()
	if r.failedJob == nil {
		r.failedJob = f
		r.cond.Signal()
	}
	// here we keep the original failed reason if there already was one, but we hadn't yet sent it.  It is likely more
	// informative of the job failing due to some problem running it, whereas this function is used to externally
	// force a Fail.
	err := r.sender.FailJob(ctx, r.failedJob)
	r.okToSend = false
	r.forceStopFunc()
	close(r.done)
	return err
}

// setComplete is an internal function to set the job to completed.  This does not send the completedJob.
func (r *Runner) setComplete(c *proto.CompletedJob) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.completedJob == nil {
		r.completedJob = c
		r.cond.Signal()
	}
}

// setFail is an internal function to set the job to failed.  This does not send the failedJob.
func (r *Runner) setFail(f *proto.FailedJob) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.failedJob == nil {
		f.JobId = r.job.GetJobId()
		r.failedJob = f
		r.cond.Signal()
	}
}

// ForceStop signals all goroutines to stop and prevents any further API calls back to coder server for this job
func (r *Runner) ForceStop() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.forceStopFunc()
	if !r.okToSend {
		return
	}
	r.okToSend = false
	close(r.done)
	// doesn't matter what we put here, since it won't get sent! Just need something to satisfy the condition in
	// Start()
	r.failedJob = &proto.FailedJob{}
	r.cond.Signal()
}

func (r *Runner) update(ctx context.Context, u *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if !r.okToSend {
		return nil, xerrors.New("update skipped; job complete or failed")
	}
	return r.sender.UpdateJob(ctx, u)
}

func (r *Runner) run() {
	// push the fail/succeed write onto the defer stack before the cleanup, so that cleanup happens before this.
	// Failures during this function should write to the _local_ failedJob variable, then return.
	var failedJob *proto.FailedJob
	var completedJob *proto.CompletedJob
	defer func() {
		if failedJob != nil {
			r.setFail(failedJob)
			return
		}
		r.setComplete(completedJob)
	}()

	defer func() {
		r.logCleanup(r.forceStopContext)

		// Cleanup the work directory after execution.
		for attempt := 0; attempt < 5; attempt++ {
			err := r.filesystem.RemoveAll(r.workDirectory)
			if err != nil {
				// On Windows, open files cannot be removed.
				// When the provisioner daemon is shutting down,
				// it may take a few milliseconds for processes to exit.
				// See: https://github.com/golang/go/issues/50510
				r.logger.Debug(r.forceStopContext, "failed to clean work directory; trying again", slog.Error(err))
				time.Sleep(250 * time.Millisecond)
				continue
			}
			r.logger.Debug(r.forceStopContext, "cleaned up work directory", slog.Error(err))
			break
		}
	}()

	err := r.filesystem.MkdirAll(r.workDirectory, 0700)
	if err != nil {
		failedJob = r.failedJobf("create work directory %q: %s", r.workDirectory, err)
		return
	}

	_, err = r.update(r.forceStopContext, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Setting up",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		failedJob = r.failedJobf("write log: %s", err)
		return
	}

	r.logger.Info(r.forceStopContext, "unpacking template source archive",
		slog.F("size_bytes", len(r.job.TemplateSourceArchive)))
	reader := tar.NewReader(bytes.NewBuffer(r.job.TemplateSourceArchive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failedJob = r.failedJobf("read template source archive: %s", err)
			return
		}
		// #nosec
		headerPath := filepath.Join(r.workDirectory, header.Name)
		if !strings.HasPrefix(headerPath, filepath.Clean(r.workDirectory)) {
			failedJob = r.failedJobf("tar attempts to target relative upper directory")
			return
		}
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0600
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = r.filesystem.MkdirAll(headerPath, mode)
			if err != nil {
				failedJob = r.failedJobf("mkdir %q: %s", headerPath, err)
				return
			}
			r.logger.Debug(context.Background(), "extracted directory", slog.F("path", headerPath))
		case tar.TypeReg:
			file, err := r.filesystem.OpenFile(headerPath, os.O_CREATE|os.O_RDWR, mode)
			if err != nil {
				failedJob = r.failedJobf("create file %q (mode %s): %s", headerPath, mode, err)
				return
			}
			// Max file size of 10MiB.
			size, err := io.CopyN(file, reader, 10<<20)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				failedJob = r.failedJobf("copy file %q: %s", headerPath, err)
				return
			}
			err = file.Close()
			if err != nil {
				failedJob = r.failedJobf("close file %q: %s", headerPath, err)
				return
			}
			r.logger.Debug(context.Background(), "extracted file",
				slog.F("size_bytes", size),
				slog.F("path", headerPath),
				slog.F("mode", mode),
			)
		}
	}

	switch jobType := r.job.Type.(type) {
	case *proto.AcquiredJob_TemplateImport_:
		r.logger.Debug(context.Background(), "acquired job is template import")

		failedJob = r.runReadmeParse()
		if failedJob == nil {
			completedJob, failedJob = r.runTemplateImport()
		}
	case *proto.AcquiredJob_TemplateDryRun_:
		r.logger.Debug(context.Background(), "acquired job is template dry-run",
			slog.F("workspace_name", jobType.TemplateDryRun.Metadata.WorkspaceName),
			slog.F("parameters", jobType.TemplateDryRun.ParameterValues),
		)
		completedJob, failedJob = r.runTemplateDryRun()
	case *proto.AcquiredJob_WorkspaceBuild_:
		r.logger.Debug(context.Background(), "acquired job is workspace provision",
			slog.F("workspace_name", jobType.WorkspaceBuild.WorkspaceName),
			slog.F("state_length", len(jobType.WorkspaceBuild.State)),
			slog.F("parameters", jobType.WorkspaceBuild.ParameterValues),
		)

		completedJob, failedJob = r.runWorkspaceBuild()
	default:
		failedJob = r.failedJobf("unknown job type %q; ensure your provisioner daemon is up-to-date",
			reflect.TypeOf(r.job.Type).String())
	}
}

func (r *Runner) heartbeat() {
	ticker := time.NewTicker(r.updateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.gracefulContext.Done():
			return
		case <-ticker.C:
		}

		resp, err := r.update(r.forceStopContext, &proto.UpdateJobRequest{
			JobId: r.job.JobId,
		})
		if err != nil {
			err = r.Fail(r.forceStopContext, r.failedJobf("send periodic update: %s", err))
			if err != nil {
				r.logger.Error(r.forceStopContext, "failed to call FailJob", slog.Error(err))
			}
			return
		}
		if !resp.Canceled {
			continue
		}
		r.logger.Info(r.forceStopContext, "attempting graceful cancelation")
		r.Cancel()
		// Hard-cancel the job after a minute of pending cancelation.
		timer := time.NewTimer(r.forceCancelInterval)
		select {
		case <-timer.C:
			r.logger.Warn(r.forceStopContext, "Cancel timed out")
			err := r.Fail(r.forceStopContext, r.failedJobf("Cancel timed out"))
			if err != nil {
				r.logger.Warn(r.forceStopContext, "failed to call FailJob", slog.Error(err))
			}
			return
		case <-r.Done():
			timer.Stop()
			return
		case <-r.forceStopContext.Done():
			timer.Stop()
			return
		}
	}
}

func (r *Runner) logCleanup(ctx context.Context) {
	_, err := r.update(ctx, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Cleaning Up",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		r.logger.Warn(ctx, "failed to log cleanup")
		return
	}
}

// ReadmeFile is the location we look for to extract documentation from template
// versions.
const ReadmeFile = "README.md"

func (r *Runner) runReadmeParse() *proto.FailedJob {
	fi, err := afero.ReadFile(r.filesystem, path.Join(r.workDirectory, ReadmeFile))
	if err != nil {
		_, err := r.update(r.forceStopContext, &proto.UpdateJobRequest{
			JobId: r.job.JobId,
			Logs: []*proto.Log{{
				Source:    proto.LogSource_PROVISIONER_DAEMON,
				Level:     sdkproto.LogLevel_DEBUG,
				Stage:     "No README.md provided",
				CreatedAt: time.Now().UTC().UnixMilli(),
			}},
		})
		if err != nil {
			return r.failedJobf("write log: %s", err)
		}

		return nil
	}

	_, err = r.update(r.forceStopContext, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Adding README.md...",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
		Readme: fi,
	})
	if err != nil {
		return r.failedJobf("write log: %s", err)
	}
	return nil
}

func (r *Runner) runTemplateImport() (*proto.CompletedJob, *proto.FailedJob) {
	// Parse parameters and update the job with the parameter specs
	_, err := r.update(r.forceStopContext, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Parsing template parameters",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		return nil, r.failedJobf("write log: %s", err)
	}
	parameterSchemas, err := r.runTemplateImportParse()
	if err != nil {
		return nil, r.failedJobf("run parse: %s", err)
	}
	updateResponse, err := r.update(r.forceStopContext, &proto.UpdateJobRequest{
		JobId:            r.job.JobId,
		ParameterSchemas: parameterSchemas,
	})
	if err != nil {
		return nil, r.failedJobf("update job: %s", err)
	}

	valueByName := map[string]*sdkproto.ParameterValue{}
	for _, parameterValue := range updateResponse.ParameterValues {
		valueByName[parameterValue.Name] = parameterValue
	}
	for _, parameterSchema := range parameterSchemas {
		_, ok := valueByName[parameterSchema.Name]
		if !ok {
			return nil, r.failedJobf("%s: %s", MissingParameterErrorText, parameterSchema.Name)
		}
	}

	// Determine persistent resources
	_, err = r.update(r.forceStopContext, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Detecting persistent resources",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		return nil, r.failedJobf("write log: %s", err)
	}
	startResources, err := r.runTemplateImportProvision(updateResponse.ParameterValues, &sdkproto.Provision_Metadata{
		CoderUrl:            r.job.GetTemplateImport().Metadata.CoderUrl,
		WorkspaceTransition: sdkproto.WorkspaceTransition_START,
	})
	if err != nil {
		return nil, r.failedJobf("template import provision for start: %s", err)
	}

	// Determine ephemeral resources.
	_, err = r.update(r.forceStopContext, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Detecting ephemeral resources",
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		return nil, r.failedJobf("write log: %s", err)
	}
	stopResources, err := r.runTemplateImportProvision(updateResponse.ParameterValues, &sdkproto.Provision_Metadata{
		CoderUrl:            r.job.GetTemplateImport().Metadata.CoderUrl,
		WorkspaceTransition: sdkproto.WorkspaceTransition_STOP,
	})
	if err != nil {
		return nil, r.failedJobf("template import provision for stop: %s", err)
	}

	return &proto.CompletedJob{
		JobId: r.job.JobId,
		Type: &proto.CompletedJob_TemplateImport_{
			TemplateImport: &proto.CompletedJob_TemplateImport{
				StartResources: startResources,
				StopResources:  stopResources,
			},
		},
	}, nil
}

// Parses parameter schemas from source.
func (r *Runner) runTemplateImportParse() ([]*sdkproto.ParameterSchema, error) {
	stream, err := r.provisioner.Parse(r.forceStopContext, &sdkproto.Parse_Request{
		Directory: r.workDirectory,
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
			r.logger.Debug(context.Background(), "parse job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)

			_, err = r.update(r.forceStopContext, &proto.UpdateJobRequest{
				JobId: r.job.JobId,
				Logs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
					Stage:     "Parse parameters",
				}},
			})
			if err != nil {
				return nil, xerrors.Errorf("update job: %w", err)
			}
		case *sdkproto.Parse_Response_Complete:
			r.logger.Info(context.Background(), "parse complete",
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
func (r *Runner) runTemplateImportProvision(values []*sdkproto.ParameterValue, metadata *sdkproto.Provision_Metadata) ([]*sdkproto.Resource, error) {
	var stage string
	switch metadata.WorkspaceTransition {
	case sdkproto.WorkspaceTransition_START:
		stage = "Detecting persistent resources"
	case sdkproto.WorkspaceTransition_STOP:
		stage = "Detecting ephemeral resources"
	}
	// use the forceStopContext so that if we attempt to gracefully cancel, the stream will still be available for us
	// to send the cancel to the provisioner
	stream, err := r.provisioner.Provision(r.forceStopContext)
	if err != nil {
		return nil, xerrors.Errorf("provision: %w", err)
	}
	defer stream.Close()
	go func() {
		select {
		case <-r.forceStopContext.Done():
			return
		case <-r.gracefulContext.Done():
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
				Directory:       r.workDirectory,
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
			r.logger.Debug(context.Background(), "template import provision job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)
			_, err = r.update(r.forceStopContext, &proto.UpdateJobRequest{
				JobId: r.job.JobId,
				Logs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
					Stage:     stage,
				}},
			})
			if err != nil {
				return nil, xerrors.Errorf("send job update: %w", err)
			}
		case *sdkproto.Provision_Response_Complete:
			if msgType.Complete.Error != "" {
				r.logger.Info(context.Background(), "dry-run provision failure",
					slog.F("error", msgType.Complete.Error),
				)

				return nil, xerrors.New(msgType.Complete.Error)
			}

			r.logger.Info(context.Background(), "parse dry-run provision successful",
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

func (r *Runner) runTemplateDryRun() (
	*proto.CompletedJob, *proto.FailedJob) {
	// Ensure all metadata fields are set as they are all optional for dry-run.
	metadata := r.job.GetTemplateDryRun().GetMetadata()
	metadata.WorkspaceTransition = sdkproto.WorkspaceTransition_START
	if metadata.CoderUrl == "" {
		metadata.CoderUrl = "http://localhost:3000"
	}
	if metadata.WorkspaceName == "" {
		metadata.WorkspaceName = "dryrun"
	}
	metadata.WorkspaceOwner = r.job.UserName
	if metadata.WorkspaceOwner == "" {
		metadata.WorkspaceOwner = "dryrunner"
	}
	if metadata.WorkspaceId == "" {
		id, err := uuid.NewRandom()
		if err != nil {
			return nil, r.failedJobf("generate random ID: %s", err)
		}
		metadata.WorkspaceId = id.String()
	}
	if metadata.WorkspaceOwnerId == "" {
		id, err := uuid.NewRandom()
		if err != nil {
			return nil, r.failedJobf("generate random ID: %s", err)
		}
		metadata.WorkspaceOwnerId = id.String()
	}

	// Run the template import provision task since it's already a dry run.
	resources, err := r.runTemplateImportProvision(
		r.job.GetTemplateDryRun().GetParameterValues(),
		metadata,
	)
	if err != nil {
		return nil, r.failedJobf("run dry-run provision job: %s", err)
	}

	return &proto.CompletedJob{
		JobId: r.job.JobId,
		Type: &proto.CompletedJob_TemplateDryRun_{
			TemplateDryRun: &proto.CompletedJob_TemplateDryRun{
				Resources: resources,
			},
		},
	}, nil
}

func (r *Runner) runWorkspaceBuild() (
	*proto.CompletedJob, *proto.FailedJob) {
	var stage string
	switch r.job.GetWorkspaceBuild().Metadata.WorkspaceTransition {
	case sdkproto.WorkspaceTransition_START:
		stage = "Starting workspace"
	case sdkproto.WorkspaceTransition_STOP:
		stage = "Stopping workspace"
	case sdkproto.WorkspaceTransition_DESTROY:
		stage = "Destroying workspace"
	}

	_, err := r.update(r.forceStopContext, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     stage,
			CreatedAt: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		return nil, r.failedJobf("write log: %s", err)
	}

	// use the forceStopContext so that if we attempt to gracefully cancel, the stream will still be available for us
	// to send the cancel to the provisioner
	stream, err := r.provisioner.Provision(r.forceStopContext)
	if err != nil {
		return nil, r.failedJobf("provision: %s", err)
	}
	defer stream.Close()
	go func() {
		select {
		case <-r.forceStopContext.Done():
			return
		case <-r.gracefulContext.Done():
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
				Directory:       r.workDirectory,
				ParameterValues: r.job.GetWorkspaceBuild().ParameterValues,
				Metadata:        r.job.GetWorkspaceBuild().Metadata,
				State:           r.job.GetWorkspaceBuild().State,
			},
		},
	})
	if err != nil {
		return nil, r.failedJobf("start provision: %s", err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil, r.failedJobf("recv workspace provision: %s", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Provision_Response_Log:
			r.logger.Debug(context.Background(), "workspace provision job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
				slog.F("workspace_build_id", r.job.GetWorkspaceBuild().WorkspaceBuildId),
			)

			_, err = r.update(r.forceStopContext, &proto.UpdateJobRequest{
				JobId: r.job.JobId,
				Logs: []*proto.Log{{
					Source:    proto.LogSource_PROVISIONER,
					Level:     msgType.Log.Level,
					CreatedAt: time.Now().UTC().UnixMilli(),
					Output:    msgType.Log.Output,
					Stage:     stage,
				}},
			})
			if err != nil {
				return nil, r.failedJobf("send job update: %s", err)
			}
		case *sdkproto.Provision_Response_Complete:
			if msgType.Complete.Error != "" {
				r.logger.Info(context.Background(), "provision failed; updating state",
					slog.F("state_length", len(msgType.Complete.State)),
				)

				return nil, &proto.FailedJob{
					JobId: r.job.JobId,
					Error: msgType.Complete.Error,
					Type: &proto.FailedJob_WorkspaceBuild_{
						WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{
							State: msgType.Complete.State,
						},
					},
				}
			}

			r.logger.Debug(context.Background(), "provision complete no error")
			r.logger.Info(context.Background(), "provision successful; marked job as complete",
				slog.F("resource_count", len(msgType.Complete.Resources)),
				slog.F("resources", msgType.Complete.Resources),
				slog.F("state_length", len(msgType.Complete.State)),
			)
			// Stop looping!
			return &proto.CompletedJob{
				JobId: r.job.JobId,
				Type: &proto.CompletedJob_WorkspaceBuild_{
					WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
						State:     msgType.Complete.State,
						Resources: msgType.Complete.Resources,
					},
				},
			}, nil
		default:
			return nil, r.failedJobf("invalid message type %T received from provisioner", msg.Type)
		}
	}
}

func (r *Runner) failedJobf(format string, args ...interface{}) *proto.FailedJob {
	return &proto.FailedJob{
		JobId: r.job.JobId,
		Error: fmt.Sprintf(format, args...),
	}
}
