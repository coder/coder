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

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

// Returns provisioner logs based on query parameters.
// The intended usage for a client to stream all logs (with JS API):
// const timestamp = new Date().getTime();
// 1. GET /logs?before=<timestamp>
// 2. GET /logs?after=<timestamp>&follow
// The combination of these responses should provide all current logs
// to the consumer, and future logs are streamed in the follow request.
func (api *api) provisionerJobLogs(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
	follow := r.URL.Query().Has("follow")
	afterRaw := r.URL.Query().Get("after")
	beforeRaw := r.URL.Query().Get("before")
	if beforeRaw != "" && follow {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: "before cannot be used with follow",
		})
		return
	}

	var after time.Time
	// Only fetch logs created after the time provided.
	if afterRaw != "" {
		afterMS, err := strconv.ParseInt(afterRaw, 10, 64)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
				Message: fmt.Sprintf("unable to parse after %q: %s", afterRaw, err),
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
				Message: fmt.Sprintf("unable to parse before %q: %s", beforeRaw, err),
			})
			return
		}
		before = time.UnixMilli(beforeMS)
	} else {
		before = database.Now()
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
				Message: fmt.Sprintf("get provisioner logs: %s", err),
			})
			return
		}
		if logs == nil {
			logs = []database.ProvisionerJobLog{}
		}
		render.Status(r, http.StatusOK)
		render.JSON(rw, r, logs)
		return
	}

	bufferedLogs := make(chan database.ProvisionerJobLog, 128)
	closeSubscribe, err := api.Pubsub.Subscribe(provisionerJobLogsChannel(job.ID), func(ctx context.Context, message []byte) {
		var logs []database.ProvisionerJobLog
		err := json.Unmarshal(message, &logs)
		if err != nil {
			api.Logger.Warn(r.Context(), fmt.Sprintf("invalid provisioner job log on channel %q: %s", provisionerJobLogsChannel(job.ID), err.Error()))
			return
		}

		for _, log := range logs {
			select {
			case bufferedLogs <- log:
			default:
				// If this overflows users could miss logs streaming. This can happen
				// if a database request takes a long amount of time, and we get a lot of logs.
				api.Logger.Warn(r.Context(), "provisioner job log overflowing channel")
			}
		}
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("subscribe to provisioner job logs: %s", err),
		})
		return
	}
	defer closeSubscribe()

	provisionerJobLogs, err := api.Database.GetProvisionerLogsByIDBetween(r.Context(), database.GetProvisionerLogsByIDBetweenParams{
		JobID:         job.ID,
		CreatedAfter:  after,
		CreatedBefore: before,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprint("get provisioner job logs: %w", err),
		})
		return
	}

	// "follow" uses the ndjson format to stream data.
	// See: https://canjs.com/doc/can-ndjson-stream.html
	rw.Header().Set("Content-Type", "application/stream+json")
	rw.WriteHeader(http.StatusOK)
	rw.(http.Flusher).Flush()

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(rw)

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
			rw.(http.Flusher).Flush()
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

func (api *api) provisionerJobResources(rw http.ResponseWriter, r *http.Request, job database.ProvisionerJob) {
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
			Message: fmt.Sprintf("get provisioner job resources: %s", err),
		})
		return
	}
	apiResources := make([]codersdk.WorkspaceResource, 0)
	for _, resource := range resources {
		if !resource.AgentID.Valid {
			apiResources = append(apiResources, convertWorkspaceResource(resource, nil))
			continue
		}
		agent, err := api.Database.GetWorkspaceAgentByResourceID(r.Context(), resource.ID)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get provisioner job agent: %s", err),
			})
			return
		}
		apiAgent, err := convertWorkspaceAgent(agent)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("convert provisioner job agent: %s", err),
			})
			return
		}
		apiResources = append(apiResources, convertWorkspaceResource(resource, &apiAgent))
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, apiResources)
}

func convertProvisionerJobLog(provisionerJobLog database.ProvisionerJobLog) codersdk.ProvisionerJobLog {
	return codersdk.ProvisionerJobLog{
		ID:        provisionerJobLog.ID,
		CreatedAt: provisionerJobLog.CreatedAt,
		Source:    provisionerJobLog.Source,
		Level:     provisionerJobLog.Level,
		Output:    provisionerJobLog.Output,
	}
}

func convertProvisionerJob(provisionerJob database.ProvisionerJob) codersdk.ProvisionerJob {
	job := codersdk.ProvisionerJob{
		ID:        provisionerJob.ID,
		CreatedAt: provisionerJob.CreatedAt,
		Error:     provisionerJob.Error.String,
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
	case database.Now().Sub(provisionerJob.UpdatedAt) > 30*time.Second:
		job.Status = codersdk.ProvisionerJobFailed
		job.Error = "Worker failed to update job in time."
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
	default:
		job.Status = codersdk.ProvisionerJobRunning
	}

	return job
}

func provisionerJobLogsChannel(jobID uuid.UUID) string {
	return fmt.Sprintf("provisioner-log-logs:%s", jobID)
}
