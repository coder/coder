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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/provisionerd/proto"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
)

const (
	MissingParameterErrorText = "missing parameter"
)

var (
	errUpdateSkipped = xerrors.New("update skipped; job complete or failed")
)

type Runner struct {
	tracer              trace.Tracer
	metrics             Metrics
	job                 *proto.AcquiredJob
	sender              JobUpdater
	quotaCommitter      QuotaCommitter
	logger              slog.Logger
	filesystem          afero.Fs
	workDirectory       string
	provisioner         sdkproto.DRPCProvisionerClient
	updateInterval      time.Duration
	forceCancelInterval time.Duration
	logBufferInterval   time.Duration

	// closed when the Runner is finished sending any updates/failed/complete.
	done chan struct{}
	// active as long as we are not canceled
	notCanceled context.Context
	cancel      context.CancelFunc
	// active as long as we haven't been force stopped
	notStopped context.Context
	stop       context.CancelFunc

	// mutex controls access to all the following variables.
	mutex *sync.Mutex
	// used to wait for the failedJob or completedJob to be populated
	cond           *sync.Cond
	flushLogsTimer *time.Timer
	queuedLogs     []*proto.Log
	failedJob      *proto.FailedJob
	completedJob   *proto.CompletedJob
	// setting this false signals that no more messages about this job should be sent.  Usually this
	// means that a terminal message like FailedJob or CompletedJob has been sent, even in the case
	// of a Cancel().  However, when someone calls Fail() or ForceStop(), we might not send the
	// terminal message, but okToSend is set to false regardless.
	okToSend bool
}

type Metrics struct {
	ConcurrentJobs *prometheus.GaugeVec
	// JobTimings also counts the total amount of jobs.
	JobTimings *prometheus.HistogramVec
}

type JobUpdater interface {
	UpdateJob(ctx context.Context, in *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error)
	FailJob(ctx context.Context, in *proto.FailedJob) error
	CompleteJob(ctx context.Context, in *proto.CompletedJob) error
}
type QuotaCommitter interface {
	CommitQuota(ctx context.Context, in *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error)
}

type Options struct {
	Updater             JobUpdater
	QuotaCommitter      QuotaCommitter
	Logger              slog.Logger
	Filesystem          afero.Fs
	WorkDirectory       string
	Provisioner         sdkproto.DRPCProvisionerClient
	UpdateInterval      time.Duration
	ForceCancelInterval time.Duration
	LogDebounceInterval time.Duration
	Tracer              trace.Tracer
	Metrics             Metrics
}

func New(
	ctx context.Context,
	job *proto.AcquiredJob,
	opts Options,
) *Runner {
	m := new(sync.Mutex)

	// we need to create our contexts here in case a call to Cancel() comes immediately.
	forceStopContext, forceStopFunc := context.WithCancel(ctx)
	gracefulContext, cancelFunc := context.WithCancel(forceStopContext)

	return &Runner{
		tracer:              opts.Tracer,
		metrics:             opts.Metrics,
		job:                 job,
		sender:              opts.Updater,
		quotaCommitter:      opts.QuotaCommitter,
		logger:              opts.Logger.With(slog.F("job_id", job.JobId)),
		filesystem:          opts.Filesystem,
		workDirectory:       opts.WorkDirectory,
		provisioner:         opts.Provisioner,
		updateInterval:      opts.UpdateInterval,
		forceCancelInterval: opts.ForceCancelInterval,
		logBufferInterval:   opts.LogDebounceInterval,
		queuedLogs:          make([]*proto.Log, 0),
		mutex:               m,
		cond:                sync.NewCond(m),
		done:                make(chan struct{}),
		okToSend:            true,
		notStopped:          forceStopContext,
		stop:                forceStopFunc,
		notCanceled:         gracefulContext,
		cancel:              cancelFunc,
	}
}

