import { asString } from "../ChatElements/runtimeTypeUtils";
import {
	isExecutePendingCommand,
	shouldRenderTool,
} from "../ChatElements/tools/toolVisibility";
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
	| (Exclude<RenderBlock, { type: "tool" }> & { sourceIndex: number })
	| { type: "tool"; tool: MergedTool }
	| { type: "unresolved-tool"; id: string }
	| { type: "suppressed-tool"; id: string }
	| { type: "read-files"; tools: readonly [MergedTool, ...MergedTool[]] };

/**
 * Pairs each tool block with the next unconsumed tool and collapses adjacent
 * read_file tools into one `read-files` block, including a run of one, so the
 * renderer switches on shape instead of looking tools up.
 *
 * Non-tool blocks carry their index in `blocks`, which never moves, so
 * collapsing a read-file run cannot renumber the keys of later blocks.
 */
export const toTimelineBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): TimelineBlock[] => {
	const timeline: TimelineBlock[] = [];
	let readFileRun: [MergedTool, ...MergedTool[]] | undefined;
	// Merged read-file messages can carry two tools with the same id, so tools
	// are consumed in order rather than looked up by id.
	let cursor = 0;

	const flushReadFileRun = () => {
		if (!readFileRun) {
			return;
		}
		timeline.push({ type: "read-files", tools: readFileRun });
		readFileRun = undefined;
	};

	for (const [sourceIndex, block] of blocks.entries()) {
		if (block.type !== "tool") {
			flushReadFileRun();
			timeline.push({ ...block, sourceIndex });
			continue;
		}

		const candidate = tools.at(cursor);
		const tool = candidate?.id === block.id ? candidate : undefined;
		if (tool) {
			cursor++;
		}

		if (!tool || isExecutePendingCommand(tool)) {
			flushReadFileRun();
			timeline.push({ type: "unresolved-tool", id: block.id });
			continue;
		}
		if (!shouldRenderTool(tool)) {
			flushReadFileRun();
			timeline.push({ type: "suppressed-tool", id: block.id });
			continue;
		}
		if (tool.name === "read_file") {
			if (readFileRun) {
				readFileRun.push(tool);
			} else {
				readFileRun = [tool];
			}
			continue;
		}

		flushReadFileRun();
		timeline.push({ type: "tool", tool });
	}

	flushReadFileRun();
	return timeline;
};
