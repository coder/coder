package runner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	MissingParameterErrorCode = "MISSING_TEMPLATE_PARAMETER"
	missingParameterErrorText = "missing parameter"

	RequiredTemplateVariablesErrorCode = "REQUIRED_TEMPLATE_VARIABLES"
	requiredTemplateVariablesErrorText = "required template variables"
)

var errorCodes = map[string]string{
	MissingParameterErrorCode:          missingParameterErrorText,
	RequiredTemplateVariablesErrorCode: requiredTemplateVariablesErrorText,
}

var errUpdateSkipped = xerrors.New("update skipped; job complete or failed")

type Runner struct {
	tracer              trace.Tracer
	metrics             Metrics
	job                 *proto.AcquiredJob
	sender              JobUpdater
	quotaCommitter      QuotaCommitter
	logger              slog.Logger
	provisioner         sdkproto.DRPCProvisionerClient
	lastUpdate          atomic.Pointer[time.Time]
	updateInterval      time.Duration
	forceCancelInterval time.Duration
	logBufferInterval   time.Duration

	// session is the provisioning session with the (possibly remote) provisioner
	session sdkproto.DRPCProvisioner_SessionClient
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
	NumDaemons     prometheus.Gauge
	// JobTimings also counts the total amount of jobs.
	JobTimings *prometheus.HistogramVec
	// WorkspaceBuilds counts workspace build successes and failures.
	WorkspaceBuilds *prometheus.CounterVec
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

	logger := opts.Logger.With(slog.F("job_id", job.JobId))
	if build := job.GetWorkspaceBuild(); build != nil {
		logger = logger.With(
			slog.F("template_name", build.Metadata.TemplateName),
			slog.F("template_version", build.Metadata.TemplateVersion),
			slog.F("workspace_build_id", build.WorkspaceBuildId),
			slog.F("workspace_id", build.Metadata.WorkspaceId),
			slog.F("workspace_name", build.Metadata.WorkspaceName),
			slog.F("workspace_owner", build.Metadata.WorkspaceOwner),
			slog.F("workspace_transition", strings.ToLower(build.Metadata.WorkspaceTransition.String())),
		)
	}

	return &Runner{
		tracer:              opts.Tracer,
		metrics:             opts.Metrics,
		job:                 job,
		sender:              opts.Updater,
		quotaCommitter:      opts.QuotaCommitter,
		logger:              logger,
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
		if build := r.job.GetWorkspaceBuild(); build != nil {
			r.metrics.WorkspaceBuilds.WithLabelValues(
				build.Metadata.WorkspaceOwner,
				build.Metadata.WorkspaceName,
				build.Metadata.TemplateName,
				build.Metadata.TemplateVersion,
				build.Metadata.WorkspaceTransition.String(),
				status,
			).Inc()
		}
		r.metrics.JobTimings.WithLabelValues(r.job.Provisioner, status).Observe(time.Since(start).Seconds())
	}()

	r.mutex.Lock()
	defer r.mutex.Unlock()
	defer r.stop()

	go r.doCleanFinish(ctx)
	go r.heartbeatRoutine(ctx)
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
			r.logger.Error(ctx, "sending FailJob failed", slog.Error(err))
		} else {
			r.logger.Debug(ctx, "sent FailedJob")
		}
	} else {
		r.logger.Debug(ctx, "sending CompletedJob")
		err := r.sender.CompleteJob(ctx, r.completedJob)
		if err != nil {
			r.logger.Error(ctx, "sending CompletedJob failed", slog.Error(err))
			err = r.sender.FailJob(ctx, r.failedJobf("internal provisionerserver error"))
			if err != nil {
				r.logger.Error(ctx, "sending FailJob failed (while CompletedJob)", slog.Error(err))
			}
		} else {
			r.logger.Debug(ctx, "sent CompletedJob")
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

func (r *Runner) sendHeartbeat(ctx context.Context) (*proto.UpdateJobResponse, error) {
	ctx, span := r.startTrace(ctx, "updateHeartbeat")
	defer span.End()

	r.mutex.Lock()
	if !r.okToSend {
		r.mutex.Unlock()
		return nil, errUpdateSkipped
	}
	r.mutex.Unlock()

	// Skip sending a heartbeat if we've sent an update recently.
	if lastUpdate := r.lastUpdate.Load(); lastUpdate != nil {
		if time.Since(*lastUpdate) < r.updateInterval {
			span.SetAttributes(attribute.Bool("heartbeat_skipped", true))
			return &proto.UpdateJobResponse{}, nil
		}
	}

	return r.update(ctx, &proto.UpdateJobRequest{
		JobId: r.job.JobId,
	})
}

func (r *Runner) update(ctx context.Context, u *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()
	defer func() {
		r.lastUpdate.Store(ptr.Ref(time.Now()))
	}()

	span.SetAttributes(
		attribute.Int64("logs_len", int64(len(u.Logs))),
		attribute.Int64("template_variables_len", int64(len(u.TemplateVariables))),
		attribute.Int64("user_variable_values_len", int64(len(u.UserVariableValues))),
		attribute.Int64("readme_len", int64(len(u.Readme))),
	)

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

	var err error
	r.session, err = r.provisioner.Session(ctx)
	if err != nil {
		failedJob = r.failedJobf("open session: %s", err)
		return
	}
	defer r.session.Close()

	defer func() {
		ctx, span := r.startTrace(ctx, tracing.FuncName())
		defer span.End()

		r.queueLog(ctx, &proto.Log{
			Source:    proto.LogSource_PROVISIONER_DAEMON,
			Level:     sdkproto.LogLevel_INFO,
			Stage:     "Cleaning Up",
			CreatedAt: time.Now().UnixMilli(),
		})
		r.flushQueuedLogs(ctx)
	}()

	completedJob, failedJob = r.do(ctx)
}

// do actually does the work of running the job
func (r *Runner) do(ctx context.Context) (*proto.CompletedJob, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     "Setting up",
		CreatedAt: time.Now().UnixMilli(),
	})

	switch jobType := r.job.Type.(type) {
	case *proto.AcquiredJob_TemplateImport_:
		r.logger.Debug(context.Background(), "acquired job is template import",
			slog.F("user_variable_values", redactVariableValues(jobType.TemplateImport.UserVariableValues)),
		)

		return r.runTemplateImport(ctx)
	case *proto.AcquiredJob_TemplateDryRun_:
		r.logger.Debug(context.Background(), "acquired job is template dry-run",
			slog.F("workspace_name", jobType.TemplateDryRun.Metadata.WorkspaceName),
			slog.F("rich_parameter_values", jobType.TemplateDryRun.RichParameterValues),
			slog.F("variable_values", redactVariableValues(jobType.TemplateDryRun.VariableValues)),
		)
		return r.runTemplateDryRun(ctx)
	case *proto.AcquiredJob_WorkspaceBuild_:
		r.logger.Debug(context.Background(), "acquired job is workspace provision",
			slog.F("workspace_name", jobType.WorkspaceBuild.WorkspaceName),
			slog.F("state_length", len(jobType.WorkspaceBuild.State)),
			slog.F("rich_parameter_values", jobType.WorkspaceBuild.RichParameterValues),
			slog.F("variable_values", redactVariableValues(jobType.WorkspaceBuild.VariableValues)),
		)
		return r.runWorkspaceBuild(ctx)
	default:
		return nil, r.failedJobf("unknown job type %q; ensure your provisioner daemon is up-to-date",
			reflect.TypeOf(r.job.Type).String())
	}
}

