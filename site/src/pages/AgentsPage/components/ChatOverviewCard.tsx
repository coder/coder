import { cn } from "cn";
import {
	CoinsIcon,
	FileIcon,
	GitBranchIcon,
	type LucideIcon,
	PinIcon,
} from "lucide-react";
import { useId, useState } from "react";
import { type UseQueryResult, useQuery } from "react-query";
import { Link, useOutletContext } from "react-router";
import { chatCost } from "#/api/queries/chats";
import type {
	Chat,
	ChatCost,
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatCostMicros } from "#/utils/currency";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { getPathBasename } from "../utils/path";
import { prNumber } from "../utils/pullRequest";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import {
	type ChatStore,
	selectSubagentStatusOverrides,
	useChatMessages,
	useChatSelector,
} from "./ChatConversation/chatStore";
import {
	type AgentAttachment,
	getLoadedAgentAttachments,
} from "./ChatConversation/messageHelpers";
import {
	getChatStatusDisplay,
	getPRIconConfig,
} from "./ChatsSidebar/tree/statusConfig";
import { DiffStatBadge } from "./DiffViewer/DiffStats";
import { computeRepoStats, type DiffStats } from "./GitPanel/GitPanel";

/** Rows a list section shows before it folds the rest behind a toggle. */
const OVERVIEW_MAX_LIST_ROWS = 3;

/**
 * The open card needs 18rem beside the 48rem transcript and composer: the
 * 16rem card, its 1rem inset, and a 1rem gap. The card and its toggle show
 * from a 66rem chat (48rem + 18rem). The room is only what the centered
 * transcript lacks, max(0, 84rem - chat width), so from 84rem on nothing
 * moves.
 */
export const overviewToggleClassName = "hidden @min-[66rem]/chat:inline-flex";
/** Set on the chat column while the card is open. */
export const overviewRoomClassName =
	"@min-[66rem]/chat:[--agents-overview-room:max(0px,calc(84rem_-_100%))]";
const overviewCardPositionClassName =
	"absolute top-full right-4 z-20 mt-2 hidden @min-[66rem]/chat:flex";

/** The right panel tabs that the overview card can open. */
export type OverviewTabId = "summary" | "git";

type OverviewSubagent = Pick<Chat, "id" | "title" | "status">;

/** What the overview card shows. Each empty section is left out. */
export type ChatOverview = {
	/**
	 * Chat tree cost. Absent without AI Gateway or before the first request;
	 * "unavailable" when the cost request failed.
	 */
	cost?: { totalMicros: number; unpricedRequests: number } | "unavailable";
	/** The cost covers the parent chat and its other subagents too. */
	isSubagent: boolean;
	/** Repositories with uncommitted changes, from the git watcher. */
	repos: Array<DiffStats & { root: string; branch: string }>;
	/** Pull requests of the chat, with their numbers. */
	pullRequests: Array<{ number: string; status: ChatDiffStatus }>;
	/** Child chats, with the live status from the chat stream. */
	subagents: OverviewSubagent[];
	/** Files the agent attached, newest first, from the loaded messages. */
	attachments: AgentAttachment[];
};

type ChatOverviewCardProps = {
	ref?: React.Ref<HTMLElement>;
	chat: Chat;
	store: ChatStore;
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	isHydratingMessages: boolean;
	isPinned: boolean;
	onPinnedChange: (pinned: boolean) => void;
	onOpenTab: (tabId: OverviewTabId) => void;
};

/**
 * Owns the data hooks, so a new message renders this card again and not the
 * whole page view.
 */
export const ChatOverviewCard: React.FC<ChatOverviewCardProps> = ({
	ref,
	chat,
	store,
	repositories,
	isHydratingMessages,
	isPinned,
	onPinnedChange,
	onOpenTab,
}) => {
	const showCost = useFeatureVisibility().aibridge;
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(chat) ?? chat.id),
		enabled: showCost,
	});
	const messages = useChatMessages(store);
	const statusOverrides = useChatSelector(store, selectSubagentStatusOverrides);
	const { activeChatChildren } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};

	const overview: ChatOverview = {
		cost: getOverviewCost(showCost, costQuery),
		isSubagent: Boolean(chat.parent_chat_id),
		repos: [...computeRepoStats(repositories)]
			.map(([root, stats]) => ({
				root,
				branch: repositories.get(root)?.branch || getPathBasename(root),
				...stats,
			}))
			.sort((a, b) => a.root.localeCompare(b.root)),
		// Branch rows have no number, so they stay out, as in the top bar.
		pullRequests: (chat.diff_statuses ?? []).flatMap((status) => {
			const number = prNumber(status);
			return number ? [{ number, status }] : [];
		}),
		subagents: (activeChatChildren ?? chat.children).map((child) => ({
			id: child.id,
			title: child.title,
			status: statusOverrides.get(child.id) ?? child.status,
		})),
		attachments: getLoadedAgentAttachments(messages, isHydratingMessages) ?? [],
	};

	return (
		<ChatOverviewCardView
			ref={ref}
			overview={overview}
			isLoading={(showCost && costQuery.isPending) || isHydratingMessages}
			isPinned={isPinned}
			onPinnedChange={onPinnedChange}
			onOpenTab={onOpenTab}
			className={overviewCardPositionClassName}
		/>
	);
};

