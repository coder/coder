package coderd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/coderd/taskname"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"

	aiagentapi "github.com/coder/agentapi-sdk-go"
)

// @Summary Create a new AI task
// @Description: EXPERIMENTAL: this endpoint is experimental and not guaranteed to be stable.
// @ID create-task
// @Security CoderSessionToken
// @Tags Experimental
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param request body codersdk.CreateTaskRequest true "Create task request"
// @Success 201 {object} codersdk.Task
// @Router /api/experimental/tasks/{user} [post]
//
// EXPERIMENTAL: This endpoint is experimental and not guaranteed to be stable.
// This endpoint creates a new task for the given user.
func (api *API) tasksCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx              = r.Context()
		apiKey           = httpmw.APIKey(r)
		auditor          = api.Auditor.Load()
		mems             = httpmw.OrganizationMembersParam(r)
		taskResourceInfo = audit.AdditionalFields{}
	)

	if mems.User != nil {
		taskResourceInfo.WorkspaceOwner = mems.User.Username
	}

	aReq, commitAudit := audit.InitRequest[database.TaskTable](rw, &audit.RequestParams{
		Audit:            *auditor,
		Log:              api.Logger,
		Request:          r,
		Action:           database.AuditActionCreate,
		AdditionalFields: taskResourceInfo,
	})

	defer commitAudit()

	var req codersdk.CreateTaskRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Fetch the template version to verify access and whether or not it has an
	// AI task.
	templateVersion, err := api.Database.GetTemplateVersionByID(ctx, req.TemplateVersionID)
	if err != nil {
		if httpapi.Is404Error(err) {
			// Avoid using httpapi.ResourceNotFound() here because this is an
			// input error and 404 would be confusing.
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Template version not found or you do not have access to this resource",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.UpdateOrganizationID(templateVersion.OrganizationID)

	if !templateVersion.HasAITask.Valid || !templateVersion.HasAITask.Bool {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: `Template does not have a valid "coder_ai_task" resource.`,
		})
		return
	}

	taskName := req.Name
	if taskName != "" {
		if err := codersdk.NameValid(taskName); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Unable to create a Task with the provided name.",
				Detail:  err.Error(),
			})
			return
		}
	}

	if taskName == "" {
		taskName = taskname.GenerateFallback()

		if anthropicAPIKey := taskname.GetAnthropicAPIKeyFromEnv(); anthropicAPIKey != "" {
			anthropicModel := taskname.GetAnthropicModelFromEnv()

			generatedName, err := taskname.Generate(ctx, req.Input, taskname.WithAPIKey(anthropicAPIKey), taskname.WithModel(anthropicModel))
			if err != nil {
				api.Logger.Error(ctx, "unable to generate task name", slog.Error(err))
			} else {
				taskName = generatedName
			}
		}
	}

	// Check if the template defines the AI Prompt parameter.
	templateParams, err := api.Database.GetTemplateVersionParameters(ctx, req.TemplateVersionID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template parameters.",
			Detail:  err.Error(),
		})
		return
	}

	var richParams []codersdk.WorkspaceBuildParameter
	if _, hasAIPromptParam := slice.Find(templateParams, func(param database.TemplateVersionParameter) bool {
		return param.Name == codersdk.AITaskPromptParameterName
	}); hasAIPromptParam {
		// Only add the AI Prompt parameter if the template defines it.
		richParams = []codersdk.WorkspaceBuildParameter{
			{Name: codersdk.AITaskPromptParameterName, Value: req.Input},
		}
	}

	createReq := codersdk.CreateWorkspaceRequest{
		Name:                    taskName,
		TemplateVersionID:       req.TemplateVersionID,
		TemplateVersionPresetID: req.TemplateVersionPresetID,
		RichParameterValues:     richParams,
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
		// templateVersion.
		//
		// If the caller can find the organization membership in the same org
		// as the template, then they can continue.
		orgIndex := slices.IndexFunc(mems.Memberships, func(mem httpmw.OrganizationMember) bool {
			return mem.OrganizationID == templateVersion.OrganizationID
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

		// Update workspace owner information for audit in case it changed.
		taskResourceInfo.WorkspaceOwner = owner.Username
	}

	// Track insert from preCreateInTX.
	var dbTaskTable database.TaskTable

	// Ensure an audit log is created for the workspace creation event.
	aReqWS, commitAuditWS := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:            *auditor,
		Log:              api.Logger,
		Request:          r,
		Action:           database.AuditActionCreate,
		AdditionalFields: taskResourceInfo,
		OrganizationID:   templateVersion.OrganizationID,
	})
	defer commitAuditWS()

	workspace, err := createWorkspace(ctx, aReqWS, apiKey.UserID, api, owner, createReq, r, &createWorkspaceOptions{
		// Before creating the workspace, ensure that this task can be created.
		preCreateInTX: func(ctx context.Context, tx database.Store) error {
			// Create task record in the database before creating the workspace so that
			// we can request that the workspace be linked to it after creation.
			dbTaskTable, err = tx.InsertTask(ctx, database.InsertTaskParams{
				ID:                 uuid.New(),
				OrganizationID:     templateVersion.OrganizationID,
				OwnerID:            owner.ID,
				Name:               taskName,
				WorkspaceID:        uuid.NullUUID{}, // Will be set after workspace creation.
				TemplateVersionID:  templateVersion.ID,
				TemplateParameters: []byte("{}"),
				Prompt:             req.Input,
				CreatedAt:          dbtime.Time(api.Clock.Now()),
			})
			if err != nil {
				return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error creating task.",
					Detail:  err.Error(),
				})
			}
			return nil
		},
		// After the workspace is created, ensure that the task is linked to it.
		postCreateInTX: func(ctx context.Context, tx database.Store, workspace database.Workspace) error {
			// Update the task record with the workspace ID after creation.
			dbTaskTable, err = tx.UpdateTaskWorkspaceID(ctx, database.UpdateTaskWorkspaceIDParams{
				ID: dbTaskTable.ID,
				WorkspaceID: uuid.NullUUID{
					UUID:  workspace.ID,
					Valid: true,
				},
			})
			if err != nil {
				return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error updating task.",
					Detail:  err.Error(),
				})
			}
			return nil
		},
	})
	if err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}

	aReq.New = dbTaskTable

	// Fetch the task to get the additional columns from the view.
	dbTask, err := api.Database.GetTaskByID(ctx, dbTaskTable.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching task.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, taskFromDBTaskAndWorkspace(dbTask, workspace))
}