func (r *Runner) configure(config *sdkproto.Config) *proto.FailedJob {
	err := r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Config{Config: config}})
	if err != nil {
		return r.failedJobf("send config: %s", err)
	}
	return nil
}

// heartbeatRoutine periodically sends updates on the job, which keeps coder server
// from assuming the job is stalled, and allows the runner to learn if the job
// has been canceled by the user.
func (r *Runner) heartbeatRoutine(ctx context.Context) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	ticker := time.NewTicker(r.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.notCanceled.Done():
			return
		case <-ticker.C:
		}

		resp, err := r.sendHeartbeat(ctx)
		if err != nil {
			// Calling Fail starts cancellation so the process will exit.
			err = r.Fail(ctx, r.failedJobf("send periodic update: %s", err))
			if err != nil {
				r.logger.Error(ctx, "failed to call FailJob", slog.Error(err))
			}
			return
		}
		if !resp.Canceled {
			ticker.Reset(r.updateInterval)
			continue
		}
		r.logger.Info(ctx, "attempting graceful cancellation")
		r.Cancel()
		// Mark the job as failed after a minute of pending cancellation.
		timer := time.NewTimer(r.forceCancelInterval)
		select {
		case <-timer.C:
			r.logger.Debug(ctx, "cancel timed out")
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

func (r *Runner) runTemplateImport(ctx context.Context) (*proto.CompletedJob, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	failedJob := r.configure(&sdkproto.Config{
		TemplateSourceArchive: r.job.GetTemplateSourceArchive(),
	})
	if failedJob != nil {
		return nil, failedJob
	}

	// Parse parameters and update the job with the parameter specs
	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     "Parsing template parameters",
		CreatedAt: time.Now().UnixMilli(),
	})
	templateVariables, readme, err := r.runTemplateImportParse(ctx)
	if err != nil {
		return nil, r.failedJobf("run parse: %s", err)
	}

	// Once Terraform template variables are parsed, the runner can pass variables
	// to store in database and filter valid ones.
	updateResponse, err := r.update(ctx, &proto.UpdateJobRequest{
		JobId:              r.job.JobId,
		TemplateVariables:  templateVariables,
		UserVariableValues: r.job.GetTemplateImport().GetUserVariableValues(),
		Readme:             readme,
	})
	if err != nil {
		return nil, r.failedJobf("update job: %s", err)
	}

	// Determine persistent resources
	r.queueLog(ctx, &proto.Log{
		Source:    proto.LogSource_PROVISIONER_DAEMON,
		Level:     sdkproto.LogLevel_INFO,
		Stage:     "Detecting persistent resources",
		CreatedAt: time.Now().UnixMilli(),
	})
	startProvision, err := r.runTemplateImportProvision(ctx, updateResponse.VariableValues, &sdkproto.Metadata{
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
	stopProvision, err := r.runTemplateImportProvision(ctx, updateResponse.VariableValues, &sdkproto.Metadata{
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
				StartResources:        startProvision.Resources,
				StopResources:         stopProvision.Resources,
				RichParameters:        startProvision.Parameters,
				ExternalAuthProviders: startProvision.ExternalAuthProviders,
			},
		},
	}, nil
}