const getOverviewCost = (
	showCost: boolean,
	costQuery: Pick<UseQueryResult<ChatCost>, "isError" | "data">,
): ChatOverview["cost"] => {
	if (!showCost) {
		return undefined;
	}
	const { data } = costQuery;
	// A failed refetch keeps the cost that already loaded.
	if (!data) {
		return costQuery.isError ? "unavailable" : undefined;
	}
	// The endpoint always returns a row, so a chat without requests reports
	// zero rather than nothing.
	if (data.request_count === 0) {
		return undefined;
	}
	return {
		totalMicros: data.total_cost_micros,
		unpricedRequests: data.unpriced_request_count,
	};
};

type ChatOverviewCardViewProps = {
	ref?: React.Ref<HTMLElement>;
	overview: ChatOverview;
	/** Cost or messages are still loading, so an empty card is not final. */
	isLoading?: boolean;
	isPinned: boolean;
	onPinnedChange: (pinned: boolean) => void;
	onOpenTab: (tabId: OverviewTabId) => void;
	className?: string;
};

/**
 * The card itself. Rows open a right panel tab through `onOpenTab`, except
 * subagent rows, which link to the subagent's chat.
 */
export const ChatOverviewCardView: React.FC<ChatOverviewCardViewProps> = ({
	ref,
	overview,
	isLoading = false,
	isPinned,
	onPinnedChange,
	onOpenTab,
	className,
}) => {
	const { cost } = overview;
	const headingId = useId();
	const isEmpty =
		cost === undefined &&
		overview.repos.length === 0 &&
		overview.pullRequests.length === 0 &&
		overview.subagents.length === 0 &&
		overview.attachments.length === 0;

	return (
		<aside
			ref={ref}
			aria-labelledby={headingId}
			className={cn(
				"flex max-h-[min(32rem,calc(100vh-10rem))] w-64 flex-col divide-y divide-solid divide-border overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-3 text-sm shadow-md",
				className,
			)}
		>
			<div className="flex items-center justify-between gap-2 py-1.5">
				<h2
					id={headingId}
					className="m-0 text-xs font-medium text-content-secondary"
				>
					Overview
				</h2>
				<Button
					variant="subtle"
					size="icon"
					className={cn(
						"-mr-1.5 size-6 text-content-secondary hover:text-content-primary",
						isPinned && "text-content-primary",
					)}
					aria-label="Pin overview"
					aria-pressed={isPinned}
					onClick={() => onPinnedChange(!isPinned)}
				>
					<PinIcon className={cn("size-3.5", isPinned && "fill-current")} />
				</Button>
			</div>

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
								formatCostMicros(cost.totalMicros)
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
									<DiffStatBadge
										additions={repo.additions}
										deletions={repo.deletions}
									/>
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
						render={({ number, status }) => (
							<PullRequestRow
								key={number}
								number={number}
								status={status}
								onClick={() => onOpenTab("git")}
							/>
						)}
					/>
				</OverviewSection>
			)}

			{overview.subagents.length > 0 && (
				<OverviewSection title="Subagents">
					<OverviewList
						items={overview.subagents}
						render={(subagent) => (
							<SubagentRow key={subagent.id} subagent={subagent} />
						)}
					/>
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
								label={attachment.file.name || "Attachment"}
								onClick={() => onOpenTab("summary")}
							/>
						)}
					/>
				</OverviewSection>
			)}
		</aside>
	);
};

const getCostSubtitle = (overview: ChatOverview): string | undefined => {
	const { cost } = overview;
	if (!cost || cost === "unavailable") {
		return undefined;
	}
	const parts: string[] = [];
	if (overview.isSubagent) {
		parts.push("Covers the whole chat");
	}
	if (cost.unpricedRequests > 0) {
		const plural = cost.unpricedRequests === 1 ? "" : "s";
		parts.push(
			`Excludes unpriced usage from ${cost.unpricedRequests} request${plural}`,
		);
	}
	return parts.length > 0 ? parts.join(" · ") : undefined;
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
		return <>{items.map(render)}</>;
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

/** A card row: a button that opens a tab, or a link to another chat. */
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
		"-mx-1.5 flex min-w-0 items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm text-content-primary no-underline hover:bg-surface-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link";
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

type PullRequestRowProps = {
	number: string;
	status: ChatDiffStatus;
	onClick: () => void;
};

const PullRequestRow: React.FC<PullRequestRowProps> = ({
	number,
	status,
	onClick,
}) => {
	const icon = getPRIconConfig(status);
	return (
		<OverviewRow
			icon={icon?.icon ?? GitBranchIcon}
			iconClassName={icon?.className}
			iconLabel={icon?.label}
			label={`#${number} ${status.pull_request_title || "Pull request"}`}
			detail={
				<DiffStatBadge
					additions={status.additions}
					deletions={status.deletions}
				/>
			}
			onClick={onClick}
		/>
	);
};

type SubagentRowProps = { subagent: OverviewSubagent };

const SubagentRow: React.FC<SubagentRowProps> = ({ subagent }) => {
	const display = getChatStatusDisplay(subagent.status);
	return (
		<OverviewRow
			icon={display.icon}
			iconClassName={display.className}
			iconLabel={display.label}
			label={subagent.title || "Untitled agent"}
			to={`/agents/${subagent.id}`}
		/>
	);
};
