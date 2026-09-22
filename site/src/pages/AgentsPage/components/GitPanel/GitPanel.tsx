import { cn } from "cn";
import {
	CheckIcon,
	CircleDotIcon,
	ExternalLinkIcon,
	GitBranchIcon,
	GitCompareArrowsIcon,
	GitMergeIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	GitPullRequestIcon,
	RefreshCwIcon,
} from "lucide-react";
import { type FC, type RefObject, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import type * as TypesGen from "#/api/typesGenerated";
import type {
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { parsePullRequestUrl } from "../../utils/pullRequest";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { DiffStatBadge } from "../DiffViewer/DiffStats";
import {
	type DiffStyle,
	DiffStyleToggle,
	loadDiffStyle,
	saveDiffStyle,
} from "../DiffViewer/DiffViewer";
import { LocalDiffPanel } from "../DiffViewer/LocalDiffPanel";
import { RemoteDiffPanel } from "../DiffViewer/RemoteDiffPanel";

type GitView =
	| { type: "remote"; refId: string }
	| { type: "local"; repoRoot: string };

// The view id for a tracked ref. Rows without origin and branch
// share the empty-key id.
const viewIdFor = (status: ChatDiffStatus): string =>
	`remote:${status.remote_origin ?? ""}:${status.git_branch ?? ""}`;

// A keyless row predates the keyed schema and the API cannot select
// it: an empty selector means the primary ref. Only the primary can
// stay selectable; any other keyless row would fetch the primary's
// diff under its own title.
const isSelectableRef = (status: ChatDiffStatus, index: number): boolean =>
	index === 0 || Boolean(status.remote_origin || status.git_branch);

// Line counts for one repo's unified diff.
const countDiffLines = (unifiedDiff: string): DiffStats => {
	let additions = 0;
	let deletions = 0;
	for (const line of unifiedDiff.split("\n")) {
		if (line.startsWith("+") && !line.startsWith("+++")) {
			additions++;
		} else if (line.startsWith("-") && !line.startsWith("---")) {
			deletions++;
		}
	}
	return { additions, deletions };
};

// Per-repo diff stats, repos with no net change excluded.
const computeRepoStats = (
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>,
): Map<string, DiffStats> => {
	const stats = new Map<string, DiffStats>();
	for (const [root, repo] of repositories.entries()) {
		if (!repo.unified_diff) {
			continue;
		}
		const repoStat = countDiffLines(repo.unified_diff);
		if (repoStat.additions > 0 || repoStat.deletions > 0) {
			stats.set(root, repoStat);
		}
	}
	return stats;
};

// Union of currently-dirty and ever-dirty repos (still known to
// the watcher), so a clean-revert does not hide the entry.
const computeLocalRepos = (
	repoStats: ReadonlyMap<string, DiffStats>,
	everDirty: ReadonlySet<string> | undefined,
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>,
): string[] => {
	const roots = new Set<string>(repoStats.keys());
	for (const root of everDirty ?? []) {
		if (repositories.has(root)) {
			roots.add(root);
		}
	}
	return Array.from(roots).sort((a, b) => a.localeCompare(b));
};

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
	/** The chat whose remote diff is displayed. */
	chatId: string;
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
	remoteDiffStats?: readonly ChatDiffStatus[];
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

// Inputs the view reconciliation needs for one render.
interface ViewFallbackInput {
	localRepos: readonly string[];
	remoteDiffStats?: readonly ChatDiffStatus[];
	primaryRefId: string;
}

// The view to render when the active one is no longer valid: the
// remote tab hid, its ref lost its row, or its repo left the set.
// Falls back to the first local repo, then the primary ref. When
// nothing is left, the remote view stands and RemoteContent shows
// its own empty state.
const fallbackView = (view: GitView, input: ViewFallbackInput): GitView => {
	const { localRepos, primaryRefId } = input;
	const showRemoteTab = (input.remoteDiffStats?.length ?? 0) > 0;

	// The remote tab can hide while a remote view is active.
	if (view.type === "remote" && !showRemoteTab) {
		if (localRepos.length > 0) {
			return { type: "local", repoRoot: localRepos[0] };
		}
		return { type: "remote", refId: primaryRefId };
	}
	if (view.type === "remote") {
		const isTracked =
			view.refId === input.primaryRefId ||
			(input.remoteDiffStats ?? []).some(
				(status, index) =>
					isSelectableRef(status, index) && viewIdFor(status) === view.refId,
			);
		if (isTracked) {
			return view;
		}
		return { type: "remote", refId: primaryRefId };
	}
	// localRepos includes ever-dirty repos with empty diffs, so the
	// active view stays valid until its root leaves the set.
	if (localRepos.includes(view.repoRoot)) {
		return view;
	}
	if (showRemoteTab) {
		return { type: "remote", refId: primaryRefId };
	}
	if (localRepos.length > 0) {
		return { type: "local", repoRoot: localRepos[0] };
	}
	return { type: "remote", refId: primaryRefId };
};

// One switcher entry for a tracked ref: PR rows show their number
// and state, branch-only rows show the branch name.
const buildRemoteItem = (status: ChatDiffStatus): ViewItem => {
	const prNumber =
		status.pr_number ?? parsePullRequestUrl(status.url ?? "")?.number;
	const state = status.pull_request_state;
	const draft = status.pull_request_draft;
	// head_branch falls back for legacy rows that predate git_branch.
	const branchName = status.git_branch || status.head_branch;
	if (prNumber) {
		return {
			kind: "remote",
			id: viewIdFor(status),
			stateLabel: prStateLabel(state, draft),
			triggerIdentifier: `PR #${prNumber}`,
			itemPrimary: `PR #${prNumber}`,
			itemSecondary: status.pull_request_title || undefined,
			stateClasses: prStateClasses(state, draft),
			icon: (
				<PrStateIcon
					state={state}
					draft={draft}
					className="size-3.5! shrink-0"
				/>
			),
		};
	}
	return {
		kind: "remote",
		id: viewIdFor(status),
		stateLabel: "Branch",
		triggerIdentifier: branchName || "Branch",
		itemPrimary: "Branch",
		itemSecondary: branchName || undefined,
		stateClasses: "text-content-secondary",
		icon: <GitBranchIcon className="size-3.5 shrink-0" />,
	};
};

// The full ref selector for a fetch. Keyless rows cannot carry
// one, so they fetch with no selector and the server picks the
// primary.
const refSelectorFor = (
	status: ChatDiffStatus | undefined,
): TypesGen.DiffStatusRef | undefined => {
	if (!status?.remote_origin && !status?.git_branch) {
		return undefined;
	}
	return {
		remote_origin: status.remote_origin ?? "",
		git_branch: status.git_branch ?? "",
	};
};

export const GitPanel: FC<GitPanelProps> = ({
	chatId,
	repositories,
	onRefresh,
	onCommit,
	isExpanded,
	isGitStatusLoading = false,
	remoteDiffStats,
	chatInputRef,
	everDirty,
}) => {
	const showRemoteTab = (remoteDiffStats?.length ?? 0) > 0;
	const hasGitContext = repositories.size > 0 || showRemoteTab;

	// The React Compiler memoizes these derivations.
	const repoStats = computeRepoStats(repositories);
	const localRepos = computeLocalRepos(repoStats, everDirty, repositories);

	// Default to the first local repo when nothing has been pushed
	// upstream yet, so the panel opens on the diff the user just made.
	const primaryRefStatus = remoteDiffStats?.[0];
	const primaryRefId = primaryRefStatus
		? viewIdFor(primaryRefStatus)
		: "remote";
	const [view, setView] = useState<GitView>(() => {
		if (!showRemoteTab && localRepos.length > 0) {
			return { type: "local", repoRoot: localRepos[0] };
		}
		return { type: "remote", refId: primaryRefId };
	});

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

	// Deriving this in render avoids an effect that would otherwise
	// need to compare refIds to keep from re-setting itself.
	const effectiveView = fallbackView(view, {
		localRepos,
		remoteDiffStats,
		primaryRefId,
	});

	const isRemoteView = effectiveView.type === "remote";

	// The selected status row, or the primary when the view has no
	// remote selector, so the panel always shows a diff.
	const viewRefId =
		effectiveView.type === "remote" ? effectiveView.refId : undefined;
	const selectedRemote =
		remoteDiffStats?.find(
			(status) => viewRefId !== undefined && viewIdFor(status) === viewRefId,
		) ?? remoteDiffStats?.[0];

	const prTitle = selectedRemote?.pull_request_title;
	const selectedPrNumber =
		selectedRemote?.pr_number ??
		parsePullRequestUrl(selectedRemote?.url ?? "")?.number;

	// The selected ref can be branch-only with no PR, so the title
	// row needs a PR number to belong to.
	const showPrTitleRow =
		isRemoteView && Boolean(selectedPrNumber) && Boolean(prTitle);

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

	const remoteItems: ViewItem[] =
		showRemoteTab && remoteDiffStats
			? remoteDiffStats.filter(isSelectableRef).map(buildRemoteItem)
			: [];

	const localItems: ViewItem[] = localRepos.map((repoRoot) => ({
		kind: "local",
		id: `local:${repoRoot}`,
		repoRoot,
		stateLabel: "Working",
		triggerIdentifier: repoLabel(repoRoot),
		itemPrimary: "Working",
		itemSecondary: repoLabel(repoRoot),
		stateClasses: "text-content-secondary",
		icon: <CircleDotIcon className="size-3.5! shrink-0 text-content-warning" />,
	}));

	const items: ViewItem[] = [...remoteItems, ...localItems];

	// The server synthesizes /tree/<branch> URLs for refs without a
	// PR, so the link needs a PR number, not just a URL.
	const viewPrUrl =
		isRemoteView && selectedPrNumber ? selectedRemote?.url : undefined;

	const activeRepoRoot =
		effectiveView.type === "local" ? effectiveView.repoRoot : undefined;
	const activeRepo =
		activeRepoRoot !== undefined ? repositories.get(activeRepoRoot) : undefined;
	const activeItem: ViewItem | undefined = isRemoteView
		? items.find((item) => item.id === viewRefId)
		: items.find(
				(item) => item.kind === "local" && item.repoRoot === activeRepoRoot,
			);

	const handleSelectItem = (item: ViewItem) => {
		if (item.kind === "remote") {
			setView({ type: "remote", refId: item.id });
		} else {
			setView({ type: "local", repoRoot: item.repoRoot });
		}
	};

	return (
		<div className="flex h-full flex-col">
			{/* Toolbar. The 16px text gutter matches the summary tab's
			   p-4, and a 24px control under pt-4 centers on the same
			   line as a leading-6 paragraph. The switcher's border and
			   inner padding hang outside the gutter so its text lands
			   on it. */}
			<div className="flex shrink-0 items-center gap-2 pl-2.5 pr-4 pt-4 pb-1">
				<div className="flex min-w-0 flex-1 items-center gap-1.5">
					<GitViewSwitcher
						items={items}
						activeItem={activeItem}
						onSelect={handleSelectItem}
					/>
					{/*
					 * The shared Button applies `disabled:pointer-events-none`,
					 * which would suppress the native `title` tooltip when the
					 * control is disabled. Wrap it in a span so the tooltip is
					 * still reachable on hover in the disabled state.
					 */}
					<span
						className="shrink-0"
						title={!hasGitContext ? GIT_NOT_SETUP_TITLE : undefined}
					>
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
				{/* The primary action follows the view: a PR opens on its
				   host, a working repo commits. */}
				{viewPrUrl && (
					<a
						href={viewPrUrl}
						target="_blank"
						rel="noreferrer"
						className="inline-flex h-6 shrink-0 items-center gap-1 text-xs font-medium text-content-link no-underline hover:underline"
					>
						View PR
						<ExternalLinkIcon className="size-3" />
					</a>
				)}
				{activeRepoRoot !== undefined && activeRepo && (
					<button
						type="button"
						onClick={() => onCommit(activeRepoRoot)}
						disabled={!activeRepo.unified_diff}
						className="inline-flex h-6 shrink-0 cursor-pointer items-center gap-1 rounded-md border border-solid border-border-default bg-surface-primary px-2 text-xs font-medium text-content-primary transition-colors hover:bg-surface-secondary disabled:pointer-events-none disabled:opacity-50"
					>
						<CheckIcon className="size-3" />
						Commit
					</button>
				)}
			</div>
			{/* PR title row: truncates with a hover tooltip only when the
			   visible text is cut off. */}
			{showPrTitleRow && (
				<div className="flex shrink-0 items-center px-4">
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
				{isRemoteView ? (
					<RemoteContent
						chatId={chatId}
						hasGitContext={hasGitContext}
						isGitStatusLoading={isGitStatusLoading}
						isExpanded={isExpanded}
						chatInputRef={chatInputRef}
						diffStyle={diffStyle}
						onDiffStyleChange={handleDiffStyleChange}
						diffStatus={selectedRemote}
						remoteRef={refSelectorFor(selectedRemote)}
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
						isExpanded={isExpanded}
						diffStyle={diffStyle}
						onDiffStyleChange={handleDiffStyleChange}
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
	onSelect: (item: ViewItem) => void;
}

const GitViewSwitcher: FC<GitViewSwitcherProps> = ({
	items,
	activeItem,
	onSelect,
}) => {
	const [open, setOpen] = useState(false);
	if (!activeItem) {
		return (
			<div
				className="inline-flex h-6 items-center gap-1.5 border border-solid border-transparent px-1.5 text-xs text-content-secondary"
				data-testid="git-panel-view-switcher"
			>
				<GitBranchIcon className="size-3.5! shrink-0" />
				<span>No changes</span>
			</div>
		);
	}

	// With nothing to switch to, the view is a plain label rather
	// than a menu trigger. It keeps the trigger's border box and
	// inner padding so the text does not shift when a second view
	// appears.
	if (items.length <= 1) {
		return (
			<div
				className="inline-flex h-6 min-w-0 max-w-full items-center gap-1.5 border border-solid border-transparent px-1.5 text-xs"
				data-testid="git-panel-view-switcher"
			>
				<span
					className={cn(
						"inline-flex size-3.5 shrink-0 items-center justify-center",
						activeItem.stateClasses,
					)}
				>
					{activeItem.icon}
				</span>
				<span className="truncate font-medium text-content-primary">
					{activeItem.triggerIdentifier}
				</span>
			</div>
		);
	}

	const remoteItems = items.filter((item) => item.kind === "remote");
	const localItems = items.filter((item) => item.kind === "local");
	const renderItem = (item: ViewItem) => (
		<DropdownMenuRadioItem key={item.id} value={item.id} className="gap-2">
			<span className="inline-flex size-3.5 shrink-0 items-center justify-center">
				{item.icon}
			</span>
			<span className="whitespace-nowrap">{item.itemPrimary}</span>
			{item.itemSecondary && (
				<span className="min-w-0 flex-1 truncate font-normal text-content-secondary">
					{item.itemSecondary}
				</span>
			)}
		</DropdownMenuRadioItem>
	);

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="inline-flex h-6 min-w-0 max-w-full cursor-pointer items-stretch overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary text-xs transition-colors hover:bg-surface-secondary"
					data-testid="git-panel-view-switcher"
					aria-label="Switch git view"
				>
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
						<ChevronDownIcon
							open={open}
							className="size-3 shrink-0 text-content-secondary"
						/>
					</span>
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				className="w-64 p-1 [&_[role^=menuitem]]:py-1 [&_[role^=menuitem]]:text-xs [&_svg]:size-3.5!"
			>
				<DropdownMenuRadioGroup
					value={activeItem.id}
					onValueChange={(id) => {
						const item = items.find((candidate) => candidate.id === id);
						if (item) {
							onSelect(item);
						}
					}}
				>
					{remoteItems.map(renderItem)}
					{remoteItems.length > 0 && localItems.length > 0 && (
						<DropdownMenuSeparator className="my-1" />
					)}
					{localItems.map(renderItem)}
				</DropdownMenuRadioGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

// ---------------------------------------------------------------
// Remote view (branch/PR diff)
// ---------------------------------------------------------------

const RemoteContent: FC<{
	chatId?: string;
	hasGitContext: boolean;
	isGitStatusLoading: boolean;
	isExpanded?: boolean;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
	diffStyle: DiffStyle;
	onDiffStyleChange: (style: DiffStyle) => void;
	diffStatus?: ChatDiffStatus;
	remoteRef?: TypesGen.DiffStatusRef;
}> = ({
	chatId,
	hasGitContext,
	isGitStatusLoading,
	isExpanded,
	chatInputRef,
	diffStyle,
	onDiffStyleChange,
	diffStatus,
	remoteRef,
}) => {
	if (!chatId || !diffStatus) {
		// Loading beats every settled state: the status row that
		// selects the message can still arrive.
		let title = GIT_NOT_SETUP_SENTENCE;
		let body = GIT_NOT_SETUP_BODY;
		if (isGitStatusLoading) {
			title = GIT_STATUS_LOADING_TITLE;
			body = GIT_STATUS_LOADING_BODY;
		} else if (hasGitContext) {
			title = "No pushed changes yet";
			body = "Once commits are pushed, the branch diff will appear here.";
		}
		return (
			<div className="flex h-full flex-col items-center justify-center p-8 text-center">
				<div className="mb-4 flex size-10 items-center justify-center rounded-lg border border-solid border-border-default bg-surface-secondary">
					{hasGitContext ? (
						<GitCompareArrowsIcon className="size-5 text-content-secondary" />
					) : (
						<GitBranchIcon className="size-5 text-content-secondary" />
					)}
				</div>
				<p className="text-sm font-medium text-content-primary">{title}</p>
				<p className="mt-1 max-w-52 text-xs text-content-secondary">{body}</p>
			</div>
		);
	}

	return (
		<RemoteDiffPanel
			chatId={chatId}
			isExpanded={isExpanded}
			chatInputRef={chatInputRef}
			diffStyle={diffStyle}
			onDiffStyleChange={onDiffStyleChange}
			diffStatus={diffStatus}
			remoteRef={remoteRef}
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
	isExpanded?: boolean;
	diffStyle: DiffStyle;
	onDiffStyleChange: (style: DiffStyle) => void;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
}> = ({
	repoRoot,
	repo,
	diffStats,
	isExpanded,
	diffStyle,
	onDiffStyleChange,
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
				diffStyle={diffStyle}
				onDiffStyleChange={onDiffStyleChange}
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
	diffStyle: DiffStyle;
	onDiffStyleChange: (style: DiffStyle) => void;
}> = ({ repoRoot, repo, diffStats, diffStyle, onDiffStyleChange }) => {
	return (
		<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-4 py-1.5">
			<div className="flex min-w-0 items-center gap-1.5 text-[13px] text-content-secondary">
				<GitBranchIcon className="size-3.5 shrink-0" />
				<span className="truncate">
					{repo.branch?.trim() || repoLabel(repoRoot)}
				</span>
				<span className="truncate opacity-50">{repoRoot}</span>
			</div>
			<div className="ml-auto flex shrink-0 items-center gap-2">
				<DiffStatBadge
					additions={diffStats.additions}
					deletions={diffStats.deletions}
				/>
				<DiffStyleToggle value={diffStyle} onChange={onDiffStyleChange} />
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
