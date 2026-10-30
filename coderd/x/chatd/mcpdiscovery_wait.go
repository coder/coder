package chatd

import "strings"

// appendWorkspaceMCPNote adds the workspace MCP discovery summary to the
// workspace-context system block so the model can explain a missing or
// incomplete tool inventory instead of guessing. An empty instruction
// block (no instruction files and no agent metadata) gets a minimal block
// of its own.
func appendWorkspaceMCPNote(instruction, summary string) string {
	if summary == "" {
		return instruction
	}
	const closing = "</workspace-context>"
	line := "\nWorkspace MCP: " + summary + "\n"
	if instruction == "" {
		return "<workspace-context>" + line + closing
	}
	if idx := strings.LastIndex(instruction, closing); idx >= 0 {
		return instruction[:idx] + line + instruction[idx:]
	}
	return instruction + line
}
