import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, RenderBlock } from "./types";

/**
 * The last block that shows where output stands. A streamed citation row stays
 * after the text it cites while that text keeps growing, so it is skipped.
 */
export const lastProgressBlock = (
	blocks: readonly RenderBlock[],
): RenderBlock | undefined =>
	blocks.findLast((block) => block.type !== "sources");

/**
 * Whether the live output needs the generic Thinking indicator because none of
 * its blocks shows that the agent is still working. Running tools and the
 * trailing reasoning block (BlockList renders it as running) animate on their
 * own. Response text shows progress only while it keeps arriving. Every other
 * block is static.
 */
export const shouldShowGenericThinking = ({
	liveStatus,
	blocks,
	tools,
}: {
	liveStatus: LiveStatusModel;
	blocks: readonly RenderBlock[];
	tools: readonly MergedTool[];
}): boolean => {
	if (liveStatus.phase === "starting") {
		return true;
	}
	if (liveStatus.phase !== "streaming") {
		return false;
	}
	if (tools.some((tool) => tool.status === "running")) {
		return false;
	}
	switch (lastProgressBlock(blocks)?.type) {
		case "thinking":
			return false;
		case "response":
			return !liveStatus.hasRecentOutput;
		default:
			return true;
	}
};