// taskFromDBTaskAndWorkspace creates a codersdk.Task response from the task
// database record and workspace.
func taskFromDBTaskAndWorkspace(dbTask database.Task, ws codersdk.Workspace) codersdk.Task {
	var taskAgentLifecycle *codersdk.WorkspaceAgentLifecycle
	var taskAgentHealth *codersdk.WorkspaceAgentHealth

	// If we have an agent ID from the task, find the agent details in the
	// workspace.
	if dbTask.WorkspaceAgentID.Valid {
	findTaskAgentLoop:
		for _, resource := range ws.LatestBuild.Resources {
			for _, agent := range resource.Agents {
				if agent.ID == dbTask.WorkspaceAgentID.UUID {
					taskAgentLifecycle = &agent.LifecycleState
					taskAgentHealth = &agent.Health
					break findTaskAgentLoop
				}
			}
		}
	}

	// Ignore 'latest app status' if it is older than the latest build and the
	// latest build is a 'start' transition. This ensures that you don't show a
	// stale app status from a previous build. For stop transitions, there is
	// still value in showing the latest app status.
	var currentState *codersdk.TaskStateEntry
	if ws.LatestAppStatus != nil {
		if ws.LatestBuild.Transition != codersdk.WorkspaceTransitionStart || ws.LatestAppStatus.CreatedAt.After(ws.LatestBuild.CreatedAt) {
			currentState = &codersdk.TaskStateEntry{
				Timestamp: ws.LatestAppStatus.CreatedAt,
				State:     codersdk.TaskState(ws.LatestAppStatus.State),
				Message:   ws.LatestAppStatus.Message,
				URI:       ws.LatestAppStatus.URI,
			}
		}
	}

	return codersdk.Task{
		ID:                      dbTask.ID,
		OrganizationID:          dbTask.OrganizationID,
		OwnerID:                 dbTask.OwnerID,
		OwnerName:               dbTask.OwnerUsername,
		OwnerAvatarURL:          dbTask.OwnerAvatarUrl,
		Name:                    dbTask.Name,
		TemplateID:              ws.TemplateID,
		TemplateVersionID:       dbTask.TemplateVersionID,
		TemplateName:            ws.TemplateName,
		TemplateDisplayName:     ws.TemplateDisplayName,
		TemplateIcon:            ws.TemplateIcon,
		WorkspaceID:             dbTask.WorkspaceID,
		WorkspaceName:           ws.Name,
		WorkspaceBuildNumber:    dbTask.WorkspaceBuildNumber.Int32,
		WorkspaceStatus:         ws.LatestBuild.Status,
		WorkspaceAgentID:        dbTask.WorkspaceAgentID,
		WorkspaceAgentLifecycle: taskAgentLifecycle,
		WorkspaceAgentHealth:    taskAgentHealth,
		WorkspaceAppID:          dbTask.WorkspaceAppID,
		InitialPrompt:           dbTask.Prompt,
		Status:                  codersdk.TaskStatus(dbTask.Status),
		CurrentState:            currentState,
		CreatedAt:               dbTask.CreatedAt,
		UpdatedAt:               ws.UpdatedAt,
	}
}

