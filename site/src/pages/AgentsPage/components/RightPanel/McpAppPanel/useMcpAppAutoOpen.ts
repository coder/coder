import { useEffect, useEffectEvent } from "react";
import type { ChatMCPApp } from "#/api/typesGenerated";
import type { ChatStore } from "../../ChatConversation/chatStore";
import { mcpAppToolResults } from "./chatMcpApps";

/**
 * Calls onOpen once for each MCP tool result with a view that arrives after
 * mount. A tool result often skips the live stream state and only comes with
 * its persisted message, so messages above newestMessageIdAtMount count too.
 */
export function useMcpAppAutoOpen(
	store: ChatStore,
	enabled: boolean,
	newestMessageIdAtMount: number,
	onOpen: (toolCallId: string, app: ChatMCPApp) => void,
): void {
	const open = useEffectEvent(onOpen);
	useEffect(() => {
		if (!enabled) {
			return;
		}
		const initial = store.getSnapshot();
		const seen = new Set(Object.keys(initial.streamState?.toolResults ?? {}));
		let lastMessages = initial.messagesByID;
		const consider = (toolCallId: string, app: ChatMCPApp | undefined) => {
			if (!app || seen.has(toolCallId)) {
				return;
			}
			seen.add(toolCallId);
			open(toolCallId, app);
		};
		const scanMessages = () => {
			for (const result of mcpAppToolResults(lastMessages.values())) {
				if (result.messageId > newestMessageIdAtMount) {
					consider(result.toolCallId, result.app);
				}
			}
		};
		// Newer messages can already be in the store when the effect runs.
		scanMessages();
		return store.subscribe(() => {
			const state = store.getSnapshot();
			for (const [toolCallId, result] of Object.entries(
				state.streamState?.toolResults ?? {},
			)) {
				consider(toolCallId, result.mcpApp);
			}
			if (state.messagesByID !== lastMessages) {
				lastMessages = state.messagesByID;
				scanMessages();
			}
		});
	}, [store, enabled, newestMessageIdAtMount]);
}