// Run executes the job.
//
// the idea here is to run two goroutines to work on the job: doCleanFinish and heartbeat, then use
// the `r.cond` to wait until the job is either complete or failed.  This function then sends the
// complete or failed message --- the exception to this is if something calls Fail() on the Runner;
// either something external, like the server getting closed, or the heartbeat goroutine timing out
// after attempting to gracefully cancel.  If something calls Fail(), then the failure is sent on
// that goroutine on the context passed into Fail(), and it marks okToSend false to signal us here
// that this function should not also send a terminal message.
func (r *Runner) Run() {
	start := time.Now()
	ctx, span := r.startTrace(r.notStopped, tracing.FuncName())
	defer span.End()

	concurrentGauge := r.metrics.ConcurrentJobs.WithLabelValues(r.job.Provisioner)
	concurrentGauge.Inc()
	defer func() {
		status := "success"
		if r.failedJob != nil {
			status = "failed"
		}

		concurrentGauge.Dec()
		r.metrics.JobTimings.WithLabelValues(r.job.Provisioner, status).Observe(time.Since(start).Seconds())
	}()

	r.mutex.Lock()
	defer r.mutex.Unlock()
	defer r.stop()

	go r.doCleanFinish(ctx)
	go r.heartbeat(ctx)
	for r.failedJob == nil && r.completedJob == nil {
		r.cond.Wait()
	}
	if !r.okToSend {
		// nothing else to do.
		return
	}
	if r.failedJob != nil {
		span.RecordError(xerrors.New(r.failedJob.Error))
		span.SetStatus(codes.Error, r.failedJob.Error)

		r.logger.Debug(ctx, "sending FailedJob")
		err := r.sender.FailJob(ctx, r.failedJob)
		if err != nil {
			r.logger.Error(ctx, "send FailJob", slog.Error(err))
		} else {
			r.logger.Info(ctx, "sent FailedJob")
		}
	} else {
		r.logger.Debug(ctx, "sending CompletedJob")
		err := r.sender.CompleteJob(ctx, r.completedJob)
		if err != nil {
			r.logger.Error(ctx, "send CompletedJob", slog.Error(err))
		} else {
			r.logger.Info(ctx, "sent CompletedJob")
		}
	}
	close(r.done)
	r.okToSend = false
}

// Cancel initiates a Cancel on the job, but allows it to keep running to do so gracefully.  Read from Done() to
// be notified when the job completes.
func (r *Runner) Cancel() {
	r.cancel()
}

func (r *Runner) Done() <-chan struct{} {
	return r.done
}

// Fail immediately halts updates and, if the job is not complete sends FailJob to the coder server. Running goroutines
// are canceled but complete asynchronously (although they are prevented from further updating the job to the coder
// server). The provided context sets how long to keep trying to send the FailJob.
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
	r.stop()
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
		f.JobId = r.job.JobId
		r.failedJob = f
		r.cond.Signal()
	}
}

// ForceStop signals all goroutines to stop and prevents any further API calls back to coder server for this job
func (r *Runner) ForceStop() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.stop()
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
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	r.mutex.Lock()
	defer r.mutex.Unlock()
	if !r.okToSend {
		return nil, errUpdateSkipped
	}
	return r.sender.UpdateJob(ctx, u)
}

// doCleanFinish wraps a call to do() with cleaning up the job and setting the terminal messages
func (r *Runner) doCleanFinish(ctx context.Context) {
	var (
		failedJob    *proto.FailedJob
		completedJob *proto.CompletedJob
	)

	// push the fail/succeed write onto the defer stack before the cleanup, so
	// that cleanup happens before this.
	defer func() {
		_, span := r.startTrace(ctx, tracing.FuncName())
		defer span.End()

		if failedJob != nil {
			r.setFail(failedJob)
			return
		}
		r.setComplete(completedJob)
	}()

	defer func() {
		ctx, span := r.startTrace(ctx, tracing.FuncName())
		defer span.End()

		r.queueLog(ctx, &proto.Log{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Cleaning Up",
			CreatedAt: time.Now().UnixMilli(),
		})

		// Cleanup the work directory after execution.
		for attempt := 0; attempt < 5; attempt++ {
			err := r.filesystem.RemoveAll(r.workDirectory)
			if err != nil {
				// On Windows, open files cannot be removed.
				// When the provisioner daemon is shutting down,
				// it may take a few milliseconds for processes to exit.
				// See: https://github.com/golang/go/issues/50510
				r.logger.Debug(ctx, "failed to clean work directory; trying again", slog.Error(err))
				time.Sleep(250 * time.Millisecond)
				continue
			}
			r.logger.Debug(ctx, "cleaned up work directory")
			break
		}

		r.flushQueuedLogs(ctx)
	}()

	completedJob, failedJob = r.do(ctx)
}

