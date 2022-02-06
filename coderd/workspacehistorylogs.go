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

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// WorkspaceHistoryLog represents a single log from workspace history.
type WorkspaceHistoryLog struct {
	ID        uuid.UUID
	CreatedAt time.Time          `json:"created_at"`
	Source    database.LogSource `json:"log_source"`
	Level     database.LogLevel  `json:"log_level"`
	Output    string             `json:"output"`
}

// Returns workspace history logs based on query parameters.
// The intended usage for a client to stream all logs (with JS API):
// const timestamp = new Date().getTime();
// 1. GET /logs?before=<timestamp>
// 2. GET /logs?after=<timestamp>&follow
// The combination of these responses should provide all current logs
// to the consumer, and future logs are streamed in the follow request.
func (api *api) workspaceHistoryLogsByName(rw http.ResponseWriter, r *http.Request) {
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

	history := httpmw.WorkspaceHistoryParam(r)
	if !follow {
		logs, err := api.Database.GetWorkspaceHistoryLogsByIDBetween(r.Context(), database.GetWorkspaceHistoryLogsByIDBetweenParams{
			WorkspaceHistoryID: history.ID,
			CreatedAfter:       after,
			CreatedBefore:      before,
		})
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get workspace history: %s", err),
			})
			return
		}
		if logs == nil {
			logs = []database.WorkspaceHistoryLog{}
		}
		render.Status(r, http.StatusOK)
		render.JSON(rw, r, logs)
		return
	}

	bufferedLogs := make(chan database.WorkspaceHistoryLog, 128)
	closeSubscribe, err := api.Pubsub.Subscribe(workspaceHistoryLogsChannel(history.ID), func(ctx context.Context, message []byte) {
		var logs []database.WorkspaceHistoryLog
		err := json.Unmarshal(message, &logs)
		if err != nil {
			api.Logger.Warn(r.Context(), fmt.Sprintf("invalid workspace log on channel %q: %s", workspaceHistoryLogsChannel(history.ID), err.Error()))
			return
		}

		for _, log := range logs {
			select {
			case bufferedLogs <- log:
			default:
				// If this overflows users could miss logs streaming. This can happen
				// if a database request takes a long amount of time, and we get a lot of logs.
				api.Logger.Warn(r.Context(), "workspace history log overflowing channel")
			}
		}
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("subscribe to workspace history logs: %s", err),
		})
		return
	}
	defer closeSubscribe()

	workspaceHistoryLogs, err := api.Database.GetWorkspaceHistoryLogsByIDBetween(r.Context(), database.GetWorkspaceHistoryLogsByIDBetweenParams{
		WorkspaceHistoryID: history.ID,
		CreatedAfter:       after,
		CreatedBefore:      before,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		workspaceHistoryLogs = []database.WorkspaceHistoryLog{}
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprint("get workspace history logs: %w", err),
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

	for _, workspaceHistoryLog := range workspaceHistoryLogs {
		err = encoder.Encode(convertWorkspaceHistoryLog(workspaceHistoryLog))
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
			err = encoder.Encode(convertWorkspaceHistoryLog(log))
			if err != nil {
				return
			}
			rw.(http.Flusher).Flush()
		case <-ticker.C:
			job, err := api.Database.GetProvisionerJobByID(r.Context(), history.ProvisionJobID)
			if err != nil {
				api.Logger.Warn(r.Context(), "streaming workspace logs; checking if job completed", slog.Error(err), slog.F("job_id", history.ProvisionJobID))
				continue
			}
			if convertProvisionerJob(job).Status.Completed() {
				return
			}
		}
	}
}

func convertWorkspaceHistoryLog(workspaceHistoryLog database.WorkspaceHistoryLog) WorkspaceHistoryLog {
	return WorkspaceHistoryLog{
		ID:        workspaceHistoryLog.ID,
		CreatedAt: workspaceHistoryLog.CreatedAt,
		Source:    workspaceHistoryLog.Source,
		Level:     workspaceHistoryLog.Level,
		Output:    workspaceHistoryLog.Output,
	}
}

func workspaceHistoryLogsChannel(workspaceHistoryID uuid.UUID) string {
	return fmt.Sprintf("workspace-history-logs:%s", workspaceHistoryID)
}
