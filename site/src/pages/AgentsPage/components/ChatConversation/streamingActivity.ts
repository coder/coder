import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, StreamState } from "./types";

const hasTextOrThinkingBlock = (streamState: StreamState | null): boolean =>
	streamState?.blocks.some(
		(block) => block.type === "response" || block.type === "thinking",
	) ?? false;

const hasRunningTool = (streamTools: readonly MergedTool[]): boolean =>
	streamTools.some((tool) => tool.status === "running");

export const shouldShowGenericThinking = ({
	liveStatus,
	streamState,
	streamTools,
}: {
	liveStatus: LiveStatusModel;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
}): boolean =>
	liveStatus.phase === "starting" ||
	(liveStatus.phase === "streaming" &&
		!hasTextOrThinkingBlock(streamState) &&
		!hasRunningTool(streamTools));