// @Summary List AI tasks
// @Description: EXPERIMENTAL: this endpoint is experimental and not guaranteed to be stable.
// @ID list-tasks
// @Security CoderSessionToken
// @Tags Experimental
// @Param q query string false "Search query for filtering tasks. Supports: owner:<username/uuid/me>, organization:<org-name/uuid>, status:<status>"
// @Success 200 {object} codersdk.TasksListResponse
// @Router /api/experimental/tasks [get]
//
// EXPERIMENTAL: This endpoint is experimental and not guaranteed to be stable.
// tasksList is an experimental endpoint to list tasks.
func (api *API) tasksList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	// Parse query parameters for filtering tasks.
	queryStr := r.URL.Query().Get("q")
	filter, errs := searchquery.Tasks(ctx, api.Database, queryStr, apiKey.UserID)
	if len(errs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid task search query.",
			Validations: errs,
		})
		return
	}

	// Fetch all tasks matching the filters from the database.
	dbTasks, err := api.Database.ListTasks(ctx, filter)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching tasks.",
			Detail:  err.Error(),
		})
		return
	}

	tasks, err := api.convertTasks(ctx, apiKey.UserID, dbTasks)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting tasks.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.TasksListResponse{
		Tasks: tasks,
		Count: len(tasks),
	})
}

