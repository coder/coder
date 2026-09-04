import { useEffect } from "react";
import { isMac } from "#/utils/platform";
import { isLetterKey } from "../utils/keyboardShortcuts";

/**
 * Global keyboard shortcuts for the Agents page.
 *
 * - Ctrl+N / Cmd+N: Create a new agent.
 * - Ctrl+K / Cmd+K: Toggle agent search.
 *
 * With vim navigation enabled, Ctrl+K / Cmd+K is left to chat navigation
 * and these bindings apply instead:
 *
 * - Ctrl+/ / Cmd+/: Toggle agent search.
 * - Ctrl+Shift+O / Cmd+Shift+O: Create a new agent.
 * - Ctrl+Shift+E / Cmd+Shift+E: Rename the active chat.
 */
export function useAgentsPageKeybindings({
	onNewAgent,
	onToggleSearch,
	onRenameActiveChat,
	vimNavigationEnabled = false,
}: {
	onNewAgent: () => void;
	onToggleSearch?: () => void;
	onRenameActiveChat?: () => void;
	vimNavigationEnabled?: boolean;
}) {
	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			const isModifierPressed = isMac() ? event.metaKey : event.ctrlKey;
			if (!isModifierPressed || event.altKey) {
				return;
			}

			// "/" is a shifted key on many layouts, so it is matched before
			// the Shift branch.
			if (event.key === "/") {
				if (vimNavigationEnabled && onToggleSearch) {
					event.preventDefault();
					onToggleSearch();
				}
				return;
			}

			if (event.shiftKey) {
				if (!vimNavigationEnabled) {
					return;
				}
				if (isLetterKey(event, "o")) {
					event.preventDefault();
					onNewAgent();
				} else if (isLetterKey(event, "e") && onRenameActiveChat) {
					event.preventDefault();
					onRenameActiveChat();
				}
				return;
			}

			if (isLetterKey(event, "n")) {
				event.preventDefault();
				onNewAgent();
				return;
			}

			if (isLetterKey(event, "k") && !vimNavigationEnabled && onToggleSearch) {
				event.preventDefault();
				onToggleSearch();
			}
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [onNewAgent, onToggleSearch, onRenameActiveChat, vimNavigationEnabled]);
}
