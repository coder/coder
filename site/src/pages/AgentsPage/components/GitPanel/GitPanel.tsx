import {
	CheckIcon,
	ChevronDownIcon,
	CircleDotIcon,
	ColumnsIcon,
	GitBranchIcon,
	GitCompareArrowsIcon,
	GitMergeIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	GitPullRequestIcon,
	RefreshCwIcon,
	RowsIcon,
} from "lucide-react";
import { type FC, type RefObject, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import type {
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { DiffStatBadge } from "../DiffViewer/DiffStats";
import {
	type DiffStyle,
	loadDiffStyle,
	saveDiffStyle,
} from "../DiffViewer/DiffViewer";
import { LocalDiffPanel } from "../DiffViewer/LocalDiffPanel";
import { RemoteDiffPanel } from "../DiffViewer/RemoteDiffPanel";

type GitView = { type: "remote" } | { type: "local"; repoRoot: string };

const GIT_NOT_SETUP_TITLE = "Git is not set up for this chat";
const GIT_NOT_SETUP_SENTENCE = "Git is not set up for this chat.";
const GIT_NOT_SETUP_BODY =
	"Git status will appear here once a Git repository is detected in the workspace.";
const GIT_STATUS_LOADING_TITLE = "Waiting for Git status";
const GIT_STATUS_LOADING_BODY = "Checking the workspace for Git repositories.";

interface DiffStats {
	additions: number;
	deletions: number;
}

interface GitPanelProps {
	/** PR tab data. Omitted if no PR is associated. */
	prTab?: {
		prNumber: number;
		chatId: string;
	};
	/** Repository data from git watcher. */
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	/** Callback to send a refresh to the git watcher. Returns false when disconnected. */
	onRefresh: () => boolean;
	/** Called when the user clicks the Commit button for a working repo. */
	onCommit: (repoRoot: string) => void;
	/** Whether the panel is in expanded/fullscreen mode. */
	isExpanded?: boolean;
	/** Whether the watcher is loading its initial repository state. */
	isGitStatusLoading?: boolean;
	/** Diff status for the remote/branch view (includes PR metadata). */
	remoteDiffStats?: ChatDiffStatus;
	/** Ref to the chat input, forwarded to RemoteDiffPanel. */
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
	/**
	 * Repo roots that have been dirty at some point during this session.
	 * Used to keep a repo's entry visible after its diff goes empty, so
	 * the view switcher does not visibly flip when the agent edits a
	 * file and then reverts it.
	 */
	everDirty?: ReadonlySet<string>;
}

function repoLabel(repoRoot: string): string {
	const segments = repoRoot.split("/").filter(Boolean);
	return segments[segments.length - 1] ?? repoRoot;
}

interface ViewItemBase {
	id: string;
	/** Left-pill label on the trigger (e.g. "Open", "Merged", "Working"). */
	stateLabel: string;
	/** Right-side label on the trigger (e.g. "PR #4847", "coder"). */
	triggerIdentifier: string;
	/** Primary text in the dropdown item (e.g. "PR #4847", "Working"). */
	itemPrimary: string;
	/** Secondary text in the dropdown item (e.g. PR title, repo name). */
	itemSecondary?: string;
	stateClasses: string;
	icon: React.ReactNode;
}

type ViewItem =
	| (ViewItemBase & { kind: "remote" })
	| (ViewItemBase & { kind: "local"; repoRoot: string });

export const GitPanel: FC<GitPanelProps> = ({
	prTab,
	repositories,
	onRefresh,
	onCommit,
	isExpanded,
	isGitStatusLoading = false,
	remoteDiffStats,
	chatInputRef,
	everDirty,
}) => {
	const hasRemoteDiff =
		(remoteDiffStats?.changed_files ?? 0) > 0 ||
		(remoteDiffStats?.additions ?? 0) > 0 ||
		(remoteDiffStats?.deletions ?? 0) > 0;

	const showRemoteTab = Boolean(prTab) || hasRemoteDiff;
	const hasGitContext = repositories.size > 0 || showRemoteTab;
	const isWaitingForGitStatus = !hasGitContext && isGitStatusLoading;

	const prTitle = remoteDiffStats?.pull_request_title;
	const prState = remoteDiffStats?.pull_request_state;
	const prDraft = remoteDiffStats?.pull_request_draft;

	// Compute per-repo diff stats from unified diffs. The React
	// Compiler memoizes these derivations.
	const repoStats = (() => {
		const stats = new Map<string, DiffStats>();
		for (const [root, repo] of repositories.entries()) {
			if (!repo.unified_diff) continue;
			let additions = 0;
			let deletions = 0;
			for (const line of repo.unified_diff.split("\n")) {
				if (line.startsWith("+") && !line.startsWith("+++")) {
					additions++;
				} else if (line.startsWith("-") && !line.startsWith("---")) {
					deletions++;
				}
			}
			if (additions > 0 || deletions > 0) {
				stats.set(root, { additions, deletions });
			}
		}
		return stats;
	})();

	// Union of currently-dirty and ever-dirty repos (still known to
	// the watcher) so a clean-revert does not hide the entry.
	const localRepos = (() => {
		const roots = new Set<string>(repoStats.keys());
		if (everDirty) {
			for (const root of everDirty) {
				if (repositories.has(root)) {
					roots.add(root);
				}
			}
		}
		return Array.from(roots).sort((a, b) => a.localeCompare(b));
	})();

	// Default to the first local repo when nothing has been pushed
	// upstream yet, so the panel opens on the diff the user just made.
	const [view, setView] = useState<GitView>(() => {
		if (!showRemoteTab && localRepos.length > 0) {
			return { type: "local", repoRoot: localRepos[0] };
		}
		return { type: "remote" };
	});

	// If the active view gets hidden, switch to the first available.
	useEffect(() => {
		if (view.type === "remote" && !showRemoteTab) {
			if (localRepos.length > 0) {
				setView({ type: "local", repoRoot: localRepos[0] });
			}
		} else if (view.type === "local") {
			// localRepos includes ever-dirty repos with empty diffs, so
			// the active view stays valid until its root leaves the set.
			if (!localRepos.includes(view.repoRoot)) {
				if (showRemoteTab) {
					setView({ type: "remote" });
				} else if (localRepos.length > 0) {
					setView({ type: "local", repoRoot: localRepos[0] });
				} else {
					setView({ type: "remote" });
				}
			}
		}
	}, [view, showRemoteTab, localRepos]);

	const [diffStyle, setDiffStyle] = useState<DiffStyle>(loadDiffStyle);

	const handleDiffStyleChange = (style: DiffStyle) => {
		saveDiffStyle(style);
		setDiffStyle(style);
	};

	const [spinning, setSpinning] = useState(false);
	const spinTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
	useEffect(() => () => clearTimeout(spinTimerRef.current), []);
	const handleRefresh = () => {
		const sent = onRefresh();
		if (!sent) {
			toast.error("Unable to refresh git status.", {
				id: "git-refresh-disconnected",
				description: "Connection lost. Reconnecting\u2026",
			});
			return;
		}
		setSpinning(true);
		clearTimeout(spinTimerRef.current);
		spinTimerRef.current = setTimeout(() => setSpinning(false), 1000);
	};

	// Reconcile a stale `view` inline so a repo removal never renders
	// as "No changes" for a frame before the effect above updates.
	// When nothing else is available, the remote view falls through;
	// RemoteContent handles its own empty/loading state.
	const effectiveView: GitView =
		view.type === "remote"
			? showRemoteTab
				? view
				: localRepos.length > 0
					? { type: "local", repoRoot: localRepos[0] }
					: view
			: localRepos.includes(view.repoRoot)
				? view
				: showRemoteTab
					? { type: "remote" }
					: localRepos.length > 0
						? { type: "local", repoRoot: localRepos[0] }
						: { type: "remote" };

	const showPrTitleRow = effectiveView.type === "remote" && prTab && prTitle;

	const [isPrTitleTruncated, setIsPrTitleTruncated] = useState(false);
	// Ref callback so the observer attaches whenever the title span
	// mounts: first render, switching back to the PR view, or a title
	// change (via `key` on the span). ResizeObserver fires once on
	// observe, which doubles as the initial truncation check.
	const observePrTitle = (el: HTMLSpanElement | null) => {
		if (!el) {
			return;
		}
		const observer = new ResizeObserver(() => {
			setIsPrTitleTruncated(el.scrollWidth > el.clientWidth);
		});
		observer.observe(el);
		return () => observer.disconnect();
	};

	const remoteHeadBranch = remoteDiffStats?.head_branch;
	const remoteItem: ViewItem | null = showRemoteTab
		? prTab
			? {
					kind: "remote",
					id: "remote",
					stateLabel: prStateLabel(prState, prDraft),
					triggerIdentifier: `PR #${prTab.prNumber}`,
					itemPrimary: `PR #${prTab.prNumber}`,
					itemSecondary: prTitle || undefined,
					stateClasses: prStateClasses(prState, prDraft),
					icon: (
						<PrStateIcon
							state={prState}
							draft={prDraft}
							className="size-3.5! shrink-0"
						/>
					),
				}
			: {
					kind: "remote",
					id: "remote",
					stateLabel: "Branch",
					triggerIdentifier: remoteHeadBranch || "Branch",
					itemPrimary: "Branch",
					itemSecondary: remoteHeadBranch || undefined,
					stateClasses: "text-content-secondary",
					icon: <GitBranchIcon className="size-3.5! shrink-0" />,
				}
		: null;

	const localItems: ViewItem[] = localRepos.map((repoRoot) => ({
		kind: "local" as const,
		id: `local:${repoRoot}`,
		repoRoot,
		stateLabel: "Working",
		triggerIdentifier: repoLabel(repoRoot),
		itemPrimary: "Working",
		itemSecondary: repoLabel(repoRoot),
		stateClasses: "text-content-secondary",
		icon: <CircleDotIcon className="size-3.5! shrink-0 text-content-warning" />,
	}));

	const items: ViewItem[] = [
		...(remoteItem ? [remoteItem] : []),
		...localItems,
	];

	const activeItem: ViewItem | undefined =
		effectiveView.type === "remote"
			? (remoteItem ?? undefined)
			: items.find(
					(item) =>
						item.kind === "local" && item.repoRoot === effectiveView.repoRoot,
				);

	const handleSelectItem = (item: ViewItem) => {
		if (item.kind === "remote") {
			setView({ type: "remote" });
		} else {
			setView({ type: "local", repoRoot: item.repoRoot });
		}
	};

	return (
		<div className="flex h-full flex-col">
			{/* Toolbar */}
			<div className="flex shrink-0 items-center gap-2 px-3 pt-1.5 pb-1">
				<div className="min-w-0 flex-1">
					<GitViewSwitcher
						items={items}
						activeItem={activeItem}
						hasRemoteItem={remoteItem !== null}
						onSelect={handleSelectItem}
					/>
				</div>
				{/* Controls */}
				<div className="flex shrink-0 items-center gap-1">
					<div className="flex h-6 items-stretch overflow-hidden rounded-md border border-solid border-border-default">
						<button
							type="button"
							onClick={() => handleDiffStyleChange("unified")}
							aria-label="Unified diff"
							disabled={!hasGitContext}
							title={!hasGitContext ? GIT_NOT_SETUP_TITLE : undefined}
							className={cn(
								"flex cursor-pointer items-center border-none px-1.5 transition-colors disabled:cursor-default disabled:opacity-50",
								diffStyle === "unified"
									? "bg-surface-quaternary/25 text-content-primary"
									: "bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
							)}
						>
							<RowsIcon className="size-3.5" />
						</button>
						<button
							type="button"
							onClick={() => handleDiffStyleChange("split")}
							aria-label="Split diff"
							disabled={!hasGitContext}
							title={!hasGitContext ? GIT_NOT_SETUP_TITLE : undefined}
							className={cn(
								"flex cursor-pointer items-center border-0 border-l border-solid border-border-default px-1.5 transition-colors disabled:cursor-default disabled:opacity-50",
								diffStyle === "split"
									? "bg-surface-quaternary/25 text-content-primary"
									: "bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
							)}
						>
							<ColumnsIcon className="size-3.5" />
						</button>
					</div>
					{/*
					 * The shared Button applies `disabled:pointer-events-none`,
					 * which would suppress the native `title` tooltip when the
					 * control is disabled. Wrap it in a span so the tooltip is
					 * still reachable on hover in the disabled state.
					 */}
					<span title={!hasGitContext ? GIT_NOT_SETUP_TITLE : undefined}>
						<Button
							variant="subtle"
							size="icon"
							onClick={handleRefresh}
							aria-label="Refresh"
							disabled={!hasGitContext}
							className="size-6 text-content-secondary hover:text-content-primary"
						>
							<RefreshCwIcon
								className={cn(
									"size-3.5",
									spinning && "motion-safe:animate-spin-once",
								)}
							/>
						</Button>
					</span>
				</div>
			</div>
			{/* PR title row: truncates with a hover tooltip only when the
			   visible text is cut off. */}
			{showPrTitleRow && (
				<div className="flex shrink-0 items-center px-3">
					<Tooltip open={isPrTitleTruncated ? undefined : false}>
						<TooltipTrigger asChild>
							<span
								key={prTitle}
								ref={observePrTitle}
								className="min-w-0 truncate text-sm font-medium text-content-primary"
								data-testid="git-panel-pr-title"
							>
								{prTitle}
							</span>
						</TooltipTrigger>
						<TooltipContent
							side="bottom"
							align="start"
							className="max-w-[260px]"
						>
							{prTitle}
						</TooltipContent>
					</Tooltip>
				</div>
			)}
			{/* Content */}
			<div className="min-h-0 flex-1">
				{effectiveView.type === "remote" ? (
					<RemoteContent
						prTab={prTab}
						hasGitContext={hasGitContext}
						isGitStatusLoading={isWaitingForGitStatus}
						isExpanded={isExpanded}
						chatInputRef={chatInputRef}
						diffStyle={diffStyle}
						diffStatus={remoteDiffStats}
					/>
				) : (
					<LocalRepoContent
						repoRoot={effectiveView.repoRoot}
						repo={repositories.get(effectiveView.repoRoot)}
						diffStats={
							repoStats.get(effectiveView.repoRoot) ?? {
								additions: 0,
								deletions: 0,
							}
						}
						onCommit={onCommit}
						isExpanded={isExpanded}
						diffStyle={diffStyle}
						chatInputRef={chatInputRef}
					/>
				)}
			</div>
		</div>
	);
};

// ---------------------------------------------------------------
// Git view switcher: dropdown for the active PR/Branch/Working view.
// ---------------------------------------------------------------

interface GitViewSwitcherProps {
	items: ReadonlyArray<ViewItem>;
	activeItem?: ViewItem;
	/**
	 * Whether a remote (PR or Branch) item exists in `items`. Controls
	 * whether local entries are visually nested (indented) under it.
	 */
	hasRemoteItem: boolean;
	onSelect: (item: ViewItem) => void;
}

const GitViewSwitcher: FC<GitViewSwitcherProps> = ({
	items,
	activeItem,
	hasRemoteItem,
	onSelect,
}) => {
	if (!activeItem) {
		return (
			<div
				className="inline-flex h-6 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 text-xs text-content-secondary"
				data-testid="git-panel-view-switcher"
			>
				<GitBranchIcon className="size-3.5! shrink-0" />
				<span>No changes</span>
			</div>
		);
	}

	const isSingleItem = items.length <= 1;

	const triggerContent = (
		<>
			<span
				className={cn(
					"inline-flex h-full items-center gap-1 rounded-l-md border-0 border-r border-solid border-border-default px-1.5 font-medium leading-none",
					activeItem.stateClasses,
				)}
			>
				<span className="inline-flex size-3.5 shrink-0 items-center justify-center">
					{activeItem.icon}
				</span>
				<span className="whitespace-nowrap">{activeItem.stateLabel}</span>
			</span>
			<span className="inline-flex min-w-0 items-center gap-1 pl-1.5 pr-1 text-content-primary">
				<span className="truncate">{activeItem.triggerIdentifier}</span>
				{!isSingleItem && (
					<ChevronDownIcon className="size-3 shrink-0 opacity-70" />
				)}
			</span>
		</>
	);

	if (isSingleItem) {
		return (
			<div
				className="inline-flex h-6 min-w-0 max-w-full items-stretch overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary text-xs"
				data-testid="git-panel-view-switcher"
			>
				{triggerContent}
			</div>
		);
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="inline-flex h-6 min-w-0 max-w-full cursor-pointer items-stretch overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary text-xs transition-colors hover:bg-surface-secondary"
					data-testid="git-panel-view-switcher"
					aria-label="Switch git view"
				>
					{triggerContent}
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				className="w-(--radix-dropdown-menu-trigger-width) min-w-[240px] p-1"
			>
				{items.map((item) => {
					const isActive = item.id === activeItem.id;
					return (
						<DropdownMenuItem
							key={item.id}
							onSelect={() => onSelect(item)}
							className={cn(
								"flex items-center gap-2 rounded-sm px-2 py-1.5 text-xs",
								// Nest local entries under the remote/PR entry
								// when one exists. Without a parent above them,
								// nesting reads as an orphan indent.
								item.kind === "local" && hasRemoteItem
									? "ml-4 mt-0.5"
									: "w-full",
								isActive && "bg-surface-secondary text-content-primary",
							)}
						>
							<span className="inline-flex size-3.5 shrink-0 items-center justify-center">
								{item.icon}
							</span>
							<span className="whitespace-nowrap font-medium">
								{item.itemPrimary}
							</span>
							{item.itemSecondary && (
								<span className="min-w-0 flex-1 truncate text-content-secondary">
									{item.itemSecondary}
								</span>
							)}
						</DropdownMenuItem>
					);
				})}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

// ---------------------------------------------------------------
// Remote view (branch/PR diff)
// ---------------------------------------------------------------

const RemoteContent: FC<{
	prTab?: { prNumber: number; chatId: string };
	hasGitContext: boolean;
	isGitStatusLoading: boolean;
	isExpanded?: boolean;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
	diffStyle: DiffStyle;
	diffStatus?: ChatDiffStatus;
}> = ({
	prTab,
	hasGitContext,
	isGitStatusLoading,
	isExpanded,
	chatInputRef,
	diffStyle,
	diffStatus,
}) => {
	if (!prTab) {
		return (
			<div className="flex h-full flex-col items-center justify-center p-8 text-center">
				<div className="mb-4 flex size-10 items-center justify-center rounded-lg border border-solid border-border-default bg-surface-secondary">
					{hasGitContext ? (
						<GitCompareArrowsIcon className="size-5 text-content-secondary" />
					) : (
						<GitBranchIcon className="size-5 text-content-secondary" />
					)}
				</div>
				<p className="text-sm font-medium text-content-primary">
					{hasGitContext
						? "No pushed changes yet"
						: isGitStatusLoading
							? GIT_STATUS_LOADING_TITLE
							: GIT_NOT_SETUP_SENTENCE}
				</p>
				<p className="mt-1 max-w-52 text-xs text-content-secondary">
					{hasGitContext
						? "Once commits are pushed, the branch diff will appear here."
						: isGitStatusLoading
							? GIT_STATUS_LOADING_BODY
							: GIT_NOT_SETUP_BODY}
				</p>
			</div>
		);
	}

	return (
		<RemoteDiffPanel
			chatId={prTab.chatId}
			isExpanded={isExpanded}
			chatInputRef={chatInputRef}
			diffStyle={diffStyle}
			diffStatus={diffStatus}
		/>
	);
};

// ---------------------------------------------------------------
// Local view (single repo)
// ---------------------------------------------------------------

const LocalRepoContent: FC<{
	repoRoot: string;
	repo: WorkspaceAgentRepoChanges | undefined;
	diffStats: DiffStats;
	onCommit: (repoRoot: string) => void;
	isExpanded?: boolean;
	diffStyle: DiffStyle;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
}> = ({
	repoRoot,
	repo,
	diffStats,
	onCommit,
	isExpanded,
	diffStyle,
	chatInputRef,
}) => {
	if (!repo) {
		return null;
	}

	return (
		<div className="flex h-full flex-col">
			<RepoHeader
				repoRoot={repoRoot}
				repo={repo}
				diffStats={diffStats}
				onCommit={() => onCommit(repoRoot)}
			/>
			<LocalDiffPanel
				repo={repo}
				isExpanded={isExpanded}
				diffStyle={diffStyle}
				chatInputRef={chatInputRef}
			/>
		</div>
	);
};

// ---------------------------------------------------------------
// Repo header for local view
// ---------------------------------------------------------------

const RepoHeader: FC<{
	repoRoot: string;
	repo: WorkspaceAgentRepoChanges;
	diffStats: DiffStats;
	onCommit: () => void;
}> = ({ repoRoot, repo, diffStats, onCommit }) => {
	return (
		<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1.5">
			<div className="flex min-w-0 items-center gap-1.5 text-[13px] text-content-secondary">
				<GitBranchIcon className="size-3.5 shrink-0" />
				<span className="truncate">
					{repo.branch?.trim() || repoLabel(repoRoot)}
				</span>
				<span className="truncate opacity-50">{repoRoot}</span>
			</div>
			<div className="ml-auto flex shrink-0 items-center gap-1.5">
				<DiffStatBadge
					additions={diffStats.additions}
					deletions={diffStats.deletions}
				/>
				<button
					type="button"
					onClick={onCommit}
					disabled={!repo.unified_diff}
					className="inline-flex cursor-pointer items-center gap-1 rounded-sm border border-solid border-border-default bg-transparent px-2 text-[13px] font-medium leading-5 text-content-primary no-underline transition-colors hover:bg-surface-secondary disabled:pointer-events-none disabled:opacity-50"
				>
					<CheckIcon className="size-3" />
					Commit
				</button>
			</div>
		</div>
	);
};

// ---------------------------------------------------------------
// PR state helpers
// ---------------------------------------------------------------

/** Human-readable state label for the view-switcher trigger. */
function prStateLabel(state: string | undefined, draft: boolean | undefined) {
	if (state === "merged") return "Merged";
	if (state === "closed") return "Closed";
	if (draft) return "Draft";
	return "Open";
}

/** Tailwind classes for the state pill on the view-switcher trigger. */
function prStateClasses(state: string | undefined, draft: boolean | undefined) {
	if (state === "merged") {
		return "text-git-merged-bright";
	}
	if (state === "closed") {
		return "text-git-deleted-bright";
	}
	if (draft) {
		return "text-content-secondary";
	}
	return "text-git-added-bright";
}

// ---------------------------------------------------------------
// PR state icon (compact, for the view switcher)
// ---------------------------------------------------------------

export const PrStateIcon: FC<{
	state?: string;
	draft?: boolean;
	className?: string;
}> = ({ state, draft, className }) => {
	const colorClass = prStateClasses(state, draft);
	if (state === "merged") {
		return <GitMergeIcon className={cn(colorClass, className)} />;
	}
	if (state === "closed") {
		return <GitPullRequestClosedIcon className={cn(colorClass, className)} />;
	}
	if (draft) {
		return <GitPullRequestDraftIcon className={cn(colorClass, className)} />;
	}
	return <GitPullRequestIcon className={cn(colorClass, className)} />;
};
