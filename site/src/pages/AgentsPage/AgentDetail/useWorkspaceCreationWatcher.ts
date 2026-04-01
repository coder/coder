import { chatKey } from "api/queries/chats";
import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "react-query";
import { useChatSelector } from "./ChatContext";
import type { StreamState } from "./types";

type ChatStoreHandle = Parameters<typeof useChatSelector>[0];

const selectStreamState = (state: { streamState: StreamState | null }) =>
	state.streamState;

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
	const streamState = useChatSelector(store, selectStreamState);
	const processedToolCallIdsRef = useRef<Set<string>>(new Set());

	// Reset processed IDs when chatID changes during render,
	// before effects run.
	const [previousChatID, setPreviousChatID] = useState(chatID);
	if (previousChatID !== chatID) {
		setPreviousChatID(chatID);
		processedToolCallIdsRef.current = new Set();
	}

	// Watch stream tool results for create_workspace completions.
	useEffect(() => {
		if (!streamState || !chatID) {
			processedToolCallIdsRef.current.clear();
			return;
		}

		let shouldInvalidateChat = false;

		for (const toolResult of Object.values(streamState.toolResults)) {
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
	}, [chatID, streamState, queryClient]);
}
