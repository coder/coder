package chattool

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const canceledBeforeRunMessage = "tool call was canceled before it ran; no changes were made"

// toolCallIDNamespace is the UUIDv5 namespace for tool call IDs. Changing
// it changes every ID, so the agent would rerun in-flight tool calls.
var toolCallIDNamespace = uuid.MustParse("0af4d6d3-691c-45dd-ac54-55ea97970ebc")

// ToolCallID returns the ID the agent uses to deduplicate a tool call. It
// is derived from the chat, the assistant message, and the provider tool
// call ID, so every replica and retry computes the same ID.
func ToolCallID(chatID uuid.UUID, messageID int64, providerToolCallID string) uuid.UUID {
	return uuid.NewSHA1(toolCallIDNamespace, fmt.Appendf(nil, "%s/%d/%s", chatID, messageID, providerToolCallID))
}

// ToolCallIDs returns the ToolCallID of each call, keyed by provider tool
// call ID. Calls with an empty or repeated provider ID get none, since
// calls sharing an ID would get each other's saved responses.
func ToolCallIDs(chatID uuid.UUID, messageID int64, calls []fantasy.ToolCallContent) map[string]uuid.UUID {
	count := make(map[string]int, len(calls))
	for _, call := range calls {
		count[call.ToolCallID]++
	}
	ids := make(map[string]uuid.UUID, len(count))
	for providerID, n := range count {
		if providerID != "" && n == 1 {
			ids[providerID] = ToolCallID(chatID, messageID, providerID)
		}
	}
	return ids
}

// CancelToolCall cancels tool call id on the agent and returns its result.
// It returns an error if there is no result, such as when an old agent
// responds with 404.
func CancelToolCall(ctx context.Context, conn workspacesdk.AgentConn, id uuid.UUID, toolName string) (fantasy.ToolResponse, error) {
	if !CanCancelToolCall(toolName) {
		return fantasy.ToolResponse{}, xerrors.Errorf("tool %q cannot be canceled on the agent", toolName)
	}
	canceled, err := conn.CancelToolCall(ctx, id)
	if err != nil {
		return fantasy.ToolResponse{}, xerrors.Errorf("cancel tool call: %w", err)
	}
	switch toolName {
	case EditFilesToolName:
		if !canceled.Received {
			return fantasy.NewTextErrorResponse(canceledBeforeRunMessage), nil
		}
		return editFilesResponse(canceled.EditFilesResult()), nil
	case WriteFileToolName:
		if !canceled.Received {
			return fantasy.NewTextErrorResponse(canceledBeforeRunMessage), nil
		}
		return writeFileResponse(canceled.WriteFileResult()), nil
	case ExecuteToolName:
		if canceled.Received {
			if _, err := canceled.StartProcessResult(); err != nil {
				return errorResult(enrichStartError(fmt.Sprintf("start process: %v", err))), nil
			}
		}
		// The process can outlive the record, so read its output either way.
		// The cancel can return before the process exits, so wait for the
		// exit.
		output, err := conn.ProcessOutput(ctx, id.String(), &workspacesdk.ProcessOutputOptions{Wait: true})
		var sdkErr *codersdk.Error
		if !canceled.Received && errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
			return fantasy.NewTextErrorResponse(canceledBeforeRunMessage), nil
		}
		if err != nil {
			return fantasy.ToolResponse{}, xerrors.Errorf("read process output: %w", err)
		}
		exited := exitedResult(output)
		if output.Canceled {
			exited.Success, exited.ExitCode, exited.Error, exited.Canceled = false, -1, "tool call was canceled while running", true
		}
		return marshalToolResponse(exited), nil
	default:
		return fantasy.ToolResponse{}, xerrors.Errorf("tool %q cannot be canceled on the agent", toolName)
	}
}

// CanCancelToolCall reports whether CancelToolCall supports tool toolName.
func CanCancelToolCall(toolName string) bool {
	switch toolName {
	case EditFilesToolName, WriteFileToolName, ExecuteToolName:
		return true
	default:
		return false
	}
}
