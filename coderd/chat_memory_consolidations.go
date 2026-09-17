package coderd

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

const chatMemoryConsolidationListLimit int32 = 20

// @Summary List chat project memory consolidations
// @ID list-chat-project-memory-consolidations
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {array} codersdk.ChatMemoryConsolidation
// @Router /api/experimental/chats/projects/{project}/memories/consolidations [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjectMemoryConsolidations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	records, err := api.Database.GetChatMemoryConsolidationsByProject(ctx, database.GetChatMemoryConsolidationsByProjectParams{ProjectID: project.ID, LimitCount: chatMemoryConsolidationListLimit})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list chat project memory consolidations.", Detail: err.Error()})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, chatMemoryConsolidations(records))
}

// @Summary List chat user memory consolidations
// @ID list-chat-user-memory-consolidations
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization query string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.ChatMemoryConsolidation
// @Router /api/experimental/chats/memories/consolidations [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatUserMemoryConsolidations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organizationID, err := uuid.Parse(r.URL.Query().Get("organization"))
	if err != nil || organizationID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "organization query parameter is required."})
		return
	}
	records, err := api.Database.GetChatMemoryConsolidationsByUser(ctx, database.GetChatMemoryConsolidationsByUserParams{UserID: apiKey.UserID, OrganizationID: organizationID, LimitCount: chatMemoryConsolidationListLimit})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{Message: "Failed to list chat user memory consolidations.", Detail: err.Error()})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, chatMemoryConsolidations(records))
}

func chatMemoryConsolidations(records []database.ChatMemoryConsolidation) []codersdk.ChatMemoryConsolidation {
	result := make([]codersdk.ChatMemoryConsolidation, len(records))
	for i, record := range records {
		result[i] = chatMemoryConsolidation(record)
	}
	return result
}

func chatMemoryConsolidation(record database.ChatMemoryConsolidation) codersdk.ChatMemoryConsolidation {
	result := codersdk.ChatMemoryConsolidation{
		ID:             record.ID,
		OrganizationID: record.OrganizationID,
		Status:         codersdk.ChatMemoryConsolidationStatus(record.Status),
		StartedAt:      record.StartedAt,
		Model:          record.Model,
		MemoriesBefore: record.MemoriesBefore,
		MemoriesAfter:  record.MemoriesAfter,
		Error:          record.Error,
		Mutations:      []codersdk.ChatMemoryMutation{},
	}
	if record.ProjectID.Valid {
		result.ProjectID = &record.ProjectID.UUID
	}
	if record.UserID.Valid {
		result.UserID = &record.UserID.UUID
	}
	if record.FinishedAt.Valid {
		result.FinishedAt = &record.FinishedAt.Time
	}
	_ = json.Unmarshal(record.Mutations, &result.Mutations)
	if result.Mutations == nil {
		result.Mutations = []codersdk.ChatMemoryMutation{}
	}
	return result
}
