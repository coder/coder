package chatd

import (
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// workspaceContextOpenTag and workspaceContextCloseTag delimit the
// workspace-context block. Untrusted context-file paths and contents
// are stripped of these literals before being written so they cannot
// close the block early and smuggle text outside it.
const (
	workspaceContextOpenTag  = "<workspace-context>"
	workspaceContextCloseTag = "</workspace-context>"
)

// formatSystemInstructions builds the <workspace-context> block from
// agent metadata and zero or more context-file parts. Non-context-file
// parts in the slice are silently skipped.
func formatSystemInstructions(
	operatingSystem, directory string,
	parts []codersdk.ChatMessagePart,
) string {
	hasContent := false
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeContextFile && part.ContextFileContent != "" {
			hasContent = true
			break
		}
	}
	if !hasContent && operatingSystem == "" && directory == "" {
		return ""
	}

	var b strings.Builder
	_, _ = b.WriteString(workspaceContextOpenTag)
	_, _ = b.WriteString("\n")
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
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeContextFile || part.ContextFileContent == "" {
			continue
		}
		_, _ = b.WriteString("\nSource: ")
		_, _ = b.WriteString(stripWorkspaceContextDelimiters(part.ContextFilePath))
		if part.ContextFileTruncated {
			_, _ = b.WriteString(" (truncated to 64KiB)")
		}
		_, _ = b.WriteString("\n")
		_, _ = b.WriteString(stripWorkspaceContextDelimiters(part.ContextFileContent))
		_, _ = b.WriteString("\n")
	}
	_, _ = b.WriteString(workspaceContextCloseTag)
	return b.String()
}

// stripWorkspaceContextDelimiters removes literal workspace-context
// block delimiters from untrusted text so a context file path or
// content cannot terminate the block early and smuggle text outside
// it. Removal repeats until the text stops changing so a crafted
// value such as "</workspace-cont</workspace-context>ext>" cannot
// reconstruct a delimiter after the surrounding characters collapse
// together.
func stripWorkspaceContextDelimiters(s string) string {
	for {
		before := s
		s = strings.ReplaceAll(s, workspaceContextCloseTag, "")
		s = strings.ReplaceAll(s, workspaceContextOpenTag, "")
		if s == before {
			return s
		}
	}
}