// do actually does the work of running the job
func (r *Runner) do(ctx context.Context) (*proto.CompletedJob, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	err := r.filesystem.MkdirAll(r.workDirectory, 0700)
	if err != nil {
		return nil, r.failedJobf("create work directory %q: %s", r.workDirectory, err)
	}

	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     "Setting up",
		CreatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return nil, r.failedJobf("write log: %s", err)
	}

	r.logger.Info(ctx, "unpacking template source archive",
		slog.F("size_bytes", len(r.job.TemplateSourceArchive)),
	)

	reader := tar.NewReader(bytes.NewBuffer(r.job.TemplateSourceArchive))
	for {
		header, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, r.failedJobf("read template source archive: %s", err)
		}
		// #nosec
		headerPath := filepath.Join(r.workDirectory, header.Name)
		if !strings.HasPrefix(headerPath, filepath.Clean(r.workDirectory)) {
			return nil, r.failedJobf("tar attempts to target relative upper directory")
		}
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0600
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = r.filesystem.MkdirAll(headerPath, mode)
			if err != nil {
				return nil, r.failedJobf("mkdir %q: %s", headerPath, err)
			}
			r.logger.Debug(context.Background(), "extracted directory", slog.F("path", headerPath))
		case tar.TypeReg:
			file, err := r.filesystem.OpenFile(headerPath, os.O_CREATE|os.O_RDWR, mode)
			if err != nil {
				return nil, r.failedJobf("create file %q (mode %s): %s", headerPath, mode, err)
			}
			// Max file size of 10MiB.
			size, err := io.CopyN(file, reader, 10<<20)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				return nil, r.failedJobf("copy file %q: %s", headerPath, err)
			}
			err = file.Close()
			if err != nil {
				return nil, r.failedJobf("close file %q: %s", headerPath, err)
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
		r.logger.Debug(context.Background(), "acquired job is template import",
			slog.F("user_variable_values", redactVariableValues(jobType.TemplateImport.UserVariableValues)),
		)

		failedJob := r.runReadmeParse(ctx)
		if failedJob != nil {
			return nil, failedJob
		}
		return r.runTemplateImport(ctx)
	case *proto.AcquiredJob_TemplateDryRun_:
		r.logger.Debug(context.Background(), "acquired job is template dry-run",
			slog.F("workspace_name", jobType.TemplateDryRun.Metadata.WorkspaceName),
			slog.F("parameters", jobType.TemplateDryRun.ParameterValues),
			slog.F("rich_parameter_values", jobType.TemplateDryRun.RichParameterValues),
			slog.F("variable_values", redactVariableValues(jobType.TemplateDryRun.VariableValues)),
		)
		return r.runTemplateDryRun(ctx)
	case *proto.AcquiredJob_WorkspaceBuild_:
		r.logger.Debug(context.Background(), "acquired job is workspace provision",
			slog.F("workspace_name", jobType.WorkspaceBuild.WorkspaceName),
			slog.F("state_length", len(jobType.WorkspaceBuild.State)),
			slog.F("parameters", jobType.WorkspaceBuild.ParameterValues),
			slog.F("rich_parameter_values", jobType.WorkspaceBuild.RichParameterValues),
			slog.F("variable_values", redactVariableValues(jobType.WorkspaceBuild.VariableValues)),
		)
		return r.runWorkspaceBuild(ctx)
	default:
		return nil, r.failedJobf("unknown job type %q; ensure your provisioner daemon is up-to-date",
			reflect.TypeOf(r.job.Type).String())
	}
}

