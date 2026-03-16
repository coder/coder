import type {
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	CheckIcon,
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
import {
	type FC,
	type RefObject,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { DiffStatBadge } from "./DiffStats";
import { type DiffStyle, loadDiffStyle, saveDiffStyle } from "./DiffViewer";
import { LocalDiffPanel } from "./LocalDiffPanel";
import { RemoteDiffPanel } from "./RemoteDiffPanel";

type GitView = { type: "remote" } | { type: "local"; repoRoot: string };

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
	/** Callback to send a refresh to the git watcher. */
	onRefresh: () => void;
	/** Called when the user clicks the Commit button in any repo tab. */
	onCommit: (repoRoot: string) => void;
	/** Whether the panel is in expanded/fullscreen mode. */
	isExpanded?: boolean;
	/** Diff status for the remote/branch view (includes PR metadata). */
	remoteDiffStats?: ChatDiffStatus;
	/** Ref to the chat input, forwarded to RemoteDiffPanel. */
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
}

function repoTabLabel(repoRoot: string): string {
	const segments = repoRoot.split("/").filter(Boolean);
	return segments[segments.length - 1] ?? repoRoot;
}

export const GitPanel: FC<GitPanelProps> = ({
	prTab,
	repositories,
	onRefresh,
	onCommit,
	isExpanded,
	remoteDiffStats,
	chatInputRef,
}) => {
	const hasRemoteStats =
		!!remoteDiffStats &&
		(remoteDiffStats.additions > 0 || remoteDiffStats.deletions > 0);

	const showRemoteTab = !!prTab || hasRemoteStats;

	const prTitle = remoteDiffStats?.pull_request_title;
	const prState = remoteDiffStats?.pull_request_state;
	const prDraft = remoteDiffStats?.pull_request_draft;

	// Compute per-repo diff stats from unified diffs.
	const repoStats = useMemo(() => {
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
	}, [repositories]);

	const localRepos = useMemo(
		() => Array.from(repoStats.keys()).sort((a, b) => a.localeCompare(b)),
		[repoStats],
	);

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
			if (!repoStats.has(view.repoRoot)) {
				if (showRemoteTab) {
					setView({ type: "remote" });
				} else if (localRepos.length > 0) {
					setView({ type: "local", repoRoot: localRepos[0] });
				}
			}
		}
	}, [view, showRemoteTab, localRepos, repoStats]);

	const [diffStyle, setDiffStyle] = useState<DiffStyle>(loadDiffStyle);

	const handleDiffStyleChange = useCallback((style: DiffStyle) => {
		saveDiffStyle(style);
		setDiffStyle(style);
	}, []);

	const [spinning, setSpinning] = useState(false);
	const spinTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
	useEffect(() => () => clearTimeout(spinTimerRef.current), []);
	const handleRefresh = useCallback(() => {
		onRefresh();
		setSpinning(true);
		clearTimeout(spinTimerRef.current);
		spinTimerRef.current = setTimeout(() => setSpinning(false), 1000);
	}, [onRefresh]);

	return (
		<div className="flex h-full flex-col">
			{/* Toolbar */}
			<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3">
				{/* Tabs — scrollable when they overflow */}
				<ScrollArea
					className="min-w-0 flex-1"
					orientation="horizontal"
					scrollBarClassName="h-1.5"
				>
					<div className="flex items-center gap-0.5 py-1.5 text-xs">
						{showRemoteTab && (
							<Button
								variant="outline"
								size="lg"
								onClick={() => setView({ type: "remote" })}
								className={cn(
									"shrink-0 h-6 min-w-0 gap-1.5 px-2 py-0 bg-surface-primary",
									view.type === "remote" &&
										"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
								)}
							>
								{prTab ? (
									<>
										<PrStateIcon
											state={prState}
											draft={prDraft}
											className="!size-4 shrink-0"
										/>
										<span className="truncate">
											{prTitle || `PR #${prTab.prNumber}`}
										</span>
									</>
								) : (
									<>
										<GitBranchIcon className="!size-3.5 shrink-0" />
										<span className="truncate">Branch</span>
									</>
								)}
							</Button>
						)}
						{localRepos.map((repoRoot) => {
							const isActive =
								view.type === "local" && view.repoRoot === repoRoot;
							return (
								<Button
									key={repoRoot}
									variant="outline"
									size="lg"
									onClick={() => setView({ type: "local", repoRoot })}
									className={cn(
										"shrink-0 h-6 min-w-0 gap-1.5 px-2 py-0 bg-surface-primary",
										isActive &&
											"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
									)}
								>
									<CircleDotIcon
										className={cn(
											"!size-3.5 shrink-0",
											isActive ? "text-amber-400" : "text-amber-400/60",
										)}
									/>
									<span className="truncate">
										Working{" "}
										<span className="opacity-50">{repoTabLabel(repoRoot)}</span>
									</span>
								</Button>
							);
						})}
					</div>
				</ScrollArea>
				{/* Controls */}
				<div className="flex shrink-0 items-center gap-1 py-1.5">
					<div className="flex h-6 items-stretch overflow-hidden rounded-md border border-solid border-border-default">
						<button
							type="button"
							onClick={() => handleDiffStyleChange("unified")}
							aria-label="Unified diff"
							className={cn(
								"flex cursor-pointer items-center border-none px-1.5 transition-colors",
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
							className={cn(
								"flex cursor-pointer items-center border-0 border-l border-solid border-border-default px-1.5 transition-colors",
								diffStyle === "split"
									? "bg-surface-quaternary/25 text-content-primary"
									: "bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
							)}
						>
							<ColumnsIcon className="size-3.5" />
						</button>
					</div>
					<Button
						variant="subtle"
						size="icon"
						onClick={handleRefresh}
						aria-label="Refresh"
						className="h-6 w-6 text-content-secondary hover:text-content-primary"
					>
						<RefreshCwIcon
							className={cn(
								"size-3.5",
								spinning && "motion-safe:animate-spin-once",
							)}
						/>
					</Button>
				</div>
			</div>
			{/* Content */}
			<div className="min-h-0 flex-1">
				{view.type === "remote" ? (
					<RemoteContent
						prTab={prTab}
						isExpanded={isExpanded}
						chatInputRef={chatInputRef}
						diffStyle={diffStyle}
						diffStatus={remoteDiffStats}
					/>
				) : (
					<LocalRepoContent
						repoRoot={view.repoRoot}
						repo={repositories.get(view.repoRoot)}
						diffStats={
							repoStats.get(view.repoRoot) ?? { additions: 0, deletions: 0 }
						}
						onCommit={onCommit}
						isExpanded={isExpanded}
						diffStyle={diffStyle}
					/>
				)}
			</div>
		</div>
	);
};
// ---------------------------------------------------------------
// Remote view (branch/PR diff)
// ---------------------------------------------------------------

const RemoteContent: FC<{
	prTab?: { prNumber: number; chatId: string };
	isExpanded?: boolean;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
	diffStyle: DiffStyle;
	diffStatus?: ChatDiffStatus;
}> = ({ prTab, isExpanded, chatInputRef, diffStyle, diffStatus }) => {
	if (!prTab) {
		return (
			<div className="flex h-full flex-col items-center justify-center p-8 text-center">
				<div className="mb-4 flex size-10 items-center justify-center rounded-lg border border-solid border-border-default bg-surface-secondary">
					<GitCompareArrowsIcon className="size-5 text-content-secondary" />
				</div>
				<p className="text-sm font-medium text-content-primary">
					No pushed changes yet
				</p>
				<p className="mt-1 max-w-52 text-xs text-content-secondary">
					Once commits are pushed, the branch diff will appear here.
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
}> = ({ repoRoot, repo, diffStats, onCommit, isExpanded, diffStyle }) => {
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

const PrStateIcon: FC<{
	state?: string;
	draft?: boolean;
	className?: string;
}> = ({ state, draft, className }) => {
	if (state === "merged") {
		return <GitMergeIcon className={cn("text-purple-400", className)} />;
	}
	if (state === "closed") {
		return (
			<GitPullRequestClosedIcon className={cn("text-red-400", className)} />
		);
	}
	if (draft) {
		return (
			<GitPullRequestDraftIcon
				className={cn("text-content-secondary", className)}
			/>
		);
	}
	return <GitPullRequestIcon className={cn("text-green-400", className)} />;
};
