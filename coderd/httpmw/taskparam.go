package httpmw

import (
	"context"
	"net/http"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/codersdk"
)

type taskParamContextKey struct{}

// TaskParam returns the task from the ExtractTaskParam handler.
func TaskParam(r *http.Request) database.Task {
	task, ok := r.Context().Value(taskParamContextKey{}).(database.Task)
	if !ok {
		panic("developer error: task param middleware not provided")
	}
	return task
}

// ExtractTaskParam grabs a task from the "task" URL parameter by UUID.
func ExtractTaskParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			taskID, parsed := ParseUUIDParam(rw, r, "task")
			if !parsed {
				return
			}
			task, err := db.GetTaskByID(ctx, taskID)
			if err != nil {
				if httpapi.Is404Error(err) {
					httpapi.ResourceNotFound(rw)
					return
				}
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching task.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, taskParamContextKey{}, task)

			if rlogger := loggermw.RequestLoggerFromContext(ctx); rlogger != nil {
				rlogger.WithFields(slog.F("task_id", task.ID), slog.F("task_name", task.Name))
			}

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
