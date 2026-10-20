package chattool

import (
	"context"

	"charm.land/fantasy"
)

// CopyAttachmentsToWorkspaceToolName is the name of the tool that copies
// workspace attachments into the chat's workspace.
const CopyAttachmentsToWorkspaceToolName = "copy_attachments_to_workspace"

// WorkspaceAttachmentStatus is how far a workspace attachment got on its
// way into the chat's workspace.
type WorkspaceAttachmentStatus string

const (
	// WorkspaceAttachmentCopied means the file is in the workspace at
	// Path.
	WorkspaceAttachmentCopied WorkspaceAttachmentStatus = "copied"
	// WorkspaceAttachmentCopying means a copy was still due or in flight
	// when the wait ended.
	WorkspaceAttachmentCopying WorkspaceAttachmentStatus = "copying"
	// WorkspaceAttachmentPending means the chat has no running workspace
	// yet, or its startup has not finished.
	WorkspaceAttachmentPending WorkspaceAttachmentStatus = "pending"
	// WorkspaceAttachmentFailed means the last copy attempt failed with
	// Error.
	WorkspaceAttachmentFailed WorkspaceAttachmentStatus = "failed"
	// WorkspaceAttachmentUnavailable means the file is no longer stored,
	// for example because the chat's attachment cap evicted it.
	WorkspaceAttachmentUnavailable WorkspaceAttachmentStatus = "unavailable"
)

// WorkspaceAttachment reports one file the user attached for the
// workspace.
type WorkspaceAttachment struct {
	FileID      string                    `json:"file_id"`
	Name        string                    `json:"name"`
	MediaType   string                    `json:"media_type,omitempty"`
	SizeBytes   int64                     `json:"size_bytes,omitempty"`
	Status      WorkspaceAttachmentStatus `json:"status"`
	Path        string                    `json:"path,omitempty"`
	WorkspaceID string                    `json:"workspace_id,omitempty"`
	Error       string                    `json:"error,omitempty"`
}

// WorkspaceAttachmentsReport lists a chat's workspace attachments. Err
// reports a failure to read or reset them; the list may then be partial.
type WorkspaceAttachmentsReport struct {
	Attachments []WorkspaceAttachment
	Err         error
}

// AwaitWorkspaceAttachmentsFn waits briefly for the chat's workspace
// attachments to be copied and reports them.
type AwaitWorkspaceAttachmentsFn func(ctx context.Context) WorkspaceAttachmentsReport

// addWorkspaceAttachments adds the chat's workspace attachments to a
// workspace tool result. Problems with the attachments go into
// attachments_error so they never fail the workspace tool itself.
func addWorkspaceAttachments(ctx context.Context, result map[string]any, await AwaitWorkspaceAttachmentsFn) {
	if await == nil {
		return
	}
	report := await(ctx)
	if len(report.Attachments) > 0 {
		result["attachments"] = report.Attachments
	}
	if report.Err != nil {
		result["attachments_error"] = report.Err.Error()
	}
}

type copyAttachmentsToWorkspaceArgs struct{}

// copyAttachmentsToWorkspaceTool runs calls in order so two calls in one
// step do not reset and wait on the same files concurrently.
type copyAttachmentsToWorkspaceTool struct {
	fantasy.AgentTool
}

func (copyAttachmentsToWorkspaceTool) SerialToolCalls() bool { return true }

// CopyAttachmentsToWorkspace returns a tool that retries failed copies of
// the chat's workspace attachments, waits for them, and reports where each
// one is. copyFn must not be nil.
func CopyAttachmentsToWorkspace(copyFn AwaitWorkspaceAttachmentsFn) fantasy.AgentTool {
	return copyAttachmentsToWorkspaceTool{AgentTool: fantasy.NewAgentTool(
		CopyAttachmentsToWorkspaceToolName,
		"Copy the files the user attached for the workspace into the chat's "+
			"workspace and report where each one is. Coder copies these "+
			"files automatically once the workspace finishes starting, and "+
			"create_workspace and start_workspace report them too, so call "+
			"this only to check on them or to retry a failed copy. Takes no "+
			"arguments. Each attachment has a status: copied (the file is at "+
			"path), copying (still in progress; call again later), pending "+
			"(the workspace is not running or has not finished starting; use "+
			"start_workspace or create_workspace), failed (see error; calling "+
			"this tool again retries), or unavailable (the file is no longer "+
			"stored; ask the user to attach it again).",
		func(ctx context.Context, _ copyAttachmentsToWorkspaceArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			report := copyFn(ctx)
			attachments := report.Attachments
			if attachments == nil {
				attachments = []WorkspaceAttachment{}
			}
			result := map[string]any{"attachments": attachments}
			if report.Err != nil {
				result["attachments_error"] = report.Err.Error()
			} else if len(attachments) == 0 {
				result["message"] = "the user has not attached any files for the workspace in this chat"
			}
			return toolResponse(result), nil
		},
	)}
}
