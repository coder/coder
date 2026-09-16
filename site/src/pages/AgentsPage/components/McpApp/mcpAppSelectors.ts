import type * as TypesGen from "#/api/typesGenerated";
import type { ChatStoreState } from "../ChatConversation/chatStore";
import {
	mergeTools,
	parseToolCallPart,
	parseToolResultPart,
} from "../ChatConversation/messageParsing";
import type {
	MergedTool,
	ParsedToolCall,
	ParsedToolResult,
	StreamState,
} from "../ChatConversation/types";

type DurableToolParts = {
	call?: TypesGen.ChatToolCallPart;
	result?: TypesGen.ChatToolResultPart;
};

const durableToolIndexCache = new WeakMap<
	Map<number, TypesGen.ChatMessage>,
	Map<string, DurableToolParts>
>();

/**
 * Indexes persisted tool-call and tool-result parts by tool call ID. Cached
 * per message map identity so selecting from it yields stable references
 * across store emissions that did not replace the messages.
 */
const durableToolIndex = (
	messagesByID: Map<number, TypesGen.ChatMessage>,
): Map<string, DurableToolParts> => {
	let index = durableToolIndexCache.get(messagesByID);
	if (index) {
		return index;
	}
	index = new Map();
	for (const message of messagesByID.values()) {
		for (const part of message.content ?? []) {
			if (part.type === "tool-call" && part.tool_call_id) {
				const entry = index.get(part.tool_call_id) ?? {};
				entry.call = part;
				index.set(part.tool_call_id, entry);
			} else if (part.type === "tool-result" && part.tool_call_id) {
				const entry = index.get(part.tool_call_id) ?? {};
				entry.result = part;
				index.set(part.tool_call_id, entry);
			}
		}
	}
	durableToolIndexCache.set(messagesByID, index);
	return index;
};

export const selectDurableToolParts =
	(toolCallId: string) =>
	(state: ChatStoreState): DurableToolParts | undefined =>
		durableToolIndex(state.messagesByID).get(toolCallId);

export const selectStreamToolCall =
	(toolCallId: string) =>
	(state: ChatStoreState): StreamState["toolCalls"][string] | undefined =>
		state.streamState?.toolCalls[toolCallId];

export const selectStreamToolResult =
	(toolCallId: string) =>
	(state: ChatStoreState): StreamState["toolResults"][string] | undefined =>
		state.streamState?.toolResults[toolCallId];

/**
 * Merges the persisted and live views of one tool call. The live stream wins
 * while it exists because it carries partially streamed args and results.
 */
export const buildBoundTool = ({
	toolCallId,
	durable,
	streamCall,
	streamResult,
	chatStatus,
}: {
	toolCallId: string;
	durable: DurableToolParts | undefined;
	streamCall: StreamState["toolCalls"][string] | undefined;
	streamResult: StreamState["toolResults"][string] | undefined;
	chatStatus: TypesGen.ChatStatus | null;
}): MergedTool | undefined => {
	const call: ParsedToolCall | undefined = streamCall
		? {
				id: streamCall.id,
				name: streamCall.name,
				args: streamCall.args,
				parsedCommands: streamCall.parsedCommands,
				mcpServerConfigId: streamCall.mcpServerConfigId,
				mcpAppResourceUri: streamCall.mcpAppResourceUri,
			}
		: durable?.call
			? parseToolCallPart(durable.call, toolCallId)
			: undefined;
	const result: ParsedToolResult | undefined =
		streamResult && !streamResult.isStreaming
			? {
					id: streamResult.id,
					name: streamResult.name,
					result: streamResult.result,
					isError: streamResult.isError,
					mcpServerConfigId: streamResult.mcpServerConfigId,
					mcpAppResourceUri: streamResult.mcpAppResourceUri,
					mcpResult: streamResult.mcpResult,
					mcpResultTruncated: streamResult.mcpResultTruncated,
				}
			: durable?.result
				? parseToolResultPart(durable.result, toolCallId)
				: undefined;
	if (!call && !result) {
		return undefined;
	}
	const isRunning = chatStatus === "running" || chatStatus === "interrupting";
	const [tool] = mergeTools(call ? [call] : [], result ? [result] : [], {
		pendingToolCallIDs: isRunning ? new Set([toolCallId]) : undefined,
	});
	return {
		...tool,
		...(streamCall?.argsRaw !== undefined && !result
			? { argsStreaming: true }
			: {}),
	};
};
