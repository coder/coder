package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/terraform-provider-coder/v2/provider"
)

// AITaskPromptParameterName is the name of the parameter used to pass prompts
// to AI tasks.
//
// Deprecated: This constant is deprecated and maintained only for backwards
// compatibility with older templates. Task prompts are now stored directly
// in the tasks.prompt database column. New code should access prompts via
// the Task.InitialPrompt field returned from task endpoints.
//
// This constant will be removed in a future major version. Templates should
// not rely on this parameter name, as the backend will continue to create it
// automatically for compatibility but reads from tasks.prompt.
const AITaskPromptParameterName = provider.TaskPromptParameterName

// CreateTaskRequest represents the request to create a new task.
//
// Experimental: This type is experimental and may change in the future.
type CreateTaskRequest struct {
	TemplateVersionID       uuid.UUID `json:"template_version_id" format:"uuid"`
	TemplateVersionPresetID uuid.UUID `json:"template_version_preset_id,omitempty" format:"uuid"`
	Input                   string    `json:"input"`
	Name                    string    `json:"name,omitempty"`
}

// CreateTask creates a new task.
//
// Experimental: This method is experimental and may change in the future.
func (c *ExperimentalClient) CreateTask(ctx context.Context, user string, request CreateTaskRequest) (Task, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/tasks/%s", user), request)
	if err != nil {
		return Task{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Task{}, ReadBodyAsError(res)
	}

	var task Task
	if err := json.NewDecoder(res.Body).Decode(&task); err != nil {
		return Task{}, err
	}

	return task, nil
}

// TaskStatus represents the status of a task.
//
// Experimental: This type is experimental and may change in the future.
type TaskStatus string

const (
	// TaskStatusPending indicates the task has been created but no workspace
	// has been provisioned yet, or the workspace build job status is unknown.
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusInitializing indicates the workspace build is pending/running,
	// the agent is connecting, or apps are initializing.
	TaskStatusInitializing TaskStatus = "initializing"
	// TaskStatusActive indicates the task's workspace is running with a
	// successful start transition, the agent is connected, and all workspace
	// apps are either healthy or disabled.
	TaskStatusActive TaskStatus = "active"
	// TaskStatusPaused indicates the task's workspace has been stopped or
	// deleted (stop/delete transition with successful job status).
	TaskStatusPaused TaskStatus = "paused"
	// TaskStatusUnknown indicates the task's status cannot be determined
	// based on the workspace build, agent lifecycle, or app health states.
	TaskStatusUnknown TaskStatus = "unknown"
	// TaskStatusError indicates the task's workspace build job has failed,
	// or the workspace apps are reporting unhealthy status.
	TaskStatusError TaskStatus = "error"
)

func AllTaskStatuses() []TaskStatus {
	return []TaskStatus{
		TaskStatusPending,
		TaskStatusInitializing,
		TaskStatusActive,
		TaskStatusPaused,
		TaskStatusError,
		TaskStatusUnknown,
	}
}

// TaskState represents the high-level lifecycle of a task.
//
// Experimental: This type is experimental and may change in the future.
type TaskState string

// TaskState enums.
const (
	// TaskStateWorking indicates the AI agent is actively processing work.
	// Reported when the agent is performing actions or the screen is changing.
	TaskStateWorking TaskState = "working"
	// TaskStateIdle indicates the AI agent's screen is stable and no work
	// is being performed. Reported automatically by the screen watcher.
	TaskStateIdle TaskState = "idle"
	// TaskStateComplete indicates the AI agent has successfully completed
	// the task. Reported via the workspace app status.
	TaskStateComplete TaskState = "complete"
	// TaskStateFailed indicates the AI agent reported a failure state.
	// Reported via the workspace app status.
	TaskStateFailed TaskState = "failed"
)

// Task represents a task.
//
// Experimental: This type is experimental and may change in the future.
type Task struct {
	ID                      uuid.UUID                `json:"id" format:"uuid" table:"id"`
	OrganizationID          uuid.UUID                `json:"organization_id" format:"uuid" table:"organization id"`
	OwnerID                 uuid.UUID                `json:"owner_id" format:"uuid" table:"owner id"`
	OwnerName               string                   `json:"owner_name" table:"owner name"`
	OwnerAvatarURL          string                   `json:"owner_avatar_url,omitempty" table:"owner avatar url"`
	Name                    string                   `json:"name" table:"name,default_sort"`
	TemplateID              uuid.UUID                `json:"template_id" format:"uuid" table:"template id"`
	TemplateVersionID       uuid.UUID                `json:"template_version_id" format:"uuid" table:"template version id"`
	TemplateName            string                   `json:"template_name" table:"template name"`
	TemplateDisplayName     string                   `json:"template_display_name" table:"template display name"`
	TemplateIcon            string                   `json:"template_icon" table:"template icon"`
	WorkspaceID             uuid.NullUUID            `json:"workspace_id" format:"uuid" table:"workspace id"`
	WorkspaceName           string                   `json:"workspace_name" table:"workspace name"`
	WorkspaceStatus         WorkspaceStatus          `json:"workspace_status,omitempty" enums:"pending,starting,running,stopping,stopped,failed,canceling,canceled,deleting,deleted" table:"workspace status"`
	WorkspaceBuildNumber    int32                    `json:"workspace_build_number,omitempty" table:"workspace build number"`
	WorkspaceAgentID        uuid.NullUUID            `json:"workspace_agent_id" format:"uuid" table:"workspace agent id"`
	WorkspaceAgentLifecycle *WorkspaceAgentLifecycle `json:"workspace_agent_lifecycle" table:"workspace agent lifecycle"`
	WorkspaceAgentHealth    *WorkspaceAgentHealth    `json:"workspace_agent_health" table:"workspace agent health"`
	WorkspaceAppID          uuid.NullUUID            `json:"workspace_app_id" format:"uuid" table:"workspace app id"`
	InitialPrompt           string                   `json:"initial_prompt" table:"initial prompt"`
	Status                  TaskStatus               `json:"status" enums:"pending,initializing,active,paused,unknown,error" table:"status"`
	CurrentState            *TaskStateEntry          `json:"current_state" table:"cs,recursive_inline,empty_nil"`
	CreatedAt               time.Time                `json:"created_at" format:"date-time" table:"created at"`
	UpdatedAt               time.Time                `json:"updated_at" format:"date-time" table:"updated at"`
}

// TaskStateEntry represents a single entry in the task's state history.
//
// Experimental: This type is experimental and may change in the future.
type TaskStateEntry struct {
	Timestamp time.Time `json:"timestamp" format:"date-time" table:"-"`
	State     TaskState `json:"state" enum:"working,idle,completed,failed" table:"state"`
	Message   string    `json:"message" table:"message"`
	URI       string    `json:"uri" table:"-"`
}

// TasksFilter filters the list of tasks.
//
// Experimental: This type is experimental and may change in the future.
type TasksFilter struct {
	// Owner can be a username, UUID, or "me".
	Owner string `json:"owner,omitempty"`
	// Organization can be an organization name or UUID.
	Organization string `json:"organization,omitempty"`
	// Status filters the tasks by their task status.
	Status TaskStatus `json:"status,omitempty"`
	// FilterQuery allows specifying a raw filter query.
	FilterQuery string `json:"filter_query,omitempty"`
}

// TaskListResponse is the response shape for tasks list.
//
// Experimental response shape for tasks list (server returns []Task).
type TasksListResponse struct {
	Tasks []Task `json:"tasks"`
	Count int    `json:"count"`
}

func (f TasksFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		var params []string
		// Make sure all user input is quoted to ensure it's parsed as a single
		// string.
		if f.Owner != "" {
			params = append(params, fmt.Sprintf("owner:%q", f.Owner))
		}
		if f.Organization != "" {
			params = append(params, fmt.Sprintf("organization:%q", f.Organization))
		}
		if f.Status != "" {
			params = append(params, fmt.Sprintf("status:%q", string(f.Status)))
		}
		if f.FilterQuery != "" {
			// If custom stuff is added, just add it on here.
			params = append(params, f.FilterQuery)
		}

		q := r.URL.Query()
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	}
}

