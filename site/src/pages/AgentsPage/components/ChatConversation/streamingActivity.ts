import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, RenderBlock } from "./types";

/**
 * Whether the live turn needs the generic Thinking indicator because nothing
 * else on screen shows that the agent is still working. Running tools and the
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
	switch (blocks.at(-1)?.type) {
		case "thinking":
			return false;
		case "response":
			return !liveStatus.hasRecentOutput;
		default:
			return true;
	}
};
