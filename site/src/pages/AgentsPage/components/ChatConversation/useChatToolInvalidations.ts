import { useEffect, useMemo, useRef } from "react";
import { useQueryClient } from "react-query";
import { invalidateCachedChat } from "#/api/queries/chats";
import { invalidateWorkspaceMutationQueries } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { type ChatStreamStore, useChatSelector } from "./chatStreamStore";
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
	store: ChatStreamStore;
	messages: readonly TypesGen.ChatMessage[];
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
	messages,
	chatID,
	organizationName,
	username,
}: UseChatToolInvalidationsOptions): void {
	const queryClient = useQueryClient();
	const streamToolResults = useChatSelector(store, selectStreamToolResults);
	const toolResults = useMemo(() => {
		const byID = new Map<string, ChatToolResult>();
		for (const result of Object.values(streamToolResults ?? {})) {
			byID.set(result.id, result);
		}
		for (const message of messages) {
			for (const part of message.content ?? []) {
				if (
					part.type !== "tool-result" ||
					part.provider_executed ||
					!part.tool_call_id ||
					!part.tool_name
				) {
					continue;
				}
				byID.set(part.tool_call_id, {
					id: part.tool_call_id,
					name: part.tool_name,
					isStreaming: false,
				});
			}
		}
		return Array.from(byID.values());
	}, [messages, streamToolResults]);
	const processedToolCallIdsRef = useRef<Set<string>>(new Set());
	const chatIDRef = useRef(chatID);

	useEffect(() => {
		if (chatIDRef.current !== chatID) {
			chatIDRef.current = chatID;
			processedToolCallIdsRef.current.clear();
		}

		if (!chatID) {
			processedToolCallIdsRef.current.clear();
			return;
		}

		let shouldInvalidateChat = false;
		let shouldInvalidateWorkspace = false;

		for (const toolResult of toolResults) {
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
			void invalidateCachedChat(queryClient, chatID);
		}

		if (shouldInvalidateWorkspace) {
			void invalidateWorkspaceMutationQueries(queryClient, {
				organizationName,
				username,
			});
		}
	}, [chatID, organizationName, queryClient, toolResults, username]);
}
