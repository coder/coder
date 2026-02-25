import { asString } from "components/ai-elements/runtimeTypeUtils";
import { appendTextBlock, mergeThinkingTitles } from "./blockUtils";
import {
	asOptionalTitle,
	ensureToolBlock,
	normalizeBlockType,
	parseToolResultIsError,
} from "./messageParsing";
import { mergeStreamPayload } from "./streamingJson";
import type { MergedTool, RenderBlock, StreamState } from "./types";

export const createEmptyStreamState = (): StreamState => ({
	blocks: [],
	toolCalls: {},
	toolResults: {},
});

/** Streaming variant — uses direct concatenation (the default joinText). */
const appendStreamTextBlock = appendTextBlock;

export const applyStreamThinkingTitle = (
	blocks: RenderBlock[],
	title?: string,
): RenderBlock[] => {
	if (!title) {
		return blocks;
	}
	const nextBlocks = [...blocks];
	const last = nextBlocks[nextBlocks.length - 1];
	if (last && last.type === "thinking") {
		const merged = mergeThinkingTitles(last.title, title);
		nextBlocks[nextBlocks.length - 1] = {
			type: "thinking",
			text: last.text,
			title: merged.title,
		};
		return nextBlocks;
	}
	nextBlocks.push({
		type: "thinking",
		text: "",
		title,
	});
	return nextBlocks;
};

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
			const title = asOptionalTitle(part.title);
			if (!text && !title) {
				return prev;
			}
			const nextBlocks = text
				? appendStreamTextBlock(nextState.blocks, "thinking", text, title)
				: applyStreamThinkingTitle(nextState.blocks, title);
			return {
				...nextState,
				blocks: nextBlocks,
			};
		}
		case "tool-call":
		case "toolcall": {
			const toolName = asString(part.tool_name);
			const existingByName = Object.values(nextState.toolCalls).find(
				(call) => call.name === toolName,
			);
			const toolCallID =
				asString(part.tool_call_id) ||
				existingByName?.id ||
				`tool-call-${Object.keys(nextState.toolCalls).length + 1}`;
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
			const toolName = asString(part.tool_name);
			const existingByName = Object.values(nextState.toolResults).find(
				(result) => result.name === toolName,
			);
			const existingCallByName = Object.values(nextState.toolCalls).find(
				(call) => call.name === toolName,
			);
			const toolCallID =
				asString(part.tool_call_id) ||
				existingByName?.id ||
				existingCallByName?.id ||
				`tool-result-${Object.keys(nextState.toolResults).length + 1}`;
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
