package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

// Returns provisioner logs based on query parameters.
// The intended usage for a client to stream all logs (with JS API):
// GET /logs
// GET /logs?after=<id>&follow
// The combination of these responses should provide all current logs
// to the consumer, and future logs are streamed in the follow request.
func (api *API) provisionerJobLogs(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
	var (
		ctx      = r.Context()
		logger   = api.Logger.With(slog.F("job_id", job.ID))
		follow   = r.URL.Query().Has("follow")
		afterRaw = r.URL.Query().Get("after")
	)

	var after int64
	// Only fetch logs created after the time provided.
	if afterRaw != "" {
		var err error
		after, err = strconv.ParseInt(afterRaw, 10, 64)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query param \"after\" must be an integer.",
				Validations: []codersdk.ValidationError{
					{Field: "after", Detail: "Must be an integer"},
				},
			})
			return
		}
	}

	if !follow {
		fetchAndWriteLogs(ctx, api.Database, job.ID, after, rw)
		return
	}

	follower := newLogFollower(ctx, logger, api.Database, api.Pubsub, rw, r, job, after)
	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	follower.follow()
}

func (api *API) provisionerJobResources(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
	ctx := r.Context()
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job hasn't completed!",
		})
		return
	}

	// nolint:gocritic // GetWorkspaceResourcesByJobID is a system function.
	resources, err := api.Database.GetWorkspaceResourcesByJobID(dbauthz.AsSystemRestricted(ctx), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching job resources.",
			Detail:  err.Error(),
		})
		return
	}
	resourceIDs := make([]uuid.UUID, 0)
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
	}

	// nolint:gocritic // GetWorkspaceAgentsByResourceIDs is a system function.
	resourceAgents, err := api.Database.GetWorkspaceAgentsByResourceIDs(dbauthz.AsSystemRestricted(ctx), resourceIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	resourceAgentIDs := make([]uuid.UUID, 0)
	for _, agent := range resourceAgents {
		resourceAgentIDs = append(resourceAgentIDs, agent.ID)
	}

	// nolint:gocritic // GetWorkspaceAppsByAgentIDs is a system function.
	apps, err := api.Database.GetWorkspaceAppsByAgentIDs(dbauthz.AsSystemRestricted(ctx), resourceAgentIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace applications.",
			Detail:  err.Error(),
		})
		return
	}

	// nolint:gocritic // GetWorkspaceAgentScriptsByAgentIDs is a system function.
	scripts, err := api.Database.GetWorkspaceAgentScriptsByAgentIDs(dbauthz.AsSystemRestricted(ctx), resourceAgentIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent scripts.",
			Detail:  err.Error(),
		})
		return
	}

	// nolint:gocritic // GetWorkspaceAgentLogSourcesByAgentIDs is a system function.
	logSources, err := api.Database.GetWorkspaceAgentLogSourcesByAgentIDs(dbauthz.AsSystemRestricted(ctx), resourceAgentIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent log sources.",
			Detail:  err.Error(),
		})
		return
	}

	// nolint:gocritic // GetWorkspaceResourceMetadataByResourceIDs is a system function.
	resourceMetadata, err := api.Database.GetWorkspaceResourceMetadataByResourceIDs(dbauthz.AsSystemRestricted(ctx), resourceIDs)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace metadata.",
			Detail:  err.Error(),
		})
		return
	}

	apiResources := make([]codersdk.WorkspaceResource, 0)
	for _, resource := range resources {
		agents := make([]codersdk.WorkspaceAgent, 0)
		for _, agent := range resourceAgents {
			if agent.ResourceID != resource.ID {
				continue
			}
			dbApps := make([]database.WorkspaceApp, 0)
			for _, app := range apps {
				if app.AgentID == agent.ID {
					dbApps = append(dbApps, app)
				}
			}
			dbScripts := make([]database.WorkspaceAgentScript, 0)
			for _, script := range scripts {
				if script.WorkspaceAgentID == agent.ID {
					dbScripts = append(dbScripts, script)
				}
			}
			dbLogSources := make([]database.WorkspaceAgentLogSource, 0)
			for _, logSource := range logSources {
				if logSource.WorkspaceAgentID == agent.ID {
					dbLogSources = append(dbLogSources, logSource)
				}
			}

			apiAgent, err := convertWorkspaceAgent(
				api.DERPMap(), *api.TailnetCoordinator.Load(), agent, convertProvisionedApps(dbApps), convertScripts(dbScripts), convertLogSources(dbLogSources), api.AgentInactiveDisconnectTimeout,
				api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
			)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error reading job agent.",
					Detail:  err.Error(),
				})
				return
			}
			agents = append(agents, apiAgent)
		}
		metadata := make([]database.WorkspaceResourceMetadatum, 0)
		for _, field := range resourceMetadata {
			if field.WorkspaceResourceID == resource.ID {
				metadata = append(metadata, field)
			}
		}
		apiResources = append(apiResources, convertWorkspaceResource(resource, agents, metadata))
	}
	sort.Slice(apiResources, func(i, j int) bool {
		return apiResources[i].Name < apiResources[j].Name
	})

	httpapi.Write(ctx, rw, http.StatusOK, apiResources)
}