// convertTasks converts database tasks to API tasks, enriching them with
// workspace information.
func (api *API) convertTasks(ctx context.Context, requesterID uuid.UUID, dbTasks []database.Task) ([]codersdk.Task, error) {
	if len(dbTasks) == 0 {
		return []codersdk.Task{}, nil
	}

	// Prepare to batch fetch workspaces.
	workspaceIDs := make([]uuid.UUID, 0, len(dbTasks))
	for _, task := range dbTasks {
		if !task.WorkspaceID.Valid {
			return nil, xerrors.New("task has no workspace ID")
		}
		workspaceIDs = append(workspaceIDs, task.WorkspaceID.UUID)
	}

	// Fetch workspaces for tasks that have workspaces.
	workspaceRows, err := api.Database.GetWorkspaces(ctx, database.GetWorkspacesParams{
		WorkspaceIds: workspaceIDs,
	})
	if err != nil {
		return nil, xerrors.Errorf("fetch workspaces: %w", err)
	}

	workspaces := database.ConvertWorkspaceRows(workspaceRows)

	// Gather associated data and convert to API workspaces.
	data, err := api.workspaceData(ctx, workspaces)
	if err != nil {
		return nil, xerrors.Errorf("fetch workspace data: %w", err)
	}

	apiWorkspaces, err := convertWorkspaces(requesterID, workspaces, data)
	if err != nil {
		return nil, xerrors.Errorf("convert workspaces: %w", err)
	}

	workspacesByID := make(map[uuid.UUID]codersdk.Workspace)
	for _, ws := range apiWorkspaces {
		workspacesByID[ws.ID] = ws
	}

	// Convert tasks to SDK format.
	result := make([]codersdk.Task, 0, len(dbTasks))
	for _, dbTask := range dbTasks {
		task := taskFromDBTaskAndWorkspace(dbTask, workspacesByID[dbTask.WorkspaceID.UUID])
		result = append(result, task)
	}

	return result, nil
}

// @Summary Get AI task by ID
// @Description: EXPERIMENTAL: this endpoint is experimental and not guaranteed to be stable.
// @ID get-task
// @Security CoderSessionToken
// @Tags Experimental
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Success 200 {object} codersdk.Task
// @Router /api/experimental/tasks/{user}/{task} [get]
//
// EXPERIMENTAL: This endpoint is experimental and not guaranteed to be stable.
// taskGet is an experimental endpoint to fetch a single AI task by ID
// (workspace ID). It returns a synthesized task response including
// prompt and status.
func (api *API) taskGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	task := httpmw.TaskParam(r)

	if !task.WorkspaceID.Valid {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching task.",
			Detail:  "Task workspace ID is invalid.",
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, task.WorkspaceID.UUID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
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

	taskResp := taskFromDBTaskAndWorkspace(task, ws)
	httpapi.Write(ctx, rw, http.StatusOK, taskResp)
}

// @Summary Delete AI task by ID
// @Description: EXPERIMENTAL: this endpoint is experimental and not guaranteed to be stable.
// @ID delete-task
// @Security CoderSessionToken
// @Tags Experimental
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Success 202 "Task deletion initiated"
// @Router /api/experimental/tasks/{user}/{task} [delete]
//
// EXPERIMENTAL: This endpoint is experimental and not guaranteed to be stable.
// taskDelete is an experimental endpoint to delete a task by ID.
// It creates a delete workspace build and returns 202 Accepted if the build was
// created.
func (api *API) taskDelete(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	task := httpmw.TaskParam(r)

	now := api.Clock.Now()

	if task.WorkspaceID.Valid {
		workspace, err := api.Database.GetWorkspaceByID(ctx, task.WorkspaceID.UUID)
		if err != nil {
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching task workspace before deleting task.",
				Detail:  err.Error(),
			})
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
	}

	_, err := api.Database.DeleteTask(ctx, database.DeleteTaskParams{
		ID:        task.ID,
		DeletedAt: dbtime.Time(now),
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete task",
			Detail:  err.Error(),
		})
		return
	}

	// Task deleted and delete build created successfully.
	rw.WriteHeader(http.StatusAccepted)
}

