import { cn } from "cn";
import {
	CoinsIcon,
	FileIcon,
	GitBranchIcon,
	type LucideIcon,
} from "lucide-react";
import { useState } from "react";
import { useQuery } from "react-query";
import { Link, useOutletContext } from "react-router";
import { chatCost } from "#/api/queries/chats";
import type {
	Chat,
	ChatDiffStatus,
	ChatStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatCostMicros } from "#/utils/currency";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
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
import { computeRepoStats } from "./GitPanel/GitPanel";

/** Rows a list section shows before it folds the rest into one "N more" row. */
const OVERVIEW_MAX_LIST_ROWS = 3;

/** The right panel tabs that the overview card can open. */
export type OverviewTabId = "summary" | "git";

export type ChatOverview = {
	/** Chat tree cost. Absent without AI Gateway or before the first request. */
	cost?: { totalMicros: number; unpricedRequests: number };
	/** The cost covers the parent chat and its other sub-agents too. */
	isSubagent: boolean;
	/** Repositories with uncommitted changes, from the git watcher. */
	repos: Array<{
		root: string;
		branch: string;
		additions: number;
		deletions: number;
	}>;
	/** Pull requests of the chat, the same refs as the top-bar PR chip. */
	pullRequests: ChatDiffStatus[];
	/** Child chats, with the live status from the chat stream. */
	subagents: Array<{ id: string; title: string; status: ChatStatus }>;
	/** Files the agent attached, newest first, from the loaded messages. */
	attachments: AgentAttachment[];
};

export const hasOverviewContent = (overview: ChatOverview): boolean =>
	overview.cost !== undefined ||
	overview.repos.length > 0 ||
	overview.pullRequests.length > 0 ||
	overview.subagents.length > 0 ||
	overview.attachments.length > 0;

type ChatOverviewCardProps = {
	chat: Chat;
	store: ChatStore;
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	isHydratingMessages: boolean;
	onOpenTab: (tabId: OverviewTabId) => void;
	className?: string;
};

/**
 * A floating summary of the chat for when the right panel is closed. The
 * data hooks live here, not in the page view, so a new message renders this
 * card again and not the whole page. The card renders nothing when it has
 * nothing to show.
 */
export const ChatOverviewCard: React.FC<ChatOverviewCardProps> = ({
	chat,
	store,
	repositories,
	isHydratingMessages,
	onOpenTab,
	className,
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(chat) ?? chat.id),
		enabled: showCost,
	});
	const messages = useChatMessages(store);
	const statusOverrides = useChatSelector(store, selectSubagentStatusOverrides);
	const { activeChatChildren } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};

	const costData = costQuery.data;
	const repoStats = computeRepoStats(repositories);
	const overview: ChatOverview = {
		// The cost endpoint always returns a row, so a chat with no requests
		// yet reports zero rather than nothing.
		cost:
			showCost && costData && costData.request_count > 0
				? {
						totalMicros: costData.total_cost_micros,
						unpricedRequests: costData.unpriced_request_count,
					}
				: undefined,
		isSubagent: Boolean(chat.parent_chat_id),
		repos: [...repoStats.entries()]
			.map(([root, stats]) => ({
				root,
				branch: repositories.get(root)?.branch || repoFolderName(root),
				...stats,
			}))
			.sort((a, b) => a.root.localeCompare(b.root)),
		pullRequests: (chat.diff_statuses ?? []).filter(
			(status) => prNumber(status) !== undefined,
		),
		subagents: (activeChatChildren ?? chat.children ?? []).map((child) => ({
			id: child.id,
			title: child.title,
			status: statusOverrides.get(child.id) ?? child.status,
		})),
		attachments: getLoadedAgentAttachments(messages, isHydratingMessages) ?? [],
	};

	if (!hasOverviewContent(overview)) {
		return null;
	}
	return (
		<ChatOverviewCardView
			overview={overview}
			onOpenTab={onOpenTab}
			className={className}
		/>
	);
};

const repoFolderName = (root: string): string =>
	root.split(/[\\/]/).filter(Boolean).at(-1) ?? root;

type ChatOverviewCardViewProps = {
	overview: ChatOverview;
	onOpenTab: (tabId: OverviewTabId) => void;
	className?: string;
};

