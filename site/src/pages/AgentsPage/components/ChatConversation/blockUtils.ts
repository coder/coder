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

type TimelineBlock =
	| Exclude<RenderBlock, { type: "tool" }>
	| { type: "tool"; tool: MergedTool }
	| { type: "pending-tool"; id: string }
	| { type: "read-files"; tools: readonly [MergedTool, ...MergedTool[]] };

/**
 * Resolves each tool block's id against `tools` and collapses runs of
 * consecutive read_file tools into one `read-files` block, so the renderer
 * switches on shape instead of looking tools up. A block whose call has not
 * arrived becomes `pending-tool`.
 */
export const toTimelineBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): TimelineBlock[] => {
	const toolByID = new Map(tools.map((tool) => [tool.id, tool]));
	const timeline: TimelineBlock[] = [];
	let readFileRun: MergedTool[] = [];

	const flushReadFileRun = () => {
		const [first, ...rest] = readFileRun;
		if (!first) {
			return;
		}
		timeline.push({ type: "read-files", tools: [first, ...rest] });
		readFileRun = [];
	};

	for (const block of blocks) {
		if (block.type !== "tool") {
			flushReadFileRun();
			timeline.push(block);
			continue;
		}

		const tool = toolByID.get(block.id);
		if (!tool) {
			flushReadFileRun();
			timeline.push({ type: "pending-tool", id: block.id });
			continue;
		}
		if (tool.name === "read_file") {
			readFileRun.push(tool);
			continue;
		}

		flushReadFileRun();
		timeline.push({ type: "tool", tool });
	}

	flushReadFileRun();
	return timeline;
};
