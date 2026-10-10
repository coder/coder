import { asString } from "../ChatElements/runtimeTypeUtils";
import type { MergedTool, RenderBlock } from "./types";

export const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

/**
 * Append a text or thinking block to a render block list, merging
 * with the previous block when the types match. Whitespace-only text
 * extends a matching block but never starts one: models stream
 * paragraph breaks as separate "\n\n" deltas, and dropping them joins
 * the paragraphs. Returns `blocks` unchanged when nothing is appended.
 */
export const appendTextBlock = (
	blocks: RenderBlock[],
	type: "response" | "thinking",
	text: string,
): RenderBlock[] => {
	const last = blocks[blocks.length - 1];
	if (last && last.type === type) {
		if (!text) {
			return blocks;
		}
		const nextBlocks = [...blocks];
		nextBlocks[nextBlocks.length - 1] = {
			type,
			text: `${last.text}${text}`,
		};
		return nextBlocks;
	}
	if (!text.trim()) {
		return blocks;
	}
	return [...blocks, { type, text }];
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
