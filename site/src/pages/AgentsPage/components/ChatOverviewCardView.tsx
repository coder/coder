import { cn } from "cn";
import {
	CoinsIcon,
	FileIcon,
	GitBranchIcon,
	type LucideIcon,
} from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router";
import type {
	Chat,
	ChatCost,
	ChatDiffStatus,
	ChatStatus,
} from "#/api/typesGenerated";
import { formatCostMicros } from "#/utils/currency";
import { buildAgentChatPath } from "../utils/navigation";
import { getAttachmentDisplayName } from "./ChatConversation/AttachmentBlocks";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import {
	getChatStatusDisplay,
	getPRIconConfig,
} from "./ChatsSidebar/tree/statusConfig";
import { DiffStatBadge } from "./DiffViewer/DiffStats";
import { type GitView, viewIdFor } from "./GitPanel/gitViews";

/** Rows a list section shows before it folds the rest behind a toggle. */
const OVERVIEW_MAX_LIST_ROWS = 3;
/**
 * Subagent rows the card shows, with no toggle: a chat can have 100+
 * subagents, and the chat tree in the sidebar already lists all of them.
 */
const OVERVIEW_MAX_SUBAGENT_ROWS = 5;

/** Where a card row leads: a right panel tab, and for Git its view. */
export type OverviewTarget =
	| { tabId: "summary" }
	| { tabId: "git"; view: GitView };

type OverviewSubagent = Pick<Chat, "id" | "title" | "status">;

type OverviewRepo = {
	root: string;
	branch: string;
	additions: number;
	deletions: number;
};

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
	repos: OverviewRepo[];
	pullRequests: Array<{ number: string; status: ChatDiffStatus }>;
	/** Child chats, with their live status. The card lists the non-idle ones. */
	subagents: OverviewSubagent[];
	/** Files in the chat's messages, newest first. */
	attachments: MessageAttachment[];
};

type ChatOverviewCardViewProps = {
	overview: ChatOverview;
	/** Cost or messages are still loading, so an empty card is not final. */
	isLoading?: boolean;
	onOpenTarget: (target: OverviewTarget) => void;
	className?: string;
};

/**
 * Leaves out empty sections. A row calls `onOpenTarget` with the right panel
 * tab, and for Git the view, that holds its details. A repository without
 * changes is not a row button, because the Git panel has nothing to show
 * for it.
 */
export const ChatOverviewCardView: React.FC<ChatOverviewCardViewProps> = ({
	overview,
	isLoading = false,
	onOpenTarget,
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
						onClick={() => onOpenTarget({ tabId: "summary" })}
					/>
				</div>
			)}

			<OverviewListSection
				title="Changes"
				items={overview.repos}
				render={(repo) => (
					<OverviewRow
						key={repo.root}
						icon={GitBranchIcon}
						label={repo.branch}
						detail={<RepoDiffStat {...repo} />}
						onClick={
							repo.additions === 0 && repo.deletions === 0
								? undefined
								: () =>
										onOpenTarget({
											tabId: "git",
											view: { type: "local", repoRoot: repo.root },
										})
						}
					/>
				)}
			/>

			<OverviewListSection
				title="Pull requests"
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
							onClick={() =>
								onOpenTarget({
									tabId: "git",
									view: { type: "remote", refId: viewIdFor(status) },
								})
							}
						/>
					);
				}}
			/>

			<SubagentsSection subagents={overview.subagents} />

			<OverviewListSection
				title="Attachments"
				items={overview.attachments}
				render={(attachment) => (
					<OverviewRow
						key={attachment.key}
						icon={FileIcon}
						label={getAttachmentDisplayName(attachment.file)}
						onClick={() => onOpenTarget({ tabId: "summary" })}
					/>
				)}
			/>
		</aside>
	);
};

type SubagentsSectionProps = { subagents: readonly OverviewSubagent[] };

/**
 * Up to five non-idle subagents, with the ones that need the user first so
 * the cap hides them last. The heading counts every subagent, so a chat
 * whose subagents are all idle still shows the count.
 */
const SubagentsSection: React.FC<SubagentsSectionProps> = ({ subagents }) => {
	if (subagents.length === 0) {
		return null;
	}
	const nonIdle = subagents
		.filter(({ status }) => !isIdle(status))
		.toSorted(
			(a, b) => Number(needsUser(b.status)) - Number(needsUser(a.status)),
		);
	return (
		<OverviewSection
			title={`Subagents · ${nonIdle.length} of ${subagents.length} not idle`}
		>
			{nonIdle.slice(0, OVERVIEW_MAX_SUBAGENT_ROWS).map((subagent) => {
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
	);
};

const isIdle = (status: ChatStatus): boolean => status === "waiting";

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

/** A clean repository shows +0 −0, where the shared badge shows nothing. */
const RepoDiffStat: React.FC<Pick<OverviewRepo, "additions" | "deletions">> = ({
	additions,
	deletions,
}) =>
	additions === 0 && deletions === 0 ? (
		<span className="font-mono text-[13px] text-content-secondary">
			+0 &minus;0
		</span>
	) : (
		<DiffStatBadge additions={additions} deletions={deletions} />
	);

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

/** A titled list, or nothing when the list is empty. */
const OverviewListSection = <T,>({
	title,
	items,
	render,
}: OverviewListProps<T> & { title: string }) =>
	items.length === 0 ? null : (
		<OverviewSection title={title}>
			<OverviewList items={items} render={render} />
		</OverviewSection>
	);

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
} & ({ onClick?: () => void; to?: never } | { to: string; onClick?: never });

/** A button with `onClick`, a link with `to`, or plain text with neither. */
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
	const className =
		"-mx-1.5 flex items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm text-content-primary";
	const interactiveClassName = cn(
		className,
		"no-underline hover:bg-surface-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
	);
	if (to !== undefined) {
		return (
			<Link to={to} className={interactiveClassName}>
				{content}
			</Link>
		);
	}
	if (!onClick) {
		return <div className={className}>{content}</div>;
	}
	return (
		<button type="button" onClick={onClick} className={interactiveClassName}>
			{content}
		</button>
	);
};
