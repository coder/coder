import { useEffect, useRef } from "react";
import { useQueryClient } from "react-query";
import { chatKey } from "#/api/queries/chats";
import { useChatSelector } from "./chatStore";
import type { StreamState } from "./types";

type ChatStoreHandle = Parameters<typeof useChatSelector>[0];

// Only extract the toolResults record from the stream state.
// This reference is stable during pure text/thinking streaming
// and only changes when a tool result actually appears, avoiding
// a re-render of AgentChatPage on every token.
const selectStreamToolResults = (state: {
	streamState: StreamState | null;
}): Record<string, { id: string; name: string }> | null =>
	state.streamState?.toolResults ?? null;

interface UseWorkspaceCreationWatcherOptions {
	store: ChatStoreHandle;
	chatID: string | undefined;
}

// Triggers chat query invalidation to resolve the workspace/agent.
const WORKSPACE_TOOL_NAMES = new Set(["create_workspace"]);

/**
 * Watches stream tool results for create_workspace completions and
 * invalidates the chat query so the sidebar can display workspace info.
 * The agent now handles all path discovery and scan triggering via
 * the PathStore — no frontend refresh needed.
 */
export function useWorkspaceCreationWatcher({
	store,
	chatID,
}: UseWorkspaceCreationWatcherOptions): void {
	const queryClient = useQueryClient();
	const toolResults = useChatSelector(store, selectStreamToolResults);
	const processedToolCallIdsRef = useRef<Set<string>>(new Set());
	const chatIDRef = useRef(chatID);

	// Watch stream tool results for create_workspace completions.
	useEffect(() => {
		// Reset processed IDs when chatID changes.
		if (chatIDRef.current !== chatID) {
			chatIDRef.current = chatID;
			processedToolCallIdsRef.current = new Set();
		}

		if (!toolResults || !chatID) {
			processedToolCallIdsRef.current.clear();
			return;
		}

		let shouldInvalidateChat = false;

		for (const toolResult of Object.values(toolResults)) {
			if (processedToolCallIdsRef.current.has(toolResult.id)) {
				continue;
			}

			if (WORKSPACE_TOOL_NAMES.has(toolResult.name)) {
				processedToolCallIdsRef.current.add(toolResult.id);
				shouldInvalidateChat = true;
			}
		}

		if (shouldInvalidateChat) {
			// Invalidate chatKey to trigger the workspace resolution
			// cascade: chat refetch → workspaceId → workspace query →
			// agent resolved → git watcher connects.
			void queryClient.invalidateQueries({
				queryKey: chatKey(chatID),
			});
		}
	}, [toolResults, queryClient, chatID]);
}
