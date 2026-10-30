package coderd

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner"
)

// @Summary List chat previews
// @ID list-chat-previews
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Success 200 {array} codersdk.ChatPreview
// @Router /api/experimental/chats/{chat}/previews [get]
// @Description
// @Description Returns one entry per preview endpoint that open_preview
// @Description opened in the chat's current workspace.
// @Description Experimental: this endpoint is subject to change.
func (api *API) listChatPreviews(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !chat.WorkspaceID.Valid {
		httpapi.Write(ctx, rw, http.StatusOK, []codersdk.ChatPreview{})
		return
	}

	results, err := api.Database.GetChatOpenPreviewResults(ctx, chat.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat previews.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, chatPreviewsFromResults(chat.WorkspaceID.UUID, results))
}

// chatPreviewsFromResults keeps one preview per agent, endpoint and URL path,
// ordered by first result. The newest result supplies the title and path.
// Results that do not decode, that do not name exactly one valid port or
// preview ID on a valid agent, that carry an invalid URL path, or that belong
// to a workspace the chat is no longer bound to are skipped, because clients
// build URLs from them.
func chatPreviewsFromResults(workspaceID uuid.UUID, results []json.RawMessage) []codersdk.ChatPreview {
	type endpoint struct {
		agentName string
		port      int32
		previewID string
		urlPath   string
	}

	previews := []codersdk.ChatPreview{}
	indexes := make(map[endpoint]int)

	for _, raw := range results {
		var result chattool.OpenPreviewResult
		if err := json.Unmarshal(raw, &result); err != nil || result.WorkspaceID != workspaceID {
			continue
		}

		hasPort := result.Port >= 1000 && result.Port <= 65535
		hasPreviewID := appurl.IsPreviewID(result.PreviewID)
		if hasPort == hasPreviewID || !provisioner.AgentNameRegex.MatchString(result.AgentName) {
			continue
		}
		if result.URLPath != "" && (!hasPort || !chattool.IsPreviewURLPath(result.URLPath)) {
			continue
		}

		preview := codersdk.ChatPreview{
			Title:     result.Title,
			AgentName: result.AgentName,
			Port:      result.Port,
			PreviewID: result.PreviewID,
			Path:      result.Path,
			URLPath:   result.URLPath,
		}
		key := endpoint{
			agentName: preview.AgentName,
			port:      preview.Port,
			previewID: preview.PreviewID,
			urlPath:   preview.URLPath,
		}

		if i, ok := indexes[key]; ok {
			previews[i] = preview
			continue
		}

		indexes[key] = len(previews)
		previews = append(previews, preview)
	}

	return previews
}
