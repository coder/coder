import { asString } from "../ChatElements/runtimeTypeUtils";
import type { MergedTool, RenderBlock } from "./types";

export const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

/**
 * Append a text or thinking block to a render block list, merging
 * with the previous block when the types match.
 */
export const appendTextBlock = (
	blocks: RenderBlock[],
	type: "response" | "thinking",
	text: string,
): RenderBlock[] => {
	if (!text.trim()) {
		return blocks;
	}
	const nextBlocks = [...blocks];
	const last = nextBlocks[nextBlocks.length - 1];
	if (last && last.type === type) {
		nextBlocks[nextBlocks.length - 1] = {
			type,
			text: `${last.text}${text}`,
		};
		return nextBlocks;
	}
	nextBlocks.push({ type, text });
	return nextBlocks;
};

/**
 * Moves each block of answer citations after the response text that follows
 * it, merging the text around it. Citations arrive while their text streams:
 * persisted messages store them before that text, and a live stream splits
 * the text around them.
 */
export const placeCitationsAfterText = (
	blocks: readonly RenderBlock[],
): RenderBlock[] => {
	const placed: RenderBlock[] = [];
	for (const block of blocks) {
		const last = placed[placed.length - 1];
		if (block.type === "response" && last?.type === "sources") {
			placed.pop();
			const previous = placed[placed.length - 1];
			if (previous?.type === "response") {
				placed[placed.length - 1] = {
					type: "response",
					text: `${previous.text}${block.text}`,
				};
			} else {
				placed.push(block);
			}
			placed.push(last);
			continue;
		}
		if (block.type === "sources" && last?.type === "sources") {
			const sources = [...last.sources];
			for (const source of block.sources) {
				if (!sources.some(({ url }) => url === source.url)) {
					sources.push(source);
				}
			}
			placed[placed.length - 1] = { type: "sources", sources };
			continue;
		}
		placed.push(block);
	}
	return placed;
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
