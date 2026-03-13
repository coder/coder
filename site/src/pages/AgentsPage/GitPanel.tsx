import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	CheckIcon,
	ColumnsIcon,
	FileDiffIcon,
	GitBranchIcon,
	GitCompareArrowsIcon,
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
import { FilesChangedPanel } from "./FilesChangedPanel";
import { RepoChangesPanel } from "./RepoChangesPanel";

type GitView = "remote" | "local";

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
	/** Diff stats for the remote/branch view. */
	remoteDiffStats?: DiffStats;
	/** Diff stats for the local/working tree view. */
	localDiffStats?: DiffStats;
	/** Ref to the chat input, forwarded to FilesChangedPanel. */
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
	localDiffStats,
	chatInputRef,
}) => {
	const hasRemoteStats =
		!!remoteDiffStats &&
		(remoteDiffStats.additions > 0 || remoteDiffStats.deletions > 0);
	const hasLocalStats =
		!!localDiffStats &&
		(localDiffStats.additions > 0 || localDiffStats.deletions > 0);

	// Default to "local" when there are only local changes and no
	// remote stats, so the user sees content immediately.
	const [view, setView] = useState<GitView>(
		!hasRemoteStats && hasLocalStats ? "local" : "remote",
	);

	// Diff style is managed here for the local view only.
	// FilesChangedPanel manages its own diff style internally.
	const [diffStyle, setDiffStyle] = useState<DiffStyle>(loadDiffStyle);

	const handleDiffStyleChange = useCallback((style: DiffStyle) => {
		saveDiffStyle(style);
		setDiffStyle(style);
	}, []);

	return (
		<div className="flex h-full flex-col">
			{/* Toolbar */}
			<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1.5">
				{/* Remote / Local segmented control */}
				<div className="flex h-6 items-stretch overflow-hidden rounded-md text-xs">
					<button
						type="button"
						onClick={() => setView("remote")}
						className={cn(
							"flex cursor-pointer items-center gap-3 border-none font-medium transition-colors outline-none focus-visible:outline-none",
							view === "remote"
								? "bg-surface-quaternary/25 text-content-primary"
								: "bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
							hasRemoteStats ? "pl-3 pr-0" : "px-3",
						)}
					>
						Remote
						{hasRemoteStats && (
							<span
								className={cn(
									"flex -my-px items-center self-stretch transition-opacity",
									view !== "remote" && "opacity-50",
								)}
							>
								<DiffStatBadge
									additions={remoteDiffStats.additions}
									deletions={remoteDiffStats.deletions}
								/>
							</span>
						)}
					</button>
					<button
						type="button"
						onClick={() => setView("local")}
						className={cn(
							"flex cursor-pointer items-center gap-3 border-0 border-l border-solid border-border-default font-medium transition-colors outline-none focus-visible:outline-none",
							view === "local"
								? "bg-surface-quaternary/25 text-content-primary"
								: "bg-surface-primary text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
							hasLocalStats ? "pl-3 pr-0" : "px-3",
						)}
					>
						Local
						{hasLocalStats && (
							<span
								className={cn(
									"flex -my-px items-center self-stretch transition-opacity",
									view !== "local" && "opacity-50",
								)}
							>
								<DiffStatBadge
									additions={localDiffStats.additions}
									deletions={localDiffStats.deletions}
								/>
							</span>
						)}
					</button>
				</div>
				<div className="flex-1" />
				{/* Split / Unified toggle — only shown for local view since
				    FilesChangedPanel has its own toggle built in. */}
				{view === "local" && (
					<div className="flex h-6 items-stretch overflow-hidden rounded-md border border-solid border-border-default text-xs">
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
				)}
			</div>

			{/* Content */}
			<div className="min-h-0 flex-1">
				{view === "remote" ? (
					<RemoteContent
						prTab={prTab}
						isExpanded={isExpanded}
						chatInputRef={chatInputRef}
					/>
				) : (
					<LocalContent
						repositories={repositories}
						onRefresh={onRefresh}
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
}> = ({ prTab, isExpanded, chatInputRef }) => {
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
		<FilesChangedPanel
			chatId={prTab.chatId}
			isExpanded={isExpanded}
			chatInputRef={chatInputRef}
		/>
	);
};

// ---------------------------------------------------------------
// Local view (working tree changes)
// ---------------------------------------------------------------

const LocalContent: FC<{
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	onRefresh: () => void;
	onCommit: (repoRoot: string) => void;
	isExpanded?: boolean;
	diffStyle: DiffStyle;
}> = ({ repositories, onRefresh, onCommit, isExpanded, diffStyle }) => {
	const repoEntries = useMemo(
		() =>
			Array.from(repositories.entries()).sort(([a], [b]) => a.localeCompare(b)),
		[repositories],
	);

	if (repoEntries.length === 0) {
		return (
			<div className="flex h-full flex-col items-center justify-center p-8 text-center">
				<div className="mb-4 flex size-10 items-center justify-center rounded-lg border border-solid border-border-default bg-surface-secondary">
					<FileDiffIcon className="size-5 text-content-secondary" />
				</div>
				<p className="text-sm font-medium text-content-primary">
					No uncommitted changes
				</p>
				<p className="mt-1 max-w-52 text-xs text-content-secondary">
					Local file modifications will appear here as you edit.
				</p>
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col">
			{repoEntries.map(([repoRoot, repo], index) => {
				const showSeparator = index > 0;

				return (
					<section
						key={repoRoot}
						className={cn(
							"flex min-h-0 flex-1 flex-col",
							showSeparator &&
								"border-0 border-t border-solid border-border-default",
						)}
					>
						<RepoHeader
							repoRoot={repoRoot}
							repo={repo}
							onRefresh={onRefresh}
							onCommit={() => onCommit(repoRoot)}
						/>
						<RepoChangesPanel
							repo={repo}
							isExpanded={isExpanded}
							diffStyle={diffStyle}
						/>
					</section>
				);
			})}
		</div>
	);
};

// ---------------------------------------------------------------
// Repo header for local view
// ---------------------------------------------------------------

const RepoHeader: FC<{
	repoRoot: string;
	repo: WorkspaceAgentRepoChanges;
	onRefresh: () => void;
	onCommit: () => void;
}> = ({ repoRoot, repo, onRefresh, onCommit }) => {
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
		<div className="flex shrink-0 items-center gap-2 bg-surface-secondary px-3 py-2">
			{/* Repo identity */}
			<div className="flex min-w-0 flex-1 items-center gap-2">
				<GitBranchIcon className="size-3.5 shrink-0 text-content-secondary" />
				<span className="truncate text-sm font-medium text-content-primary">
					{repo.branch?.trim() || repoTabLabel(repoRoot)}
				</span>
				{repo.branch?.trim() && (
					<span className="truncate text-xs text-content-secondary">
						{repoTabLabel(repoRoot)}
					</span>
				)}
			</div>

			{/* Actions */}
			<div className="flex shrink-0 items-center gap-1">
				<Button
					size="sm"
					onClick={onCommit}
					disabled={!repo.unified_diff}
					className="h-7 gap-1.5 border border-transparent bg-surface-invert-primary px-2 text-xs text-content-invert hover:bg-surface-invert-secondary active:opacity-80"
				>
					<CheckIcon className="size-3" />
					Commit
				</Button>
				<Button
					variant="subtle"
					size="icon"
					onClick={handleRefresh}
					aria-label="Refresh"
					className="h-7 w-7 text-content-secondary hover:text-content-primary"
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
	);
};
