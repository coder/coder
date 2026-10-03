package chatd

import (
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// workspaceContextScopeLine tells the model how the listed Source paths
// relate to each other. The user's ~/.coder files are global by contract,
// whatever the working directory is, so the exception is unconditional.
const workspaceContextScopeLine = "Instruction files under ~/.coder apply to the whole conversation; every other Source path applies to its own directory tree, and a nested file refines the ones above it for paths beneath it."

// formatSystemInstructions builds the <workspace-context> block from
// agent metadata and zero or more context-file parts. Non-context-file
// parts in the slice are silently skipped. note, when set, is printed after
// the header lines: it explains an empty file list, or names pinned files
// that could not be rendered next to the ones that were.
func formatSystemInstructions(
	operatingSystem, directory, note string,
	parts []codersdk.ChatMessagePart,
) string {
	hasContent := false
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeContextFile && part.ContextFileContent != "" {
			hasContent = true
			break
		}
	}
	if !hasContent && note == "" && operatingSystem == "" && directory == "" {
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
	if note != "" {
		_, _ = b.WriteString(note)
		_, _ = b.WriteString("\n")
	}
	if !hasContent {
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
