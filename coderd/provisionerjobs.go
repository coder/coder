package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// Returns provisioner logs based on query parameters.
// The intended usage for a client to stream all logs (with JS API):
// const timestamp = new Date().getTime();
// 1. GET /logs?before=<timestamp>
// 2. GET /logs?after=<timestamp>&follow
// The combination of these responses should provide all current logs
// to the consumer, and future logs are streamed in the follow request.
func (api *API) provisionerJobLogs(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
	follow := r.URL.Query().Has("follow")
	afterRaw := r.URL.Query().Get("after")
	beforeRaw := r.URL.Query().Get("before")
	if beforeRaw != "" && follow {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Query param \"before\" cannot be used with \"follow\".",
		})
		return
	}

	var after time.Time
	// Only fetch logs created after the time provided.
	if afterRaw != "" {
		afterMS, err := strconv.ParseInt(afterRaw, 10, 64)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: "Query param \"after\" must be an integer.",
				Validations: []httpapi.Error{
					{Field: "after", Detail: "Must be an integer"},
				},
			})
			return
		}
		after = time.UnixMilli(afterMS)
	} else {
		if follow {
			after = database.Now()
		}
	}
	var before time.Time
	// Only fetch logs created before the time provided.
	if beforeRaw != "" {
		beforeMS, err := strconv.ParseInt(beforeRaw, 10, 64)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: "Query param \"before\" must be an integer.",
				Validations: []httpapi.Error{
					{Field: "before", Detail: "Must be an integer"},
				},
			})
			return
		}
		before = time.UnixMilli(beforeMS)
	} else {
		// If we're following, we don't want logs before a timestamp!
		if !follow {
			before = database.Now()
		}
	}

	if !follow {
		logs, err := api.Database.GetProvisionerLogsByIDBetween(r.Context(), database.GetProvisionerLogsByIDBetweenParams{
			JobID:         job.ID,
			CreatedAfter:  after,
			CreatedBefore: before,
		})
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: "Internal error fetching provisioner logs.",
				Detail:  err.Error(),
			})
			return
		}
		if logs == nil {
			logs = []database.ProvisionerJobLog{}
		}

		httpapi.Write(rw, http.StatusOK, convertProvisionerJobLogs(logs))
		return
	}

	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, wsNetConn := websocketNetConn(r.Context(), conn, websocket.MessageText)
	defer wsNetConn.Close() // Also closes conn.

	bufferedLogs := make(chan database.ProvisionerJobLog, 128)
	closeSubscribe, err := api.Pubsub.Subscribe(provisionerJobLogsChannel(job.ID), func(ctx context.Context, message []byte) {
		var logs []database.ProvisionerJobLog
		err := json.Unmarshal(message, &logs)
		if err != nil {
			api.Logger.Warn(ctx, fmt.Sprintf("invalid provisioner job log on channel %q: %s", provisionerJobLogsChannel(job.ID), err.Error()))
			return
		}

		for _, log := range logs {
			select {
			case bufferedLogs <- log:
			default:
				// If this overflows users could miss logs streaming. This can happen
				// if a database request takes a long amount of time, and we get a lot of logs.
				api.Logger.Warn(ctx, "provisioner job log overflowing channel")
			}
		}
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error watching provisioner logs.",
			Detail:  err.Error(),
		})
		return
	}
	defer closeSubscribe()

	provisionerJobLogs, err := api.Database.GetProvisionerLogsByIDBetween(ctx, database.GetProvisionerLogsByIDBetweenParams{
		JobID:         job.ID,
		CreatedAfter:  after,
		CreatedBefore: before,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching provisioner logs.",
			Detail:  err.Error(),
		})
		return
	}

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(wsNetConn)
	for _, provisionerJobLog := range provisionerJobLogs {
		err = encoder.Encode(convertProvisionerJobLog(provisionerJobLog))
		if err != nil {
			return
		}
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case log := <-bufferedLogs:
			err = encoder.Encode(convertProvisionerJobLog(log))
			if err != nil {
				return
			}
		case <-ticker.C:
			job, err := api.Database.GetProvisionerJobByID(r.Context(), job.ID)
			if err != nil {
				api.Logger.Warn(r.Context(), "streaming job logs; checking if completed", slog.Error(err), slog.F("job_id", job.ID.String()))
				continue
			}
			if job.CompletedAt.Valid {
				return
			}
		}
	}
}

func (api *API) provisionerJobResources(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	resources, err := api.Database.GetWorkspaceResourcesByJobID(r.Context(), job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching job resources.",
			Detail:  err.Error(),
		})
		return
	}
	resourceIDs := make([]uuid.UUID, 0)
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ID)
	}
	resourceAgents, err := api.Database.GetWorkspaceAgentsByResourceIDs(r.Context(), resourceIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	resourceAgentIDs := make([]uuid.UUID, 0)
	for _, agent := range resourceAgents {
		resourceAgentIDs = append(resourceAgentIDs, agent.ID)
	}
	apps, err := api.Database.GetWorkspaceAppsByAgentIDs(r.Context(), resourceAgentIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: "Internal error fetching workspace applications.",
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

			apiAgent, err := convertWorkspaceAgent(agent, convertApps(dbApps), api.AgentInactiveDisconnectTimeout)
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: "Internal error reading job agent.",
					Detail:  err.Error(),
				})
				return
			}
			agents = append(agents, apiAgent)
		}
		apiResources = append(apiResources, convertWorkspaceResource(resource, agents))
	}

	httpapi.Write(rw, http.StatusOK, apiResources)
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
		ID:            provisionerJob.ID,
		CreatedAt:     provisionerJob.CreatedAt,
		Error:         provisionerJob.Error.String,
		StorageSource: provisionerJob.StorageSource,
	}
	// Applying values optional to the struct.
	if provisionerJob.StartedAt.Valid {
		job.StartedAt = &provisionerJob.StartedAt.Time
	}
	if provisionerJob.CompletedAt.Valid {
		job.CompletedAt = &provisionerJob.CompletedAt.Time
	}
	if provisionerJob.WorkerID.Valid {
		job.WorkerID = &provisionerJob.WorkerID.UUID
	}

	switch {
	case provisionerJob.CanceledAt.Valid:
		if provisionerJob.CompletedAt.Valid {
			job.Status = codersdk.ProvisionerJobCanceled
		} else {
			job.Status = codersdk.ProvisionerJobCanceling
		}
	case !provisionerJob.StartedAt.Valid:
		job.Status = codersdk.ProvisionerJobPending
	case provisionerJob.CompletedAt.Valid:
		if job.Error == "" {
			job.Status = codersdk.ProvisionerJobSucceeded
		} else {
			job.Status = codersdk.ProvisionerJobFailed
		}
	case database.Now().Sub(provisionerJob.UpdatedAt) > 30*time.Second:
		job.Status = codersdk.ProvisionerJobFailed
		job.Error = "Worker failed to update job in time."
	default:
		job.Status = codersdk.ProvisionerJobRunning
	}

	return job
}

func provisionerJobLogsChannel(jobID uuid.UUID) string {
	return fmt.Sprintf("provisioner-log-logs:%s", jobID)
}
