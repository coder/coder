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

type GitView =
	| { type: "remote" }
	| { type: "remote-extra"; index: number }
	| { type: "local"; repoRoot: string };

// Hardcoded extra PR tabs for demo purposes. In production these
// would come from the backend when multi-PR support is added.
const EXTRA_PR_TABS: readonly {
	title: string;
	state: string;
	draft: boolean;
	number: number;
}[] = [
	{
		title: "fix: increase icon sizes for wcag 2.2 compliance",
		state: "open",
		draft: false,
		number: 2,
	},
];

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
	/** Called when the user clicks the Commit button in any repo tab. */
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
	 * Used to keep a repo's tab visible after its diff goes empty, so the
	 * tab strip does not visibly flip when the agent edits a file and
	 * then reverts it.
	 */
	everDirty?: ReadonlySet<string>;
}

function repoTabLabel(repoRoot: string): string {
	const segments = repoRoot.split("/").filter(Boolean);
	return segments[segments.length - 1] ?? repoRoot;
}

/** Builds the icon + label for a given view. */
const viewLabel = (
	view: GitView,
	prTab: GitPanelProps["prTab"],
	prTitle: string | undefined,
	prState: string | undefined,
	prDraft: boolean | undefined,
): { icon: React.ReactNode; text: string } => {
	if (view.type === "remote") {
		if (prTab) {
			return {
				icon: (
					<PrStateIcon
						state={prState}
						draft={prDraft}
						className="!size-4 shrink-0"
					/>
				),
				text: prTitle || `PR #${prTab.prNumber}`,
			};
		}
		return {
			icon: <GitBranchIcon className="!size-3.5 shrink-0" />,
			text: "Branch",
		};
	}
	if (view.type === "remote-extra") {
		const extra = EXTRA_PR_TABS[view.index];
		if (extra) {
			return {
				icon: (
					<PrStateIcon
						state={extra.state}
						draft={extra.draft}
						className="!size-4 shrink-0"
					/>
				),
				text: extra.title,
			};
		}
	}
	if (view.type === "local") {
		return {
			icon: <CircleDotIcon className="!size-3.5 shrink-0 text-content-warning" />,
			text: `Working ${repoTabLabel(view.repoRoot)}`,
		};
	}
	return {
		icon: <GitBranchIcon className="!size-3.5 shrink-0" />,
		text: "Unknown",
	};
};

interface GitViewSelectorProps {
	view: GitView;
	setView: (view: GitView) => void;
	showRemoteTab: boolean;
	prTab: GitPanelProps["prTab"];
	prTitle: string | undefined;
	prState: string | undefined;
	prDraft: boolean | undefined;
	localRepos: string[];
}

