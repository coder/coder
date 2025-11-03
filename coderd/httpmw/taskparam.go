package httpmw

import (
	"context"
	"database/sql"
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

func fetchTaskByUUID(ctx context.Context, db database.Store, taskParam string, _ uuid.UUID) (database.Task, error) {
	taskID, err := uuid.Parse(taskParam)
	if err != nil {
		// Not a valid UUID, skip this strategy
		return database.Task{}, sql.ErrNoRows
	}
	task, err := db.GetTaskByID(ctx, taskID)
	if err != nil {
		return database.Task{}, xerrors.Errorf("fetch task by uuid: %w", err)
	}
	return task, nil
}

func fetchTaskByName(ctx context.Context, db database.Store, taskParam string, ownerID uuid.UUID) (database.Task, error) {
	task, err := db.GetTaskByOwnerIDAndName(ctx, database.GetTaskByOwnerIDAndNameParams{
		OwnerID: ownerID,
		Name:    taskParam,
	})
	if err != nil {
		return database.Task{}, xerrors.Errorf("fetch task by name: %w", err)
	}
	return task, nil
}

func fetchTaskByWorkspaceName(ctx context.Context, db database.Store, taskParam string, ownerID uuid.UUID) (database.Task, error) {
	workspace, err := db.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
		OwnerID: ownerID,
		Name:    taskParam,
	})
	if err != nil {
		return database.Task{}, xerrors.Errorf("fetch workspace by name: %w", err)
	}
	// Check if workspace has an associated task before querying
	if !workspace.TaskID.Valid {
		return database.Task{}, sql.ErrNoRows
	}
	task, err := db.GetTaskByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		return database.Task{}, xerrors.Errorf("fetch task by workspace id: %w", err)
	}
	return task, nil
}

type taskFetchFunc func(ctx context.Context, db database.Store, taskParam string, ownerID uuid.UUID) (database.Task, error)

var (
	taskLookupStrategyNames = []string{"uuid", "name", "workspace"}
	taskLookupStrategyFuncs = []taskFetchFunc{fetchTaskByUUID, fetchTaskByName, fetchTaskByWorkspaceName}
)

// ExtractTaskParam grabs a task from the "task" URL parameter.
// It supports three lookup strategies with cascading fallback:
// 1. Task UUID (primary)
// 2. Task name scoped to owner (secondary)
// 3. Workspace name scoped to owner (tertiary, for legacy links)
//
// This middleware depends on ExtractOrganizationMembersParam being in the chain
// to provide the owner context for name-based lookups.
func ExtractTaskParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Get the task parameter value. We can't use ParseUUIDParam here because
			// we need to support non-UUID values (task names and workspace names) and
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

			// Try each strategy in order until one succeeds
			var task database.Task
			var foundBy string
			for i, fetch := range taskLookupStrategyFuncs {
				t, err := fetch(ctx, db, taskParam, ownerID)
				if err == nil {
					task = t
					foundBy = taskLookupStrategyNames[i]
					break
				}
				if !httpapi.Is404Error(err) {
					// Real error (not just "not found")
					httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
						Message: "Internal error fetching task.",
						Detail:  err.Error(),
					})
					return
				}
				// Continue to next strategy on 404
			}

			// If no strategy succeeded, return 404
			if foundBy == "" {
				httpapi.ResourceNotFound(rw)
				return
			}

			ctx = context.WithValue(ctx, taskParamContextKey{}, task)

			if rlogger := loggermw.RequestLoggerFromContext(ctx); rlogger != nil {
				rlogger.WithFields(
					slog.F("task_id", task.ID),
					slog.F("task_name", task.Name),
					slog.F("task_lookup_strategy", foundBy),
				)
			}

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