// @Summary Update AI task input
// @Description: EXPERIMENTAL: this endpoint is experimental and not guaranteed to be stable.
// @ID update-task-input
// @Security CoderSessionToken
// @Tags Experimental
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Param request body codersdk.UpdateTaskInputRequest true "Update task input request"
// @Success 204
// @Router /api/experimental/tasks/{user}/{task}/input [patch]
//
// EXPERIMENTAL: This endpoint is experimental and not guaranteed to be stable.
// taskUpdateInput allows modifying a task's prompt before the agent executes it.
func (api *API) taskUpdateInput(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx              = r.Context()
		task             = httpmw.TaskParam(r)
		auditor          = api.Auditor.Load()
		taskResourceInfo = audit.AdditionalFields{}
	)

	aReq, commitAudit := audit.InitRequest[database.TaskTable](rw, &audit.RequestParams{
		Audit:            *auditor,
		Log:              api.Logger,
		Request:          r,
		Action:           database.AuditActionWrite,
		AdditionalFields: taskResourceInfo,
	})
	defer commitAudit()
	aReq.Old = task.TaskTable()
	aReq.UpdateOrganizationID(task.OrganizationID)

	var req codersdk.UpdateTaskInputRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var updatedTask database.TaskTable
	if err := api.Database.InTx(func(tx database.Store) error {
		task, err := tx.GetTaskByID(ctx, task.ID)
		if err != nil {
			return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to fetch task.",
				Detail:  err.Error(),
			})
		}

		if task.Status == database.TaskStatusInitializing || task.Status == database.TaskStatusActive {
			return httperror.NewResponseError(http.StatusConflict, codersdk.Response{
				Message: "Cannot update input while task is initializing or active.",
				Detail:  "Please stop the task's workspace before updating the input.",
			})
		}

		updatedTask, err = tx.UpdateTaskPrompt(ctx, database.UpdateTaskPromptParams{
			ID:     task.ID,
			Prompt: req.Input,
		})
		if err != nil {
			return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update task input.",
				Detail:  err.Error(),
			})
		}

		return nil
	}, nil); err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}

	aReq.New = updatedTask

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Send input to AI task
// @Description: EXPERIMENTAL: this endpoint is experimental and not guaranteed to be stable.
// @ID send-task-input
// @Security CoderSessionToken
// @Tags Experimental
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Param request body codersdk.TaskSendRequest true "Task input request"
// @Success 204 "Input sent successfully"
// @Router /api/experimental/tasks/{user}/{task}/send [post]
//
// EXPERIMENTAL: This endpoint is experimental and not guaranteed to be stable.
// taskSend submits task input to the task app by dialing the agent
// directly over the tailnet. We enforce ApplicationConnect RBAC on the
// workspace and validate the task app health.
func (api *API) taskSend(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task := httpmw.TaskParam(r)

	var req codersdk.TaskSendRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Input == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Task input is required.",
		})
		return
	}

	if err := api.authAndDoWithTaskAppClient(r, task, func(ctx context.Context, client *http.Client, appURL *url.URL) error {
		agentAPIClient, err := aiagentapi.NewClient(appURL.String(), aiagentapi.WithHTTPClient(client))
		if err != nil {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Failed to create agentapi client.",
				Detail:  err.Error(),
			})
		}

		statusResp, err := agentAPIClient.GetStatus(ctx)
		if err != nil {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Failed to get status from task app.",
				Detail:  err.Error(),
			})
		}

		if statusResp.Status != aiagentapi.StatusStable {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Task app is not ready to accept input.",
				Detail:  fmt.Sprintf("Status: %s", statusResp.Status),
			})
		}

		_, err = agentAPIClient.PostMessage(ctx, aiagentapi.PostMessageParams{
			Content: req.Input,
			Type:    aiagentapi.MessageTypeUser,
		})
		if err != nil {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Task app rejected the message.",
				Detail:  err.Error(),
			})
		}

		return nil
	}); err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get AI task logs
