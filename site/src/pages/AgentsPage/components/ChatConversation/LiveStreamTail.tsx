import { ChatStatusCallout } from "./ChatStatusCallout";
import type { LiveStatusModel } from "./liveStatusModel";

interface LiveStreamTailContentProps {
	isTranscriptEmpty: boolean;
	liveStatus: LiveStatusModel;
}

// The live assistant turn renders as a timeline row, so the tail below the
// transcript only carries the empty state and the terminal failure callout.
export const LiveStreamTailContent = ({
	isTranscriptEmpty,
	liveStatus,
}: LiveStreamTailContentProps) => {
	const terminalStatus = liveStatus.phase === "failed" ? liveStatus : null;
	const shouldRenderEmptyState =
		isTranscriptEmpty && liveStatus.phase === "idle";

	if (!shouldRenderEmptyState && !terminalStatus) {
		return null;
	}

	return (
		<div
			className={
				isTranscriptEmpty
					? "flex flex-col gap-2"
					: "mt-2 flex flex-col gap-2 empty:mt-0"
			}
		>
			{shouldRenderEmptyState && (
				<div className="py-12 text-center text-content-secondary">
					<p className="text-sm">Start a conversation with your agent.</p>
				</div>
			)}
			{terminalStatus && <ChatStatusCallout status={terminalStatus} />}
		</div>
	);
};