// Tasks lists all tasks belonging to the user or specified owner.
//
// Experimental: This method is experimental and may change in the future.
func (c *ExperimentalClient) Tasks(ctx context.Context, filter *TasksFilter) ([]Task, error) {
	if filter == nil {
		filter = &TasksFilter{}
	}

	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/tasks", nil, filter.asRequestOption())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var tres TasksListResponse
	if err := json.NewDecoder(res.Body).Decode(&tres); err != nil {
		return nil, err
	}

	return tres.Tasks, nil
}

// TaskByID fetches a single experimental task by its ID.
//
// Experimental: This method is experimental and may change in the future.
func (c *ExperimentalClient) TaskByID(ctx context.Context, id uuid.UUID) (Task, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/tasks/%s/%s", "me", id.String()), nil)
	if err != nil {
		return Task{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Task{}, ReadBodyAsError(res)
	}

	var task Task
	if err := json.NewDecoder(res.Body).Decode(&task); err != nil {
		return Task{}, err
	}

	return task, nil
}

func splitTaskIdentifier(identifier string) (owner string, taskName string, err error) {
	parts := strings.Split(identifier, "/")

	switch len(parts) {
	case 1:
		owner = Me
		taskName = parts[0]
	case 2:
		owner = parts[0]
		taskName = parts[1]
	default:
		return "", "", xerrors.Errorf("invalid task identifier: %q", identifier)
	}
	return owner, taskName, nil
}

// TaskByIdentifier fetches and returns a task by an identifier, which may be
// either a UUID, a name (for a task owned by the current user), or a
// "user/task" combination, where user is either a username or UUID.
//
// Since there is no TaskByOwnerAndName endpoint yet, this function uses the
// list endpoint with filtering when a name is provided.
func (c *ExperimentalClient) TaskByIdentifier(ctx context.Context, identifier string) (Task, error) {
	identifier = strings.TrimSpace(identifier)

	// Try parsing as UUID first.
	if taskID, err := uuid.Parse(identifier); err == nil {
		return c.TaskByID(ctx, taskID)
	}

	// Not a UUID, treat as identifier.
	owner, taskName, err := splitTaskIdentifier(identifier)
	if err != nil {
		return Task{}, err
	}

	tasks, err := c.Tasks(ctx, &TasksFilter{
		Owner: owner,
	})
	if err != nil {
		return Task{}, xerrors.Errorf("list tasks for owner %q: %w", owner, err)
	}

	if taskID, err := uuid.Parse(taskName); err == nil {
		// Find task by ID.
		for _, task := range tasks {
			if task.ID == taskID {
				return task, nil
			}
		}
	} else {
		// Find task by name.
		for _, task := range tasks {
			if task.Name == taskName {
				return task, nil
			}
		}
	}

	// Mimic resource not found from API.
	var notFoundErr error = &Error{
		Response: Response{Message: "Resource not found or you do not have access to this resource"},
	}
	return Task{}, xerrors.Errorf("task %q not found for owner %q: %w", taskName, owner, notFoundErr)
}

// DeleteTask deletes a task by its ID.
//
// Experimental: This method is experimental and may change in the future.
func (c *ExperimentalClient) DeleteTask(ctx context.Context, user string, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/tasks/%s/%s", user, id.String()), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		return ReadBodyAsError(res)
	}
	return nil
}

