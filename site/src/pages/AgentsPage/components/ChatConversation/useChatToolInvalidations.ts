import { useEffect, useRef } from "react";
import { useQueryClient } from "react-query";
import { chatKey } from "#/api/queries/chats";
import { invalidateWorkspaceMutationQueries } from "#/api/queries/workspaces";
import { type ChatStore, useChatSelector } from "./chatStore";
import type { StreamState } from "./types";

type ChatToolResult = Pick<
	StreamState["toolResults"][string],
	"id" | "name" | "isStreaming"
>;

// Only extract the toolResults record from the stream state.
// This reference is stable during pure text/thinking streaming
// and only changes when a tool result actually appears, avoiding
// a re-render of AgentChatPage on every token.
const selectStreamToolResults = (state: {
	streamState: StreamState | null;
}): Record<string, ChatToolResult> | null =>
	state.streamState?.toolResults ?? null;

interface UseChatToolInvalidationsOptions {
	store: ChatStore;
	chatID: string | undefined;
	organizationName: string;
	username: string;
}

const CHAT_WORKSPACE_BINDING_TOOL_NAMES = new Set(["create_workspace"]);
const WORKSPACE_MUTATION_TOOL_NAMES = new Set([
	"create_workspace",
	"start_workspace",
	"stop_workspace",
]);

/**
 * Watches completed chat tool results and invalidates derived UI data for the
 * server state those tools may have changed.
 */
export function useChatToolInvalidations({
	store,
	chatID,
	organizationName,
	username,
}: UseChatToolInvalidationsOptions): void {
	const queryClient = useQueryClient();
	const toolResults = useChatSelector(store, selectStreamToolResults);
	const processedToolCallIdsRef = useRef<Set<string>>(new Set());
	const chatIDRef = useRef(chatID);

	useEffect(() => {
		if (chatIDRef.current !== chatID) {
			chatIDRef.current = chatID;
			processedToolCallIdsRef.current.clear();
		}

		if (!toolResults || !chatID) {
			processedToolCallIdsRef.current.clear();
			return;
		}

		let shouldInvalidateChat = false;
		let shouldInvalidateWorkspace = false;

		for (const toolResult of Object.values(toolResults)) {
			if (
				toolResult.isStreaming ||
				processedToolCallIdsRef.current.has(toolResult.id)
			) {
				continue;
			}

			const changesChatWorkspaceBinding = CHAT_WORKSPACE_BINDING_TOOL_NAMES.has(
				toolResult.name,
			);
			const changesWorkspace = WORKSPACE_MUTATION_TOOL_NAMES.has(
				toolResult.name,
			);
			if (!changesChatWorkspaceBinding && !changesWorkspace) {
				continue;
			}

			processedToolCallIdsRef.current.add(toolResult.id);
			shouldInvalidateChat =
				shouldInvalidateChat || changesChatWorkspaceBinding;
			shouldInvalidateWorkspace = shouldInvalidateWorkspace || changesWorkspace;
		}

		if (shouldInvalidateChat) {
			void queryClient.invalidateQueries({
				queryKey: chatKey(chatID),
			});
		}

		if (shouldInvalidateWorkspace) {
			void invalidateWorkspaceMutationQueries(queryClient, {
				organizationName,
				username,
			});
		}
	}, [chatID, organizationName, queryClient, toolResults, username]);
}
