import { cn } from "cn";
import {
	CoinsIcon,
	FileIcon,
	GitBranchIcon,
	ListIcon,
	type LucideIcon,
} from "lucide-react";
import { useId, useState } from "react";
import { type UseQueryResult, useQuery } from "react-query";
import { Link, useOutletContext } from "react-router";
import { chatCost } from "#/api/queries/chats";
import type {
	Chat,
	ChatCost,
	ChatDiffStatus,
	ChatStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatCostMicros } from "#/utils/currency";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { useChatAttachments } from "../hooks/useChatAttachments";
import { buildAgentChatPath } from "../utils/navigation";
import { prNumber } from "../utils/pullRequest";
import { getAttachmentDisplayName } from "./ChatConversation/AttachmentBlocks";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import {
	type ChatStore,
	selectSubagentStatusOverrides,
	useChatSelector,
} from "./ChatConversation/chatStore";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import {
	getChatStatusDisplay,
	getPRIconConfig,
} from "./ChatsSidebar/tree/statusConfig";
import { DiffStatBadge } from "./DiffViewer/DiffStats";
import { computeRepoStats, repoLabel } from "./GitPanel/GitPanel";

/** Rows a list section shows before it folds the rest behind a toggle. */
const OVERVIEW_MAX_LIST_ROWS = 3;
/**
 * Subagent rows the card shows, with no toggle: a chat can have 100+
 * subagents, and the chat tree in the sidebar already lists all of them.
 */
const OVERVIEW_MAX_SUBAGENT_ROWS = 5;

/** The right panel tabs that the overview card can open. */
export type OverviewTabId = "summary" | "git";

type OverviewSubagent = Pick<Chat, "id" | "title" | "status">;

/** What the overview card shows. */
export type ChatOverview = {
	/**
	 * Chat tree cost. Absent without AI Gateway or before the first request;
	 * "unavailable" when the cost request failed.
	 */
	cost?:
		| Pick<ChatCost, "total_cost_micros" | "unpriced_request_count">
		| "unavailable";
	/** The cost covers the parent chat and its other subagents too. */
	isSubagent: boolean;
	/** Every git watcher repository, clean ones too, labeled by branch. */
	repos: Array<{
		root: string;
		branch: string;
		additions: number;
		deletions: number;
	}>;
	pullRequests: Array<{ number: string; status: ChatDiffStatus }>;
	/** Child chats, with their live status. The card lists the non-idle ones. */
	subagents: OverviewSubagent[];
	/** Files in the chat's messages, newest first. */
	attachments: MessageAttachment[];
};

/** The overview toggle state. ChatTopBar renders the toggle only with it. */
export type ChatOverviewToggleProps = {
	isOpen: boolean;
	onToggle: () => void;
};

/** The top bar button that shows and hides the card, at the card's widths. */
export const ChatOverviewToggle: React.FC<ChatOverviewToggleProps> = ({
	isOpen,
	onToggle,
}) => (
	<Button
		variant="subtle"
		size="icon"
		onClick={onToggle}
		className={cn(
			"hidden size-7 text-content-secondary hover:text-content-primary @min-[84rem]/chat:inline-flex",
			isOpen && "bg-surface-secondary text-content-primary",
		)}
		aria-label="Toggle overview"
		aria-expanded={isOpen}
	>
		<ListIcon className="size-4" />
	</Button>
);

type ChatOverviewCardProps = {
	chat: Chat;
	store: ChatStore;
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	hasMoreMessages: boolean;
	isHydratingMessages: boolean;
	onOpenTab: (tabId: OverviewTabId) => void;
};

/**
 * Owns the data hooks, so a new message renders this card again and not the
 * whole page view.
 */
