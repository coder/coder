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
			// Skip empty and whitespace-only deltas so they don't
			// create a non-null StreamState with empty blocks, which
			// would prematurely end the "starting" phase.
			if (!part.text?.trim()) {
				return prev;
			}
			return {
				...nextState,
				blocks: appendTextBlock(nextState.blocks, "response", part.text),
			};
		}
		case "reasoning": {
			if (!part.text?.trim()) {
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
						parsedCommands: part.parsed_commands ?? existing?.parsedCommands,
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
			if (part.result_reset) {
				const toolResults = { ...nextState.toolResults };
				delete toolResults[toolCallID];
				return {
					...nextState,
					blocks: ensureToolBlock(nextState.blocks, toolCallID),
					toolResults,
				};
			}
			if (
				part.result_delta === "" &&
				part.result === undefined &&
				!part.is_error
			) {
				return {
					...nextState,
					blocks: ensureToolBlock(nextState.blocks, toolCallID),
				};
			}

			const nextResult = mergeStreamPayload(
				existing?.result,
				existing?.resultRaw,
				part.result,
				part.result_delta,
			);
			const nextToolName = part.tool_name || existing?.name || "Tool";
			const isFinalResult = part.result !== undefined || part.is_error;
			const isStreaming = isFinalResult
				? false
				: existing?.isStreaming || Boolean(part.result_delta);
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
						isStreaming: isStreaming || undefined,
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

const getStreamToolStatus = (
	result: StreamState["toolResults"][string] | undefined,
): MergedTool["status"] => {
	if (!result) {
		return "running";
	}
	if (result.isStreaming) {
		return "running";
	}
	return result.isError ? "error" : "completed";
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
			status: getStreamToolStatus(result),
			mcpServerConfigId: call.mcpServerConfigId || result?.mcpServerConfigId,
			modelIntent: call.modelIntent,
			parsedCommands: call.parsedCommands,
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
					status: getStreamToolStatus(result),
					mcpServerConfigId: result.mcpServerConfigId,
				});
			}
		}
	}

	return merged;
};
