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
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatCostMicros } from "#/utils/currency";
import type { EarlierAttachmentsRequest } from "../hooks/useChatAttachments";
import { buildAgentChatPath } from "../utils/navigation";
import { getAttachmentDisplayName } from "./ChatConversation/AttachmentBlocks";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import {
	getChatStatusDisplay,
	getPRIconConfig,
} from "./ChatsSidebar/tree/statusConfig";
import { DiffStatBadge } from "./DiffViewer/DiffStats";
import { type DiffStats, type GitView, viewIdFor } from "./GitPanel/gitViews";
import { RetryButton } from "./RetryButton";

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

type OverviewRepo = DiffStats & { root: string; branch: string };

/** Card data. Empty lists and an absent cost hide their sections. */
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
	repos: OverviewRepo[];
	pullRequests: Array<{ number: string; status: ChatDiffStatus }>;
	/** Child chats, with their live status. The card lists the non-idle ones. */
	subagents: OverviewSubagent[];
	/** Files in the chat's messages, newest first. */
	attachments: MessageAttachment[];
};

type ChatOverviewCardViewProps = {
	overview: ChatOverview;
	isLoading?: boolean;
	/** A failed request shows its error and a retry in Attachments. */
	earlierAttachmentsRequest?: Pick<
		EarlierAttachmentsRequest,
		"error" | "isRetrying" | "retry"
	>;
	onOpenTarget: (target: OverviewTarget) => void;
	className?: string;
};

/**
 * A repository without changes is plain text, because the Git panel has
 * nothing to show for it.
 */
