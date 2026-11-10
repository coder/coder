import { cn } from "cn";
import {
	CoinsIcon,
	FileIcon,
	FolderGit2Icon,
	GitBranchIcon,
	type LucideIcon,
} from "lucide-react";
import { useId, useState } from "react";
import type { ChatCost, ChatDiffStatus } from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatCostMicros } from "#/utils/currency";
import { getAttachmentDisplayName } from "./ChatConversation/AttachmentBlocks";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import { getPRIconConfig } from "./ChatsSidebar/tree/statusConfig";
import { DiffStatBadge } from "./DiffViewer/DiffStats";

const OVERVIEW_MAX_LIST_ROWS = 3;
const OVERVIEW_CARD_MAX_HEIGHT = "36rem";
const OVERVIEW_CARD_VIEWPORT_MAX_HEIGHT = "calc(100vh - 6rem)";

/** Where a card row leads: the right panel tab that holds its details. */
export type OverviewTarget = { tabId: "summary" } | { tabId: "git" };

type OverviewRepo = {
	root: string;
	/** The row label. `root` stays the row's key, so names may repeat. */
	name: string;
	/** Empty when HEAD is on no branch, such as a detached HEAD. */
	branch: string;
	additions: number;
	deletions: number;
};

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
	/** Files in the chat's messages, newest first. */
	attachments: MessageAttachment[];
};

type ChatOverviewCardViewProps = {
	overview: ChatOverview;
	isLoading?: boolean;
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
	onOpenTarget,
	className,
}) => {
	const { cost } = overview;
	const headingId = useId();
	const isEmpty =
		cost === undefined &&
		overview.repos.length === 0 &&
		overview.pullRequests.length === 0 &&
		overview.attachments.length === 0;

	return (
		<aside
			aria-labelledby={headingId}
			aria-busy={isLoading}
			className={cn(
				"flex w-64 flex-col divide-y divide-solid divide-border overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-1.5 text-sm shadow-md",
				className,
			)}
			style={{
				maxHeight: `min(${OVERVIEW_CARD_MAX_HEIGHT}, ${OVERVIEW_CARD_VIEWPORT_MAX_HEIGHT})`,
			}}
		>
			<h2
				id={headingId}
				className="m-0 px-1.5 py-2 text-xs font-medium text-content-secondary"
			>
				Overview
			</h2>

			{isEmpty &&
				(isLoading ? (
					<div role="status" className="flex flex-col gap-2 px-1.5 py-2">
						<span className="sr-only">Loading overview</span>
						{[0, 1, 2].map((row) => (
							<div key={row} aria-hidden className="flex items-center gap-2">
								<Skeleton variant="circular" className="size-4 shrink-0" />
								<Skeleton variant="text" className="flex-1" />
							</div>
						))}
					</div>
				) : (
					<p className="m-0 px-1.5 py-2 text-xs text-content-secondary">
						Nothing to show yet. Cost, repositories, pull requests, and
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
				title="Repositories"
				items={overview.repos}
				render={(repo) => (
					<OverviewRow
						key={repo.root}
						icon={FolderGit2Icon}
						label={repo.name}
						subtitle={repo.branch || "No branch"}
						detail={<RepoDiffStat {...repo} />}
						onClick={
							hasChanges(repo)
								? () => onOpenTarget({ tabId: "git" })
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
							onClick={() => onOpenTarget({ tabId: "git" })}
						/>
					);
				}}
			/>

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

type RepoChanges = Pick<OverviewRepo, "additions" | "deletions">;

const hasChanges = ({ additions, deletions }: RepoChanges) =>
	additions > 0 || deletions > 0;

/** A clean repository shows +0 −0, where the shared badge shows nothing. */
const RepoDiffStat: React.FC<RepoChanges> = (changes) =>
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
				className="m-0 px-1.5 pb-1 text-xs font-normal text-content-secondary"
			>
				{title}
			</h3>
			{children}
		</section>
	);
};

type OverviewListSectionProps<T> = {
	title: string;
	items: readonly T[];
	render: (item: T) => React.ReactNode;
};

/**
 * The toggle stays rendered in both states, so keyboard focus stays on it.
 * Its label names the section, so a screen reader's list of buttons tells
 * the toggles apart.
 */
const OverviewListSection = <T,>({
	title,
	items,
	render,
}: OverviewListSectionProps<T>) => {
	const [expanded, setExpanded] = useState(false);
	if (items.length === 0) {
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
					className="rounded-md py-1 pl-7.5 pr-1.5 text-left text-xs text-content-secondary hover:bg-surface-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					{expanded
						? `Show fewer ${title.toLowerCase()}`
						: `Show all ${items.length} ${title.toLowerCase()}`}
				</button>
			)}
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
	onClick?: () => void;
};

/**
 * A button with `onClick`, or plain text without it. A button's label and
 * subtitle truncate, and a tooltip shows them in full only when they are cut
 * off, so a row that fits gets no accessible description that repeats its
 * name. Plain text wraps instead and takes no tab stop.
 */
const OverviewRow: React.FC<OverviewRowProps> = ({
	icon: Icon,
	iconClassName,
	iconLabel,
	label,
	subtitle,
	detail,
	onClick,
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
	const textClassName = onClick ? "truncate" : "break-words";
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
				ref={onClick ? observeText : undefined}
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
		"flex items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm text-content-primary";
	if (!onClick) {
		return <div className={className}>{content}</div>;
	}
	return (
		<Tooltip open={isTruncated ? undefined : false}>
			<TooltipTrigger asChild>
				<button
					type="button"
					onClick={onClick}
					className={cn(
						className,
						"hover:bg-surface-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
					)}
				>
					{content}
				</button>
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
