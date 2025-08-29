package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/taskname"
	"github.com/coder/coder/v2/codersdk"
)

// This endpoint is experimental and not guaranteed to be stable, so we're not
// generating public-facing documentation for it.
func (api *API) aiTasksPrompts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	buildIDsParam := r.URL.Query().Get("build_ids")
	if buildIDsParam == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "build_ids query parameter is required",
		})
		return
	}

	// Parse build IDs
	buildIDStrings := strings.Split(buildIDsParam, ",")
	buildIDs := make([]uuid.UUID, 0, len(buildIDStrings))
	for _, idStr := range buildIDStrings {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid build ID format: %s", idStr),
				Detail:  err.Error(),
			})
			return
		}
		buildIDs = append(buildIDs, id)
	}

	parameters, err := api.Database.GetWorkspaceBuildParametersByBuildIDs(ctx, buildIDs)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build parameters.",
			Detail:  err.Error(),
		})
		return
	}

	promptsByBuildID := make(map[string]string, len(parameters))
	for _, param := range parameters {
		if param.Name != codersdk.AITaskPromptParameterName {
			continue
		}
		buildID := param.WorkspaceBuildID.String()
		promptsByBuildID[buildID] = param.Value
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AITasksPromptsResponse{
		Prompts: promptsByBuildID,
	})
}

// This endpoint is experimental and not guaranteed to be stable, so we're not
// generating public-facing documentation for it.
func (api *API) tasksCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		apiKey  = httpmw.APIKey(r)
		auditor = api.Auditor.Load()
		mems    = httpmw.OrganizationMembersParam(r)
	)

	var req codersdk.CreateTaskRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	hasAITask, err := api.Database.GetTemplateVersionHasAITask(ctx, req.TemplateVersionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || rbac.IsUnauthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching whether the template version has an AI task.",
			Detail:  err.Error(),
		})
		return
	}
	if !hasAITask {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf(`Template does not have required parameter %q`, codersdk.AITaskPromptParameterName),
		})
		return
	}

	taskName := taskname.GenerateFallback()
	if anthropicAPIKey := taskname.GetAnthropicAPIKeyFromEnv(); anthropicAPIKey != "" {
		anthropicModel := taskname.GetAnthropicModelFromEnv()

		generatedName, err := taskname.Generate(ctx, req.Prompt, taskname.WithAPIKey(anthropicAPIKey), taskname.WithModel(anthropicModel))
		if err != nil {
			api.Logger.Error(ctx, "unable to generate task name", slog.Error(err))
		} else {
			taskName = generatedName
		}
	}

	createReq := codersdk.CreateWorkspaceRequest{
		Name:                    taskName,
		TemplateVersionID:       req.TemplateVersionID,
		TemplateVersionPresetID: req.TemplateVersionPresetID,
		RichParameterValues: []codersdk.WorkspaceBuildParameter{
			{Name: codersdk.AITaskPromptParameterName, Value: req.Prompt},
		},
	}

	var owner workspaceOwner
	if mems.User != nil {
		// This user fetch is an optimization path for the most common case of creating a
		// task for 'Me'.
		//
		// This is also required to allow `owners` to create workspaces for users
		// that are not in an organization.
		owner = workspaceOwner{
			ID:        mems.User.ID,
			Username:  mems.User.Username,
			AvatarURL: mems.User.AvatarURL,
		}
	} else {
		// A task can still be created if the caller can read the organization
		// member. The organization is required, which can be sourced from the
		// template.
		//
		// TODO: This code gets called twice for each workspace build request.
		//   This is inefficient and costs at most 2 extra RTTs to the DB.
		//   This can be optimized. It exists as it is now for code simplicity.
		//   The most common case is to create a workspace for 'Me'. Which does
		//   not enter this code branch.
		template, err := requestTemplate(ctx, createReq, api.Database)
		if err != nil {
			httperror.WriteResponseError(ctx, rw, err)
			return
		}

		// If the caller can find the organization membership in the same org
		// as the template, then they can continue.
		orgIndex := slices.IndexFunc(mems.Memberships, func(mem httpmw.OrganizationMember) bool {
			return mem.OrganizationID == template.OrganizationID
		})
		if orgIndex == -1 {
			httpapi.ResourceNotFound(rw)
			return
		}

		member := mems.Memberships[orgIndex]
		owner = workspaceOwner{
			ID:        member.UserID,
			Username:  member.Username,
			AvatarURL: member.AvatarURL,
		}
	}

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: owner.Username,
		},
	})

	defer commitAudit()
	w, err := createWorkspace(ctx, aReq, apiKey.UserID, api, owner, createReq, r)
	if err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, w)
}

