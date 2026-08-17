import { MessageScroller } from "@shadcn/react/message-scroller";
import { ChatStatusCallout } from "./ChatStatusCallout";
import type { LiveStatusModel } from "./liveStatusModel";

interface LiveStreamTailContentProps {
	isTranscriptEmpty: boolean;
	liveStatus: LiveStatusModel;
}

// The live assistant turn renders as a timeline row, so the tail below the
// transcript only carries the empty state.
export const LiveStreamTailContent = ({
	isTranscriptEmpty,
	liveStatus,
}: LiveStreamTailContentProps) => {
	if (!isTranscriptEmpty || liveStatus.phase !== "idle") {
		return null;
	}

	return (
		<div className="py-12 text-center text-content-secondary">
			<p className="text-sm">Start a conversation with your agent.</p>
		</div>
	);
};

// Terminal failures render as a transcript row so a long error scrolls with
// the conversation instead of squeezing the composer out of the panel.
export const TerminalStatusRow = ({
	liveStatus,
}: {
	liveStatus: LiveStatusModel;
}) => {
	if (liveStatus.phase !== "failed") {
		return null;
	}
	return (
		<MessageScroller.Item messageId="terminal-status">
			<ChatStatusCallout status={liveStatus} />
		</MessageScroller.Item>
	);
};
