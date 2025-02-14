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

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/websocket"
)

// @Summary Get provisioner job
// @ID get-provisioner-job
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID" format(uuid)
// @Param job path string true "Job ID" format(uuid)
// @Success 200 {object} codersdk.ProvisionerJob
// @Router /organizations/{organization}/provisionerjobs/{job} [get]
func (api *API) provisionerJob(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	jobID, ok := httpmw.ParseUUIDParam(rw, r, "job")
	if !ok {
		return
	}

	jobs, ok := api.handleAuthAndFetchProvisionerJobs(rw, r, []uuid.UUID{jobID})
	if !ok {
		return
	}
	if len(jobs) == 0 {
		httpapi.ResourceNotFound(rw)
		return
	}
	if len(jobs) > 1 || jobs[0].ProvisionerJob.ID != jobID {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  "Database returned an unexpected job.",
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerJobWithQueuePosition(jobs[0]))
}

// @Summary Get provisioner jobs
// @ID get-provisioner-jobs
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID" format(uuid)
// @Param limit query int false "Page limit"
// @Param ids query []string false "Filter results by job IDs" format(uuid)
// @Param status query codersdk.ProvisionerJobStatus false "Filter results by status" enums(pending,running,succeeded,canceling,canceled,failed)
// @Param tags query object false "Provisioner tags to filter by (JSON of the form {'tag1':'value1','tag2':'value2'})"
// @Success 200 {array} codersdk.ProvisionerJob
// @Router /organizations/{organization}/provisionerjobs [get]
func (api *API) provisionerJobs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	jobs, ok := api.handleAuthAndFetchProvisionerJobs(rw, r, nil)
	if !ok {
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.List(jobs, convertProvisionerJobWithQueuePosition))
}

// handleAuthAndFetchProvisionerJobs is an internal method shared by
// provisionerJob and provisionerJobs. If ok is false the caller should
// return immediately because the response has already been written.
func (api *API) handleAuthAndFetchProvisionerJobs(rw http.ResponseWriter, r *http.Request, ids []uuid.UUID) (_ []database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerRow, ok bool) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	// For now, only owners and template admins can access provisioner jobs.
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceProvisionerJobs.InOrg(org.ID)) {
		httpapi.ResourceNotFound(rw)
		return nil, false
	}

	qp := r.URL.Query()
	p := httpapi.NewQueryParamParser()
	limit := p.PositiveInt32(qp, 50, "limit")
	status := p.Strings(qp, nil, "status")
	if ids == nil {
		ids = p.UUIDs(qp, nil, "ids")
	}
	tagsRaw := p.String(qp, "", "tags")
	p.ErrorExcessParams(qp)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: p.Errors,
		})
		return nil, false
	}

	tags := database.StringMap{}
	if tagsRaw != "" {
		if err := tags.Scan([]byte(tagsRaw)); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid tags query parameter",
				Detail:  err.Error(),
			})
			return nil, false
		}
	}

	jobs, err := api.Database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner(ctx, database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams{
		OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
		Status:         slice.StringEnums[database.ProvisionerJobStatus](status),
		Limit:          sql.NullInt32{Int32: limit, Valid: limit > 0},
		IDs:            ids,
		Tags:           tags,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return nil, false
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner jobs.",
			Detail:  err.Error(),
		})
		return nil, false
	}

	return jobs, true
}

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

			apiAgent, err := db2sdk.WorkspaceAgent(
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
		ID:             provisionerJob.ID,
		OrganizationID: provisionerJob.OrganizationID,
		CreatedAt:      provisionerJob.CreatedAt,
		Type:           codersdk.ProvisionerJobType(provisionerJob.Type),
		Error:          provisionerJob.Error.String,
		ErrorCode:      codersdk.JobErrorCode(provisionerJob.ErrorCode.String),
		FileID:         provisionerJob.FileID,
		Tags:           provisionerJob.Tags,
		QueuePosition:  int(pj.QueuePosition),
		QueueSize:      int(pj.QueueSize),
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

	// Only unmarshal input if it exists, this should only be zero in testing.
	if len(provisionerJob.Input) > 0 {
		if err := json.Unmarshal(provisionerJob.Input, &job.Input); err != nil {
			job.Input.Error = xerrors.Errorf("decode input %s: %w", provisionerJob.Input, err).Error()
		}
	}

	return job
}

func convertProvisionerJobWithQueuePosition(pj database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerRow) codersdk.ProvisionerJob {
	job := convertProvisionerJob(database.GetProvisionerJobsByIDsWithQueuePositionRow{
		ProvisionerJob: pj.ProvisionerJob,
		QueuePosition:  pj.QueuePosition,
		QueueSize:      pj.QueueSize,
	})
	job.AvailableWorkers = pj.AvailableWorkers
	job.Metadata = codersdk.ProvisionerJobMetadata{
		TemplateVersionName: pj.TemplateVersionName,
		TemplateID:          pj.TemplateID.UUID,
		TemplateName:        pj.TemplateName,
		TemplateDisplayName: pj.TemplateDisplayName,
		TemplateIcon:        pj.TemplateIcon,
		WorkspaceName:       pj.WorkspaceName,
	}
	if pj.WorkspaceID.Valid {
		job.Metadata.WorkspaceID = &pj.WorkspaceID.UUID
	}
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
	enc    *wsjson.Encoder[codersdk.ProvisionerJobLog]

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
	f.enc = wsjson.NewEncoder[codersdk.ProvisionerJobLog](f.conn, websocket.MessageText)

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
		err := f.enc.Encode(convertProvisionerJobLog(log))
		if err != nil {
			return xerrors.Errorf("error writing to websocket: %w", err)
		}
		f.after = log.ID
		f.logger.Debug(f.ctx, "wrote log to websocket", slog.F("id", log.ID))
	}
	return nil
}
