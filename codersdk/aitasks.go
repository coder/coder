package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

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

type CreateAITasksRequest struct {
	Name                    string    `json:"name"`
	TemplateVersionID       uuid.UUID `json:"template_version_id" format:"uuid"`
	TemplateVersionPresetID uuid.UUID `json:"template_version_preset_id,omitempty" format:"uuid"`
	Prompt                  string    `json:"prompt"`
}

func (c *ExperimentalClient) AITasksCreate(ctx context.Context, request CreateAITasksRequest) (Workspace, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/aitasks", request)
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
