import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, StreamState } from "./types";

const hasTextOrThinkingBlock = (streamState: StreamState | null): boolean =>
	streamState?.blocks.some(
		(block) => block.type === "response" || block.type === "thinking",
	) ?? false;

// A tool block shows its own progress, so the generic shimmer would double up.
// A block whose tool has not arrived renders the waiting placeholder, which is
// a running row like any other.
const hasRunningToolBlock = (
	streamState: StreamState | null,
	streamTools: readonly MergedTool[],
): boolean =>
	streamState?.blocks.some((block) => {
		if (block.type !== "tool") {
			return false;
		}
		const tool = streamTools.find((candidate) => candidate.id === block.id);
		return !tool || tool.status === "running";
	}) ?? false;

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