// tasksFromWorkspaces converts a slice of API workspaces into tasks, fetching
// prompts and mapping status/state. This method enforces that only AI task
// workspaces are given.
func (api *API) tasksFromWorkspaces(ctx context.Context, apiWorkspaces []codersdk.Workspace) ([]codersdk.Task, error) {
	// Fetch prompts for each workspace build and map by build ID.
	buildIDs := make([]uuid.UUID, 0, len(apiWorkspaces))
	for _, ws := range apiWorkspaces {
		buildIDs = append(buildIDs, ws.LatestBuild.ID)
	}
	parameters, err := api.Database.GetWorkspaceBuildParametersByBuildIDs(ctx, buildIDs)
	if err != nil {
		return nil, err
	}
	promptsByBuildID := make(map[uuid.UUID]string, len(parameters))
	for _, p := range parameters {
		if p.Name == codersdk.AITaskPromptParameterName {
			promptsByBuildID[p.WorkspaceBuildID] = p.Value
		}
	}

	tasks := make([]codersdk.Task, 0, len(apiWorkspaces))
	for _, ws := range apiWorkspaces {
		// TODO(DanielleMaywood):
		// This just picks up the first agent it discovers.
		// This approach _might_ break when a task has multiple agents,
		// depending on which agent was found first.
		//
		// We explicitly do not have support for running tasks
		// inside of a sub agent at the moment, so we can be sure
		// that any sub agents are not the agent we're looking for.
		var taskAgentID uuid.NullUUID
		var taskAgentLifecycle *codersdk.WorkspaceAgentLifecycle
		var taskAgentHealth *codersdk.WorkspaceAgentHealth
		for _, resource := range ws.LatestBuild.Resources {
			for _, agent := range resource.Agents {
				if agent.ParentID.Valid {
					continue
				}

				taskAgentID = uuid.NullUUID{Valid: true, UUID: agent.ID}
				taskAgentLifecycle = &agent.LifecycleState
				taskAgentHealth = &agent.Health
				break
			}
		}

		var currentState *codersdk.TaskStateEntry
		if ws.LatestAppStatus != nil {
			currentState = &codersdk.TaskStateEntry{
				Timestamp: ws.LatestAppStatus.CreatedAt,
				State:     codersdk.TaskState(ws.LatestAppStatus.State),
				Message:   ws.LatestAppStatus.Message,
				URI:       ws.LatestAppStatus.URI,
			}
		}

		tasks = append(tasks, codersdk.Task{
			ID:                      ws.ID,
			OrganizationID:          ws.OrganizationID,
			OwnerID:                 ws.OwnerID,
			OwnerName:               ws.OwnerName,
			Name:                    ws.Name,
			TemplateID:              ws.TemplateID,
			TemplateName:            ws.TemplateName,
			TemplateDisplayName:     ws.TemplateDisplayName,
			TemplateIcon:            ws.TemplateIcon,
			WorkspaceID:             uuid.NullUUID{Valid: true, UUID: ws.ID},
			WorkspaceAgentID:        taskAgentID,
			WorkspaceAgentLifecycle: taskAgentLifecycle,
			WorkspaceAgentHealth:    taskAgentHealth,
			CreatedAt:               ws.CreatedAt,
			UpdatedAt:               ws.UpdatedAt,
			InitialPrompt:           promptsByBuildID[ws.LatestBuild.ID],
			Status:                  ws.LatestBuild.Status,
			CurrentState:            currentState,
		})
	}

	return tasks, nil
}

// tasksListResponse wraps a list of experimental tasks.
//
// Experimental: Response shape is experimental and may change.
type tasksListResponse struct {
	Tasks []codersdk.Task `json:"tasks"`
	Count int             `json:"count"`
}

