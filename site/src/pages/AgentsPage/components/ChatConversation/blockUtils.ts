import { asString } from "../ChatElements/runtimeTypeUtils";
import type { MergedTool, RenderBlock } from "./types";

export const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

const textBlock = (
	type: "response" | "thinking",
	text: string,
	narration: boolean,
): RenderBlock =>
	type === "response" && narration ? { type, text, narration } : { type, text };

/**
 * Append a text or thinking block to a render block list, merging
 * with the previous block when the types match. Text never merges
 * across a provider-executed call or between narration and an answer.
 */
export const appendTextBlock = (
	blocks: RenderBlock[],
	type: "response" | "thinking",
	text: string,
	narration = false,
): RenderBlock[] => {
	if (!text.trim()) {
		return blocks;
	}
	const nextBlocks = [...blocks];
	const last = nextBlocks[nextBlocks.length - 1];
	const keepsApart =
		last?.type === "response" &&
		(last.beforeProviderTool || (last.narration ?? false) !== narration);
	if (last && last.type === type && !keepsApart) {
		nextBlocks[nextBlocks.length - 1] = textBlock(
			type,
			`${last.text}${text}`,
			narration,
		);
		return nextBlocks;
	}
	nextBlocks.push(textBlock(type, text, narration));
	return nextBlocks;
};

type ToolGroupRenderBlock = {
	type: "tool-group";
	ids: string[];
};

type TimelineRenderBlock = RenderBlock | ToolGroupRenderBlock;

export const groupSequentialReadFileBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): TimelineRenderBlock[] => {
	const toolByID = new Map(tools.map((tool) => [tool.id, tool]));
	const grouped: TimelineRenderBlock[] = [];
	let currentReadFileIDs: string[] = [];

	const flushReadFileIDs = () => {
		if (currentReadFileIDs.length === 0) {
			return;
		}
		if (currentReadFileIDs.length === 1) {
			grouped.push({ type: "tool", id: currentReadFileIDs[0] });
		} else {
			grouped.push({
				type: "tool-group",
				ids: currentReadFileIDs,
			});
		}
		currentReadFileIDs = [];
	};

	for (const block of blocks) {
		if (block.type === "tool") {
			const tool = toolByID.get(block.id);
			if (tool?.name === "read_file") {
				currentReadFileIDs.push(block.id);
				continue;
			}
		}

		flushReadFileIDs();
		grouped.push(block);
	}

	flushReadFileIDs();
	return grouped;
};
