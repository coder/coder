package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentapisdk "github.com/coder/agentapi-sdk-go"
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
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create a new AI task
// @ID create-a-new-ai-task
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Tasks
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param request body codersdk.CreateTaskRequest true "Create task request"
// @Success 201 {object} codersdk.Task
// @Router /tasks/{user} [post]
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

	taskDisplayName := strings.TrimSpace(req.DisplayName)
	if taskDisplayName != "" {
		if len(taskDisplayName) > 64 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Display name must be 64 characters or less.",
			})
			return
		}
	}

	// Generate task name and display name if either is not provided
	if taskName == "" || taskDisplayName == "" {
		generatedTaskName := taskname.Generate(ctx, api.Logger, req.Input)

		if taskName == "" {
			taskName = generatedTaskName.Name
		}
		if taskDisplayName == "" {
			taskDisplayName = generatedTaskName.DisplayName
		}
	}

	createReq := codersdk.CreateWorkspaceRequest{
		Name:                    taskName,
		TemplateVersionID:       req.TemplateVersionID,
		TemplateVersionPresetID: req.TemplateVersionPresetID,
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
				DisplayName:        taskDisplayName,
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
	var taskAppHealth *codersdk.WorkspaceAppHealth

	if dbTask.WorkspaceAgentLifecycleState.Valid {
		taskAgentLifecycle = ptr.Ref(codersdk.WorkspaceAgentLifecycle(dbTask.WorkspaceAgentLifecycleState.WorkspaceAgentLifecycleState))
	}
	if dbTask.WorkspaceAppHealth.Valid {
		taskAppHealth = ptr.Ref(codersdk.WorkspaceAppHealth(dbTask.WorkspaceAppHealth.WorkspaceAppHealth))
	}

	// If we have an agent ID from the task, find the agent health info
	if dbTask.WorkspaceAgentID.Valid {
	findTaskAgentLoop:
		for _, resource := range ws.LatestBuild.Resources {
			for _, agent := range resource.Agents {
				if agent.ID == dbTask.WorkspaceAgentID.UUID {
					taskAgentHealth = &agent.Health
					break findTaskAgentLoop
				}
			}
		}
	}

	currentState := deriveTaskCurrentState(dbTask, ws, taskAgentLifecycle, taskAppHealth)

	return codersdk.Task{
		ID:                      dbTask.ID,
		OrganizationID:          dbTask.OrganizationID,
		OwnerID:                 dbTask.OwnerID,
		OwnerName:               dbTask.OwnerUsername,
		OwnerAvatarURL:          dbTask.OwnerAvatarUrl,
		Name:                    dbTask.Name,
		DisplayName:             dbTask.DisplayName,
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

// deriveTaskCurrentState determines the current state of a task based on the
// workspace's latest app status and initialization phase.
// Returns nil if no valid state can be determined.
func deriveTaskCurrentState(
	dbTask database.Task,
	ws codersdk.Workspace,
	taskAgentLifecycle *codersdk.WorkspaceAgentLifecycle,
	taskAppHealth *codersdk.WorkspaceAppHealth,
) *codersdk.TaskStateEntry {
	var currentState *codersdk.TaskStateEntry

	// Ignore 'latest app status' if it is older than the latest build and the
	// latest build is a 'start' transition. This ensures that you don't show a
	// stale app status from a previous build. For stop transitions, there is
	// still value in showing the latest app status.
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

	// If no valid agent state was found for the current build and the task is initializing,
	// provide a descriptive initialization message.
	if currentState == nil && dbTask.Status == database.TaskStatusInitializing {
		message := "Initializing workspace"

		switch {
		case ws.LatestBuild.Status == codersdk.WorkspaceStatusPending ||
			ws.LatestBuild.Status == codersdk.WorkspaceStatusStarting:
			message = fmt.Sprintf("Workspace is %s", ws.LatestBuild.Status)
		case taskAgentLifecycle != nil:
			switch {
			case *taskAgentLifecycle == codersdk.WorkspaceAgentLifecycleCreated:
				message = "Agent is connecting"
			case *taskAgentLifecycle == codersdk.WorkspaceAgentLifecycleStarting:
				message = "Agent is starting"
			case *taskAgentLifecycle == codersdk.WorkspaceAgentLifecycleReady:
				if taskAppHealth != nil && *taskAppHealth == codersdk.WorkspaceAppHealthInitializing {
					message = "App is initializing"
				} else {
					// In case the workspace app is not initializing,
					// the overall task status should be updated accordingly
					message = "Initializing workspace applications"
				}
			default:
				// In case the workspace agent is not initializing,
				// the overall task status should be updated accordingly
				message = "Initializing workspace agent"
			}
		}

		currentState = &codersdk.TaskStateEntry{
			Timestamp: ws.LatestBuild.CreatedAt,
			State:     codersdk.TaskStateWorking,
			Message:   message,
			URI:       "",
		}
	}

	return currentState
}

// @Summary List AI tasks
// @ID list-ai-tasks
// @Security CoderSessionToken
// @Produce json
// @Tags Tasks
// @Param q query string false "Search query for filtering tasks. Supports: owner:<username/uuid/me>, organization:<org-name/uuid>, status:<status>"
// @Success 200 {object} codersdk.TasksListResponse
// @Router /tasks [get]
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

	workspaces, err := database.ConvertWorkspaceRows(workspaceRows)
	if err != nil {
		return nil, xerrors.Errorf("convert workspace rows: %w", err)
	}

	// Gather associated data and convert to API workspaces.
	data, err := api.workspaceData(ctx, workspaces)
	if err != nil {
		return nil, xerrors.Errorf("fetch workspace data: %w", err)
	}

	apiWorkspaces, err := convertWorkspaces(
		ctx,
		api.Experiments,
		api.Logger,
		requesterID,
		workspaces,
		data,
	)
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

// @Summary Get AI task by ID or name
// @ID get-ai-task-by-id-or-name
// @Security CoderSessionToken
// @Produce json
// @Tags Tasks
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Success 200 {object} codersdk.Task
// @Router /tasks/{user}/{task} [get]
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
		ctx,
		api.Experiments,
		api.Logger,
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

// @Summary Delete AI task
// @ID delete-ai-task
// @Security CoderSessionToken
// @Tags Tasks
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Success 202
// @Router /tasks/{user}/{task} [delete]
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

	// As an implementation detail of the workspace build transition, we also delete
	// the associated task. This means that we have a race between provisionerdserver
	// and here with deleting the task. In a real world scenario we'll never lose the
	// race but we should still handle it anyways.
	_, err := api.Database.DeleteTask(ctx, database.DeleteTaskParams{
		ID:        task.ID,
		DeletedAt: dbtime.Time(now),
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
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
// @ID update-ai-task-input
// @Security CoderSessionToken
// @Accept json
// @Tags Tasks
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Param request body codersdk.UpdateTaskInputRequest true "Update task input request"
// @Success 204
// @Router /tasks/{user}/{task}/input [patch]
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

	if strings.TrimSpace(req.Input) == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Task input is required.",
		})
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

		if task.Status != database.TaskStatusPaused {
			return httperror.NewResponseError(http.StatusConflict, codersdk.Response{
				Message: "Unable to update task input, task must be paused.",
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
// @ID send-input-to-ai-task
// @Security CoderSessionToken
// @Accept json
// @Tags Tasks
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Param request body codersdk.TaskSendRequest true "Task input request"
// @Success 204
// @Router /tasks/{user}/{task}/send [post]
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
		agentAPIClient, err := agentapisdk.NewClient(appURL.String(), agentapisdk.WithHTTPClient(client))
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

		if statusResp.Status != agentapisdk.StatusStable {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Task app is not ready to accept input.",
				Detail:  fmt.Sprintf("Status: %s", statusResp.Status),
			})
		}

		_, err = agentAPIClient.PostMessage(ctx, agentapisdk.PostMessageParams{
			Content: req.Input,
			Type:    agentapisdk.MessageTypeUser,
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

// convertAgentAPIMessagesToLogEntries converts AgentAPI messages to
// TaskLogEntry format.
func convertAgentAPIMessagesToLogEntries(messages []agentapisdk.Message) ([]codersdk.TaskLogEntry, error) {
	logs := make([]codersdk.TaskLogEntry, 0, len(messages))
	for _, m := range messages {
		var typ codersdk.TaskLogType
		switch m.Role {
		case agentapisdk.RoleUser:
			typ = codersdk.TaskLogTypeInput
		case agentapisdk.RoleAgent:
			typ = codersdk.TaskLogTypeOutput
		default:
			return nil, xerrors.Errorf("invalid agentapi message role %q", m.Role)
		}
		logs = append(logs, codersdk.TaskLogEntry{
			ID:      int(m.Id),
			Content: m.Content,
			Type:    typ,
			Time:    m.Time,
		})
	}
	return logs, nil
}

// @Summary Get AI task logs
// @ID get-ai-task-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Tasks
// @Param user path string true "Username, user ID, or 'me' for the authenticated user"
// @Param task path string true "Task ID, or task name"
// @Success 200 {object} codersdk.TaskLogsResponse
// @Router /tasks/{user}/{task}/logs [get]
func (api *API) taskLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task := httpmw.TaskParam(r)

	switch task.Status {
	case database.TaskStatusActive:
		// Active tasks: fetch live logs from AgentAPI.
		out, err := api.fetchLiveTaskLogs(r, task)
		if err != nil {
			httperror.WriteResponseError(ctx, rw, err)
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, out)

	case database.TaskStatusPaused, database.TaskStatusPending, database.TaskStatusInitializing:
		// In pause, pending and initializing states, we attempt to fetch
		// the snapshot from database to provide continuity.
		out, err := api.fetchSnapshotTaskLogs(ctx, task.ID)
		if err != nil {
			httperror.WriteResponseError(ctx, rw, err)
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, out)

	default:
		// Cases: database.TaskStatusError, database.TaskStatusUnknown.
		// - Error: snapshot would be stale from previous pause.
		// - Unknown: cannot determine reliable state.
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Cannot fetch logs for task in current state.",
			Detail:  fmt.Sprintf("Task status is %q.", task.Status),
		})
	}
}

func (api *API) fetchLiveTaskLogs(r *http.Request, task database.Task) (codersdk.TaskLogsResponse, error) {
	var out codersdk.TaskLogsResponse
	err := api.authAndDoWithTaskAppClient(r, task, func(ctx context.Context, client *http.Client, appURL *url.URL) error {
		agentAPIClient, err := agentapisdk.NewClient(appURL.String(), agentapisdk.WithHTTPClient(client))
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

		logs, err := convertAgentAPIMessagesToLogEntries(messagesResp.Messages)
		if err != nil {
			return httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
				Message: "Invalid task app response.",
				Detail:  err.Error(),
			})
		}

		out = codersdk.TaskLogsResponse{
			Count: len(logs),
			Logs:  logs,
		}
		return nil
	})
	return out, err
}

