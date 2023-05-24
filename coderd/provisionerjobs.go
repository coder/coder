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

	"github.com/google/uuid"
	"go.uber.org/atomic"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/db2sdk"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
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
		actor, _ = dbauthz.ActorFromContext(ctx)
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
		logs, err := api.Database.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
			JobID:        job.ID,
			CreatedAfter: after,
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

		logger.Debug(ctx, "Finished non-follow job logs")
		httpapi.Write(ctx, rw, http.StatusOK, convertProvisionerJobLogs(logs))
		return
	}

	// if we are following logs, start the subscription before we query the database, so that we don't miss any logs
	// between the end of our query and the start of the subscription.  We might get duplicates, so we'll keep track
	// of processed IDs.
	var bufferedLogs <-chan *database.ProvisionerJobLog
	if follow {
		bl, closeFollow, err := api.followProvisionerJobLogs(actor, job.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error watching provisioner logs.",
				Detail:  err.Error(),
			})
			return
		}
		defer closeFollow()
		bufferedLogs = bl
	}

	logs, err := api.Database.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
		JobID:        job.ID,
		CreatedAfter: after,
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
	job, err = api.Database.GetProvisionerJobByID(ctx, job.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if job.CompletedAt.Valid {
		// job was complete before we queried the database for historical logs
		return
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug(context.Background(), "job logs context canceled")
			return
		case log, ok := <-bufferedLogs:
			// A nil log is sent when complete!
			if !ok || log == nil {
				logger.Debug(context.Background(), "reached the end of published logs")
				return
			}
			if logIdsDone[log.ID] {
				logger.Debug(ctx, "subscribe duplicated log",
					slog.F("stage", log.Stage))
			} else {
				logger.Debug(ctx, "subscribe encoding log",
					slog.F("stage", log.Stage))
				err = encoder.Encode(convertProvisionerJobLog(*log))
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

			apiAgent, err := convertWorkspaceAgent(
				api.DERPMap, *api.TailnetCoordinator.Load(), agent, convertApps(dbApps), api.AgentInactiveDisconnectTimeout,
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

func convertProvisionerJob(provisionerJob database.ProvisionerJob) codersdk.ProvisionerJob {
	job := codersdk.ProvisionerJob{
		ID:        provisionerJob.ID,
		CreatedAt: provisionerJob.CreatedAt,
		Error:     provisionerJob.Error.String,
		ErrorCode: codersdk.JobErrorCode(provisionerJob.ErrorCode.String),
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
	job.Status = db2sdk.ProvisionerJobStatus(provisionerJob)

	return job
}

func provisionerJobLogsChannel(jobID uuid.UUID) string {
	return fmt.Sprintf("provisioner-log-logs:%s", jobID)
}

// provisionerJobLogsMessage is the message type published on the provisionerJobLogsChannel() channel
type provisionerJobLogsMessage struct {
	CreatedAfter int64 `json:"created_after"`
	EndOfLogs    bool  `json:"end_of_logs,omitempty"`
}

func (api *API) followProvisionerJobLogs(actor rbac.Subject, jobID uuid.UUID) (<-chan *database.ProvisionerJobLog, func(), error) {
	logger := api.Logger.With(slog.F("job_id", jobID))

	var (
		// With debug logging enabled length = 128 is insufficient
		bufferedLogs  = make(chan *database.ProvisionerJobLog, 1024)
		endOfLogs     atomic.Bool
		lastSentLogID atomic.Int64
	)

	sendLog := func(log *database.ProvisionerJobLog) {
		select {
		case bufferedLogs <- log:
			logger.Debug(context.Background(), "subscribe buffered log", slog.F("stage", log.Stage))
			lastSentLogID.Store(log.ID)
		default:
			// If this overflows users could miss logs streaming. This can happen
			// we get a lot of logs and consumer isn't keeping up.  We don't want to block the pubsub,
			// so just drop them.
			logger.Warn(context.Background(), "provisioner job log overflowing channel")
		}
	}

	closeSubscribe, err := api.Pubsub.Subscribe(
		provisionerJobLogsChannel(jobID),
		func(ctx context.Context, message []byte) {
			if endOfLogs.Load() {
				return
			}
			jlMsg := provisionerJobLogsMessage{}
			err := json.Unmarshal(message, &jlMsg)
			if err != nil {
				logger.Warn(ctx, "invalid provisioner job log on channel", slog.Error(err))
				return
			}

			// CreatedAfter is sent when logs are streaming!
			if jlMsg.CreatedAfter != 0 {
				logs, err := api.Database.GetProvisionerLogsAfterID(dbauthz.As(ctx, actor), database.GetProvisionerLogsAfterIDParams{
					JobID:        jobID,
					CreatedAfter: jlMsg.CreatedAfter,
				})
				if err != nil {
					logger.Warn(ctx, "get provisioner logs", slog.Error(err))
					return
				}
				for _, log := range logs {
					if endOfLogs.Load() {
						// An end of logs message came in while we were fetching
						// logs or processing them!
						return
					}
					log := log
					sendLog(&log)
				}
			}

			// EndOfLogs is sent when logs are done streaming.
			// We don't want to end the stream until we've sent all the logs,
			// so we fetch logs after the last ID we've seen and send them!
			if jlMsg.EndOfLogs {
				endOfLogs.Store(true)
				logs, err := api.Database.GetProvisionerLogsAfterID(dbauthz.As(ctx, actor), database.GetProvisionerLogsAfterIDParams{
					JobID:        jobID,
					CreatedAfter: lastSentLogID.Load(),
				})
				if err != nil {
					logger.Warn(ctx, "get provisioner logs", slog.Error(err))
					return
				}
				for _, log := range logs {
					log := log
					sendLog(&log)
				}
				logger.Debug(ctx, "got End of Logs")
				bufferedLogs <- nil
			}
		},
	)
	if err != nil {
		return nil, nil, err
	}
	// We don't need to close the bufferedLogs channel because it will be garbage collected!
	return bufferedLogs, closeSubscribe, nil
}
