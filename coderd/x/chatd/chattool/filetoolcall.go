package chattool

import (
	"context"
	"errors"
	"fmt"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Registered names of the file tools that send tool call headers.
const (
	EditFilesToolName = "edit_files"
	WriteFileToolName = "write_file"
)

// checkFileText tells the model how to learn whether a file tool call
// changed the file.
const checkFileText = "Check the file before changing it again."

// fileToolText holds the words a file tool's results use.
type fileToolText struct {
	// action names the request, change what it changes, and record a
	// recorded change of this tool call.
	action, change, record string
}

func fileToolTexts(toolName string) fileToolText {
	if toolName == WriteFileToolName {
		return fileToolText{action: "write file", change: "write", record: "a write"}
	}
	return fileToolText{action: "edit files", change: "edit", record: "an edit"}
}

// fileToolConnErrorResult converts a failure to connect to the workspace
// agent into the result of the file tool call toolName in ctx. An earlier
// attempt of the tool call may have applied the change, unless the
// workspace has no agent or was deleted.
func fileToolConnErrorResult(ctx context.Context, toolName string, err error) fantasy.ToolResponse {
	if _, ok := ToolCallIdentityFromContext(ctx); ok && ctx.Err() == nil &&
		!errors.Is(err, ErrWorkspaceHasNoAgent) && !errors.Is(err, ErrWorkspaceDeleted) {
		text := fileToolTexts(toolName)
		return fantasy.NewTextErrorResponse(UnknownOutcome(AgentUnreachableReason(err),
			"an earlier attempt may have applied the "+text.change, checkFileText))
	}
	return fantasy.NewTextErrorResponse(err.Error())
}

// fileRequestErrorResult converts an edit_files or write_file request
// error into the result of the tool call in ctx when the error depends on
// the tool call: the agent's refusal, or a failure without a readable
// answer. ok is false for other errors, including an error answer from
// the agent, which the tool reports as it always has.
func fileRequestErrorResult(ctx context.Context, toolName string, err error) (result fantasy.ToolResponse, ok bool) {
	if err == nil {
		return fantasy.ToolResponse{}, false
	}
	text := fileToolTexts(toolName)
	_, hasID := ToolCallIdentityFromContext(ctx)
	kind, code := ClassifyAgentError(err)
	switch {
	case kind == AgentErrorRefused:
		switch code {
		case workspacesdk.ToolCallErrorAgentStartedAfterToolCall:
			return agentRestartedFileResult(text), true
		case workspacesdk.ToolCallErrorInputMismatch:
			return fantasy.NewTextErrorResponse(fmt.Sprintf("%s: this request changed nothing because %s for this "+
				"tool call already exists with a different input: %v", text.action, text.record, err)), true
		default:
			// stale_tool_call and tool_call_canceled reach only a stale
			// attempt, whose commit fails the history version fence.
			return fantasy.NewTextErrorResponse(fmt.Sprintf("%s: %v", text.action, err)), true
		}
	// A canceled ctx means the result will not be committed.
	case !hasID || kind == AgentErrorResponse || ctx.Err() != nil:
		return fantasy.ToolResponse{}, false
	case kind == AgentErrorUnreachable:
		return fantasy.NewTextErrorResponse(UnknownOutcome(AgentUnreachableReason(err),
			"the "+text.change+" may have been applied", checkFileText)), true
	default:
		return fantasy.NewTextErrorResponse(UnknownOutcome(AgentUnreadableReason(err),
			"the "+text.change+" may have been applied", checkFileText)), true
	}
}

// agentRestartedFileResult is the result of a file tool call the
// workspace agent answered with agent_started_after_tool_call.
func agentRestartedFileResult(text fileToolText) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse(UnknownOutcome(AgentRestartedReason,
		"the "+text.change+" may have been applied before the restart", checkFileText))
}

// InterruptFileToolCall asks the workspace agent to cancel the edit_files
// or write_file tool call id, named toolName, and returns the result the
// tool call gets. The agent cannot stop an edit in progress, so it waits
// for it: a started call gets the result the tool returns for the
// agent's recorded answer. ok is false when the answer does not describe
// the tool call: an error answer other than agent_started_after_tool_call,
// including the 404 of an agent without the cancel route.
func InterruptFileToolCall(ctx context.Context, conn workspacesdk.AgentConn, toolName string, id ToolCallIdentity) (result fantasy.ToolResponse, ok bool) {
	cancelCtx := workspacesdk.WithToolCall(ctx, id.AgentToolCall())
	var (
		resp workspacesdk.CancelFileToolCallResponse
		err  error
	)
	if toolName == WriteFileToolName {
		resp, err = conn.CancelWriteFile(cancelCtx, id.UUID())
	} else {
		resp, err = conn.CancelEditFiles(cancelCtx, id.UUID())
	}
	text := fileToolTexts(toolName)
	if err != nil {
		return canceledFileToolCallErrorResult(text, err)
	}
	if !resp.Started {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("not applied: the %s was canceled before the workspace agent received it.", text.change)), true
	}
	if toolName == WriteFileToolName {
		return writeFileResult(resp.WriteFileResult()), true
	}
	return editFilesResult(resp.EditFilesResult()), true
}

// canceledFileToolCallErrorResult converts a cancel request error for a
// file tool call into its result.
func canceledFileToolCallErrorResult(text fileToolText, err error) (result fantasy.ToolResponse, ok bool) {
	switch kind, code := ClassifyAgentError(err); {
	case kind == AgentErrorRefused && code == workspacesdk.ToolCallErrorAgentStartedAfterToolCall:
		return agentRestartedFileResult(text), true
	case kind == AgentErrorRefused, kind == AgentErrorResponse:
		return fantasy.ToolResponse{}, false
	case kind == AgentErrorUnreachable:
		return agentUnreachableFileResult(text, err), true
	default:
		return fantasy.NewTextErrorResponse(UnknownOutcome(AgentUnreadableReason(err),
			"the "+text.change+" may have been applied", checkFileText)), true
	}
}

// AgentUnreachableFileToolCallResult returns the result of an interrupted
// edit_files or write_file call, named toolName, when the workspace agent
// could not be reached to cancel it.
func AgentUnreachableFileToolCallResult(toolName string, err error) fantasy.ToolResponse {
	return agentUnreachableFileResult(fileToolTexts(toolName), err)
}

func agentUnreachableFileResult(text fileToolText, err error) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse(UnknownOutcome(AgentUnreachableReason(err),
		"the "+text.change+" may have been applied", checkFileText))
}
