package chattool

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// toolCallIDNamespace is the UUIDv5 namespace of tool call IDs. Changing
// it changes every tool call ID, so the agent would run a retried call
// again.
var toolCallIDNamespace = uuid.MustParse("0af4d6d3-691c-45dd-ac54-55ea97970ebc")

// ToolCallID returns the ID the workspace agent knows the tool call by:
// the call with provider tool call ID providerToolCallID in assistant
// message messageID of chat chatID. Every replica and attempt computes
// the same ID from committed rows, so the agent runs the call once.
func ToolCallID(chatID uuid.UUID, messageID int64, providerToolCallID string) uuid.UUID {
	return uuid.NewSHA1(toolCallIDNamespace, fmt.Appendf(nil, "%s/%d/%s", chatID, messageID, providerToolCallID))
}

// CancelToolCall cancels the execute, edit_files, or write_file call with
// tool call ID id on the agent and returns the call's result. It returns
// false for other tools and when the agent gives no answer, including the
// 404 of an agent without the cancel route.
func CancelToolCall(ctx context.Context, conn workspacesdk.AgentConn, id uuid.UUID, toolName string) (fantasy.ToolResponse, bool) {
	if toolName != ExecuteToolName && toolName != "edit_files" && toolName != "write_file" {
		return fantasy.ToolResponse{}, false
	}
	resp, err := conn.CancelToolCall(ctx, id)
	switch {
	case err != nil:
		return fantasy.ToolResponse{}, false
	case !resp.Received && toolName == ExecuteToolName:
		return errorResult("not run: canceled before the agent received it"), true
	case !resp.Received:
		return fantasy.NewTextErrorResponse("not applied: canceled before the agent received it"), true
	case toolName == "edit_files":
		return editFilesResponse(resp.EditFilesResult()), true
	case toolName == "write_file":
		return writeFileResponse(resp.WriteFileResult()), true
	}
	if _, err := resp.StartProcessResult(); err != nil {
		return errorResult(enrichStartError(fmt.Sprintf("start process: %v", err))), true
	}
	output, err := conn.ProcessOutput(ctx, id.String(), nil)
	if err != nil {
		return fantasy.ToolResponse{}, false
	}
	result := exitedResult(output)
	if output.Canceled {
		result.Success, result.ExitCode, result.Error = false, -1, "canceled by the user"
	}
	return marshalToolResponse(result), true
}
