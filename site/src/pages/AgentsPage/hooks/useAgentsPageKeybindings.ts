import { useEffect } from "react";
import {
	getDefaultVimModifier,
	isLetterKey,
	isModifierPressed,
	isSlashKey,
	type VimModifier,
} from "../utils/keyboardShortcuts";

/**
 * Global keyboard shortcuts for the Agents page.
 *
 * - Ctrl+N / Cmd+N: Create a new agent.
 * - Ctrl+K / Cmd+K: Toggle agent search.
 *
 * With vim navigation enabled, these bindings apply using the configured
 * vim modifier:
 *
 * - Modifier+/: Toggle agent search.
 * - Modifier+Shift+O: Create a new agent.
 * - Modifier+Shift+E: Rename the active chat.
 *
 * The two default bindings keep the platform modifier (Cmd on macOS,
 * Ctrl elsewhere). When the vim modifier is the platform modifier,
 * Ctrl+K / Cmd+K is left to chat navigation and search is only
 * reachable through Modifier+/.
 */
export function useAgentsPageKeybindings({
	onNewAgent,
	onToggleSearch,
	onRenameActiveChat,
	vimNavigationEnabled,
	vimModifier,
}: {
	onNewAgent: () => void;
	onToggleSearch?: () => void;
	onRenameActiveChat?: () => void;
	vimNavigationEnabled: boolean;
	vimModifier: VimModifier;
}) {
	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			const platformModifier = getDefaultVimModifier();
			const isPlatformChord = isModifierPressed(event, platformModifier);
			const isVimChord =
				vimNavigationEnabled && isModifierPressed(event, vimModifier);
			const searchKeyTakenByNavigation =
				vimNavigationEnabled && vimModifier === platformModifier;

			// "/" is a shifted key on many layouts, so it is matched before
			// the Shift branch.
			if (isVimChord && isSlashKey(event)) {
				if (onToggleSearch) {
					event.preventDefault();
					onToggleSearch();
				}
				return;
			}

			if (event.shiftKey) {
				if (!isVimChord) {
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

			if (!isPlatformChord) {
				return;
			}

			if (isLetterKey(event, "n")) {
				event.preventDefault();
				onNewAgent();
				return;
			}

			if (
				isLetterKey(event, "k") &&
				!searchKeyTakenByNavigation &&
				onToggleSearch
			) {
				event.preventDefault();
				onToggleSearch();
			}
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [
		onNewAgent,
		onToggleSearch,
		onRenameActiveChat,
		vimNavigationEnabled,
		vimModifier,
	]);
}
