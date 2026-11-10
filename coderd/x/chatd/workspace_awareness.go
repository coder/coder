package chatd

import (
	"slices"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// legacyWorkspaceAwarenessTexts holds the workspace awareness texts that
// chat creation saved as system messages before turns built awareness from
// the chat's current workspace binding. Those rows stay in existing chats,
// and their text is the only thing that identifies them. Entries must stay
// byte-identical to the saved rows.
var legacyWorkspaceAwarenessTexts = map[string]struct{}{
	// Attached, unchanged since coder/coder#23213.
	`This chat is attached to a workspace. You can use workspace tools like execute, read_file, write_file, etc.`: {},
	// Detached, before coder/coder#25615.
	`There is no workspace associated with this chat yet. Create one using the create_workspace tool before using workspace tools like execute, read_file, write_file, etc.`: {},
	// Detached, from coder/coder#25615 until coder/coder#29579.
	`No workspace is attached to this chat yet.
Do not create or start a workspace by default. Many requests can be completed using the conversation, provider tools such as web_search when available, or configured external MCP tools.
Workspace tools such as execute, read_file, write_file, and edit_files require an attached workspace. Only call create_workspace or start_workspace when the user explicitly asks for a workspace-backed task, or when the task cannot be completed without inspecting, editing, or running files in a workspace.
If a workspace is needed, use list_templates and read_template as needed before create_workspace.`: {},
	`No workspace is attached to this chat yet.
Do not create or start a workspace by default. Many requests can be completed using the conversation, provider tools such as web_search when available, or configured external MCP tools.
Workspace tools such as execute, read_file, write_file, and edit_files require an attached workspace. Only call create_workspace or start_workspace when the user explicitly asks for a workspace-backed task, or when the task cannot be completed without inspecting, editing, or running files in a workspace.
If a workspace is needed, use list_templates before create_workspace and follow its next_step. Call read_template only when you need template parameter or preset details.`: {},
	`No workspace is attached to this chat yet.
Do not create or start a workspace by default. Many requests can be completed using the conversation, provider tools such as web_search when available, or configured external MCP tools.
Workspace tools such as execute, read_file, write_file, and edit_files require an attached workspace. This delegated chat cannot create or start a workspace. If workspace-backed work is required, report that need to the parent agent instead of trying workspace tools.`: {},
	// Detached, from coder/coder#29579 until CODAGT-1501.
	`This chat started without an attached workspace. Follow subsequent workspace tool results and context for its current state.
Use the conversation and available tools, skills, and MCPs when they are sufficient for the request.
Workspace tools such as execute, read_file, write_file, and edit_files require an attached workspace. If no workspace is attached, create a suitable workspace with create_workspace when missing tools, skills, MCPs, or context prevent progress, or workspace-backed work is needed. Use the workspace's available context and capabilities to continue the user's request. Workspace readiness does not guarantee that skills, MCP tools, or context have finished loading. Use capabilities that are actually exposed, and continue with workspace file and shell tools where possible instead of recreating the workspace.
Requests such as "fix this bug" or "build this app" authorize the workspace setup needed to complete them; the user does not need to request a workspace separately. Do not refuse solely because no workspace is attached. If setup is blocked, explain the specific blocker or required user choice.
Answer questions and self-contained code examples directly when the conversation and available tools are sufficient.
If a workspace is needed, use list_templates before create_workspace and follow its next_step. Call read_template only when you need template parameter or preset details.`: {},
	`This chat started without an attached workspace. Follow subsequent workspace tool results and context for its current state.
Use the conversation and available tools, skills, and MCPs when they are sufficient for the request.
Workspace tools such as execute, read_file, write_file, and edit_files require an attached workspace. This delegated chat cannot create or start a workspace. If workspace-backed work is required, report that need to the parent agent instead of trying workspace tools.`: {},
}

// isLegacyWorkspaceAwareness reports whether msg is workspace awareness that
// chat creation saved.
func isLegacyWorkspaceAwareness(msg database.ChatMessage) bool {
	if msg.Role != database.ChatMessageRoleSystem {
		return false
	}
	parts, err := chatprompt.ParseContent(msg)
	if err != nil || len(parts) != 1 || parts[0].Type != codersdk.ChatMessagePartTypeText {
		return false
	}
	_, ok := legacyWorkspaceAwarenessTexts[parts[0].Text]
	return ok
}

// withoutLegacyWorkspaceAwareness returns a copy of rows without workspace
// awareness that chat creation saved.
func withoutLegacyWorkspaceAwareness(rows []database.ChatMessage) []database.ChatMessage {
	return slices.DeleteFunc(slices.Clone(rows), isLegacyWorkspaceAwareness)
}
