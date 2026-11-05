import type { AgentChatSendShortcut } from "#/api/typesGenerated";

export const DEFAULT_AGENT_CHAT_SEND_SHORTCUT: AgentChatSendShortcut = "enter";
export const MODIFIER_AGENT_CHAT_SEND_SHORTCUT: AgentChatSendShortcut =
	"modifier_enter";

/**
 * Reports whether plain Enter sends an agent chat message. Touch keyboards
 * need plain Enter for newlines, so a coarse pointer never sends on Enter.
 */
export function enterSendsAgentChatMessage(
	shortcut: AgentChatSendShortcut,
	isCoarsePointer: boolean,
): boolean {
	return shortcut !== MODIFIER_AGENT_CHAT_SEND_SHORTCUT && !isCoarsePointer;
}

/**
 * Returns the shortcut hints that the send button shows. Mobile viewports
 * show no hints.
 */
export function getAgentChatSendShortcutHints(
	enterSends: boolean,
	isMobile: boolean,
): {
	sendShortcutLabel: string | undefined;
	sendButtonKeyShortcuts: string | undefined;
	sendAfterTurnShortcutLabel: string | undefined;
} {
	if (isMobile) {
		return {
			sendShortcutLabel: undefined,
			sendButtonKeyShortcuts: undefined,
			sendAfterTurnShortcutLabel: undefined,
		};
	}
	return enterSends
		? {
				sendShortcutLabel: "Enter",
				sendButtonKeyShortcuts: "Enter",
				sendAfterTurnShortcutLabel: "Cmd/Ctrl+Enter",
			}
		: {
				sendShortcutLabel: "Cmd/Ctrl+Enter",
				sendButtonKeyShortcuts: "Control+Enter Meta+Enter",
				sendAfterTurnShortcutLabel: "Option/Alt+Enter",
			};
}

export function getAgentChatSendShortcut(
	storedShortcut: AgentChatSendShortcut | undefined,
	isLoading: boolean,
): AgentChatSendShortcut {
	if (storedShortcut) {
		return storedShortcut;
	}
	// Keep the loading fallback conservative. If a user saved
	// modifier_enter, falling back to enter before preferences load can
	// send a draft when they intended to insert a newline.
	return isLoading
		? MODIFIER_AGENT_CHAT_SEND_SHORTCUT
		: DEFAULT_AGENT_CHAT_SEND_SHORTCUT;
}