// TaskSendRequest is used to send task input to the tasks sidebar app.
//
// Experimental: This type is experimental and may change in the future.
type TaskSendRequest struct {
	Input string `json:"input"`
}

// TaskSend submits task input to the tasks sidebar app.
//
// Experimental: This method is experimental and may change in the future.
func (c *ExperimentalClient) TaskSend(ctx context.Context, user string, id uuid.UUID, req TaskSendRequest) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/tasks/%s/%s/send", user, id.String()), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// TaskLogType indicates the source of a task log entry.
//
// Experimental: This type is experimental and may change in the future.
type TaskLogType string

// TaskLogType enums.
const (
	TaskLogTypeInput  TaskLogType = "input"
	TaskLogTypeOutput TaskLogType = "output"
)

// TaskLogEntry represents a single log entry for a task.
//
// Experimental: This type is experimental and may change in the future.
type TaskLogEntry struct {
	ID      int         `json:"id" table:"id"`
	Content string      `json:"content" table:"content"`
	Type    TaskLogType `json:"type" enum:"input,output" table:"type"`
	Time    time.Time   `json:"time" format:"date-time" table:"time,default_sort"`
}

// TaskLogsResponse contains the logs for a task.
//
// Experimental: This type is experimental and may change in the future.
type TaskLogsResponse struct {
	Logs []TaskLogEntry `json:"logs"`
}

// TaskLogs retrieves logs from the task's sidebar app via the experimental API.
//
// Experimental: This method is experimental and may change in the future.
func (c *ExperimentalClient) TaskLogs(ctx context.Context, user string, id uuid.UUID) (TaskLogsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/tasks/%s/%s/logs", user, id.String()), nil)
	if err != nil {
		return TaskLogsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return TaskLogsResponse{}, ReadBodyAsError(res)
	}

	var logs TaskLogsResponse
	if err := json.NewDecoder(res.Body).Decode(&logs); err != nil {
		return TaskLogsResponse{}, xerrors.Errorf("decoding task logs response: %w", err)
	}

	return logs, nil
}