func (api *API) fetchSnapshotTaskLogs(ctx context.Context, taskID uuid.UUID) (codersdk.TaskLogsResponse, error) {
	snapshot, err := api.Database.GetTaskSnapshot(ctx, taskID)
	if err != nil {
		if httpapi.IsUnauthorizedError(err) {
			return codersdk.TaskLogsResponse{}, httperror.NewResponseError(http.StatusNotFound, codersdk.Response{
				Message: "Resource not found.",
			})
		}
		if errors.Is(err, sql.ErrNoRows) {
			// No snapshot exists yet, return empty logs. Snapshot is true
			// because this field indicates whether the data is from the
			// live task app (false) or not (true). Since the task is
			// paused/initializing/pending, we cannot fetch live logs, so
			// snapshot must be true even with no snapshot data.
			return codersdk.TaskLogsResponse{
				Count:    0,
				Logs:     []codersdk.TaskLogEntry{},
				Snapshot: true,
			}, nil
		}
		return codersdk.TaskLogsResponse{}, httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching task snapshot.",
			Detail:  err.Error(),
		})
	}

	// Unmarshal envelope with pre-populated data field to decode once.
	envelope := TaskLogSnapshotEnvelope{
		Data: &agentapisdk.GetMessagesResponse{},
	}
	if err := json.Unmarshal(snapshot.LogSnapshot, &envelope); err != nil {
		return codersdk.TaskLogsResponse{}, httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error decoding task snapshot.",
			Detail:  err.Error(),
		})
	}

	// Validate snapshot format.
	if envelope.Format != "agentapi" {
		return codersdk.TaskLogsResponse{}, httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Unsupported task snapshot format.",
			Detail:  fmt.Sprintf("Expected format %q, got %q.", "agentapi", envelope.Format),
		})
	}

	// Extract agentapi data from envelope (already decoded into the correct type).
	messagesResp, ok := envelope.Data.(*agentapisdk.GetMessagesResponse)
	if !ok {
		return codersdk.TaskLogsResponse{}, httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error decoding snapshot data.",
			Detail:  "Unexpected data type in envelope.",
		})
	}

	// Convert agentapi messages to log entries.
	logs, err := convertAgentAPIMessagesToLogEntries(messagesResp.Messages)
	if err != nil {
		return codersdk.TaskLogsResponse{}, httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Invalid snapshot data.",
			Detail:  err.Error(),
		})
	}

	return codersdk.TaskLogsResponse{
		Count:      len(logs),
		Logs:       logs,
		Snapshot:   true,
		SnapshotAt: ptr.Ref(snapshot.LogSnapshotCreatedAt),
	}, nil
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

