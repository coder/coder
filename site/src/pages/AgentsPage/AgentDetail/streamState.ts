import { asString } from "components/ai-elements/runtimeTypeUtils";
import { appendTextBlock } from "./blockUtils";
import {
	asOptionalTitle,
	ensureToolBlock,
	normalizeBlockType,
	parseToolResultIsError,
} from "./messageParsing";
import { mergeStreamPayload } from "./streamingJson";
import type { MergedTool, RenderBlock, StreamState } from "./types";

let nextFallbackID = 0;

export const createEmptyStreamState = (): StreamState => ({
	blocks: [],
	toolCalls: {},
	toolResults: {},
	sources: [],
});

/** Streaming variant — uses direct concatenation (the default joinText). */
const appendStreamTextBlock = appendTextBlock;

export const applyMessagePartToStreamState = (
	prev: StreamState | null,
	part: Record<string, unknown>,
): StreamState | null => {
	const partType = normalizeBlockType(part.type);
	const nextState: StreamState = prev ?? createEmptyStreamState();

	switch (partType) {
		case "text": {
			const text = asString(part.text);
			if (!text) {
				return prev;
			}
			return {
				...nextState,
				blocks: appendStreamTextBlock(nextState.blocks, "response", text),
			};
		}
		case "reasoning":
		case "thinking": {
			const text = asString(part.text);
			if (!text) {
				return prev;
			}
			const title = asOptionalTitle(part.title);
			return {
				...nextState,
				blocks: appendStreamTextBlock(
					nextState.blocks,
					"thinking",
					text,
					title,
				),
			};
		}
		case "tool-call":
		case "toolcall": {
			// Provider-executed tool calls (e.g. web_search) are
			// handled natively by the provider — skip rendering them
			// as tool cards.
			if (part.provider_executed) {
				return prev;
			}
			const toolName = asString(part.tool_name);
			const existingByName = Object.values(nextState.toolCalls).find(
				(call) => call.name === toolName,
			);
			const toolCallID =
				asString(part.tool_call_id) ||
				(existingByName && !existingByName.args ? existingByName.id : null) ||
				`tool-call-${Object.keys(nextState.toolCalls).length + 1}-${++nextFallbackID}`;
			const existing = nextState.toolCalls[toolCallID];
			const nextArgs = mergeStreamPayload(
				existing?.args,
				existing?.argsRaw,
				part.args,
				part.args_delta,
			);

			return {
				...nextState,
				blocks: ensureToolBlock(nextState.blocks, toolCallID),
				toolCalls: {
					...nextState.toolCalls,
					[toolCallID]: {
						id: toolCallID,
						name: toolName || existing?.name || "Tool",
						args: nextArgs.value,
						argsRaw: nextArgs.rawText,
					},
				},
			};
		}
		case "tool-result":
		case "toolresult": {
			// Skip synthetic results for provider-executed tools.
			if (part.provider_executed) {
				return prev;
			}
			const toolName = asString(part.tool_name);
			const existingByName = Object.values(nextState.toolResults).find(
				(result) => result.name === toolName,
			);
			const existingCallByName = Object.values(nextState.toolCalls).find(
				(call) => call.name === toolName,
			);
			const toolCallID =
				asString(part.tool_call_id) ||
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
				part.result_delta,
			);
			const nextToolName = toolName || existing?.name || "Tool";
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
					},
				},
			};
		}
		case "file":
			if (!part.media_type || (!part.data && !part.file_id)) {
				return prev;
			}
			return {
				...nextState,
				blocks: [
					...nextState.blocks,
					part as Extract<RenderBlock, { type: "file" }>,
				],
			};
		case "source": {
			const url = asString(part.url);
			const title = asString(part.title);
			if (!url) {
				return prev;
			}
			const source = { url, title: title || url };
			// Still populate the flat list for backward compat.
			if (nextState.sources.some((s) => s.url === url)) {
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
		default:
			return prev;
	}
};

export const buildStreamTools = (
	streamState: StreamState | null,
): MergedTool[] => {
	if (!streamState) {
		return [];
	}
	const calls = Object.values(streamState.toolCalls);
	const seen = new Set<string>();
	const merged: MergedTool[] = [];

	for (const call of calls) {
		seen.add(call.id);
		const result = streamState.toolResults[call.id];
		merged.push({
			id: call.id,
			name: call.name,
			args: call.args,
			result: result?.result,
			isError: result?.isError ?? false,
			status: result ? (result.isError ? "error" : "completed") : "running",
		});
	}

	for (const result of Object.values(streamState.toolResults)) {
		if (!seen.has(result.id)) {
			merged.push({
				id: result.id,
				name: result.name,
				result: result.result,
				isError: result.isError,
				status: result.isError ? "error" : "completed",
			});
		}
	}

	return merged;
};