// Parses template variables and README from source.
func (r *Runner) runTemplateImportParse(ctx context.Context) (
	vars []*sdkproto.TemplateVariable, readme []byte, err error,
) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	err = r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Parse{Parse: &sdkproto.ParseRequest{}}})
	if err != nil {
		return nil, nil, xerrors.Errorf("parse source: %w", err)
	}
	for {
		msg, err := r.session.Recv()
		if err != nil {
			return nil, nil, xerrors.Errorf("recv parse source: %w", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Response_Log:
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
		case *sdkproto.Response_Parse:
			pc := msgType.Parse
			r.logger.Debug(context.Background(), "parse complete",
				slog.F("template_variables", pc.TemplateVariables),
				slog.F("readme_len", len(pc.Readme)),
				slog.F("error", pc.Error),
			)
			if pc.Error != "" {
				return nil, nil, xerrors.Errorf("parse error: %s", pc.Error)
			}

			return msgType.Parse.TemplateVariables, msgType.Parse.Readme, nil
		default:
			return nil, nil, xerrors.Errorf("invalid message type %q received from provisioner",
				reflect.TypeOf(msg.Type).String())
		}
	}
}

type templateImportProvision struct {
	Resources             []*sdkproto.Resource
	Parameters            []*sdkproto.RichParameter
	ExternalAuthProviders []string
}

// Performs a dry-run provision when importing a template.
// This is used to detect resources that would be provisioned for a workspace in various states.
// It doesn't define values for rich parameters as they're unknown during template import.
func (r *Runner) runTemplateImportProvision(ctx context.Context, variableValues []*sdkproto.VariableValue, metadata *sdkproto.Metadata) (*templateImportProvision, error) {
	return r.runTemplateImportProvisionWithRichParameters(ctx, variableValues, nil, metadata)
}

