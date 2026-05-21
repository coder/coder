import { useEffect } from "react";
import { isMac } from "#/utils/platform";

/**
 * Global keyboard shortcuts for the Agents page.
 *
 * - Ctrl+N / Cmd+N: Create a new agent.
 * - Ctrl+K / Cmd+K: Open agent search.
 */
export function useAgentsPageKeybindings({
	onNewAgent,
	onOpenSearch,
}: {
	onNewAgent: () => void;
	onOpenSearch?: () => void;
}) {
	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			// Ignore events originating from inputs / textareas / contenteditable
			// so we don't hijack normal typing.
			const target = event.target;
			if (target instanceof HTMLElement) {
				const tag = target.tagName;
				if (tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable) {
					return;
				}
			}

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

			if (key === "k" && onOpenSearch) {
				event.preventDefault();
				onOpenSearch();
			}
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [onNewAgent, onOpenSearch]);
}
