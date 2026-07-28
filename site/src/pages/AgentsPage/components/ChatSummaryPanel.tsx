import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so the chat and cost queries don't run while the tab is hidden. */
	isVisible: boolean;
};

export const ChatSummaryPanel: FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
}) => {
	const { experiments } = useDashboard();
	// Cost is derived from AI Gateway interception data, so it is unavailable
	// when the gateway is off or unlicensed.
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const showCost =
		Boolean(useFeatureVisibility().aibridge) &&
		experiments.includes("ai-gateway-cost-control");
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });

	const chatData = chatQuery.data;
	// Cost covers the whole chat tree, so every chat in a tree shares one
	// cache entry keyed by the root. Waiting for the chat keeps a subagent
	// from caching the tree total under its own id.
	const rootChatId = chatData?.root_chat_id ?? chatId;
	const costQuery = useQuery({
		...chatCost(rootChatId),
		enabled: isVisible && showCost && chatData !== undefined,
	});

	let content: ReactNode = null;
	if (chatQuery.isError) {
		content = <ErrorAlert error={chatQuery.error} />;
	} else if (chatData) {
		content = (
			<ChatSummary
				summary={chatData.summary}
				isSubagent={Boolean(chatData.parent_chat_id)}
				createdAt={chatData.created_at}
				updatedAt={chatData.updated_at}
				costMicros={costQuery.data?.total_cost_micros}
				unpricedRequestCount={costQuery.data?.unpriced_request_count}
				showCost={showCost}
				isCostLoading={costQuery.isLoading}
				costError={costQuery.isError}
			/>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col overflow-y-auto p-4">
			{content}
		</div>
	);
};