// Performs a dry-run provision with provided rich parameters.
// This is used to detect resources that would be provisioned for a workspace in various states.
func (r *Runner) runTemplateImportProvisionWithRichParameters(
	ctx context.Context,
	variableValues []*sdkproto.VariableValue,
	richParameterValues []*sdkproto.RichParameterValue,
	metadata *sdkproto.Metadata,
) (*templateImportProvision, error) {
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
	err := r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Plan{Plan: &sdkproto.PlanRequest{
		Metadata:            metadata,
		RichParameterValues: richParameterValues,
		VariableValues:      variableValues,
	}}})
	if err != nil {
		return nil, xerrors.Errorf("start provision: %w", err)
	}
	nevermind := make(chan struct{})
	defer close(nevermind)
	go func() {
		select {
		case <-nevermind:
			return
		case <-r.notStopped.Done():
			return
		case <-r.notCanceled.Done():
			_ = r.session.Send(&sdkproto.Request{
				Type: &sdkproto.Request_Cancel{
					Cancel: &sdkproto.CancelRequest{},
				},
			})
		}
	}()

	for {
		msg, err := r.session.Recv()
		if err != nil {
			return nil, xerrors.Errorf("recv import provision: %w", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Response_Log:
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
		case *sdkproto.Response_Plan:
			c := msgType.Plan
			if c.Error != "" {
				r.logger.Info(context.Background(), "dry-run provision failure",
					slog.F("error", c.Error),
				)

				return nil, xerrors.New(c.Error)
			}

			r.logger.Info(context.Background(), "parse dry-run provision successful",
				slog.F("resource_count", len(c.Resources)),
				slog.F("resources", c.Resources),
			)

			return &templateImportProvision{
				Resources:             c.Resources,
				Parameters:            c.Parameters,
				ExternalAuthProviders: c.GitAuthProviders,
			}, nil
		default:
			return nil, xerrors.Errorf("invalid message type %q received from provisioner",
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

	failedJob := r.configure(&sdkproto.Config{
		TemplateSourceArchive: r.job.GetTemplateSourceArchive(),
	})
	if failedJob != nil {
		return nil, failedJob
	}

	// Run the template import provision task since it's already a dry run.
	provision, err := r.runTemplateImportProvisionWithRichParameters(ctx,
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
				Resources: provision.Resources,
			},
		},
	}, nil
}

func (r *Runner) buildWorkspace(ctx context.Context, stage string, req *sdkproto.Request) (
	*sdkproto.Response, *proto.FailedJob,
) {
	// use the notStopped so that if we attempt to gracefully cancel, the stream
	// will still be available for us to send the cancel to the provisioner
	err := r.session.Send(req)
	if err != nil {
		return nil, r.failedWorkspaceBuildf("start provision: %s", err)
	}
	nevermind := make(chan struct{})
	defer close(nevermind)
	go func() {
		select {
		case <-nevermind:
			return
		case <-r.notStopped.Done():
			return
		case <-r.notCanceled.Done():
			_ = r.session.Send(&sdkproto.Request{
				Type: &sdkproto.Request_Cancel{
					Cancel: &sdkproto.CancelRequest{},
				},
			})
		}
	}()

	for {
		msg, err := r.session.Recv()
		if err != nil {
			return nil, r.failedWorkspaceBuildf("recv workspace provision: %s", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Response_Log:
			r.logProvisionerJobLog(context.Background(), msgType.Log.Level, "workspace provisioner job logged",
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
		default:
			// Stop looping!
			return msg, nil
		}
	}
}

func (r *Runner) commitQuota(ctx context.Context, resources []*sdkproto.Resource) *proto.FailedJob {
	cost := sumDailyCost(resources)
	r.logger.Debug(ctx, "committing quota",
		slog.F("resources", resources),
		slog.F("cost", cost),
	)
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

	failedJob := r.configure(&sdkproto.Config{
		TemplateSourceArchive: r.job.GetTemplateSourceArchive(),
		State:                 r.job.GetWorkspaceBuild().State,
		ProvisionerLogLevel:   r.job.GetWorkspaceBuild().LogLevel,
	})
	if failedJob != nil {
		return nil, failedJob
	}

	resp, failed := r.buildWorkspace(ctx, "Planning infrastructure", &sdkproto.Request{
		Type: &sdkproto.Request_Plan{
			Plan: &sdkproto.PlanRequest{
				Metadata:              r.job.GetWorkspaceBuild().Metadata,
				RichParameterValues:   r.job.GetWorkspaceBuild().RichParameterValues,
				VariableValues:        r.job.GetWorkspaceBuild().VariableValues,
				ExternalAuthProviders: r.job.GetWorkspaceBuild().ExternalAuthProviders,
			},
		},
	})
	if failed != nil {
		return nil, failed
	}
	planComplete := resp.GetPlan()
	if planComplete == nil {
		return nil, r.failedWorkspaceBuildf("invalid message type %T received from provisioner", resp.Type)
	}
	if planComplete.Error != "" {
		r.logger.Warn(context.Background(), "plan request failed",
			slog.F("error", planComplete.Error),
		)

		return nil, &proto.FailedJob{
			JobId: r.job.JobId,
			Error: planComplete.Error,
			Type: &proto.FailedJob_WorkspaceBuild_{
				WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{},
			},
		}
	}

	r.logger.Info(context.Background(), "plan request successful",
		slog.F("resource_count", len(planComplete.Resources)),
		slog.F("resources", planComplete.Resources),
	)
	r.flushQueuedLogs(ctx)
	if commitQuota {
		failed = r.commitQuota(ctx, planComplete.Resources)
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

	resp, failed = r.buildWorkspace(ctx, applyStage, &sdkproto.Request{
		Type: &sdkproto.Request_Apply{
			Apply: &sdkproto.ApplyRequest{
				Metadata: r.job.GetWorkspaceBuild().Metadata,
			},
		},
	})
	if failed != nil {
		return nil, failed
	}
	applyComplete := resp.GetApply()
	if applyComplete == nil {
		return nil, r.failedWorkspaceBuildf("invalid message type %T received from provisioner", resp.Type)
	}
	if applyComplete.Error != "" {
		r.logger.Warn(context.Background(), "apply failed; updating state",
			slog.F("error", applyComplete.Error),
			slog.F("state_len", len(applyComplete.State)),
		)

		return nil, &proto.FailedJob{
			JobId: r.job.JobId,
			Error: applyComplete.Error,
			Type: &proto.FailedJob_WorkspaceBuild_{
				WorkspaceBuild: &proto.FailedJob_WorkspaceBuild{
					State: applyComplete.State,
				},
			},
		}
	}

	r.logger.Info(context.Background(), "apply successful",
		slog.F("resource_count", len(applyComplete.Resources)),
		slog.F("resources", applyComplete.Resources),
		slog.F("state_len", len(applyComplete.State)),
	)
	r.flushQueuedLogs(ctx)

	return &proto.CompletedJob{
		JobId: r.job.JobId,
		Type: &proto.CompletedJob_WorkspaceBuild_{
			WorkspaceBuild: &proto.CompletedJob_WorkspaceBuild{
				State:     applyComplete.State,
				Resources: applyComplete.Resources,
			},
		},
	}, nil
}

func (r *Runner) failedWorkspaceBuildf(format string, args ...interface{}) *proto.FailedJob {
	failedJob := r.failedJobf(format, args...)
	failedJob.Type = &proto.FailedJob_WorkspaceBuild_{}
	return failedJob
}

func (r *Runner) failedJobf(format string, args ...interface{}) *proto.FailedJob {
	message := fmt.Sprintf(format, args...)
	var code string

	for c, m := range errorCodes {
		if strings.Contains(message, m) {
			code = c
			break
		}
	}
	return &proto.FailedJob{
		JobId:     r.job.JobId,
		Error:     message,
		ErrorCode: code,
	}
}

func (r *Runner) startTrace(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return r.tracer.Start(ctx, name, append(opts, trace.WithAttributes(
		semconv.ServiceNameKey.String("coderd.provisionerd"),
		attribute.String("job_id", r.job.JobId),
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

// logProvisionerJobLog logs a message from the provisioner daemon at the appropriate level.
func (r *Runner) logProvisionerJobLog(ctx context.Context, logLevel sdkproto.LogLevel, msg string, fields ...any) {
	switch logLevel {
	case sdkproto.LogLevel_TRACE:
		r.logger.Debug(ctx, msg, fields...) // There's no trace, so we'll just use debug.
	case sdkproto.LogLevel_DEBUG:
		r.logger.Debug(ctx, msg, fields...)
	case sdkproto.LogLevel_INFO:
		r.logger.Info(ctx, msg, fields...)
	case sdkproto.LogLevel_WARN:
		r.logger.Warn(ctx, msg, fields...)
	case sdkproto.LogLevel_ERROR:
		r.logger.Error(ctx, msg, fields...)
	default: // should never happen, but we should not explode either.
		r.logger.Info(ctx, msg, fields...)
	}
}
