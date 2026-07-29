import { toTimelineBlocks } from "./blockUtils";
import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, StreamState } from "./types";

const hasTextOrThinkingBlock = (streamState: StreamState | null): boolean =>
	streamState?.blocks.some(
		(block) => block.type === "response" || block.type === "thinking",
	) ?? false;

// A rendered tool row shows its own progress, so the generic shimmer would
// double up. An unresolved block renders the waiting placeholder, which is a
// running row like any other, while a suppressed block renders nothing at all.
const hasRunningToolBlock = (
	streamState: StreamState | null,
	streamTools: readonly MergedTool[],
): boolean =>
	toTimelineBlocks(streamState?.blocks ?? [], streamTools).some((block) => {
		switch (block.type) {
			case "unresolved-tool":
				return true;
			case "tool":
				return block.tool.status === "running";
			case "read-files":
				return block.tools.some((tool) => tool.status === "running");
			default:
				return false;
		}
	});

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
		!hasRunningToolBlock(streamState, streamTools));
