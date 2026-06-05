package chattool

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type ReadFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type ReadFileArgs struct {
	Path   string `json:"path"`
	Offset *int64 `json:"offset,omitempty"`
	Limit  *int64 `json:"limit,omitempty"`
}

func ReadFile(options ReadFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_file",
		"Read a file or list a directory in the workspace. "+
			"For files, returns line-numbered content. "+
			"For directories, returns a non-recursive listing. "+
			"The offset parameter is a 1-based line number or directory entry (default: 1). "+
			"The limit parameter is the number of lines or directory entries to return (default: 2000). "+
			"For large files and directories, use offset and limit to paginate.",
		func(ctx context.Context, args ReadFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeReadFileTool(ctx, conn, args)
		},
	)
}

func executeReadFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ReadFileArgs,
) (fantasy.ToolResponse, error) {
	if args.Path == "" {
		return fantasy.NewTextErrorResponse("path is required"), nil
	}

	offset := int64(1) // 1-based line number default
	limit := int64(0)  // 0 means use server default (2000)
	if args.Offset != nil {
		offset = *args.Offset
	}
	if args.Limit != nil {
		limit = *args.Limit
	}

	resp, err := conn.ReadFileLines(ctx, args.Path, offset, limit, workspacesdk.DefaultReadFileLinesLimits())
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	if !resp.Success {
		if readFileLinesErrorIsDirectory(resp.Error) {
			return executeReadFileDirectoryListing(ctx, conn, args, offset, limit, resp.Error)
		}
		return fantasy.NewTextErrorResponse(resp.Error), nil
	}

	return toolResponse(map[string]any{
		"content":     resp.Content,
		"file_size":   resp.FileSize,
		"total_lines": resp.TotalLines,
		"lines_read":  resp.LinesRead,
	}), nil
}

// readFileLinesErrorIsDirectory returns true when the ReadFileLines error
// indicates that the path is a directory. The workspace agent's file-read
// handler emits this prefix for directories.
func readFileLinesErrorIsDirectory(err string) bool {
	return strings.HasPrefix(strings.TrimSpace(err), workspacesdk.ReadFileLinesNotFileErrorPrefix)
}

// executeReadFileDirectoryListing falls back to the agent's directory-list
// endpoint after ReadFileLines reports that the path is a directory.
func executeReadFileDirectoryListing(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ReadFileArgs,
	offset int64,
	limit int64,
	readErr string,
) (fantasy.ToolResponse, error) {
	resp, err := conn.LS(ctx, args.Path, workspacesdk.LSRequest{
		Relativity: workspacesdk.LSRelativityRoot,
	})
	if err != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("%s; failed to list directory: %s", readErr, err),
		), nil
	}

	listing, err := directoryListingResult(resp, offset, limit)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return toolResponse(map[string]any{
		"content":              listing.content,
		"is_directory":         true,
		"absolute_path":        resp.AbsolutePath,
		"absolute_path_string": resp.AbsolutePathString,
		"entries_read":         listing.entriesRead,
		"total_entries":        len(resp.Contents),
		"truncated":            listing.truncated,
	}), nil
}

type directoryListing struct {
	content     string
	entriesRead int
	truncated   bool
}

// directoryListingResult applies read_file pagination semantics to an LS
// response and keeps the formatted listing within the tool output budget.
func directoryListingResult(resp workspacesdk.LSResponse, offset, limit int64) (directoryListing, error) {
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 {
		limit = workspacesdk.DefaultMaxResponseLines
	}

	totalEntries := len(resp.Contents)
	if totalEntries == 0 {
		return directoryListing{}, nil
	}
	if offset > int64(totalEntries) {
		return directoryListing{}, xerrors.Errorf("offset %d exceeds directory entry count %d", offset, totalEntries)
	}

	start := int(offset - 1)
	remaining := totalEntries - start
	entriesToRead := remaining
	if limit < int64(remaining) {
		entriesToRead = int(limit)
	}
	end := start + entriesToRead

	content, entriesRead, byteTruncated := formatDirectoryListing(
		resp.Contents[start:end],
		offset,
		int(workspacesdk.DefaultMaxResponseBytes),
	)
	return directoryListing{
		content:     content,
		entriesRead: entriesRead,
		truncated:   byteTruncated || start+entriesRead < totalEntries,
	}, nil
}

// formatDirectoryListing formats directory entries until the line or byte
// budget is exhausted. It returns the formatted content, entries read, and
// whether the byte budget truncated the listing.
func formatDirectoryListing(entries []workspacesdk.LSFile, offset int64, maxBytes int) (string, int, bool) {
	var b strings.Builder
	for i, entry := range entries {
		line := fmt.Sprintf("%d\t%s\n", offset+int64(i), lsFileDisplayName(entry))
		if b.Len()+len(line) > maxBytes {
			return b.String(), i, true
		}
		_, _ = b.WriteString(line)
	}
	return b.String(), len(entries), false
}

// lsFileDisplayName returns the stable display form shared by tools that show
// LS entries to the model.
func lsFileDisplayName(entry workspacesdk.LSFile) string {
	if entry.IsDir {
		return entry.Name + "/"
	}
	return entry.Name
}
