package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coder/terraform-provider-coder/v2/provider"
)

const AITaskPromptParameterName = provider.TaskPromptParameterName

type AITasksPromptsResponse struct {
	// Prompts is a map of workspace build IDs to prompts.
	Prompts map[string]string `json:"prompts"`
}

// AITaskPrompts returns prompts for multiple workspace builds by their IDs.
func (c *ExperimentalClient) AITaskPrompts(ctx context.Context, buildIDs []uuid.UUID) (AITasksPromptsResponse, error) {
	if len(buildIDs) == 0 {
		return AITasksPromptsResponse{
			Prompts: make(map[string]string),
		}, nil
	}

	// Convert UUIDs to strings and join them
	buildIDStrings := make([]string, len(buildIDs))
	for i, id := range buildIDs {
		buildIDStrings[i] = id.String()
	}
	buildIDsParam := strings.Join(buildIDStrings, ",")

	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/aitasks/prompts", nil, WithQueryParam("build_ids", buildIDsParam))
	if err != nil {
		return AITasksPromptsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AITasksPromptsResponse{}, ReadBodyAsError(res)
	}
	var prompts AITasksPromptsResponse
	return prompts, json.NewDecoder(res.Body).Decode(&prompts)
}

type CreateTaskRequest struct {
	TemplateVersionID       uuid.UUID `json:"template_version_id" format:"uuid"`
	TemplateVersionPresetID uuid.UUID `json:"template_version_preset_id,omitempty" format:"uuid"`
	Prompt                  string    `json:"prompt"`
}

func (c *ExperimentalClient) CreateTask(ctx context.Context, user string, request CreateTaskRequest) (Workspace, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/tasks/%s", user), request)
	if err != nil {
		return Workspace{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Workspace{}, ReadBodyAsError(res)
	}

	var workspace Workspace
	if err := json.NewDecoder(res.Body).Decode(&workspace); err != nil {
		return Workspace{}, err
	}

	return workspace, nil
}

// TaskState represents the high-level lifecycle of a task.
//
// Experimental: This type is experimental and may change in the future.
type TaskState string

const (
	TaskStateWorking   TaskState = "working"
	TaskStateIdle      TaskState = "idle"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
)

// Task represents a task.
//
// Experimental: This type is experimental and may change in the future.
type Task struct {
	ID                      uuid.UUID                `json:"id" format:"uuid" table:"id"`
	OrganizationID          uuid.UUID                `json:"organization_id" format:"uuid" table:"organization id"`
	OwnerID                 uuid.UUID                `json:"owner_id" format:"uuid" table:"owner id"`
	OwnerName               string                   `json:"owner_name" table:"owner name"`
	Name                    string                   `json:"name" table:"name,default_sort"`
	TemplateID              uuid.UUID                `json:"template_id" format:"uuid" table:"template id"`
	TemplateName            string                   `json:"template_name" table:"template name"`
	TemplateDisplayName     string                   `json:"template_display_name" table:"template display name"`
	TemplateIcon            string                   `json:"template_icon" table:"template icon"`
	WorkspaceID             uuid.NullUUID            `json:"workspace_id" format:"uuid" table:"workspace id"`
	WorkspaceAgentID        uuid.NullUUID            `json:"workspace_agent_id" format:"uuid" table:"workspace agent id"`
	WorkspaceAgentLifecycle *WorkspaceAgentLifecycle `json:"workspace_agent_lifecycle" table:"workspace agent lifecycle"`
	WorkspaceAgentHealth    *WorkspaceAgentHealth    `json:"workspace_agent_health" table:"workspace agent health"`
	InitialPrompt           string                   `json:"initial_prompt" table:"initial prompt"`
	Status                  WorkspaceStatus          `json:"status" enums:"pending,starting,running,stopping,stopped,failed,canceling,canceled,deleting,deleted" table:"status"`
	CurrentState            *TaskStateEntry          `json:"current_state" table:"cs,recursive_inline"`
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
	// Status is a task status.
	Status string `json:"status,omitempty" typescript:"-"`
	// Offset is the number of tasks to skip before returning results.
	Offset int `json:"offset,omitempty" typescript:"-"`
	// Limit is a limit on the number of tasks returned.
	Limit int `json:"limit,omitempty" typescript:"-"`
}

// Tasks lists all tasks belonging to the user or specified owner.
//
// Experimental: This method is experimental and may change in the future.
func (c *ExperimentalClient) Tasks(ctx context.Context, filter *TasksFilter) ([]Task, error) {
	if filter == nil {
		filter = &TasksFilter{}
	}

	var wsFilter WorkspaceFilter
	wsFilter.Owner = filter.Owner
	wsFilter.Status = filter.Status
	page := Pagination{
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}

	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/tasks", nil, wsFilter.asRequestOption(), page.asRequestOption())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	// Experimental response shape for tasks list (server returns []Task).
	type tasksListResponse struct {
		Tasks []Task `json:"tasks"`
		Count int    `json:"count"`
	}
	var tres tasksListResponse
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