// heartbeat periodically sends updates on the job, which keeps coder server from assuming the job
// is stalled, and allows the runner to learn if the job has been canceled by the user.
func (r *Runner) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(r.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.notCanceled.Done():
			return
		case <-ticker.C:
		}

		resp, err := r.update(ctx, &proto.UpdateJobRequest{
			JobId: r.job.JobId,
		})
		if err != nil {
			err = r.Fail(ctx, r.failedJobf("send periodic update: %s", err))
			if err != nil {
				r.logger.Error(ctx, "failed to call FailJob", slog.Error(err))
			}
			return
		}
		if !resp.Canceled {
			continue
		}
		r.logger.Info(ctx, "attempting graceful cancelation")
		r.Cancel()
		// Hard-cancel the job after a minute of pending cancelation.
		timer := time.NewTimer(r.forceCancelInterval)
		select {
		case <-timer.C:
			r.logger.Warn(ctx, "Cancel timed out")
			err := r.Fail(ctx, r.failedJobf("Cancel timed out"))
			if err != nil {
				r.logger.Warn(ctx, "failed to call FailJob", slog.Error(err))
			}
			return
		case <-r.Done():
			timer.Stop()
			return
		case <-r.notStopped.Done():
			timer.Stop()
			return
		}
	}
}

// ReadmeFile is the location we look for to extract documentation from template
// versions.
const ReadmeFile = "README.md"

func (r *Runner) runReadmeParse(ctx context.Context) *proto.FailedJob {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	fi, err := afero.ReadFile(r.filesystem, path.Join(r.workDirectory, ReadmeFile))
	if err != nil {
		r.queueLog(ctx, &proto.Log{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_DEBUG,
			Stage:     "No README.md provided",
			CreatedAt: time.Now().UnixMilli(),
		})
		return nil
	}

	_, err = r.update(ctx, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs: []*proto.Log{{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Adding README.md...",
			CreatedAt: time.Now().UnixMilli(),
		}},
		Readme: fi,
	})
	if err != nil {
		return r.failedJobf("write log: %s", err)
	}
	return nil
}

func (r *Runner) runTemplateImport(ctx context.Context) (*proto.CompletedJob, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	// Parse parameters and update the job with the parameter specs
	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     "Parsing template parameters",
		CreatedAt: time.Now().UnixMilli(),
	})
	parameterSchemas, templateVariables, err := r.runTemplateImportParse(ctx)
	if err != nil {
		return nil, r.failedJobf("run parse: %s", err)
	}

	// Once Terraform template variables are parsed, the runner can pass variables
	// to store in database and filter valid ones.
	updateResponse, err := r.update(ctx, &proto.UpdateJobRequest{
		JobId:              r.job.JobId,
		ParameterSchemas:   parameterSchemas,
		TemplateVariables:  templateVariables,
		UserVariableValues: r.job.GetTemplateImport().GetUserVariableValues(),
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
	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     "Detecting persistent resources",
		CreatedAt: time.Now().UnixMilli(),
	})
	startResources, parameters, err := r.runTemplateImportProvision(ctx, updateResponse.ParameterValues, updateResponse.VariableValues, &sdkproto.Provision_Metadata{
		CoderUrl:            r.job.GetTemplateImport().Metadata.CoderUrl,
		WorkspaceTransition: sdkproto.WorkspaceTransition_START,
	})
	if err != nil {
		return nil, r.failedJobf("template import provision for start: %s", err)
	}

	// Determine ephemeral resources.
	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     "Detecting ephemeral resources",
		CreatedAt: time.Now().UnixMilli(),
	})
	stopResources, _, err := r.runTemplateImportProvision(ctx, updateResponse.ParameterValues, updateResponse.VariableValues, &sdkproto.Provision_Metadata{
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
				RichParameters: parameters,
			},
		},
	}, nil
}

