import type * as TypesGen from "#/api/typesGenerated";
import { appendTextBlock } from "./blockUtils";
import { ensureToolBlock, parseToolResultIsError } from "./messageParsing";
import { mergeStreamPayload } from "./streamingJson";
import type { MergedTool, RenderBlock, StreamState } from "./types";

let nextFallbackID = 0;

export const createEmptyStreamState = (): StreamState => ({
	blocks: [],
	toolCalls: {},
	toolResults: {},
	sources: [],
});

export const applyMessagePartToStreamState = (
	prev: StreamState | null,
	part: TypesGen.ChatMessagePart,
): StreamState | null => {
	const nextState: StreamState = prev ?? createEmptyStreamState();

	switch (part.type) {
		case "text": {
			if (!part.text) {
				return prev;
			}
			return {
				...nextState,
				blocks: appendTextBlock(nextState.blocks, "response", part.text),
			};
		}
		case "reasoning": {
			if (!part.text) {
				return prev;
			}
			return {
				...nextState,
				blocks: appendTextBlock(nextState.blocks, "thinking", part.text),
			};
		}
		case "tool-call": {
			// Provider-executed tool calls (e.g. web_search) are
			// handled natively by the provider — skip rendering them
			// as tool cards.
			if (part.provider_executed) {
				return prev;
			}
			const existingByName = Object.values(nextState.toolCalls).find(
				(call) => call.name === part.tool_name,
			);
			const toolCallID =
				part.tool_call_id ||
				(existingByName && !existingByName.args ? existingByName.id : null) ||
				`tool-call-${Object.keys(nextState.toolCalls).length + 1}-${++nextFallbackID}`;
			const existing = nextState.toolCalls[toolCallID];
			const nextArgs = mergeStreamPayload(
				existing?.args,
				existing?.argsRaw,
				part.args,
				part.args_delta,
			);

			// Extract model_intent from the incrementally parsed args.
			const merged = nextArgs.value as Record<string, unknown> | undefined;
			const modelIntent =
				typeof merged?.model_intent === "string"
					? merged.model_intent
					: existing?.modelIntent;

			return {
				...nextState,
				blocks: ensureToolBlock(nextState.blocks, toolCallID),
				toolCalls: {
					...nextState.toolCalls,
					[toolCallID]: {
						id: toolCallID,
						name: part.tool_name || existing?.name || "Tool",
						args: nextArgs.value,
						argsRaw: nextArgs.rawText,
						mcpServerConfigId:
							part.mcp_server_config_id || existing?.mcpServerConfigId,
						modelIntent,
					},
				},
			};
		}
		case "tool-result": {
			// Skip synthetic results for provider-executed tools.
			if (part.provider_executed) {
				return prev;
			}
			const existingByName = Object.values(nextState.toolResults).find(
				(result) => result.name === part.tool_name,
			);
			const existingCallByName = Object.values(nextState.toolCalls).find(
				(call) => call.name === part.tool_name,
			);
			const toolCallID =
				part.tool_call_id ||
				(existingByName && !existingByName.result ? existingByName.id : null) ||
				(existingCallByName && !nextState.toolResults[existingCallByName.id]
					? existingCallByName.id
					: null) ||
				`tool-result-${Object.keys(nextState.toolResults).length + 1}-${++nextFallbackID}`;
			const existing = nextState.toolResults[toolCallID];
			const nextResult = mergeStreamPayload(
				existing?.result,
				existing?.resultRaw,
				part.result,
				undefined, // no delta: tool results arrive complete, not streamed incrementally
			);
			const nextToolName = part.tool_name || existing?.name || "Tool";
			const nextIsError =
				existing?.isError ||
				parseToolResultIsError(nextToolName, part, nextResult.value);

			return {
				...nextState,
				blocks: ensureToolBlock(nextState.blocks, toolCallID),
				toolResults: {
					...nextState.toolResults,
					[toolCallID]: {
						id: toolCallID,
						name: nextToolName,
						result: nextResult.value,
						resultRaw: nextResult.rawText,
						isError: nextIsError,
						mcpServerConfigId:
							part.mcp_server_config_id || existing?.mcpServerConfigId,
					},
				},
			};
		}
		case "file": {
			if (!part.data && !part.file_id) {
				return prev;
			}
			return {
				...nextState,
				blocks: [...nextState.blocks, part],
			};
		}
		case "source": {
			if (!part.url) {
				return prev;
			}
			const source = { url: part.url, title: part.title || part.url };
			// Still populate the flat list for backward compat.
			if (nextState.sources.some((s) => s.url === part.url)) {
				return prev;
			}
			const newSources = [...nextState.sources, source];
			// Group consecutive sources into a single inline
			// block at the current position in the block list.
			const lastBlock = nextState.blocks[nextState.blocks.length - 1];
			let newBlocks: RenderBlock[];
			if (lastBlock && lastBlock.type === "sources") {
				// Append to existing sources block.
				newBlocks = [...nextState.blocks];
				newBlocks[newBlocks.length - 1] = {
					type: "sources",
					sources: [...lastBlock.sources, source],
				};
			} else {
				newBlocks = [
					...nextState.blocks,
					{ type: "sources", sources: [source] },
				];
			}
			return {
				...nextState,
				sources: newSources,
				blocks: newBlocks,
			};
		}
		// file-reference parts only appear in persisted messages
		// from user input, never via SSE streaming.
		case "file-reference":
		// context-file parts are metadata-only; no streaming
		// render needed.
		case "context-file":
		// skill parts are metadata-only; no streaming render
		// needed.
		case "skill":
			return prev;
		default: {
			const _exhaustive: never = part;
			return prev;
		}
	}
};

export const buildStreamTools = (
	toolCalls: StreamState["toolCalls"] | null | undefined,
	toolResults: StreamState["toolResults"] | null | undefined,
): MergedTool[] => {
	if (!toolCalls) {
		return [];
	}
	const calls = Object.values(toolCalls);
	const seen = new Set<string>();
	const merged: MergedTool[] = [];

	for (const call of calls) {
		seen.add(call.id);
		const result = toolResults?.[call.id];
		merged.push({
			id: call.id,
			name: call.name,
			args: call.args,
			result: result?.result,
			isError: result?.isError ?? false,
			status: result ? (result.isError ? "error" : "completed") : "running",
			mcpServerConfigId: call.mcpServerConfigId || result?.mcpServerConfigId,
			modelIntent: call.modelIntent,
		});
	}

	if (toolResults) {
		for (const result of Object.values(toolResults)) {
			if (!seen.has(result.id)) {
				merged.push({
					id: result.id,
					name: result.name,
					result: result.result,
					isError: result.isError,
					status: result.isError ? "error" : "completed",
					mcpServerConfigId: result.mcpServerConfigId,
				});
			}
		}
	}

	return merged;
};