/** The card itself: data in, tab requests out. */
export const ChatOverviewCardView: React.FC<ChatOverviewCardViewProps> = ({
	overview,
	onOpenTab,
	className,
}) => {
	const { cost } = overview;
	const costNote = [
		overview.isSubagent ? "Whole chat tree" : undefined,
		cost && cost.unpricedRequests > 0
			? `Excludes ${cost.unpricedRequests} unpriced request${cost.unpricedRequests === 1 ? "" : "s"}`
			: undefined,
	]
		.filter(Boolean)
		.join(" · ");

	return (
		<aside
			aria-label="Chat overview"
			className={cn(
				"flex max-h-[min(32rem,calc(100vh-10rem))] w-64 flex-col divide-y divide-solid divide-border overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-3 text-sm shadow-md",
				className,
			)}
		>
			{cost && (
				<div className="flex flex-col py-2">
					<OverviewRow
						icon={CoinsIcon}
						label="Cost"
						sub={costNote || undefined}
						detail={formatCostMicros(cost.totalMicros)}
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
									<LineStats
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
						render={(status) => (
							<PullRequestRow
								key={status.url ?? prNumber(status)}
								status={status}
								onClick={() => onOpenTab("git")}
							/>
						)}
					/>
				</OverviewSection>
			)}

			{overview.subagents.length > 0 && (
				<OverviewSection title="Sub-agents">
					<OverviewList
						items={overview.subagents}
						render={(subagent) => (
							<SubagentRow key={subagent.id} {...subagent} />
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

const OverviewSection: React.FC<React.PropsWithChildren<{ title: string }>> = ({
	title,
	children,
}) => (
	<section aria-label={title} className="flex flex-col py-2">
		<h3 className="m-0 pb-1 text-xs font-normal text-content-secondary">
			{title}
		</h3>
		{children}
	</section>
);

/** The first rows of a list, then one row that shows the rest. */
const OverviewList = <T,>({
	items,
	render,
}: {
	items: readonly T[];
	render: (item: T) => React.ReactNode;
}) => {
	const [expanded, setExpanded] = useState(false);
	const hidden = items.length - OVERVIEW_MAX_LIST_ROWS;
	const shown =
		expanded || hidden <= 0 ? items : items.slice(0, OVERVIEW_MAX_LIST_ROWS);
	return (
		<>
			{shown.map(render)}
			{!expanded && hidden > 0 && (
				<button
					type="button"
					onClick={() => setExpanded(true)}
					className={cn(rowClassName, "pl-7 text-xs text-content-secondary")}
				>
					{hidden} more
				</button>
			)}
		</>
	);
};

const rowClassName =
	"-mx-1.5 flex min-w-0 items-center gap-2 rounded-md border-0 bg-transparent px-1.5 py-1 text-left text-sm text-content-primary no-underline cursor-pointer hover:bg-surface-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link";

type OverviewRowContentProps = {
	icon: LucideIcon;
	iconClassName?: string;
	iconLabel?: string;
	label: string;
	sub?: string;
	detail?: React.ReactNode;
};

const OverviewRowContent: React.FC<OverviewRowContentProps> = ({
	icon: Icon,
	iconClassName,
	iconLabel,
	label,
	sub,
	detail,
}) => (
	<>
		<Icon
			className={cn("size-4 shrink-0 text-content-secondary", iconClassName)}
			{...(iconLabel
				? { role: "img", "aria-label": iconLabel }
				: { "aria-hidden": true })}
		/>
		<span className="flex min-w-0 flex-1 flex-col">
			<span className="truncate">{label}</span>
			{sub && (
				<span className="truncate text-xs text-content-secondary">{sub}</span>
			)}
		</span>
		{detail && <span className="shrink-0 tabular-nums">{detail}</span>}
	</>
);

const OverviewRow: React.FC<
	OverviewRowContentProps & { onClick: () => void }
> = ({ onClick, ...content }) => (
	<button type="button" onClick={onClick} className={rowClassName}>
		<OverviewRowContent {...content} />
	</button>
);

const LineStats: React.FC<{ additions: number; deletions: number }> = ({
	additions,
	deletions,
}) => (
	<span className="inline-flex items-center gap-1 text-xs">
		<span className="text-git-added-bright">
			+{additions.toLocaleString("en-US")}
		</span>
		<span className="text-git-deleted-bright">
			&minus;{deletions.toLocaleString("en-US")}
		</span>
	</span>
);

const PullRequestRow: React.FC<{
	status: ChatDiffStatus;
	onClick: () => void;
}> = ({ status, onClick }) => {
	const icon = getPRIconConfig(status);
	const number = prNumber(status);
	const title = status.pull_request_title || "Pull request";
	return (
		<OverviewRow
			icon={icon?.icon ?? GitBranchIcon}
			iconClassName={icon?.className}
			iconLabel={icon?.label}
			label={number ? `#${number} ${title}` : title}
			detail={
				status.additions > 0 || status.deletions > 0 ? (
					<LineStats
						additions={status.additions}
						deletions={status.deletions}
					/>
				) : undefined
			}
			onClick={onClick}
		/>
	);
};

const SubagentRow: React.FC<{
	id: string;
	title: string;
	status: ChatStatus;
}> = ({ id, title, status }) => {
	const display = getChatStatusDisplay(status);
	return (
		<Link to={`/agents/${id}`} className={rowClassName}>
			<OverviewRowContent
				icon={display.icon}
				iconClassName={display.className}
				iconLabel={display.label}
				label={title || "Untitled agent"}
			/>
		</Link>
	);
};
