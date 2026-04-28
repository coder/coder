package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// AttachFileOptions configures the attach_file tool.
type AttachFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	StoreFile        StoreFileFunc
}

// AttachFileArgs are the arguments for the attach_file tool.
type AttachFileArgs struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

// AttachFile returns a tool that stores a workspace file as a durable chat
// attachment so the user can download it directly from the conversation.
func AttachFile(options AttachFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"attach_file",
		"Attach a workspace file to the current chat so the user can download it directly from the conversation. "+
			"Use this when the user should receive an artifact such as a screenshot, log, patch, or document. "+
			"Pass an absolute file path. The file must already exist in the workspace.",
		func(ctx context.Context, args AttachFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			if options.StoreFile == nil {
				return fantasy.NewTextErrorResponse("file storage is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeAttachFileTool(ctx, conn, args, options.StoreFile)
		},
	)
}

func executeAttachFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args AttachFileArgs,
	storeFile StoreFileFunc,
) (fantasy.ToolResponse, error) {
	path := strings.TrimSpace(args.Path)
	if path == "" {
		return fantasy.NewTextErrorResponse("path is required (use an absolute path, e.g. /home/coder/build.log)"), nil
	}

	attachment, size, err := storeWorkspaceAttachment(
		ctx,
		conn,
		path,
		strings.TrimSpace(args.Name),
		storeFile,
	)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return WithAttachments(toolResponse(map[string]any{
		"ok":         true,
		"path":       path,
		"file_id":    attachment.FileID.String(),
		"name":       attachment.Name,
		"media_type": attachment.MediaType,
		"size":       size,
	}), attachment), nil
}
