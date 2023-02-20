package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// Returns provisioner logs based on query parameters.
// The intended usage for a client to stream all logs (with JS API):
// const timestamp = new Date().getTime();
// 1. GET /logs?before=<id>
// 2. GET /logs?after=<id>&follow
// The combination of these responses should provide all current logs
// to the consumer, and future logs are streamed in the follow request.
func (api *API) provisionerJobLogs(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
	var (
		ctx       = r.Context()
		actor, _  = dbauthz.ActorFromContext(ctx)
		logger    = api.Logger.With(slog.F("job_id", job.ID))
		follow    = r.URL.Query().Has("follow")
		afterRaw  = r.URL.Query().Get("after")
		beforeRaw = r.URL.Query().Get("before")
	)
	if beforeRaw != "" && follow {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Query param \"before\" cannot be used with \"follow\".",
		})
		return
	}

	// if we are following logs, start the subscription before we query the database, so that we don't miss any logs
	// between the end of our query and the start of the subscription.  We might get duplicates, so we'll keep track
	// of processed IDs.
	var bufferedLogs <-chan database.ProvisionerJobLog
	if follow {
		bl, closeFollow, err := api.followLogs(actor, job.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error watching provisioner logs.",
				Detail:  err.Error(),
			})
			return
		}
		defer closeFollow()
		bufferedLogs = bl

		// Next query the job itself to see if it is complete.  If so, the historical query to the database will return
		// the full set of logs.  It's a little sad to have to query the job again, given that our caller definitely
		// has, but we need to query it *after* we start following the pubsub to avoid a race condition where the job
		// completes between the prior query and the start of following the pubsub.  A more substantial refactor could
		// avoid this, but not worth it for one fewer query at this point.
		job, err = api.Database.GetProvisionerJobByID(ctx, job.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error querying job.",
				Detail:  err.Error(),
			})
			return
		}
	}

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
	var before int64
	// Only fetch logs created before the time provided.
	if beforeRaw != "" {
		var err error
		before, err = strconv.ParseInt(beforeRaw, 10, 64)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query param \"before\" must be an integer.",
				Validations: []codersdk.ValidationError{
					{Field: "before", Detail: "Must be an integer"},
				},
			})
			return
		}
	}

	logs, err := api.Database.GetProvisionerLogsByIDBetween(ctx, database.GetProvisionerLogsByIDBetweenParams{
		JobID:         job.ID,
		CreatedAfter:  after,
		CreatedBefore: before,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner logs.",
			Detail:  err.Error(),
		})
		return
	}
	if logs == nil {
		logs = []database.ProvisionerJobLog{}
	}

	if !follow {
		logger.Debug(ctx, "Finished non-follow job logs")
		httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerJobLogs(logs))
		return
	}

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	go httpapi.Heartbeat(ctx, conn)

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close() // Also closes conn.

	logIdsDone := make(map[int64]bool)

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(wsNetConn)
	for _, provisionerJobLog := range logs {
		logIdsDone[provisionerJobLog.ID] = true
		err = encoder.Encode(convertProvisionerJobLog(provisionerJobLog))
		if err != nil {
			return
		}
	}
	if job.CompletedAt.Valid {
		// job was complete before we queried the database for historical logs, meaning we got everything.  No need
		// to stream anything from the bufferedLogs.
		return
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug(context.Background(), "job logs context canceled")
			return
		case log, ok := <-bufferedLogs:
			if !ok {
				logger.Debug(context.Background(), "done with published logs")
				return
			}
			if logIdsDone[log.ID] {
				logger.Debug(ctx, "subscribe duplicated log",
					slog.F("stage", log.Stage))
			} else {
				logger.Debug(ctx, "subscribe encoding log",
					slog.F("stage", log.Stage))
				err = encoder.Encode(convertProvisionerJobLog(log))
				if err != nil {
					return
				}
			}
		}
	}
}

