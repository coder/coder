package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

func TestTaskParam(t *testing.T) {
	t.Parallel()

	// Create all fixtures once - they're only read, never modified
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	_, token := dbgen.APIKey(t, db, database.APIKey{
		UserID: user.ID,
	})
	org := dbgen.Organization(t, db, database.Organization{})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID: uuid.NullUUID{
			UUID:  tpl.ID,
			Valid: true,
		},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
	})
	task := dbgen.Task(t, db, database.TaskTable{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		TemplateVersionID: tv.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		Prompt:            "test prompt",
	})
	workspaceNoTask := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
	})
	taskFoundByUUID := dbgen.Task(t, db, database.TaskTable{
		Name:              "found-by-uuid",
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		TemplateVersionID: tv.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		Prompt:            "test prompt",
	})
	// To test precedence of UUID over name, we create another task with the same name as the UUID task
	_ = dbgen.Task(t, db, database.TaskTable{
		Name:              taskFoundByUUID.ID.String(),
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		TemplateVersionID: tv.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		Prompt:            "test prompt",
	})
	workspaceSharedName := dbgen.Workspace(t, db, database.WorkspaceTable{
		Name:           "shared-name",
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
	})
	// We create a task with the same name as the workspace shared name.
	_ = dbgen.Task(t, db, database.TaskTable{
		Name:              "task-different-name",
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		TemplateVersionID: tv.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspaceSharedName.ID, Valid: true},
		Prompt:            "test prompt",
	})

	makeRequest := func(userID uuid.UUID, sessionToken string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, sessionToken)

		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("user", userID.String())
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
		return r
	}

	makeRouter := func(handler http.HandlerFunc) chi.Router {
		rtr := chi.NewRouter()
		rtr.Use(
			httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
				DB:              db,
				RedirectToLogin: false,
			}),
			httpmw.ExtractOrganizationMembersParam(db, func(r *http.Request, _ policy.Action, _ rbac.Objecter) bool {
				return true
			}),
			httpmw.ExtractTaskParam(db),
		)
		rtr.Get("/", handler)
		return rtr
	}

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		rtr := chi.NewRouter()
		rtr.Use(httpmw.ExtractTaskParam(db))
		rtr.Get("/", func(w http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "this should never get called")
		})
		r := httptest.NewRequest("GET", "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "this should never get called")
		})
		r := makeRequest(user.ID, token)
		chi.RouteContext(r.Context()).URLParams.Add("task", uuid.NewString())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			foundTask := httpmw.TaskParam(r)
			assert.Equal(t, task.ID.String(), foundTask.ID.String())
		})
		r := makeRequest(user.ID, token)
		chi.RouteContext(r.Context()).URLParams.Add("task", task.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("FoundByTaskName", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			foundTask := httpmw.TaskParam(r)
			assert.Equal(t, task.ID.String(), foundTask.ID.String())
		})
		r := makeRequest(user.ID, token)
		chi.RouteContext(r.Context()).URLParams.Add("task", task.Name)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("NotFoundByWorkspaceName", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "this should never get called")
		})
		r := makeRequest(user.ID, token)
		chi.RouteContext(r.Context()).URLParams.Add("task", workspace.Name)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("CaseInsensitiveTaskName", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			foundTask := httpmw.TaskParam(r)
			assert.Equal(t, task.ID.String(), foundTask.ID.String())
		})
		r := makeRequest(user.ID, token)
		// Look up with different case
		chi.RouteContext(r.Context()).URLParams.Add("task", strings.ToUpper(task.Name))
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("UUIDTakesPrecedence", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			foundTask := httpmw.TaskParam(r)
			assert.Equal(t, taskFoundByUUID.ID.String(), foundTask.ID.String())
		})
		r := makeRequest(user.ID, token)
		// Look up by UUID - should find the first task, not the one named with the UUID
		chi.RouteContext(r.Context()).URLParams.Add("task", taskFoundByUUID.ID.String())
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("NotFoundWhenNoMatch", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "this should never get called")
		})
		r := makeRequest(user.ID, token)
		chi.RouteContext(r.Context()).URLParams.Add("task", "nonexistent-name")
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("WorkspaceWithoutTask", func(t *testing.T) {
		t.Parallel()
		rtr := makeRouter(func(w http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "this should never get called")
		})
		r := makeRequest(user.ID, token)
		// Look up by workspace name, but workspace has no task
		chi.RouteContext(r.Context()).URLParams.Add("task", workspaceNoTask.Name)
		rw := httptest.NewRecorder()
		rtr.ServeHTTP(rw, r)

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})
}