// Parses template variables and parameter schemas from source.
func (r *Runner) runTemplateImportParse(ctx context.Context) ([]*sdkproto.ParameterSchema, []*sdkproto.TemplateVariable, error) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	stream, err := r.provisioner.Parse(ctx, &sdkproto.Parse_Request{
		Directory: r.workDirectory,
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("parse source: %w", err)
	}
	defer stream.Close()
	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil, nil, xerrors.Errorf("recv parse source: %w", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Parse_Response_Log:
			r.logger.Debug(context.Background(), "parse job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)

			r.queueLog(ctx, &proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UnixMilli(),
				Output:    msgType.Log.Output,
				Stage:     "Parse parameters",
			})
		case *sdkproto.Parse_Response_Complete:
			r.logger.Info(context.Background(), "parse complete",
				slog.F("parameter_schemas", msgType.Complete.ParameterSchemas),
				slog.F("template_variables", msgType.Complete.TemplateVariables),
			)

			return msgType.Complete.ParameterSchemas, msgType.Complete.TemplateVariables, nil
		default:
			return nil, nil, xerrors.Errorf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String())
		}
	}
}

// Performs a dry-run provision when importing a template.
// This is used to detect resources that would be provisioned for a workspace in various states.
// It doesn't define values for rich parameters as they're unknown during template import.
func (r *Runner) runTemplateImportProvision(ctx context.Context, values []*sdkproto.ParameterValue, variableValues []*sdkproto.VariableValue, metadata *sdkproto.Provision_Metadata) ([]*sdkproto.Resource, []*sdkproto.RichParameter, error) {
	return r.runTemplateImportProvisionWithRichParameters(ctx, values, variableValues, nil, metadata)
}

// Performs a dry-run provision with provided rich parameters.
// This is used to detect resources that would be provisioned for a workspace in various states.
func (r *Runner) runTemplateImportProvisionWithRichParameters(ctx context.Context, values []*sdkproto.ParameterValue, variableValues []*sdkproto.VariableValue, richParameterValues []*sdkproto.RichParameterValue, metadata *sdkproto.Provision_Metadata) ([]*sdkproto.Resource, []*sdkproto.RichParameter, error) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	var stage string
	switch metadata.WorkspaceTransition {
	case sdkproto.WorkspaceTransition_START:
		stage = "Detecting persistent resources"
	case sdkproto.WorkspaceTransition_STOP:
		stage = "Detecting ephemeral resources"
	}
	// use the notStopped so that if we attempt to gracefully cancel, the stream will still be available for us
	// to send the cancel to the provisioner
	stream, err := r.provisioner.Provision(ctx)
	if err != nil {
		return nil, nil, xerrors.Errorf("provision: %w", err)
	}
	defer stream.Close()
	go func() {
		select {
		case <-r.notStopped.Done():
			return
		case <-r.notCanceled.Done():
			_ = stream.Send(&sdkproto.Provision_Request{
				Type: &sdkproto.Provision_Request_Cancel{
					Cancel: &sdkproto.Provision_Cancel{},
				},
			})
		}
	}()
	err = stream.Send(&sdkproto.Provision_Request{
		Type: &sdkproto.Provision_Request_Plan{
			Plan: &sdkproto.Provision_Plan{
				Config: &sdkproto.Provision_Config{
					Directory: r.workDirectory,
					Metadata:  metadata,
				},
				ParameterValues:     values,
				RichParameterValues: richParameterValues,
				VariableValues:      variableValues,
			},
		},
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("start provision: %w", err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			return nil, nil, xerrors.Errorf("recv import provision: %w", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Provision_Response_Log:
			r.logger.Debug(context.Background(), "template import provision job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)
			r.queueLog(ctx, &proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UnixMilli(),
				Output:    msgType.Log.Output,
				Stage:     stage,
			})
		case *sdkproto.Provision_Response_Complete:
			if msgType.Complete.Error != "" {
				r.logger.Info(context.Background(), "dry-run provision failure",
					slog.F("error", msgType.Complete.Error),
				)

				return nil, nil, xerrors.New(msgType.Complete.Error)
			}

			if len(msgType.Complete.Parameters) > 0 && len(values) > 0 {
				r.logger.Info(context.Background(), "template uses rich parameters which can't be used together with legacy parameters")
				return nil, nil, xerrors.Errorf("invalid use of rich parameters")
			}

			r.logger.Info(context.Background(), "parse dry-run provision successful",
				slog.F("resource_count", len(msgType.Complete.Resources)),
				slog.F("resources", msgType.Complete.Resources),
				slog.F("state_length", len(msgType.Complete.State)),
			)

			return msgType.Complete.Resources, msgType.Complete.Parameters, nil
		default:
			return nil, nil, xerrors.Errorf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String())
		}
	}
}