func (api *API) provisionerJobResources(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
	ctx := r.Context()
	if !job.CompletedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	resources, err := api.Database.GetWorkspaceResourcesByJobID(ctx, job.ID)
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
	resourceAgents, err := api.Database.GetWorkspaceAgentsByResourceIDs(ctx, resourceIDs)
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
	apps, err := api.Database.GetWorkspaceAppsByAgentIDs(ctx, resourceAgentIDs)
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
	resourceMetadata, err := api.Database.GetWorkspaceResourceMetadataByResourceIDs(ctx, resourceIDs)
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

			apiAgent, err := convertWorkspaceAgent(api.DERPMap, *api.TailnetCoordinator.Load(), agent, convertApps(dbApps), api.AgentInactiveDisconnectTimeout, api.DeploymentConfig.AgentFallbackTroubleshootingURL.Value)
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

func convertProvisionerJob(provisionerJob database.ProvisionerJob) codersdk.ProvisionerJob {
	job := codersdk.ProvisionerJob{
		ID:        provisionerJob.ID,
		CreatedAt: provisionerJob.CreatedAt,
		Error:     provisionerJob.Error.String,
		FileID:    provisionerJob.FileID,
		Tags:      provisionerJob.Tags,
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
	job.Status = ConvertProvisionerJobStatus(provisionerJob)

	return job
}

func ConvertProvisionerJobStatus(provisionerJob database.ProvisionerJob) codersdk.ProvisionerJobStatus {
	switch {
	case provisionerJob.CanceledAt.Valid:
		if !provisionerJob.CompletedAt.Valid {
			return codersdk.ProvisionerJobCanceling
		}
		if provisionerJob.Error.String == "" {
			return codersdk.ProvisionerJobCanceled
		}
		return codersdk.ProvisionerJobFailed
	case !provisionerJob.StartedAt.Valid:
		return codersdk.ProvisionerJobPending
	case provisionerJob.CompletedAt.Valid:
		if provisionerJob.Error.String == "" {
			return codersdk.ProvisionerJobSucceeded
		}
		return codersdk.ProvisionerJobFailed
	case database.Now().Sub(provisionerJob.UpdatedAt) > 30*time.Second:
		provisionerJob.Error.String = "Worker failed to update job in time."
		return codersdk.ProvisionerJobFailed
	default:
		return codersdk.ProvisionerJobRunning
	}
}

func provisionerJobLogsChannel(jobID uuid.UUID) string {
	return fmt.Sprintf("provisioner-log-logs:%s", jobID)
}

// provisionerJobLogsMessage is the message type published on the provisionerJobLogsChannel() channel
type provisionerJobLogsMessage struct {
	CreatedAfter int64 `json:"created_after"`
	EndOfLogs    bool  `json:"end_of_logs,omitempty"`
}

func (api *API) followLogs(actor rbac.Subject, jobID uuid.UUID) (<-chan database.ProvisionerJobLog, func(), error) {
	logger := api.Logger.With(slog.F("job_id", jobID))

	var (
		closed       = make(chan struct{})
		bufferedLogs = make(chan database.ProvisionerJobLog, 128)
		logMut       = &sync.Mutex{}
	)
	closeSubscribe, err := api.Pubsub.Subscribe(
		provisionerJobLogsChannel(jobID),
		func(ctx context.Context, message []byte) {
			select {
			case <-closed:
				return
			default:
			}

			jlMsg := provisionerJobLogsMessage{}
			err := json.Unmarshal(message, &jlMsg)
			if err != nil {
				logger.Warn(ctx, "invalid provisioner job log on channel", slog.Error(err))
				return
			}

			if jlMsg.CreatedAfter != 0 {
				logs, err := api.Database.GetProvisionerLogsByIDBetween(dbauthz.As(ctx, actor), database.GetProvisionerLogsByIDBetweenParams{
					JobID:        jobID,
					CreatedAfter: jlMsg.CreatedAfter,
				})
				if err != nil {
					logger.Warn(ctx, "get provisioner logs", slog.Error(err))
					return
				}

				for _, log := range logs {
					// Sadly we have to use a mutex here because events may be
					// handled out of order due to golang goroutine scheduling
					// semantics (even though Postgres guarantees ordering of
					// notifications).
					logMut.Lock()
					select {
					case <-closed:
						logMut.Unlock()
						return
					default:
					}

					select {
					case bufferedLogs <- log:
						logger.Debug(ctx, "subscribe buffered log", slog.F("stage", log.Stage))
					default:
						// If this overflows users could miss logs streaming. This can happen
						// we get a lot of logs and consumer isn't keeping up.  We don't want to block the pubsub,
						// so just drop them.
						logger.Warn(ctx, "provisioner job log overflowing channel")
					}
					logMut.Unlock()
				}
			}

			if jlMsg.EndOfLogs {
				// This mutex is to guard double-closes.
				logMut.Lock()
				select {
				case <-closed:
					logMut.Unlock()
					return
				default:
				}
				logger.Debug(ctx, "got End of Logs")

				close(closed)
				close(bufferedLogs)
				logMut.Unlock()
			}
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return bufferedLogs, closeSubscribe, nil
}
