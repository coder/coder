package chatd

import (
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// workspaceContextScopeLine tells the model how the listed Source paths
// relate to each other.
const workspaceContextScopeLine = "Each Source path scopes its instructions to that directory tree; a nested file refines the files above it for paths beneath it."

// formatSystemInstructions builds the <workspace-context> block from
// agent metadata and zero or more context-file parts. Non-context-file
// parts in the slice are silently skipped. emptyNote is printed in place
// of the file list when no part has content, so the model learns why no
// instruction file is listed instead of receiving no block at all.
func formatSystemInstructions(
	operatingSystem, directory, emptyNote string,
	parts []codersdk.ChatMessagePart,
) string {
	hasContent := false
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeContextFile && part.ContextFileContent != "" {
			hasContent = true
			break
		}
	}
	if !hasContent && emptyNote == "" && operatingSystem == "" && directory == "" {
		return ""
	}

	var b strings.Builder
	_, _ = b.WriteString("<workspace-context>\n")
	if operatingSystem != "" {
		_, _ = b.WriteString("Operating System: ")
		_, _ = b.WriteString(operatingSystem)
		_, _ = b.WriteString("\n")
	}
	if directory != "" {
		_, _ = b.WriteString("Working Directory: ")
		_, _ = b.WriteString(directory)
		_, _ = b.WriteString("\n")
	}
	if !hasContent {
		if emptyNote != "" {
			_, _ = b.WriteString(emptyNote)
			_, _ = b.WriteString("\n")
		}
		_, _ = b.WriteString("</workspace-context>")
		return b.String()
	}
	_, _ = b.WriteString(workspaceContextScopeLine)
	_, _ = b.WriteString("\n")
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeContextFile || part.ContextFileContent == "" {
			continue
		}
		_, _ = b.WriteString("\nSource: ")
		_, _ = b.WriteString(part.ContextFilePath)
		if part.ContextFileTruncated {
			_, _ = b.WriteString(" (truncated to 64KiB)")
		}
		_, _ = b.WriteString("\n")
		_, _ = b.WriteString(part.ContextFileContent)
		_, _ = b.WriteString("\n")
	}
	_, _ = b.WriteString("</workspace-context>")
	return b.String()
}
