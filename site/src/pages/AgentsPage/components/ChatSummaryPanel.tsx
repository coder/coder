import type { FC, ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { chat, chatCost, refreshChatContext } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import { ChatSummary } from "./ChatSummary";
import {
	type AgentContextUsage,
	ChatSummaryResources,
} from "./ChatSummaryResources";

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so the chat and cost queries don't run while the tab is hidden. */
	isVisible: boolean;
	contextUsage: AgentContextUsage | null;
};

export const ChatSummaryPanel: FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
	contextUsage,
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const queryClient = useQueryClient();
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });

	const chatData = chatQuery.data;
	const rootChatId = getChatCostTreeID(chatData) ?? chatId;
	const costQuery = useQuery({
		...chatCost(rootChatId),
		enabled: isVisible && showCost && chatData !== undefined,
	});
	const refreshContextMutation = useMutation(
		refreshChatContext(queryClient, chatId),
	);

	let content: ReactNode = null;
	if (chatQuery.isError) {
		content = <ErrorAlert error={chatQuery.error} />;
	} else if (chatData) {
		content = (
			<>
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
				<ChatSummaryResources
					usage={{ ...contextUsage, context: chatData.context }}
					onRefreshContext={() =>
						refreshContextMutation.mutate(undefined, {
							onSuccess: () => toast.success("Context refreshed."),
							onError: () => toast.error("Failed to refresh context."),
						})
					}
					isRefreshingContext={refreshContextMutation.isPending}
				/>
			</>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col overflow-y-auto p-4 scrollbar-gutter-stable scrollbar-thin">
			{content}
		</div>
	);
};