func convertProvisionerJobLogs(provisionerJobLogs []database.ProvisionerJobLog) []codersdk.ProvisionerJobLog {
	sdk := make([]codersdk.ProvisionerJobLog, 0, len(provisionerJobLogs))
	for _, log := range provisionerJobLogs {
		sdk = append(sdk, convertProvisionerJobLog(log))
	}
	return sdk
}

func convertProvisionerJobLog(provisionerJobLog database.ProvisionerJobLog) codersdk.ProvisionerJobLog {
	return codersdk.ProvisionerJobLog{
		ID:        provisionerJobLog.ID,
		CreatedAt: provisionerJobLog.CreatedAt,
		Source:    codersdk.LogSource(provisionerJobLog.Source),
		Level:     codersdk.LogLevel(provisionerJobLog.Level),
		Stage:     provisionerJobLog.Stage,
		Output:    provisionerJobLog.Output,
	}
}

func convertProvisionerJob(pj database.GetProvisionerJobsByIDsWithQueuePositionRow) codersdk.ProvisionerJob {
	provisionerJob := pj.ProvisionerJob
	job := codersdk.ProvisionerJob{
		ID:            provisionerJob.ID,
		CreatedAt:     provisionerJob.CreatedAt,
		Error:         provisionerJob.Error.String,
		ErrorCode:     codersdk.JobErrorCode(provisionerJob.ErrorCode.String),
		FileID:        provisionerJob.FileID,
		Tags:          provisionerJob.Tags,
		QueuePosition: int(pj.QueuePosition),
		QueueSize:     int(pj.QueueSize),
	}
	// Applying values optional to the struct.
	if provisionerJob.StartedAt.Valid {
		job.StartedAt = &provisionerJob.StartedAt.Time
	}
	if provisionerJob.CompletedAt.Valid {
		job.CompletedAt = &provisionerJob.CompletedAt.Time
	}
	if provisionerJob.CanceledAt.Valid {
		job.CanceledAt = &provisionerJob.CanceledAt.Time
	}
	if provisionerJob.WorkerID.Valid {
		job.WorkerID = &provisionerJob.WorkerID.UUID
	}
	job.Status = codersdk.ProvisionerJobStatus(pj.ProvisionerJob.JobStatus)

	return job
}

func fetchAndWriteLogs(ctx context.Context, db database.Store, jobID uuid.UUID, after int64, rw http.ResponseWriter) {
	logs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
		JobID:        jobID,
		CreatedAfter: after,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner logs.",
			Detail:  err.Error(),
		})
		return
	}
	if logs == nil {
		logs = []database.ProvisionerJobLog{}
	}
	httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerJobLogs(logs))
}

func jobIsComplete(logger slog.Logger, job database.ProvisionerJob) bool {
	status := codersdk.ProvisionerJobStatus(job.JobStatus)
	switch status {
	case codersdk.ProvisionerJobCanceled:
		return true
	case codersdk.ProvisionerJobFailed:
		return true
	case codersdk.ProvisionerJobSucceeded:
		return true
	case codersdk.ProvisionerJobPending:
		return false
	case codersdk.ProvisionerJobCanceling:
		return false
	case codersdk.ProvisionerJobRunning:
		return false
	default:
		logger.Error(context.Background(),
			"can't convert the provisioner job status",
			slog.F("job_id", job.ID), slog.F("status", status))
		return false
	}
}

type logFollower struct {
	ctx    context.Context
	logger slog.Logger
	db     database.Store
	pubsub pubsub.Pubsub
	r      *http.Request
	rw     http.ResponseWriter
	conn   *websocket.Conn

	jobID         uuid.UUID
	after         int64
	complete      bool
	notifications chan provisionersdk.ProvisionerJobLogsNotifyMessage
	errors        chan error
}

func newLogFollower(
	ctx context.Context, logger slog.Logger, db database.Store, ps pubsub.Pubsub,
	rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob, after int64,
) *logFollower {
	return &logFollower{
		ctx:           ctx,
		logger:        logger,
		db:            db,
		pubsub:        ps,
		r:             r,
		rw:            rw,
		jobID:         job.ID,
		after:         after,
		complete:      jobIsComplete(logger, job),
		notifications: make(chan provisionersdk.ProvisionerJobLogsNotifyMessage),
		errors:        make(chan error),
	}
}