const (
	// taskSnapshotMaxSize is the maximum size for task log snapshots (64KB).
	// Protects against excessive memory usage and database payload sizes.
	taskSnapshotMaxSize = 64 * 1024
)

// TaskLogSnapshotEnvelope wraps a task log snapshot with format metadata.
type TaskLogSnapshotEnvelope struct {
	Format string `json:"format"`
	Data   any    `json:"data"`
}

// @Summary Upload task log snapshot
// @ID upload-task-log-snapshot
// @Security CoderSessionToken
// @Accept json
// @Tags Tasks
// @Param task path string true "Task ID" format(uuid)
// @Param format query string true "Snapshot format" enums(agentapi)
// @Param request body object true "Raw snapshot payload (structure depends on format parameter)"
// @Success 204
// @Router /workspaceagents/me/tasks/{task}/log-snapshot [post]
func (api *API) postWorkspaceAgentTaskLogSnapshot(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx         = r.Context()
		latestBuild = httpmw.LatestBuild(r)
	)

	// Parse task ID from path.
	taskIDStr := chi.URLParam(r, "task")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid task ID format.",
			Detail:  err.Error(),
		})
		return
	}

	// Validate format parameter (required).
	p := httpapi.NewQueryParamParser().RequiredNotEmpty("format")
	format := p.String(r.URL.Query(), "", "format")
	p.ErrorExcessParams(r.URL.Query())
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: p.Errors,
		})
		return
	}
	if format != "agentapi" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid format parameter.",
			Detail:  fmt.Sprintf(`Only "agentapi" format is currently supported, got %q.`, format),
		})
		return
	}

	// Verify task exists before reading the potentially large payload.
	// This prevents DoS attacks where attackers spam large payloads for
	// non-existent or deleted tasks, forcing us to read 64KB into memory
	// and do expensive JSON operations before the database rejects it.
	// The UpsertTaskSnapshot will re-fetch for RBAC validation, but this
	// early check protects against malicious load.
	task, err := api.Database.GetTaskByID(ctx, taskID)
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

	// Reject deleted tasks early.
	if task.DeletedAt.Valid {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Verify task belongs to this agent's workspace.
	if !task.WorkspaceID.Valid || task.WorkspaceID.UUID != latestBuild.WorkspaceID {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Limit payload size to avoid excessive memory or data usage.
	r.Body = http.MaxBytesReader(rw, r.Body, taskSnapshotMaxSize)

	// Create envelope to store validated payload.
	envelope := TaskLogSnapshotEnvelope{
		Format: format,
	}

	switch format {
	case "agentapi":
		var payload agentapisdk.GetMessagesResponse
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to decode request payload.",
				Detail:  err.Error(),
			})
			return
		}
		// Verify messages field exists (can be empty array).
		if payload.Messages == nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid agentapi payload structure.",
				Detail:  `Missing required "messages" field.`,
			})
			return
		}
		envelope.Data = payload
	default:
		// Defensive branch, we already validated "agentapi" format but may add
		// more formats in the future.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid format parameter.",
			Detail:  fmt.Sprintf(`Only "agentapi" format is currently supported, got %q.`, format),
		})
		return
	}

	// Marshal envelope with validated payload in a single pass.
	snapshotJSON, err := json.Marshal(envelope)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create snapshot envelope.",
			Detail:  err.Error(),
		})
		return
	}

	// Upsert to database using agent's RBAC context.
	err = api.Database.UpsertTaskSnapshot(ctx, database.UpsertTaskSnapshotParams{
		TaskID:               task.ID,
		LogSnapshot:          json.RawMessage(snapshotJSON),
		LogSnapshotCreatedAt: dbtime.Time(api.Clock.Now()),
	})
	if err != nil {
		if httpapi.IsUnauthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error storing snapshot.",
			Detail:  err.Error(),
		})
		return
	}

	api.Logger.Debug(ctx, "stored task log snapshot",
		slog.F("task_id", task.ID),
		slog.F("workspace_id", latestBuild.WorkspaceID),
		slog.F("snapshot_size_bytes", len(snapshotJSON)))

	rw.WriteHeader(http.StatusNoContent)
}
