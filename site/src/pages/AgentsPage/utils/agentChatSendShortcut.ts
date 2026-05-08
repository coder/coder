import type { AgentChatSendShortcut } from "#/api/typesGenerated";

export const DEFAULT_AGENT_CHAT_SEND_SHORTCUT: AgentChatSendShortcut = "enter";
export const MODIFIER_AGENT_CHAT_SEND_SHORTCUT: AgentChatSendShortcut =
	"modifier_enter";

export function getAgentChatSendShortcut(
	storedShortcut: AgentChatSendShortcut | undefined,
	isLoading: boolean,
): AgentChatSendShortcut {
	if (storedShortcut) {
		return storedShortcut;
	}
	return isLoading
		? MODIFIER_AGENT_CHAT_SEND_SHORTCUT
		: DEFAULT_AGENT_CHAT_SEND_SHORTCUT;
}
