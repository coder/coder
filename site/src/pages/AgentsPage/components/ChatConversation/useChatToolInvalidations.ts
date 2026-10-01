import { useEffect, useEffectEvent, useRef } from "react";
import { useQueryClient } from "react-query";
import {
	invalidateAutomationChats,
	invalidateChatAutomations,
} from "#/api/queries/chatAutomations";
import {
	invalidateChatEntity,
	invalidateChatListQueries,
	invalidateChatsByWorkspace,
} from "#/api/queries/chats";
import { invalidateWorkspaceMutationQueries } from "#/api/queries/workspaces";
import { asString } from "../ChatElements/runtimeTypeUtils";
import { parseArgs } from "../ChatElements/tools/utils";
import { type ChatStore, useChatSelector } from "./chatStore";
import type { StreamState } from "./types";

type ChatToolResult = Pick<
	StreamState["toolResults"][string],
	"id" | "name" | "isStreaming" | "isError"
>;

// Only extract the toolResults record from the stream state.
// This reference is stable during pure text/thinking streaming
// and only changes when a tool result actually appears, avoiding
// a re-render of AgentChatPage on every token.
const selectStreamToolResults = (state: {
	streamState: StreamState | null;
}): Record<string, ChatToolResult> | null =>
	state.streamState?.toolResults ?? null;

type UseChatToolInvalidationsOptions = {
	store: ChatStore;
	chatID: string | undefined;
	organizationName: string;
	username: string;
};

const CHAT_WORKSPACE_BINDING_TOOL_NAMES = new Set(["create_workspace"]);
const WORKSPACE_MUTATION_TOOL_NAMES = new Set([
	"create_workspace",
	"start_workspace",
	"stop_workspace",
]);
const MANAGE_AUTOMATIONS_WRITE_ACTIONS = new Set([
	"create",
	"update",
	"enable",
	"disable",
	"delete",
	"run_now",
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
	// Not subscribed: a tool call's args land before its result, so the
	// snapshot is current whenever a new result triggers the effect. The
	// server persists the assistant tool-call message before its tools run
	// and the live stream resets, so a streamed result usually has only the
	// durable call.
	const readToolCallArgs = useEffectEvent((toolCallID: string): unknown => {
		const { streamState, messagesByID, orderedMessageIDs } =
			store.getSnapshot();
		const liveCall = streamState?.toolCalls[toolCallID];
		if (liveCall) {
			return liveCall.args;
		}
		for (const messageID of orderedMessageIDs.toReversed()) {
			const message = messagesByID.get(messageID);
			if (message?.role !== "assistant") {
				continue;
			}
			const part = message.content?.find(
				(part) => part.type === "tool-call" && part.tool_call_id === toolCallID,
			);
			if (part?.type === "tool-call") {
				return part.args;
			}
		}
		return undefined;
	});

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
		let shouldInvalidateAutomations = false;
		let shouldInvalidateAutomationRuns = false;

		for (const toolResult of Object.values(toolResults)) {
			if (
				toolResult.isStreaming ||
				processedToolCallIdsRef.current.has(toolResult.id)
			) {
				continue;
			}

			if (toolResult.name === "manage_automations") {
				const action = asString(
					parseArgs(readToolCallArgs(toolResult.id))?.action,
				);
				if (
					toolResult.isError ||
					!MANAGE_AUTOMATIONS_WRITE_ACTIONS.has(action)
				) {
					continue;
				}
				processedToolCallIdsRef.current.add(toolResult.id);
				shouldInvalidateAutomations = true;
				if (action === "run_now") {
					shouldInvalidateAutomationRuns = true;
				}
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
			void invalidateChatEntity(queryClient, chatID);
			void invalidateChatsByWorkspace(queryClient);
		}

		if (shouldInvalidateWorkspace) {
			void invalidateWorkspaceMutationQueries(queryClient, {
				organizationName,
				username,
			});
		}

		// Tool results carry no organization ID, so every organization's
		// automations refetch.
		if (shouldInvalidateAutomations) {
			void invalidateChatAutomations(queryClient);
		}

		// A run can start a new chat, which the chat lists must show.
		if (shouldInvalidateAutomationRuns) {
			void invalidateAutomationChats(queryClient);
			void invalidateChatListQueries(queryClient);
		}
	}, [chatID, organizationName, queryClient, toolResults, username]);
}
