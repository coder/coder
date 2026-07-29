import { asString } from "../ChatElements/runtimeTypeUtils";
import { isToolPendingArgs } from "../ChatElements/tools/toolVisibility";
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
	| { type: "read-files"; tools: readonly [MergedTool, ...MergedTool[]] };

/**
 * Resolves each tool block's id against `tools` and collapses adjacent
 * read_file tools into one `read-files` block, including a run of one, so the
 * renderer switches on shape instead of looking tools up. A block becomes
 * `unresolved-tool` while its tool is still arriving: either no tool carries
 * the id yet, or the tool's arguments have not streamed far enough to render a
 * row. A tool that is deliberately hidden keeps its `tool` block, so `<Tool>`
 * stays the one place that decides to render nothing.
 *
 * Non-tool blocks carry their index in `blocks`, which never moves, so
 * collapsing a read-file run cannot renumber the keys of later blocks.
 */
export const toTimelineBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
): TimelineBlock[] => {
	// Merged read-file messages can carry two tools with the same id, so each
	// block takes the next one instead of all of them collapsing onto the last.
	const toolsByID = new Map<string, MergedTool[]>();
	for (const tool of tools) {
		const queued = toolsByID.get(tool.id);
		if (queued) {
			queued.push(tool);
		} else {
			toolsByID.set(tool.id, [tool]);
		}
	}
	const timeline: TimelineBlock[] = [];
	let readFileRun: [MergedTool, ...MergedTool[]] | undefined;

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

		const tool = toolsByID.get(block.id)?.shift();
		if (!tool || isToolPendingArgs(tool)) {
			flushReadFileRun();
			timeline.push({ type: "unresolved-tool", id: block.id });
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
