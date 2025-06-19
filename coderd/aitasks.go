package coderd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get AI tasks prompts
// @ID get-ai-tasks-prompts
// @Security CoderSessionToken
// @Produce json
// @Tags AITasks
// @Param build_ids query string true "Comma-separated workspace build IDs" format(uuid)
// @Success 200 {object} codersdk.AITasksPromptsResponse
// @Router /aitasks/prompts [get]
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
