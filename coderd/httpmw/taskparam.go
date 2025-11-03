package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

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

// ExtractTaskParam grabs a task from the "task" URL parameter.
// It supports two lookup strategies:
// 1. Task UUID (primary)
// 2. Task name scoped to owner (secondary)
//
// This middleware depends on ExtractOrganizationMembersParam being in the chain
// to provide the owner context for name-based lookups.
func ExtractTaskParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Get the task parameter value. We can't use ParseUUIDParam here because
			// we need to support non-UUID values (task names) and
			// attempt all lookup strategies.
			taskParam := chi.URLParam(r, "task")
			if taskParam == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "\"task\" must be provided.",
				})
				return
			}

			// Get owner from OrganizationMembersParam middleware for name-based lookups
			members := OrganizationMembersParam(r)
			ownerID := members.UserID()

			task, err := fetchTaskWithFallback(ctx, db, taskParam, ownerID)
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
				rlogger.WithFields(
					slog.F("task_id", task.ID),
					slog.F("task_name", task.Name),
				)
			}

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

func fetchTaskWithFallback(ctx context.Context, db database.Store, taskParam string, ownerID uuid.UUID) (database.Task, error) {
	// Attempt to first lookup the task by UUID.
	taskID, err := uuid.Parse(taskParam)
	if err == nil {
		task, err := db.GetTaskByID(ctx, taskID)
		if err == nil {
			return task, nil
		}
		// There may be a task named with a valid UUID. Fall back to name lookup in this case.
		if !errors.Is(err, sql.ErrNoRows) {
			return database.Task{}, xerrors.Errorf("fetch task by uuid: %w", err)
		}
	}

	// taskParam not a valid UUID, OR valid UUID but not found, so attempt lookup by name.
	task, err := db.GetTaskByOwnerIDAndName(ctx, database.GetTaskByOwnerIDAndNameParams{
		OwnerID: ownerID,
		Name:    taskParam,
	})
	if err != nil {
		return database.Task{}, xerrors.Errorf("fetch task by name: %w", err)
	}
	return task, nil
}