export const ChatOverviewCardView: React.FC<ChatOverviewCardViewProps> = ({
	overview,
	isLoading = false,
	earlierAttachmentsRequest,
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
		overview.attachments.length === 0 &&
		!earlierAttachmentsRequest?.error;

	return (
		<aside
			aria-labelledby={headingId}
			aria-busy={isLoading}
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

			{isEmpty &&
				(isLoading ? (
					<div role="status" className="flex flex-col gap-2 py-2">
						<span className="sr-only">Loading overview</span>
						{[0, 1, 2].map((row) => (
							<div key={row} aria-hidden className="flex items-center gap-2">
								<Skeleton variant="circular" className="size-4 shrink-0" />
								<Skeleton variant="text" className="flex-1" />
							</div>
						))}
					</div>
				) : (
					<p className="m-0 py-2 text-xs text-content-secondary">
						Nothing to show yet. Cost, changes, pull requests, subagents, and
						attachments show up here as the agent works.
					</p>
				))}

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
							hasChanges(repo)
								? () =>
										onOpenTarget({
											tabId: "git",
											view: { type: "local", repoRoot: repo.root },
										})
								: undefined
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
			>
				{earlierAttachmentsRequest?.error ? (
					<ErrorAlert
						error={earlierAttachmentsRequest.error}
						showDebugDetail={false}
						className="mt-1 p-3"
						actions={
							<RetryButton
								variant="outline"
								size="sm"
								isRetrying={earlierAttachmentsRequest.isRetrying}
								onRetry={earlierAttachmentsRequest.retry}
							/>
						}
					/>
				) : null}
			</OverviewListSection>
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
	return parts.length > 0 ? parts.join(" · ") : undefined;
};

const hasChanges = ({ additions, deletions }: DiffStats) =>
	additions > 0 || deletions > 0;

/** A clean repository shows +0 −0, where the shared badge shows nothing. */
const RepoDiffStat: React.FC<DiffStats> = (changes) =>
	hasChanges(changes) ? (
		<DiffStatBadge {...changes} />
	) : (
		<span className="font-mono text-[13px] text-content-secondary">
			+0 &minus;0
		</span>
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

type OverviewListSectionProps<T> = React.PropsWithChildren<{
	title: string;
	items: readonly T[];
	render: (item: T) => React.ReactNode;
}>;

/**
 * A titled list, or nothing when the list and `children` are empty. It
 * shows the first rows and a toggle for the rest, then `children`. The toggle
 * stays rendered in both states, so keyboard focus stays on it. Its label
 * names the section, so a screen reader's list of buttons tells the toggles
 * apart.
 */
const OverviewListSection = <T,>({
	title,
	items,
	render,
	children,
}: OverviewListSectionProps<T>) => {
	const [expanded, setExpanded] = useState(false);
	if (items.length === 0 && !children) {
		return null;
	}
	const hiddenCount = items.length - OVERVIEW_MAX_LIST_ROWS;
	return (
		<OverviewSection title={title}>
			{(expanded ? items : items.slice(0, OVERVIEW_MAX_LIST_ROWS)).map(render)}
			{hiddenCount > 0 && (
				<button
					type="button"
					onClick={() => setExpanded(!expanded)}
					className="-mx-1.5 rounded-md py-1 pl-7.5 pr-1.5 text-left text-xs text-content-secondary hover:bg-surface-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					{expanded
						? `Show fewer ${title.toLowerCase()}`
						: `Show ${hiddenCount} more ${title.toLowerCase()}`}
				</button>
			)}
			{children}
		</OverviewSection>
	);
};

type OverviewRowProps = {
	icon: LucideIcon;
	iconClassName?: string;
	iconLabel?: string;
	label: string;
	subtitle?: string;
	detail?: React.ReactNode;
} & ({ onClick?: () => void; to?: never } | { to: string; onClick?: never });

/**
 * A button with `onClick`, a link with `to`, or plain text with neither. A
 * button's or link's label and subtitle truncate, and a tooltip shows them in
 * full only when they are cut off, so a row that fits gets no accessible
 * description that repeats its name. Plain text wraps instead and takes no
 * tab stop.
 */
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
	const [isTruncated, setIsTruncated] = useState(false);
	// ResizeObserver fires once on observe, which doubles as the first check.
	// The key below remounts the text, and so observes it again, when it
	// changes.
	const observeText = (element: HTMLSpanElement | null) => {
		if (!element) {
			return;
		}
		const observer = new ResizeObserver(() => {
			setIsTruncated(
				[...element.children].some(
					(child) => child.scrollWidth > child.clientWidth,
				),
			);
		});
		observer.observe(element);
		return () => observer.disconnect();
	};
	const isInteractive = onClick !== undefined || to !== undefined;
	const textClassName = isInteractive ? "truncate" : "break-words";
	const content = (
		<>
			<Icon
				className={cn("size-4 shrink-0 text-content-secondary", iconClassName)}
				{...(iconLabel
					? { role: "img", "aria-label": iconLabel }
					: { "aria-hidden": true })}
			/>
			<span
				key={`${label}\n${subtitle ?? ""}`}
				ref={isInteractive ? observeText : undefined}
				className="flex min-w-0 flex-1 flex-col"
			>
				<span className={textClassName}>{label}</span>
				{subtitle && (
					<span className={cn(textClassName, "text-xs text-content-secondary")}>
						{subtitle}
					</span>
				)}
			</span>
			{detail && <span className="shrink-0 tabular-nums">{detail}</span>}
		</>
	);
	const className =
		"-mx-1.5 flex items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm text-content-primary";
	if (!isInteractive) {
		return <div className={className}>{content}</div>;
	}
	const interactiveClassName = cn(
		className,
		"no-underline hover:bg-surface-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
	);
	return (
		<Tooltip open={isTruncated ? undefined : false}>
			<TooltipTrigger asChild>
				{to !== undefined ? (
					<Link to={to} className={interactiveClassName}>
						{content}
					</Link>
				) : (
					<button
						type="button"
						onClick={onClick}
						className={interactiveClassName}
					>
						{content}
					</button>
				)}
			</TooltipTrigger>
			<TooltipContent side="left" className="max-w-xs">
				<p className="m-0 break-words">{label}</p>
				{subtitle && (
					<p className="m-0 break-words text-content-secondary">{subtitle}</p>
				)}
			</TooltipContent>
		</Tooltip>
	);
};
