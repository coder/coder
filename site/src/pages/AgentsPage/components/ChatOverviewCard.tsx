import { cn } from "cn";
import { ListIcon } from "lucide-react";
import { type UseQueryResult, useQuery } from "react-query";
import { useOutletContext } from "react-router";
import { chatCost } from "#/api/queries/chats";
import type {
	Chat,
	ChatCost,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { useChatAttachments } from "../hooks/useChatAttachments";
import { prNumber } from "../utils/pullRequest";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import {
	type ChatStore,
	selectSubagentStatusOverrides,
	useChatSelector,
} from "./ChatConversation/chatStore";
import {
	chatOverviewCardPositionClassName,
	chatOverviewHiddenWhenNarrowClassName,
} from "./ChatOverviewCardShell";
import {
	type ChatOverview,
	ChatOverviewCardView,
	type OverviewTarget,
} from "./ChatOverviewCardView";
import { countRepoChanges, repoLabel } from "./GitPanel/gitViews";

/** The card's stored open state and the handler that flips it. */
export type ChatOverviewToggleProps = {
	isOpen: boolean;
	/** The card's `id`, named in `aria-controls` while the card shows. */
	cardId: string;
	onToggle: () => void;
};

/** The top bar button that shows and hides the card. */
export const ChatOverviewToggle: React.FC<ChatOverviewToggleProps> = ({
	isOpen,
	cardId,
	onToggle,
}) => (
	<Tooltip>
		<TooltipTrigger asChild>
			<Button
				variant="subtle"
				size="icon"
				onClick={onToggle}
				className={cn(
					"size-7 text-content-secondary hover:text-content-primary",
					isOpen && "bg-surface-secondary text-content-primary",
					chatOverviewHiddenWhenNarrowClassName,
				)}
				aria-label="Toggle overview"
				aria-expanded={isOpen}
				aria-controls={isOpen ? cardId : undefined}
			>
				<ListIcon className="size-4" />
			</Button>
		</TooltipTrigger>
		<TooltipContent side="bottom">Toggle overview</TooltipContent>
	</Tooltip>
);

type ChatOverviewCardProps = {
	id: string;
	chat: Chat;
	store: ChatStore;
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	hasMoreMessages: boolean;
	onOpenTarget: (target: OverviewTarget) => void;
};

/**
 * Owns the data hooks, so a new message renders this card again and not the
 * whole page view.
 */
export const ChatOverviewCard: React.FC<ChatOverviewCardProps> = ({
	id,
	chat,
	store,
	repositories,
	hasMoreMessages,
	onOpenTarget,
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(chat) ?? chat.id),
		enabled: showCost,
	});
	const statusOverrides = useChatSelector(store, selectSubagentStatusOverrides);
	const { activeChatChildren } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};
	const { attachments, earlierAttachmentsRequest } = useChatAttachments({
		chatId: chat.id,
		store,
		hasMoreMessages,
		enabled: true,
	});

	const overview: ChatOverview = {
		cost: getOverviewCost(showCost, costQuery),
		isSubagent: Boolean(chat.parent_chat_id),
		repos: [...repositories]
			.map(([root, repo]) => ({
				root,
				name: repoLabel(root),
				branch: repo.branch.trim(),
				...countRepoChanges(repo),
			}))
			.sort((a, b) => a.root.localeCompare(b.root)),
		// Branch rows have no number, so they stay out.
		pullRequests: (chat.diff_statuses ?? []).flatMap((status) => {
			const number = prNumber(status);
			return number ? [{ number, status }] : [];
		}),
		subagents: (activeChatChildren ?? chat.children).map((child) => ({
			id: child.id,
			title: child.title,
			status: statusOverrides.get(child.id) ?? child.status,
		})),
		attachments: attachments ?? [],
	};

	return (
		<ChatOverviewCardView
			id={id}
			overview={overview}
			isLoading={
				(showCost && costQuery.isPending) || earlierAttachmentsRequest.isLoading
			}
			earlierAttachmentsRequest={earlierAttachmentsRequest}
			onOpenTarget={onOpenTarget}
			className={chatOverviewCardPositionClassName}
		/>
	);
};

/**
 * The cost row's state. A failed refetch keeps the cost that loaded, and a
 * chat without AI Gateway requests has no cost row.
 */
export const getOverviewCost = (
	showCost: boolean,
	costQuery: Pick<UseQueryResult<ChatCost>, "isError" | "data">,
): ChatOverview["cost"] => {
	if (!showCost) {
		return undefined;
	}
	const { data } = costQuery;
	if (!data) {
		return costQuery.isError ? "unavailable" : undefined;
	}
	// The endpoint always returns a row, so a chat without requests reports
	// zero rather than nothing.
	return data.request_count === 0 ? undefined : data;
};
