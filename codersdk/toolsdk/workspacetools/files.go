package workspacetools

import (
	"context"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ReadFileResult is the structured response from the read file tool.
// Keep fields in alphabetical order: Coder Agents tool responses use
// sorted JSON keys.
type ReadFileResult struct {
	Content    string `json:"content"`
	FileSize   int64  `json:"file_size"`
	LinesRead  int    `json:"lines_read"`
	TotalLines int    `json:"total_lines"`
}

// ReadFile reads line-numbered content from a workspace file. offset is
// a 1-based line number (nil means 1) and limit is a line count (nil
// uses the agent default).
func ReadFile(ctx context.Context, conn workspacesdk.AgentConn, path string, offset, limit *int64) (ReadFileResult, error) {
	if path == "" {
		return ReadFileResult{}, xerrors.New("path is required")
	}
	lineOffset := int64(1)
	lineLimit := int64(0) // The agent applies its default.
	if offset != nil {
		lineOffset = *offset
	}
	if limit != nil {
		lineLimit = *limit
	}
	resp, err := conn.ReadFileLines(ctx, path, lineOffset, lineLimit, workspacesdk.DefaultReadFileLinesLimits())
	if err != nil {
		return ReadFileResult{}, err
	}
	if !resp.Success {
		return ReadFileResult{}, xerrors.New(resp.Error)
	}
	return ReadFileResult{
		Content:    resp.Content,
		FileSize:   resp.FileSize,
		TotalLines: resp.TotalLines,
		LinesRead:  resp.LinesRead,
	}, nil
}

// ValidateFileEdits trims each path in place and rejects batches the
// agent would otherwise apply partially or not at all.
func ValidateFileEdits(files []workspacesdk.FileEdits) error {
	if len(files) == 0 {
		return xerrors.New("files is required")
	}
	for i := range files {
		files[i].Path = strings.TrimSpace(files[i].Path)
		if files[i].Path == "" {
			return xerrors.Errorf(
				"files[%d].path is required; provide the absolute path of the file to edit; no files in this batch were applied", i,
			)
		}
		if len(files[i].Edits) == 0 {
			return xerrors.Errorf(
				"files[%d].edits must contain at least one edit; no files in this batch were applied", i,
			)
		}
	}
	return nil
}

// EditFilesResult is the structured success response from the edit
// files tool. Keep fields in alphabetical order: Coder Agents tool
// responses use sorted JSON keys.
type EditFilesResult struct {
	Files []workspacesdk.FileEditResult `json:"files"`
	OK    bool                          `json:"ok"`
}

// EditFiles applies validated edits and returns a per-file diff.
func EditFiles(ctx context.Context, conn workspacesdk.AgentConn, files []workspacesdk.FileEdits) (EditFilesResult, error) {
	resp, err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{
		Files:       files,
		IncludeDiff: true,
	})
	if err != nil {
		return EditFilesResult{}, err
	}
	return EditFilesResult{OK: true, Files: resp.Files}, nil
}

// OKResult is the structured success response from the write file tool.
type OKResult struct {
	OK bool `json:"ok"`
}

// AgentAPIErrorMessage preserves the agent's actionable message while
// dropping the transport metadata (HTTP method, URL, status code) that
// codersdk.Error.Error() prefixes.
func AgentAPIErrorMessage(err error) string {
	sdkErr, ok := codersdk.AsError(err)
	if !ok || sdkErr.Message == "" {
		return err.Error()
	}
	var sb strings.Builder
	_, _ = sb.WriteString(sdkErr.Message)
	if sdkErr.Helper != "" {
		_, _ = sb.WriteString(": " + sdkErr.Helper)
	}
	if sdkErr.Detail != "" {
		_, _ = sb.WriteString(": " + sdkErr.Detail)
	}
	for _, v := range sdkErr.Validations {
		_, _ = sb.WriteString("\n- " + v.Field + ": " + v.Detail)
	}
	return sb.String()
}
