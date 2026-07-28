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
	| { type: "tool-group"; tools: readonly [MergedTool, ...MergedTool[]] };

/**
 * Blocks carry their tool from here on, so the timeline never resolves an id
 * that might be missing. A tool-result part can create a block before its call
 * arrives: while streaming that renders as a pending tool, and once the stream
 * settles the block is dropped.
 */
export const groupSequentialReadFileBlocks = (
	blocks: readonly RenderBlock[],
	tools: readonly MergedTool[],
	isStreaming = false,
): TimelineBlock[] => {
	const toolByID = new Map(tools.map((tool) => [tool.id, tool]));
	const grouped: TimelineBlock[] = [];
	let readFileRun: MergedTool[] = [];

	const flushReadFileRun = () => {
		const [first, ...rest] = readFileRun;
		if (!first) {
			return;
		}
		grouped.push(
			rest.length === 0
				? { type: "tool", tool: first }
				: { type: "tool-group", tools: [first, ...rest] },
		);
		readFileRun = [];
	};

	for (const block of blocks) {
		if (block.type !== "tool") {
			flushReadFileRun();
			grouped.push(block);
			continue;
		}

		const tool = toolByID.get(block.id);
		if (!tool) {
			flushReadFileRun();
			if (isStreaming) {
				grouped.push({
					type: "tool",
					tool: {
						id: block.id,
						name: "Tool",
						status: "running",
						isError: false,
					},
				});
			}
			continue;
		}
		if (tool.name === "read_file") {
			readFileRun.push(tool);
			continue;
		}

		flushReadFileRun();
		grouped.push({ type: "tool", tool });
	}

	flushReadFileRun();
	return grouped;
};