// tasksList is an experimental endpoint to list AI tasks by mapping
// workspaces to a task-shaped response.
func (api *API) tasksList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	// Support standard pagination/filters for workspaces.
	page, ok := ParsePagination(rw, r)
	if !ok {
		return
	}
	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.Workspaces(ctx, api.Database, queryStr, page, api.AgentInactiveDisconnectTimeout)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid workspace search query.",
			Validations: errs,
		})
		return
	}

	// Ensure that we only include AI task workspaces in the results.
	filter.HasAITask = sql.NullBool{Valid: true, Bool: true}

	if filter.OwnerUsername == "me" {
		filter.OwnerID = apiKey.UserID
		filter.OwnerUsername = ""
	}

	prepared, err := api.HTTPAuth.AuthorizeSQLFilter(r, policy.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error preparing sql filter.",
			Detail:  err.Error(),
		})
		return
	}

	// Order with requester's favorites first, include summary row.
	filter.RequesterID = apiKey.UserID
	filter.WithSummary = true

	workspaceRows, err := api.Database.GetAuthorizedWorkspaces(ctx, filter, prepared)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  err.Error(),
		})
		return
	}
	if len(workspaceRows) == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  "Workspace summary row is missing.",
		})
		return
	}
	if len(workspaceRows) == 1 {
		httpapi.Write(ctx, rw, http.StatusOK, tasksListResponse{
			Tasks: []codersdk.Task{},
			Count: 0,
		})
		return
	}

	// Skip summary row.
	workspaceRows = workspaceRows[:len(workspaceRows)-1]

	workspaces := database.ConvertWorkspaceRows(workspaceRows)

	// Gather associated data and convert to API workspaces.
	data, err := api.workspaceData(ctx, workspaces)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}
	apiWorkspaces, err := convertWorkspaces(apiKey.UserID, workspaces, data)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspaces.",
			Detail:  err.Error(),
		})
		return
	}

	tasks, err := api.tasksFromWorkspaces(ctx, apiWorkspaces)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching task prompts and states.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, tasksListResponse{
		Tasks: tasks,
		Count: len(tasks),
	})
}

// taskGet is an experimental endpoint to fetch a single AI task by ID
// (workspace ID). It returns a synthesized task response including
// prompt and status.
func (api *API) taskGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	idStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(idStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid UUID %q for task ID.", idStr),
		})
		return
	}

	// For now, taskID = workspaceID, once we have a task data model in
	// the DB, we can change this lookup.
	workspaceID := taskID
	workspace, err := api.Database.GetWorkspaceByID(ctx, workspaceID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}

	data, err := api.workspaceData(ctx, []database.Workspace{workspace})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}
	if len(data.builds) == 0 || len(data.templates) == 0 {
		httpapi.ResourceNotFound(rw)
		return
	}
	if data.builds[0].HasAITask == nil || !*data.builds[0].HasAITask {
		httpapi.ResourceNotFound(rw)
		return
	}

	appStatus := codersdk.WorkspaceAppStatus{}
	if len(data.appStatuses) > 0 {
		appStatus = data.appStatuses[0]
	}

	ws, err := convertWorkspace(
		apiKey.UserID,
		workspace,
		data.builds[0],
		data.templates[0],
		api.Options.AllowWorkspaceRenames,
		appStatus,
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace.",
			Detail:  err.Error(),
		})
		return
	}

	tasks, err := api.tasksFromWorkspaces(ctx, []codersdk.Workspace{ws})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching task prompt and state.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, tasks[0])
}

// taskDelete is an experimental endpoint to delete a task by ID (workspace ID).
// It creates a delete workspace build and returns 202 Accepted if the build was
// created.
func (api *API) taskDelete(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	idStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(idStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid UUID %q for task ID.", idStr),
		})
		return
	}

	// For now, taskID = workspaceID, once we have a task data model in
	// the DB, we can change this lookup.
	workspaceID := taskID
	workspace, err := api.Database.GetWorkspaceByID(ctx, workspaceID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}

	data, err := api.workspaceData(ctx, []database.Workspace{workspace})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
		return
	}
	if len(data.builds) == 0 || len(data.templates) == 0 {
		httpapi.ResourceNotFound(rw)
		return
	}
	if data.builds[0].HasAITask == nil || !*data.builds[0].HasAITask {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Construct a request to the workspace build creation handler to
	// initiate deletion.
	buildReq := codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
		Reason:     "Deleted via tasks API",
	}

	_, err = api.postWorkspaceBuildsInternal(
		ctx,
		apiKey,
		workspace,
		buildReq,
		func(action policy.Action, object rbac.Objecter) bool {
			return api.Authorize(r, action, object)
		},
		audit.WorkspaceBuildBaggageFromRequest(r),
	)
	if err != nil {
		httperror.WriteWorkspaceBuildError(ctx, rw, err)
		return
	}

	// Delete build created successfully.
	rw.WriteHeader(http.StatusAccepted)
}
