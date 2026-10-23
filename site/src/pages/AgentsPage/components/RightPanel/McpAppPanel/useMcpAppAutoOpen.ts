import { useEffect, useEffectEvent } from "react";
import type { ChatMCPApp } from "#/api/typesGenerated";
import type { useChatStore } from "../../ChatConversation/chatStore";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

/**
 * Calls onOpen once for each new live MCP tool result with a view, so its
 * tab opens at once. Results that are already in the stream when the
 * hook subscribes do not open a tab.
 */
export function useMcpAppAutoOpen(
	store: ChatStoreHandle,
	enabled: boolean,
	onOpen: (toolCallId: string, app: ChatMCPApp) => void,
): void {
	const open = useEffectEvent(onOpen);
	useEffect(() => {
		if (!enabled) {
			return;
		}
		const seen = new Set(
			Object.keys(store.getSnapshot().streamState?.toolResults ?? {}),
		);
		return store.subscribe(() => {
			const results = store.getSnapshot().streamState?.toolResults ?? {};
			for (const [toolCallId, result] of Object.entries(results)) {
				if (!result.mcpApp || seen.has(toolCallId)) {
					continue;
				}
				seen.add(toolCallId);
				open(toolCallId, result.mcpApp);
			}
		});
	}, [store, enabled]);
}
