package chattool

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Registered names of the file tools that send tool call headers.
const (
	EditFilesToolName = "edit_files"
	WriteFileToolName = "write_file"
)

// fileToolChange names what a file tool changes, for result text.
func fileToolChange(toolName string) string {
	if toolName == WriteFileToolName {
		return "write"
	}
	return "edit"
}

// fileToolCallErrorResult converts an edit_files or write_file request
// error into the tool result when the error depends on the tool call in
// ctx: the agent's refusal, or a failure without an agent response. ok is
// false for other errors. action names the request and change names what
// it would have changed, for example "write".
func fileToolCallErrorResult(ctx context.Context, action, change string, err error) (result fantasy.ToolResponse, ok bool) {
	var tcErr *workspacesdk.ToolCallError
	if !errors.As(err, &tcErr) {
		var sdkErr *codersdk.Error
		_, hasID := ToolCallIdentityFromContext(ctx)
		// Without an agent response the request may have reached the
		// agent. A canceled ctx means the result will not be committed.
		if hasID && !errors.As(err, &sdkErr) && ctx.Err() == nil {
			return fantasy.NewTextErrorResponse(fmt.Sprintf("outcome unknown: the workspace agent could not be reached "+
				"after the %s request was sent, so the %s may have been applied: %v. Check the file before changing it again.",
				change, change, err)), true
		}
		return fantasy.ToolResponse{}, false
	}
	switch tcErr.Code {
	case workspacesdk.ToolCallErrorAgentStartedAfterToolCall:
		return agentRestartedFileResult(change), true
	case workspacesdk.ToolCallErrorInputMismatch:
		return fantasy.NewTextErrorResponse(fmt.Sprintf("%s: the workspace agent has a record of this tool call "+
			"with a different input, so the %s was not applied: %v", action, change, tcErr)), true
	default:
		// stale_tool_call and tool_call_canceled reach only a stale
		// attempt, whose commit fails the history version fence.
		return fantasy.NewTextErrorResponse(fmt.Sprintf("%s: %v", action, tcErr)), true
	}
}

// agentRestartedFileResult is the result of a file tool call the
// workspace agent answered with agent_started_after_tool_call.
func agentRestartedFileResult(change string) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse(fmt.Sprintf("outcome unknown: the workspace agent restarted after this tool call, "+
		"so the %s may have been applied before the restart. Check the file before changing it again.", change))
}

// InterruptFileToolCall asks the workspace agent to cancel the edit_files
// or write_file tool call id, named toolName, and returns the result the
// tool call gets. The agent cannot stop an edit in progress, so it waits
// for it: a started call gets the result the tool would have returned. ok
// is false when the answer does not describe the tool call: an error
// response other than agent_started_after_tool_call, including the 404
// of an agent without the cancel route.
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
	change := fileToolChange(toolName)
	if err != nil {
		return canceledFileToolCallErrorResult(change, err)
	}
	if !resp.Started {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("not applied: the %s was canceled before the workspace agent received it.", change)), true
	}
	// ctx carries no tool call identity, so a recorded error keeps the
	// text the tool gives it.
	if toolName == WriteFileToolName {
		return writeFileResult(ctx, resp.WriteFileResult()), true
	}
	editResp, editErr := resp.EditFilesResult()
	return editFilesResult(ctx, editResp, editErr), true
}

// canceledFileToolCallErrorResult converts a cancel request error for a
// file tool call into its result.
func canceledFileToolCallErrorResult(change string, err error) (result fantasy.ToolResponse, ok bool) {
	var tcErr *workspacesdk.ToolCallError
	if errors.As(err, &tcErr) {
		if tcErr.Code == workspacesdk.ToolCallErrorAgentStartedAfterToolCall {
			return agentRestartedFileResult(change), true
		}
		return fantasy.ToolResponse{}, false
	}
	var sdkErr *codersdk.Error
	if errors.As(err, &sdkErr) {
		return fantasy.ToolResponse{}, false
	}
	// The HTTP client returns *url.Error when no response arrived.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return agentUnreachableFileResult(change, err), true
	}
	return fantasy.NewTextErrorResponse(fmt.Sprintf("outcome unknown: the workspace agent answered the cancel request, "+
		"but its response could not be read, so the %s may have been applied: %v. Check the file before changing it again.",
		change, err)), true
}

// AgentUnreachableFileToolCallResult returns the result of an interrupted
// edit_files or write_file call, named toolName, when the workspace agent
// could not be reached to cancel it.
func AgentUnreachableFileToolCallResult(toolName string, err error) fantasy.ToolResponse {
	return agentUnreachableFileResult(fileToolChange(toolName), err)
}

func agentUnreachableFileResult(change string, err error) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse(fmt.Sprintf("outcome unknown: the workspace agent could not be reached "+
		"to cancel the %s, so the %s may have been applied: %v. Check the file before changing it again.", change, change, err))
}
