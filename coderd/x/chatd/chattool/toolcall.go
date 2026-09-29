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

// ToolCallIDs returns the ToolCallID of each of calls, the unresolved
// calls of assistant message messageID, by provider tool call ID. A call
// whose provider tool call ID is empty or repeats gets none and runs as
// before tool call IDs: calls sharing an ID would get each other's saved
// responses from the agent.
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

// CancelToolCall cancels the execute, edit_files, or write_file call with
// tool call ID id on the agent and returns the call's result. An error
// means the agent gave no usable response, including the 404 of an agent
// without the cancel route.
func CancelToolCall(ctx context.Context, conn workspacesdk.AgentConn, id uuid.UUID, toolName string) (fantasy.ToolResponse, error) {
	canceled, err := conn.CancelToolCall(ctx, id)
	if err != nil {
		return fantasy.ToolResponse{}, xerrors.Errorf("cancel tool call: %w", err)
	}
	switch {
	case toolName == "edit_files" && canceled.Received:
		return editFilesResponse(canceled.EditFilesResult()), nil
	case toolName == "write_file" && canceled.Received:
		return writeFileResponse(canceled.WriteFileResult()), nil
	case toolName != ExecuteToolName:
		return fantasy.NewTextErrorResponse("not applied: canceled before the agent received it"), nil
	}
	if canceled.Received {
		if _, err := canceled.StartProcessResult(); err != nil {
			return errorResult(enrichStartError(fmt.Sprintf("start process: %v", err))), nil
		}
	}
	// Read the output even when the agent has no record of the call: a
	// process started with the tool call ID outlives the record, and the
	// cancel killed it. The agent responds to the cancel request after it
	// sends the kill, which can be before the process exits, so wait for
	// the exit to read all of its output. ctx bounds the wait.
	output, err := conn.ProcessOutput(ctx, id.String(), &workspacesdk.ProcessOutputOptions{Wait: true})
	var sdkErr *codersdk.Error
	if !canceled.Received && errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
		return errorResult("not run: canceled before the agent received it"), nil
	}
	if err != nil {
		return fantasy.ToolResponse{}, xerrors.Errorf("read process output: %w", err)
	}
	exited := exitedResult(output)
	if output.Canceled {
		exited.Success, exited.ExitCode, exited.Error = false, -1, "canceled by the user"
	}
	return marshalToolResponse(exited), nil
}