func (r *Runner) runTemplateDryRun(ctx context.Context) (*proto.CompletedJob, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

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
	resources, _, err := r.runTemplateImportProvisionWithRichParameters(ctx,
		r.job.GetTemplateDryRun().GetParameterValues(),
		r.job.GetTemplateDryRun().GetVariableValues(),
		r.job.GetTemplateDryRun().GetRichParameterValues(),
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

func (r *Runner) buildWorkspace(ctx context.Context, stage string, req *sdkproto.Provision_Request) (
	*sdkproto.Provision_Complete, *proto.FailedJob,
) {
	// use the notStopped so that if we attempt to gracefully cancel, the stream
	// will still be available for us to send the cancel to the provisioner
	stream, err := r.provisioner.Provision(ctx)
	if err != nil {
		return nil, r.failedJobf("provision: %s", err)
	}
	defer stream.Close()
	go func() {
		select {
		case <-r.notStopped.Done():
			return
		case <-r.notCanceled.Done():
			_ = stream.Send(&sdkproto.Provision_Request{
				Type: &sdkproto.Provision_Request_Cancel{
					Cancel: &sdkproto.Provision_Cancel{},
				},
			})
		}
	}()

	err = stream.Send(req)
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

			r.queueLog(ctx, &proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UnixMilli(),
				Output:    msgType.Log.Output,
				Stage:     stage,
			})
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
			r.logger.Info(context.Background(), "provision successful",
				slog.F("resource_count", len(msgType.Complete.Resources)),
				slog.F("resources", msgType.Complete.Resources),
				slog.F("state_length", len(msgType.Complete.State)),
			)
			// Stop looping!
			return msgType.Complete, nil
		default:
			return nil, r.failedJobf("invalid message type %T received from provisioner", msg.Type)
		}
	}
}

func (r *Runner) commitQuota(ctx context.Context, resources []*sdkproto.Resource) *proto.FailedJob {
	cost := sumDailyCost(resources)
	if cost == 0 {
		return nil
	}

	const stage = "Commit quota"

	resp, err := r.quotaCommitter.CommitQuota(ctx, &proto.CommitQuotaRequest{
		JobId:     r.job.JobId,
		DailyCost: int32(cost),
	})
	if err != nil {
		r.queueLog(ctx, &proto.Log{
			Source:    proto.LogSource_PROVISIONER,
			Level:     sdkproto.LogLevel_ERROR,
			CreatedAt: time.Now().UnixMilli(),
			Output:    fmt.Sprintf("Failed to commit quota: %+v", err),
			Stage:     stage,
		})
		return r.failedJobf("commit quota: %+v", err)
	}
	for _, line := range []string{
		fmt.Sprintf("Build cost       —   %v", cost),
		fmt.Sprintf("Budget           —   %v", resp.Budget),
		fmt.Sprintf("Credits consumed —   %v", resp.CreditsConsumed),
	} {
		r.queueLog(ctx, &proto.Log{
			Source:    proto.LogSource_PROVISIONER,
			Level:     sdkproto.LogLevel_INFO,
			CreatedAt: time.Now().UnixMilli(),
			Output:    line,
			Stage:     stage,
		})
	}

	if !resp.Ok {
		r.queueLog(ctx, &proto.Log{
			Source:    proto.LogSource_PROVISIONER,
			Level:     sdkproto.LogLevel_WARN,
			CreatedAt: time.Now().UnixMilli(),
			Output:    "This build would exceed your quota. Failing.",
			Stage:     stage,
		})
		return r.failedJobf("insufficient quota")
	}
	return nil
}