func (f *logFollower) follow() {
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithCancel(f.ctx)
	defer cancel()
	// note that we only need to subscribe to updates if the job is not yet
	// complete.
	if !f.complete {
		subCancel, err := f.pubsub.SubscribeWithErr(
			provisionersdk.ProvisionerJobLogsNotifyChannel(f.jobID),
			f.listener,
		)
		if err != nil {
			httpapi.Write(f.ctx, f.rw, http.StatusInternalServerError, codersdk.Response{
				Message: "failed to subscribe to job updates",
				Detail:  err.Error(),
			})
			return
		}
		defer subCancel()
		// Move cancel up the stack so it happens before unsubscribing,
		// otherwise we can end up in a deadlock due to how the
		// in-memory pubsub does mutex locking on send/unsubscribe.
		defer cancel()

		// we were provided `complete` prior to starting this subscription, so
		// we also need to check whether the job is now complete, in case the
		// job completed between the last time we queried the job and the start
		// of the subscription.  If the job completes after this, we will get
		// a notification on the subscription.
		job, err := f.db.GetProvisionerJobByID(f.ctx, f.jobID)
		if err != nil {
			httpapi.Write(f.ctx, f.rw, http.StatusInternalServerError, codersdk.Response{
				Message: "failed to query job",
				Detail:  err.Error(),
			})
			return
		}
		f.complete = jobIsComplete(f.logger, job)
		f.logger.Debug(f.ctx, "queried job after subscribe", slog.F("complete", f.complete))
	}

	var err error
	f.conn, err = websocket.Accept(f.rw, f.r, nil)
	if err != nil {
		httpapi.Write(f.ctx, f.rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	defer f.conn.Close(websocket.StatusNormalClosure, "done")
	go httpapi.Heartbeat(f.ctx, f.conn)

	// query for logs once right away, so we can get historical data from before
	// subscription
	if err := f.query(); err != nil {
		if f.ctx.Err() == nil && !xerrors.Is(err, io.EOF) {
			// neither context expiry, nor EOF, close and log
			f.logger.Error(f.ctx, "failed to query logs", slog.Error(err))
			err = f.conn.Close(websocket.StatusInternalError, err.Error())
			if err != nil {
				f.logger.Warn(f.ctx, "failed to close webscoket", slog.Error(err))
			}
		}
		return
	}

	// no need to wait if the job is done
	if f.complete {
		return
	}
	for {
		select {
		case err := <-f.errors:
			// we've dropped at least one notification.  This can happen if we
			// lose database connectivity.  We don't know whether the job is
			// now complete since we could have missed the end of logs message.
			// We could soldier on and retry, but loss of database connectivity
			// is fairly serious, so instead just 500 and bail out.  Client
			// can retry and hopefully find a healthier node.
			f.logger.Error(f.ctx, "dropped or corrupted notification", slog.Error(err))
			err = f.conn.Close(websocket.StatusInternalError, err.Error())
			if err != nil {
				f.logger.Warn(f.ctx, "failed to close webscoket", slog.Error(err))
			}
			return
		case <-f.ctx.Done():
			// client disconnect
			return
		case n := <-f.notifications:
			if n.EndOfLogs {
				// safe to return here because we started the subscription,
				// and then queried at least once, so we will have already
				// gotten all logs prior to the start of our subscription.
				return
			}
			err = f.query()
			if err != nil {
				if f.ctx.Err() == nil && !xerrors.Is(err, io.EOF) {
					// neither context expiry, nor EOF, close and log
					f.logger.Error(f.ctx, "failed to query logs", slog.Error(err))
					err = f.conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("%s", err.Error()))
					if err != nil {
						f.logger.Warn(f.ctx, "failed to close webscoket", slog.Error(err))
					}
				}
				return
			}
		}
	}
}

func (f *logFollower) listener(_ context.Context, message []byte, err error) {
	// in this function we always pair writes to channels with a select on the context
	// otherwise we could block a goroutine if the follow() method exits.
	if err != nil {
		select {
		case <-f.ctx.Done():
		case f.errors <- err:
		}
		return
	}
	var n provisionersdk.ProvisionerJobLogsNotifyMessage
	err = json.Unmarshal(message, &n)
	if err != nil {
		select {
		case <-f.ctx.Done():
		case f.errors <- err:
		}
		return
	}
	select {
	case <-f.ctx.Done():
	case f.notifications <- n:
	}
}

// query fetches the latest job logs from the database and writes them to the
// connection.
func (f *logFollower) query() error {
	f.logger.Debug(f.ctx, "querying logs", slog.F("after", f.after))
	logs, err := f.db.GetProvisionerLogsAfterID(f.ctx, database.GetProvisionerLogsAfterIDParams{
		JobID:        f.jobID,
		CreatedAfter: f.after,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("error fetching logs: %w", err)
	}
	for _, log := range logs {
		logB, err := json.Marshal(convertProvisionerJobLog(log))
		if err != nil {
			return xerrors.Errorf("error marshaling log: %w", err)
		}
		err = f.conn.Write(f.ctx, websocket.MessageText, logB)
		if err != nil {
			return xerrors.Errorf("error writing to websocket: %w", err)
		}
		f.after = log.ID
		f.logger.Debug(f.ctx, "wrote log to websocket", slog.F("id", log.ID))
	}
	return nil
}