export const ChatOverviewCard: React.FC<ChatOverviewCardProps> = ({
	chat,
	store,
	repositories,
	hasMoreMessages,
	isHydratingMessages,
	onOpenTab,
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(chat) ?? chat.id),
		enabled: showCost,
	});
	const statusOverrides = useChatSelector(store, selectSubagentStatusOverrides);
	const { activeChatChildren } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};
	const { attachments, earlier } = useChatAttachments({
		chatId: chat.id,
		store,
		hasMoreMessages,
		isHydratingMessages,
		enabled: true,
	});
	const repoStats = computeRepoStats(repositories);

	const overview: ChatOverview = {
		cost: getOverviewCost(showCost, costQuery),
		isSubagent: Boolean(chat.parent_chat_id),
		repos: [...repositories]
			.map(([root, repo]) => ({
				root,
				branch: repo.branch.trim() || repoLabel(root),
				...(repoStats.get(root) ?? { additions: 0, deletions: 0 }),
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
			overview={overview}
			isLoading={
				(showCost && costQuery.isPending) ||
				isHydratingMessages ||
				earlier.isLoading
			}
			onOpenTab={onOpenTab}
			// The 16rem card with its 1rem inset and 1rem gap fits beside the
			// centered 48rem transcript from an 84rem chat on.
			className="absolute top-full right-4 z-20 mt-2 hidden @min-[84rem]/chat:flex"
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

type ChatOverviewCardViewProps = {
	overview: ChatOverview;
	/** Cost or messages are still loading, so an empty card is not final. */
	isLoading?: boolean;
	onOpenTab: (tabId: OverviewTabId) => void;
	className?: string;
};

/**
 * Leaves out empty sections. A row calls `onOpenTab` with the right panel
 * tab that holds its details.
 */
export const ChatOverviewCardView: React.FC<ChatOverviewCardViewProps> = ({
	overview,
	isLoading = false,
	onOpenTab,
	className,
}) => {
	const { cost } = overview;
	const headingId = useId();
	const nonIdleSubagents = getNonIdleSubagents(overview.subagents);
	const isEmpty =
		cost === undefined &&
		overview.repos.length === 0 &&
		overview.pullRequests.length === 0 &&
		overview.subagents.length === 0 &&
		overview.attachments.length === 0;

	return (
		<aside
			aria-labelledby={headingId}
			className={cn(
				"flex max-h-[min(32rem,calc(100vh-10rem))] w-64 flex-col divide-y divide-solid divide-border overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-3 text-sm shadow-md",
				className,
			)}
		>
			<h2
				id={headingId}
				className="m-0 py-2 text-xs font-medium text-content-secondary"
			>
				Overview
			</h2>

			{isEmpty && (
				<p className="m-0 py-2 text-xs text-content-secondary">
					{isLoading
						? "Loading…"
						: "Nothing to show yet. Cost, changes, pull requests, subagents, and attachments show up here as the agent works."}
				</p>
			)}

			{cost && (
				<div className="flex flex-col py-2">
					<OverviewRow
						icon={CoinsIcon}
						label="Cost"
						subtitle={getCostSubtitle(overview)}
						detail={
							cost === "unavailable" ? (
								<span className="text-content-secondary">Unavailable</span>
							) : (
								formatCostMicros(cost.total_cost_micros)
							)
						}
						onClick={() => onOpenTab("summary")}
					/>
				</div>
			)}

			{overview.repos.length > 0 && (
				<OverviewSection title="Changes">
					<OverviewList
						items={overview.repos}
						render={(repo) => (
							<OverviewRow
								key={repo.root}
								icon={GitBranchIcon}
								label={repo.branch}
								detail={
									repo.additions === 0 && repo.deletions === 0 ? (
										<span className="font-mono text-[13px] text-content-secondary">
											+0 &minus;0
										</span>
									) : (
										<DiffStatBadge
											additions={repo.additions}
											deletions={repo.deletions}
										/>
									)
								}
								onClick={() => onOpenTab("git")}
							/>
						)}
					/>
				</OverviewSection>
			)}

			{overview.pullRequests.length > 0 && (
				<OverviewSection title="Pull requests">
					<OverviewList
						items={overview.pullRequests}
						render={({ number, status }) => {
							const iconConfig = getPRIconConfig(status);
							return (
								<OverviewRow
									key={status.url ?? number}
									icon={iconConfig?.icon ?? GitBranchIcon}
									iconClassName={iconConfig?.className}
									iconLabel={iconConfig?.label}
									label={`#${number} ${status.pull_request_title || "Pull request"}`}
									detail={
										<DiffStatBadge
											additions={status.additions}
											deletions={status.deletions}
										/>
									}
									onClick={() => onOpenTab("git")}
								/>
							);
						}}
					/>
				</OverviewSection>
			)}

			{overview.subagents.length > 0 && (
				<OverviewSection
					title={`Subagents · ${nonIdleSubagents.length} of ${overview.subagents.length} not idle`}
				>
					{nonIdleSubagents
						.slice(0, OVERVIEW_MAX_SUBAGENT_ROWS)
						.map((subagent) => {
							const display = getChatStatusDisplay(subagent.status);
							return (
								<OverviewRow
									key={subagent.id}
									icon={display.icon}
									iconClassName={display.className}
									iconLabel={display.label}
									label={subagent.title || "Untitled subagent"}
									to={buildAgentChatPath({ chatId: subagent.id })}
								/>
							);
						})}
				</OverviewSection>
			)}

			{overview.attachments.length > 0 && (
				<OverviewSection title="Attachments">
					<OverviewList
						items={overview.attachments}
						render={(attachment) => (
							<OverviewRow
								key={attachment.key}
								icon={FileIcon}
								label={getAttachmentDisplayName(attachment.file)}
								onClick={() => onOpenTab("summary")}
							/>
						)}
					/>
				</OverviewSection>
			)}
		</aside>
	);
};

/**
 * Non-idle subagents, with error and requires_action first so the row cap
 * hides them last. The order within each group is kept.
 */
const getNonIdleSubagents = (
	subagents: readonly OverviewSubagent[],
): OverviewSubagent[] =>
	subagents
		.filter(({ status }) => status !== "waiting")
		.toSorted(
			(a, b) => Number(needsUser(b.status)) - Number(needsUser(a.status)),
		);

const needsUser = (status: ChatStatus): boolean =>
	status === "error" || status === "requires_action";

const getCostSubtitle = (overview: ChatOverview): string | undefined => {
	const { cost } = overview;
	if (!cost || cost === "unavailable") {
		return undefined;
	}
	const parts: string[] = [];
	if (overview.isSubagent) {
		parts.push("Covers the whole chat");
	}
	const unpriced = cost.unpriced_request_count;
	if (unpriced > 0) {
		parts.push(
			`Excludes unpriced usage from ${unpriced} request${unpriced === 1 ? "" : "s"}`,
		);
	}
	return parts.join(" · ");
};

type OverviewSectionProps = React.PropsWithChildren<{ title: string }>;

const OverviewSection: React.FC<OverviewSectionProps> = ({
	title,
	children,
}) => {
	const headingId = useId();
	return (
		<section aria-labelledby={headingId} className="flex flex-col py-2">
			<h3
				id={headingId}
				className="m-0 pb-1 text-xs font-normal text-content-secondary"
			>
				{title}
			</h3>
			{children}
		</section>
	);
};

type OverviewListProps<T> = {
	items: readonly T[];
	render: (item: T) => React.ReactNode;
};

/**
 * The first rows of a list and a toggle for the rest. The toggle stays
 * rendered in both states, so keyboard focus stays on it.
 */
const OverviewList = <T,>({ items, render }: OverviewListProps<T>) => {
	const [expanded, setExpanded] = useState(false);
	const hiddenCount = items.length - OVERVIEW_MAX_LIST_ROWS;
	if (hiddenCount <= 0) {
		return items.map(render);
	}
	return (
		<>
			{(expanded ? items : items.slice(0, OVERVIEW_MAX_LIST_ROWS)).map(render)}
			<button
				type="button"
				aria-expanded={expanded}
				onClick={() => setExpanded(!expanded)}
				className="-mx-1.5 rounded-md py-1 pl-7.5 pr-1.5 text-left text-xs text-content-secondary hover:bg-surface-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
			>
				{expanded ? "Show less" : `Show ${hiddenCount} more`}
			</button>
		</>
	);
};

type OverviewRowProps = {
	icon: LucideIcon;
	iconClassName?: string;
	/** Names the icon for screen readers when it carries a state. */
	iconLabel?: string;
	label: string;
	subtitle?: string;
	detail?: React.ReactNode;
} & ({ onClick: () => void; to?: never } | { to: string; onClick?: never });

const OverviewRow: React.FC<OverviewRowProps> = ({
	icon: Icon,
	iconClassName,
	iconLabel,
	label,
	subtitle,
	detail,
	onClick,
	to,
}) => {
	const className =
		"-mx-1.5 flex items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm text-content-primary no-underline hover:bg-surface-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link";
	const content = (
		<>
			<Icon
				className={cn("size-4 shrink-0 text-content-secondary", iconClassName)}
				{...(iconLabel
					? { role: "img", "aria-label": iconLabel }
					: { "aria-hidden": true })}
			/>
			<span className="flex min-w-0 flex-1 flex-col">
				<span className="truncate">{label}</span>
				{subtitle && (
					<span className="truncate text-xs text-content-secondary">
						{subtitle}
					</span>
				)}
			</span>
			{detail && <span className="shrink-0 tabular-nums">{detail}</span>}
		</>
	);
	if (to !== undefined) {
		return (
			<Link to={to} className={className}>
				{content}
			</Link>
		);
	}
	return (
		<button type="button" onClick={onClick} className={className}>
			{content}
		</button>
	);
};
