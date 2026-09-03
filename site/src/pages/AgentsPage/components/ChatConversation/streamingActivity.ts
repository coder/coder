import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, RenderBlock } from "./types";

const hasTextOrThinkingBlock = (blocks: readonly RenderBlock[]): boolean =>
	blocks.some(
		(block) => block.type === "response" || block.type === "thinking",
	);

const hasRunningTool = (tools: readonly MergedTool[]): boolean =>
	tools.some((tool) => tool.status === "running");

export const shouldShowGenericThinking = ({
	liveStatus,
	blocks,
	tools,
}: {
	liveStatus: LiveStatusModel;
	blocks: readonly RenderBlock[];
	tools: readonly MergedTool[];
}): boolean =>
	liveStatus.phase === "starting" ||
	(liveStatus.phase === "streaming" &&
		!hasTextOrThinkingBlock(blocks) &&
		!hasRunningTool(tools));
