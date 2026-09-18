import { MessageScroller } from "@shadcn/react/message-scroller";
import type { FC } from "react";
import { useParams } from "react-router";
import { AgentChatPage } from "./AgentChatPage";

// Mounts two chats side by side in one React tree. This is the
// benchmark scenario for chat rendering performance: two long, streaming
// transcripts plus composer interaction on the same main thread.
const AgentChatComparePage: FC = () => {
	const { leftId, rightId } = useParams<{ leftId: string; rightId: string }>();
	if (!leftId || !rightId) {
		return null;
	}
	return (
		<div className="flex h-full min-h-0 w-full">
			{[leftId, rightId].map((chatId) => (
				<div
					key={chatId}
					className="flex min-w-0 flex-1 flex-col border-border border-r last:border-r-0"
					data-testid={`compare-pane-${chatId}`}
				>
					<MessageScroller.Provider autoScroll defaultScrollPosition="end">
						<AgentChatPage chatId={chatId} />
					</MessageScroller.Provider>
				</div>
			))}
		</div>
	);
};

export default AgentChatComparePage;
