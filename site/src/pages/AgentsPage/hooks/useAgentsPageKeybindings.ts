import { useEffect } from "react";
import { isMac } from "#/utils/platform";

/**
 * Global keyboard shortcuts for the Agents page.
 *
 * - Ctrl+N / Cmd+N: Create a new agent.
 * - Ctrl+K / Cmd+K: Toggle agent search.
 * - Ctrl+, / Cmd+,: Open settings.
 */
export function useAgentsPageKeybindings({
	onNewAgent,
	onToggleSearch,
	onOpenSettings,
}: {
	onNewAgent: () => void;
	onToggleSearch?: () => void;
	onOpenSettings?: () => void;
}) {
	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			const isModifierPressed = isMac() ? event.metaKey : event.ctrlKey;
			if (!isModifierPressed || event.altKey || event.shiftKey) {
				return;
			}

			const key = event.key.toLowerCase();
			if (key === "n") {
				event.preventDefault();
				onNewAgent();
				return;
			}

			if (key === "k" && onToggleSearch) {
				event.preventDefault();
				onToggleSearch();
				return;
			}

			if (key === "," && onOpenSettings) {
				event.preventDefault();
				onOpenSettings();
			}
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [onNewAgent, onToggleSearch, onOpenSettings]);
}
