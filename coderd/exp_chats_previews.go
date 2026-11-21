package coderd

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
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
// @Description Returns the previews open_preview opened in the chat's current
// @Description workspace, in the order they were first opened. Port previews are
// @Description listed once per agent, port, and URL path, and file and directory
// @Description previews once per agent and path. The newest result supplies the
// @Description title and preview ID.
// @Description Experimental: this endpoint is subject to change.
func (api *API) listChatPreviews(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)

	if !chat.WorkspaceID.Valid {
		httpapi.Write(ctx, rw, http.StatusOK, []codersdk.ChatPreview{})
		return
	}

	results, err := api.Database.GetChatOpenPreviewResults(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat previews.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, chatPreviewsFromResults(chat.WorkspaceID.UUID, results))
}

// chatPreviewsFromResults takes results oldest first and keeps one preview per
// agent, port, and URL path, or per agent and path, in first-open order,
// with the newest result's title and preview ID. Clients build URLs from these,
// so results that are not a valid preview of workspaceID are skipped.
func chatPreviewsFromResults(workspaceID uuid.UUID, results []json.RawMessage) []codersdk.ChatPreview {
	type endpoint struct {
		agentName string
		port      int32
		path      string
		urlPath   string
	}

	previews := []codersdk.ChatPreview{}
	indexes := make(map[endpoint]int)

	for _, raw := range results {
		var result chattool.OpenPreviewResult
		if err := json.Unmarshal(chatprompt.DecodeNulInJSON(raw), &result); err != nil || result.WorkspaceID != workspaceID {
			continue
		}

		hasPort, hasPreviewID := result.Port != 0, result.PreviewID != ""
		if hasPort == hasPreviewID || !provisioner.AgentNameRegex.MatchString(result.AgentName) ||
			hasPort && (result.Port < appurl.PreviewMinimumPort || result.Port > 65535) ||
			hasPreviewID && !appurl.IsPreviewID(result.PreviewID) {
			continue
		}
		if result.URLPath != "" && !chattool.IsPreviewURLPath(result.URLPath) {
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
		// Keyed by path, not ID, because the agent gives a path a new ID after
		// it restarts and the newest ID is the one that works.
		key := endpoint{
			agentName: preview.AgentName,
			port:      preview.Port,
			path:      preview.Path,
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
