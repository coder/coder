import { useEffect } from "react";

/**
 * Global keyboard shortcuts for the Agents page.
 *
 * - Ctrl+N / Cmd+N — Create a new agent.
 */
export function useAgentsPageKeybindings({
	onNewAgent,
}: {
	onNewAgent: () => void;
}) {
	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			// Ignore events originating from inputs / textareas / contenteditable
			// so we don't hijack normal typing.
			const target = event.target as HTMLElement | null;
			if (target) {
				const tag = target.tagName;
				if (tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable) {
					return;
				}
			}

			// Ctrl+N / Cmd+N — new agent
			if (event.key === "n" && (event.metaKey || event.ctrlKey)) {
				event.preventDefault();
				onNewAgent();
			}
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [onNewAgent]);
}
