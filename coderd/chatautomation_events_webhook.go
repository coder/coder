package coderd

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// maxMCPEventWebhookBody is the delivery size that the events extension asks
// servers to stay under.
const maxMCPEventWebhookBody = 256 << 10

// @Summary Receive MCP event webhook delivery
// @ID receive-chat-automation-mcp-event-webhook
// @Tags Chats
// @Accept json
// @Produce json
// @Param automation path string true "Chat automation ID" format(uuid)
// @Success 200
// @Router /api/experimental/chats/automations/{automation}/mcp-events [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatAutomationMCPEvents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseChatAutomationID(rw, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(rw, r.Body, maxMCPEventWebhookBody))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{Message: "The delivery is larger than 256 KiB."})
		return
	case err != nil:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Failed to read the delivery."})
		return
	case api.chatDaemon == nil:
		httpapi.Write(ctx, rw, http.StatusServiceUnavailable, codersdk.Response{Message: http.StatusText(http.StatusServiceUnavailable)})
		return
	}
	status, challenge := api.chatDaemon.ReceiveMCPEventWebhook(ctx, id, r.Header, body)
	if challenge != "" {
		httpapi.Write(ctx, rw, status, map[string]string{"challenge": challenge})
		return
	}
	httpapi.Write(ctx, rw, status, codersdk.Response{Message: http.StatusText(status)})
}

// unsubscribeChatAutomationWebhook ends a webhook subscription through the
// chat daemon, which reaches MCP servers with the owner's credentials.
func (api *API) unsubscribeChatAutomationWebhook(ctx context.Context, owner uuid.UUID, sub database.ChatAutomationMCPSubscription) {
	if api.chatDaemon != nil {
		api.chatDaemon.UnsubscribeWebhook(ctx, owner, sub)
	}
}
