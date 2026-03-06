import { parsePatchFiles } from "@pierre/diffs";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Columns2Icon,
	FolderIcon,
	GitPullRequestIcon,
	MaximizeIcon,
	MinimizeIcon,
	PanelLeftIcon,
	Rows3Icon,
} from "lucide-react";
import { type FC, useCallback, useId, useMemo, useState } from "react";
import { cn } from "utils/cn";
import { DiffStatNumbers } from "./DiffStats";
import { DIFF_STYLE_KEY, type DiffStyle, loadDiffStyle } from "./DiffViewer";
import { FilesChangedPanel } from "./FilesChangedPanel";
import { RepoChangesPanel } from "./RepoChangesPanel";

interface SidebarTabViewProps {
	/** PR tab data. Omitted if no PR is associated. */
	prTab?: {
		prNumber: number;
		chatId: string;
	};
	/** Repository tabs from git watcher. */
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	/** Workspace info for the header. */
	workspace?: {
		name: string;
		ownerName: string;
	};
	/** Callback to send a refresh to the git watcher. */
	onRefresh: () => void;
	/** Called when the user clicks the Commit button in any repo tab. */
	onCommit: (repoRoot: string) => void;
	/** Whether the panel is in expanded/fullscreen mode. */
	isExpanded: boolean;
	/** Callback to toggle expanded state. */
	onToggleExpanded: () => void;
	/** Whether the left sidebar is collapsed. */
	isSidebarCollapsed?: boolean;
	/** Callback to toggle left sidebar. */
	onToggleSidebarCollapsed?: () => void;
	/** Shown in center when expanded. */
	chatTitle?: string;
	/** PR diff stats for the PR tab. */
	diffStatus?: { additions?: number; deletions?: number };
}

function repoTabLabel(repoRoot: string): string {
	const segments = repoRoot.split("/").filter(Boolean);
	return segments[segments.length - 1] ?? repoRoot;
}

function computeDiffStats(unifiedDiff: string | undefined): {
	additions: number;
	deletions: number;
} {
	if (!unifiedDiff) return { additions: 0, deletions: 0 };
	try {
		const patches = parsePatchFiles(unifiedDiff);
		let additions = 0;
		let deletions = 0;
		for (const patch of patches) {
			for (const file of patch.files) {
				for (const hunk of file.hunks) {
					additions += hunk.additionLines;
					deletions += hunk.deletionLines;
				}
			}
		}
		return { additions, deletions };
	} catch {
		return { additions: 0, deletions: 0 };
	}
}