const GitViewSelector: FC<GitViewSelectorProps> = ({
	view,
	setView,
	showRemoteTab,
	prTab,
	prTitle,
	prState,
	prDraft,
	localRepos,
}) => {
	const allViews: GitView[] = [
		...(showRemoteTab ? [{ type: "remote" } as const] : []),
		...EXTRA_PR_TABS.map((_, i) => ({ type: "remote-extra" as const, index: i })),
		...localRepos.map((r) => ({ type: "local" as const, repoRoot: r })),
	];

	const active = viewLabel(view, prTab, prTitle, prState, prDraft);

	// Single tab: just show the label inline, no dropdown.
	if (allViews.length <= 1) {
		return (
			<div className="flex min-w-0 flex-1 items-center gap-1.5 py-1.5 text-xs font-medium text-content-primary">
				{active.icon}
				<span className="truncate">{active.text}</span>
			</div>
		);
	}

	// Multiple tabs: dropdown selector.
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 border-0 bg-transparent py-1.5 text-xs font-medium text-content-primary"
				>
					{active.icon}
					<span className="truncate">{active.text}</span>
					<ChevronDownIcon className="size-3.5 shrink-0 opacity-60" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className="max-w-64">
				{allViews.map((v) => {
					const label = viewLabel(v, prTab, prTitle, prState, prDraft);
					const isActive =
						v.type === view.type &&
						(v.type === "remote" ||
							(v.type === "remote-extra" &&
								view.type === "remote-extra" &&
								v.index === view.index) ||
							(v.type === "local" &&
								view.type === "local" &&
								v.repoRoot === view.repoRoot));
					const key =
						v.type === "remote"
							? "remote"
							: v.type === "remote-extra"
								? `remote-extra-${v.index}`
								: v.repoRoot;
					return (
						<DropdownMenuItem
							key={key}
							onClick={() => setView(v)}
							className={cn(
								"gap-1.5 text-xs",
								isActive && "bg-surface-secondary text-content-primary",
							)}
						>
							{label.icon}
							<span className="truncate">{label.text}</span>
							{isActive && (
								<CheckIcon className="ml-auto size-3.5 shrink-0" />
							)}
						</DropdownMenuItem>
					);
				})}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

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

	// Compute per-repo diff stats from unified diffs.
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
	// the watcher) so a clean-revert does not hide the tab.
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

	// Default to the first local repo when there are only local
	// changes and no remote stats.
	const [view, setView] = useState<GitView>(() => {
		if (!showRemoteTab && localRepos.length > 0) {
			return { type: "local", repoRoot: localRepos[0] };
		}
		return { type: "remote" };
	});

	// If the active tab gets hidden, switch to the first available.
	useEffect(() => {
		if (view.type === "remote" && !showRemoteTab) {
			if (localRepos.length > 0) {
				setView({ type: "local", repoRoot: localRepos[0] });
			}
		} else if (view.type === "local") {
			// localRepos includes ever-dirty repos with empty diffs, so
			// the active tab stays valid until its root leaves the set.
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

	return (
		<div className="flex h-full flex-col">
			{/* Toolbar */}
			<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3">
				{/* Tab selector — dropdown when multiple tabs, inline label when single */}
				<GitViewSelector
					view={view}
					setView={setView}
					showRemoteTab={showRemoteTab}
						prTab={prTab}
					prTitle={prTitle}
						prState={prState}
						prDraft={prDraft}
						localRepos={localRepos}
					/>
				{/* Controls */}
				<div className="flex shrink-0 items-center gap-1 py-1.5">
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
			{/* Content */}
			<div className="min-h-0 flex-1">
				{view.type === "remote" || view.type === "remote-extra" ? (
					<RemoteContent
						prTab={prTab}
						hasGitContext={hasGitContext}
						isGitStatusLoading={isWaitingForGitStatus}
						isExpanded={isExpanded}
						chatInputRef={chatInputRef}
						diffStyle={diffStyle}
						diffStatus={remoteDiffStats}
					/>
				) : view.type === "local" ? (
					<LocalRepoContent
						repoRoot={view.repoRoot}
						repo={repositories.get(view.repoRoot)}
						diffStats={
							repoStats.get(view.repoRoot) ?? { additions: 0, deletions: 0 }
						}
						onCommit={onCommit}
						isExpanded={isExpanded}
						diffStyle={diffStyle}
						chatInputRef={chatInputRef}
					/>
				) : null}
			</div>
		</div>
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
					{repo.branch?.trim() || repoTabLabel(repoRoot)}
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
					className="inline-flex cursor-pointer items-center gap-1 rounded-sm border border-solid border-border-default bg-transparent px-2 text-[13px] font-medium leading-5 text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary disabled:pointer-events-none disabled:opacity-50"
				>
					<CheckIcon className="size-3" />
					Commit
				</button>
			</div>
		</div>
	);
};

// ---------------------------------------------------------------
// PR state icon (compact, for the tab bar)
// ---------------------------------------------------------------

export const PrStateIcon: FC<{
	state?: string;
	draft?: boolean;
	className?: string;
}> = ({ state, draft, className }) => {
	if (state === "merged") {
		return <GitMergeIcon className={cn("text-git-merged-bright", className)} />;
	}
	if (state === "closed") {
		return (
			<GitPullRequestClosedIcon
				className={cn("text-git-deleted-bright", className)}
			/>
		);
	}
	if (draft) {
		return (
			<GitPullRequestDraftIcon
				className={cn("text-content-secondary", className)}
			/>
		);
	}
	return (
		<GitPullRequestIcon className={cn("text-git-added-bright", className)} />
	);
};