func (r *Runner) runWorkspaceBuild(ctx context.Context) (*proto.CompletedJob, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	var (
		applyStage  string
		commitQuota bool
	)
	switch r.job.GetWorkspaceBuild().Metadata.WorkspaceTransition {
	case sdkproto.WorkspaceTransition_START:
		applyStage = "Starting workspace"
		commitQuota = true
	case sdkproto.WorkspaceTransition_STOP:
		applyStage = "Stopping workspace"
		commitQuota = true
	case sdkproto.WorkspaceTransition_DESTROY:
		applyStage = "Destroying workspace"
	}

	config := &sdkproto.Provision_Config{
		Directory: r.workDirectory,
		Metadata:  r.job.GetWorkspaceBuild().Metadata,
		State:     r.job.GetWorkspaceBuild().State,
	}

	completedPlan, failed := r.buildWorkspace(ctx, "Planning infrastructure", &sdkproto.Provision_Request{
		Type: &sdkproto.Provision_Request_Plan{
			Plan: &sdkproto.Provision_Plan{
				Config:              config,
				ParameterValues:     r.job.GetWorkspaceBuild().ParameterValues,
				RichParameterValues: r.job.GetWorkspaceBuild().RichParameterValues,
				VariableValues:      r.job.GetWorkspaceBuild().VariableValues,
			},
		},
	})
	if failed != nil {
		return nil, failed
	}
	r.flushQueuedLogs(ctx)
	if commitQuota {
		failed = r.commitQuota(ctx, completedPlan.GetResources())
		r.flushQueuedLogs(ctx)
		if failed != nil {
			return nil, failed
		}
	}

	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     applyStage,
		CreatedAt: time.Now().UnixMilli(),
	})

	completedApply, failed := r.buildWorkspace(ctx, applyStage, &sdkproto.Provision_Request{
		Type: &sdkproto.Provision_Request_Apply{
			Apply: &sdkproto.Provision_Apply{
				Config: config,
				Plan:   completedPlan.GetPlan(),
			},
		},
	})
	if failed != nil {
		return nil, failed
	}
	r.flushQueuedLogs(ctx)

	return &proto.CompletedJob{
		JobId: r.job.JobId,
		Type: &proto.CompletedJob_WorkspaceBuild_{
			WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
				State:     completedApply.GetState(),
				Resources: completedApply.GetResources(),
			},
		},
	}, nil
}

func (r *Runner) failedJobf(format string, args ...interface{}) *proto.FailedJob {
	return &proto.FailedJob{
		JobId: r.job.JobId,
		Error: fmt.Sprintf(format, args...),
	}
}

func (r *Runner) startTrace(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return r.tracer.Start(ctx, name, append(opts, trace.WithAttributes(
		semconv.ServiceNameKey.String("coderd.provisionerd"),
	))...)
}

// queueLog adds a log to the buffer and debounces a timer
// if one exists to flush the logs. It stores a maximum of
// 100 log lines before flushing as a safe-guard mechanism.
func (r *Runner) queueLog(ctx context.Context, log *proto.Log) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.queuedLogs = append(r.queuedLogs, log)
	if r.flushLogsTimer != nil {
		r.flushLogsTimer.Reset(r.logBufferInterval)
		return
	}
	// This can be configurable if there are a ton of logs.
	if len(r.queuedLogs) > 100 {
		// Flushing logs requires a lock, so this can happen async.
		go r.flushQueuedLogs(ctx)
		return
	}
	r.flushLogsTimer = time.AfterFunc(r.logBufferInterval, func() {
		r.flushQueuedLogs(ctx)
	})
}

func (r *Runner) flushQueuedLogs(ctx context.Context) {
	r.mutex.Lock()
	if r.flushLogsTimer != nil {
		r.flushLogsTimer.Stop()
	}
	logs := r.queuedLogs
	r.queuedLogs = make([]*proto.Log, 0)
	r.mutex.Unlock()
	_, err := r.update(ctx, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
		Logs:  logs,
	})
	if err != nil {
		if errors.Is(err, errUpdateSkipped) {
			return
		}
		r.logger.Error(ctx, "flush queued logs", slog.Error(err))
	}
}

func redactVariableValues(variableValues []*sdkproto.VariableValue) []*sdkproto.VariableValue {
	var redacted []*sdkproto.VariableValue
	for _, v := range variableValues {
		if v.Sensitive {
			redacted = append(redacted, &sdkproto.VariableValue{
				Name:      v.Name,
				Value:     "*redacted*",
				Sensitive: true,
			})
			continue
		}
		redacted = append(redacted, v)
	}
	return redacted
}
