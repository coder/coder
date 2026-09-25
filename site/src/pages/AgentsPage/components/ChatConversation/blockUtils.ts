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
 * Whitespace-only text extends a matching block but never starts one:
 * models stream paragraph breaks as separate "\n\n" deltas, and
 * dropping them joins the paragraphs. Returns `blocks` unchanged when
 * nothing is appended.
 */
export const appendTextBlock = (
	blocks: RenderBlock[],
	type: "response" | "thinking",
	text: string,
	narration = false,
): RenderBlock[] => {
	const last = blocks[blocks.length - 1];
	const keepsApart =
		last?.type === "response" &&
		(last.beforeProviderTool || (last.narration ?? false) !== narration);
	if (last && last.type === type && !keepsApart) {
		if (!text) {
			return blocks;
		}
		const nextBlocks = [...blocks];
		nextBlocks[nextBlocks.length - 1] = textBlock(
			type,
			`${last.text}${text}`,
			narration,
		);
		return nextBlocks;
	}
	if (!text.trim()) {
		return blocks;
	}
	return [...blocks, textBlock(type, text, narration)];
};

/**
 * Moves each block of answer citations after the response text that follows
 * it, merging the text around it when appendTextBlock would. Citations arrive
 * while their text streams: persisted messages store them before that text,
 * and a live stream splits the text around them.
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
			const merges =
				previous?.type === "response" &&
				!previous.beforeProviderTool &&
				(previous.narration ?? false) === (block.narration ?? false);
			if (merges) {
				placed[placed.length - 1] = {
					...block,
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
