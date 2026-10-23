import { useEffect, useEffectEvent } from "react";
import type { ChatMCPApp } from "#/api/typesGenerated";
import type { useChatStore } from "../../ChatConversation/chatStore";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

/**
 * Calls onOpen once for each new MCP tool result with a view, so its tab
 * opens at once. A result is new when it streams in after the hook
 * subscribes, or when it arrives in a message newer than
 * newestMessageIdAtMount. The tool result often skips the stream state
 * and only arrives with the persisted message, so both are watched.
 * Older pages that load later have lower IDs and open nothing.
 */
export function useMcpAppAutoOpen(
	store: ChatStoreHandle,
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
		const scanMessages = (messages: typeof lastMessages) => {
			for (const message of messages.values()) {
				if (message.id <= newestMessageIdAtMount) {
					continue;
				}
				for (const part of message.content ?? []) {
					if (part.type === "tool-result" && part.tool_call_id) {
						consider(part.tool_call_id, part.mcp_app);
					}
				}
			}
		};
		// Messages newer than the mount baseline can already be in the
		// store when the effect runs.
		scanMessages(lastMessages);
		return store.subscribe(() => {
			const state = store.getSnapshot();
			for (const [toolCallId, result] of Object.entries(
				state.streamState?.toolResults ?? {},
			)) {
				consider(toolCallId, result.mcpApp);
			}
			if (state.messagesByID !== lastMessages) {
				lastMessages = state.messagesByID;
				scanMessages(lastMessages);
			}
		});
	}, [store, enabled, newestMessageIdAtMount]);
}