// @Description: EXPERIMENTAL: this endpoint is experimental and not guaranteed to be stable.
// @ID get-task-logs
// @Security CoderSessionToken
// @Tags Experimental
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Success 200 {object} codersdk.TaskLogsResponse
// @Router /api/experimental/tasks/{user}/{task}/logs [get]
//
// EXPERIMENTAL: This endpoint is experimental and not guaranteed to be stable.
// taskLogs reads task output by dialing the agent directly over the tailnet.
// We enforce ApplicationConnect RBAC on the workspace and validate the task app health.
func (api *API) taskLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task := httpmw.TaskParam(r)

	var out codersdk.TaskLogsResponse
	if err := api.authAndDoWithTaskAppClient(r, task, func(ctx context.Context, client *http.Client, appURL *url.URL) error {
		agentAPIClient, err := aiagentapi.NewClient(appURL.String(), aiagentapi.WithHTTPClient(client))
		if err != nil {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Failed to create agentapi client.",
				Detail:  err.Error(),
			})
		}

		messagesResp, err := agentAPIClient.GetMessages(ctx)
		if err != nil {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Failed to get messages from task app.",
				Detail:  err.Error(),
			})
		}

		logs := make([]codersdk.TaskLogEntry, 0, len(messagesResp.Messages))
		for _, m := range messagesResp.Messages {
			var typ codersdk.TaskLogType
			switch m.Role {
			case aiagentapi.RoleUser:
				typ = codersdk.TaskLogTypeInput
			case aiagentapi.RoleAgent:
				typ = codersdk.TaskLogTypeOutput
			default:
				return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
					Message: "Invalid task app response message role.",
					Detail:  fmt.Sprintf(`Expected "user" or "agent", got %q.`, m.Role),
				})
			}
			logs = append(logs, codersdk.TaskLogEntry{
				ID:      int(m.Id),
				Content: m.Content,
				Type:    typ,
				Time:    m.Time,
			})
		}
		out = codersdk.TaskLogsResponse{Logs: logs}
		return nil
	}); err != nil {
		httperror.WriteResponseError(ctx, rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// authAndDoWithTaskAppClient centralizes the shared logic to:
//
//   - Fetch the task workspace
//   - Authorize ApplicationConnect on the workspace
//   - Validate the AI task and task app health
//   - Dial the agent and construct an HTTP client to the apps loopback URL
//
// The provided callback receives the context, an HTTP client that dials via the
// agent, and the base app URL (as a value URL) to perform any request.
func (api *API) authAndDoWithTaskAppClient(
	r *http.Request,
	task database.Task,
	do func(ctx context.Context, client *http.Client, appURL *url.URL) error,
) error {
	ctx := r.Context()

	if task.Status != database.TaskStatusActive {
		return httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
			Message: "Task status must be active.",
			Detail:  fmt.Sprintf("Task status is %q, it must be %q to interact with the task.", task.Status, codersdk.TaskStatusActive),
		})
	}
	if !task.WorkspaceID.Valid {
		return httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
			Message: "Task does not have a workspace.",
		})
	}
	if !task.WorkspaceAppID.Valid {
		return httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
			Message: "Task does not have a workspace app.",
		})
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, task.WorkspaceID.UUID)
	if err != nil {
		if httpapi.Is404Error(err) {
			return httperror.ErrResourceNotFound
		}
		return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
	}

	// Connecting to applications requires ApplicationConnect on the workspace.
	if !api.Authorize(r, policy.ActionApplicationConnect, workspace) {
		return httperror.ErrResourceNotFound
	}

	apps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, task.WorkspaceAgentID.UUID)
	if err != nil {
		return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resources.",
			Detail:  err.Error(),
		})
	}

	var app *database.WorkspaceApp
	for _, a := range apps {
		if a.ID == task.WorkspaceAppID.UUID {
			app = &a
			break
		}
	}

	// Build the direct app URL and dial the agent.
	appURL := app.Url.String
	if appURL == "" {
		return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Task app URL is not configured.",
		})
	}
	parsedURL, err := url.Parse(appURL)
	if err != nil {
		return httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error parsing task app URL.",
			Detail:  err.Error(),
		})
	}
	if parsedURL.Scheme != "http" {
		return httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
			Message: "Only http scheme is supported for direct agent-dial.",
		})
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second*30)
	defer dialCancel()
	agentConn, release, err := api.agentProvider.AgentConn(dialCtx, task.WorkspaceAgentID.UUID)
	if err != nil {
		return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
			Message: "Failed to reach task app endpoint.",
			Detail:  err.Error(),
		})
	}
	defer release()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return agentConn.DialContext(ctx, network, addr)
			},
		},
	}
	return do(ctx, client, parsedURL)
}