export const SidebarTabView: FC<SidebarTabViewProps> = ({
	prTab,
	repositories,
	onRefresh,
	onCommit,
	isExpanded,
	onToggleExpanded,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	chatTitle,
	diffStatus,
}) => {
	const tabIdPrefix = useId();
	const repoEntries = Array.from(repositories.entries()).sort(([a], [b]) =>
		a.localeCompare(b),
	);

	const hasPR = Boolean(prTab);
	const hasRepos = repoEntries.length > 0;

	// Default active tab: PR if present, otherwise first repo.
	const defaultTab = hasPR
		? "pr"
		: repoEntries.length > 0
			? repoEntries[0][0]
			: null;

	const [activeTab, setActiveTab] = useState<string | null>(defaultTab);

	const [diffStyle, setDiffStyle] = useState<DiffStyle>(loadDiffStyle);
	const handleSetDiffStyle = useCallback((style: DiffStyle) => {
		setDiffStyle(style);
		localStorage.setItem(DIFF_STYLE_KEY, style);
	}, []);

	// Derive the effective tab inline to avoid a one-frame flash when
	// activeTab is stale or null but a valid default exists.
	const effectiveTab =
		activeTab !== null &&
		(activeTab === "pr" ? hasPR : repositories.has(activeTab))
			? activeTab
			: defaultTab;

	// Compute diff stats for all repo tabs and cache them.
	const repoDiffStats = useMemo(() => {
		const statsMap = new Map<
			string,
			{ additions: number; deletions: number }
		>();
		for (const [repoRoot, repo] of repoEntries) {
			statsMap.set(repoRoot, computeDiffStats(repo.unified_diff));
		}
		return statsMap;
	}, [repoEntries]);

	const prDiffAdditions = diffStatus?.additions ?? 0;
	const prDiffDeletions = diffStatus?.deletions ?? 0;

	if (!hasPR && !hasRepos) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden border-0 border-l border-solid bg-surface-primary">
				{/* Tab bar – always visible for the expand button. */}
				<div
					role="tablist"
					className="flex shrink-0 items-center gap-1 px-1 py-1.5"
				>
					<div className="min-w-0 flex-1 text-center">
						{isExpanded && chatTitle && (
							<span className="truncate text-sm text-content-primary">
								{chatTitle}
							</span>
						)}
					</div>
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleExpanded}
						aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
						className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary"
					>
						{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
					</Button>
				</div>
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No changes to display.
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-full min-w-0 flex-col overflow-hidden border-0 border-l border-solid bg-surface-primary">
			{/* Tab bar */}
			<div
				role="tablist"
				className="flex shrink-0 items-center gap-1 px-1 py-1.5"
			>
				{/* Sidebar toggle – only when expanded and sidebar is collapsed */}
				{isExpanded && isSidebarCollapsed && onToggleSidebarCollapsed && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleSidebarCollapsed}
						aria-label="Expand sidebar"
						className="mr-1 h-7 w-7 shrink-0"
					>
						<PanelLeftIcon />
					</Button>
				)}

				{/* Tabs – scrolls internally so right-side buttons stay visible */}
				<div className="flex min-w-0 items-center overflow-x-auto rounded-lg bg-surface-secondary p-0.5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
					{hasPR && prTab && (
						<button
							type="button"
							id={`${tabIdPrefix}-tab-pr`}
							role="tab"
							aria-selected={effectiveTab === "pr"}
							onClick={() => setActiveTab("pr")}
							className={cn(
								"flex shrink-0 items-center gap-1.5 rounded-md border-0 px-2 py-1 text-xs font-medium transition-colors cursor-pointer outline-none",
								effectiveTab === "pr"
									? "bg-surface-primary text-content-primary shadow-sm"
									: "bg-transparent text-content-secondary hover:text-content-primary",
							)}
						>
							<GitPullRequestIcon className="h-3.5 w-3.5" />#{prTab.prNumber}
							{(prDiffAdditions > 0 || prDiffDeletions > 0) && (
								<DiffStatNumbers
									additions={prDiffAdditions}
									deletions={prDiffDeletions}
									className="ml-1 text-[10px]"
								/>
							)}
						</button>
					)}
					{repoEntries.map(([repoRoot]) => {
						const stats = repoDiffStats.get(repoRoot);
						const additions = stats?.additions ?? 0;
						const deletions = stats?.deletions ?? 0;
						return (
							<button
								type="button"
								id={`${tabIdPrefix}-tab-${repoRoot}`}
								role="tab"
								aria-selected={effectiveTab === repoRoot}
								key={repoRoot}
								onClick={() => setActiveTab(repoRoot)}
								className={cn(
									"flex shrink-0 items-center gap-1.5 rounded-md border-0 px-2 py-1 text-xs font-medium transition-colors cursor-pointer outline-none",
									effectiveTab === repoRoot
										? "bg-surface-primary text-content-primary shadow-sm"
										: "bg-transparent text-content-secondary hover:text-content-primary",
								)}
							>
								<FolderIcon className="h-3.5 w-3.5" />
								{repoTabLabel(repoRoot)}
								{(additions > 0 || deletions > 0) && (
									<DiffStatNumbers
										additions={additions}
										deletions={deletions}
										className="ml-1 text-[10px]"
									/>
								)}
							</button>
						);
					})}
				</div>

				{/* Center: chat title when expanded */}
				<div className="min-w-0 flex-1 text-center">
					{isExpanded && chatTitle && (
						<span className="truncate text-sm text-content-primary">
							{chatTitle}
						</span>
					)}
				</div>

				{/* Diff style toggle */}
				<div className="flex shrink-0 items-center gap-1">
					<Button
						variant={diffStyle === "unified" ? "outline" : "subtle"}
						size="lg"
						onClick={() => handleSetDiffStyle("unified")}
						className={cn(
							"min-w-0 h-6 px-2 py-0",
							diffStyle === "unified" && "bg-surface-secondary",
						)}
						aria-label="Unified diff view"
					>
						<Rows3Icon className="!p-0 !size-3.5" />
					</Button>
					<Button
						variant={diffStyle === "split" ? "outline" : "subtle"}
						size="lg"
						onClick={() => handleSetDiffStyle("split")}
						className={cn(
							"min-w-0 h-6 px-2 py-0",
							diffStyle === "split" && "bg-surface-secondary",
						)}
						aria-label="Split diff view"
					>
						<Columns2Icon className="!p-0 !size-3.5" />
					</Button>
				</div>

				{/* Right side: expand/contract button */}
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggleExpanded}
					aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
					className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary"
				>
					{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
				</Button>
			</div>

			{/* Tab content */}
			<div
				role="tabpanel"
				aria-labelledby={
					effectiveTab ? `${tabIdPrefix}-tab-${effectiveTab}` : undefined
				}
				className="min-h-0 flex-1"
			>
				{effectiveTab === "pr" && prTab ? (
					<FilesChangedPanel
						chatId={prTab.chatId}
						isExpanded={isExpanded}
						diffStyle={diffStyle}
					/>
				) : effectiveTab && repositories.has(effectiveTab) ? (
					<RepoChangesPanel
						repo={repositories.get(effectiveTab)!}
						onRefresh={onRefresh}
						onCommit={() => onCommit(effectiveTab)}
						isExpanded={isExpanded}
						diffStyle={diffStyle}
					/>
				) : null}
			</div>
		</div>
	);
};
