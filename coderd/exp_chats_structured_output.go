package coderd

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// structuredOutputErrorMessages are the fixed messages of failure codes.
var structuredOutputErrorMessages = map[codersdk.ChatStructuredOutputErrorCode]string{
	codersdk.ChatStructuredOutputErrorCodeValidationExhausted: "The model's structured output was rejected 3 times.",
	codersdk.ChatStructuredOutputErrorCodeNotProduced:         "The turn ended without a valid structured output.",
	codersdk.ChatStructuredOutputErrorCodeInterrupted:         "The turn was interrupted before a valid structured output.",
	codersdk.ChatStructuredOutputErrorCodeGenerationFailed:    "The turn failed before a valid structured output.",
	codersdk.ChatStructuredOutputErrorCodeConfigurationError:  "The chat's model or tools cannot produce structured output.",
}

// getChatMessageStructuredOutput returns the result of the user message
// that set response_format. It works with the experiment off, so
// existing results stay readable.
//
// @Summary Get chat message structured output
// @ID get-chat-message-structured-output
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param chat path string true "Chat ID" format(uuid)
// @Param message path int true "Message ID"
// @Success 200 {object} codersdk.ChatStructuredOutput
// @Router /api/v2/chats/{chat}/messages/{message}/structured-output [get]
func (api *API) getChatMessageStructuredOutput(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chat := httpmw.ChatParam(r)
	messageID, err := strconv.ParseInt(chi.URLParam(r, "message"), 10, 64)
	if err != nil || messageID <= 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat message ID.",
			Detail:  "Message ID must be a positive integer.",
		})
		return
	}
	result, err := api.Database.GetChatStructuredOutput(ctx, database.GetChatStructuredOutputParams{ChatID: chat.ID, MessageID: messageID})
	if errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, convertChatStructuredOutput(result))
}

func convertChatStructuredOutput(result database.ChatStructuredOutput) codersdk.ChatStructuredOutput {
	out := codersdk.ChatStructuredOutput{
		ChatID:        result.ChatID,
		MessageID:     result.MessageID,
		Status:        codersdk.ChatStructuredOutputStatus(result.Status),
		RejectedCalls: result.RejectedCalls,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	}
	if result.Status == database.ChatStructuredOutputStatusSucceeded {
		out.Value = result.Value.RawMessage
	}
	if result.ErrorCode.Valid {
		code := codersdk.ChatStructuredOutputErrorCode(result.ErrorCode.String)
		out.Error = &codersdk.ChatStructuredOutputError{Code: code, Message: structuredOutputErrorMessages[code]}
	}
	return out
}
