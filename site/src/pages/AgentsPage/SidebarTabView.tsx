import { parsePatchFiles } from "@pierre/diffs";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	ChevronLeftIcon,
	ChevronRightIcon,
	Columns2Icon,
	MaximizeIcon,
	MinimizeIcon,
	PanelLeftIcon,
	Rows3Icon,
	XIcon,
} from "lucide-react";
import {
	type FC,
	type RefObject,
	useCallback,
	useEffect,
	useId,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { DiffStatBadge } from "./DiffStats";
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
	/** Callback to close the panel (used on mobile). */
	onClose?: () => void;
	/** Ref to the chat input, forwarded to FilesChangedPanel. */
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
}

/** How far (px) each chevron click scrolls the tab strip. */
const TAB_SCROLL_AMOUNT = 120;

/**
 * Tracks whether the tab scroll container overflows and
 * exposes scroll helpers for the chevron buttons.
 */
function useTabScroll() {
	const ref = useRef<HTMLDivElement>(null);
	const [canScrollLeft, setCanScrollLeft] = useState(false);
	const [canScrollRight, setCanScrollRight] = useState(false);

	const update = useCallback(() => {
		const el = ref.current;
		if (!el) return;
		setCanScrollLeft(el.scrollLeft > 0);
		setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 1);
	}, []);

	useEffect(() => {
		const el = ref.current;
		if (!el) return;

		// Initial check.
		update();

		// Re-check on scroll.
		el.addEventListener("scroll", update, { passive: true });

		// Re-check when the container or its children resize.
		const ro = new ResizeObserver(update);
		ro.observe(el);

		return () => {
			el.removeEventListener("scroll", update);
			ro.disconnect();
		};
	}, [update]);

	const scrollLeft = useCallback(() => {
		ref.current?.scrollBy({
			left: -TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	}, []);

	const scrollRight = useCallback(() => {
		ref.current?.scrollBy({
			left: TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	}, []);

	return { ref, canScrollLeft, canScrollRight, scrollLeft, scrollRight };
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
	onClose,
	chatInputRef,
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
	const hasPrDiffStats = prDiffAdditions > 0 || prDiffDeletions > 0;

	const tabScroll = useTabScroll();

	if (!hasPR && !hasRepos) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
				{/* Tab bar – always visible for the expand button. */}
				<div
					role="tablist"
					className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1"
				>
					<div className="min-w-0 shrink-0 text-center">
						{isExpanded && chatTitle && (
							<span className="truncate text-sm text-content-primary">
								{chatTitle}
							</span>
						)}
					</div>
					{onClose && (
						<Button
							variant="subtle"
							size="icon"
							onClick={onClose}
							aria-label="Close panel"
							className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:hidden"
						>
							<XIcon />
						</Button>
					)}
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleExpanded}
						aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
						className="hidden h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:inline-flex"
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
		<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
			{/* Tab bar */}
			<div
				role="tablist"
				className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1"
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
				{/* Scrollable tab strip with overlay chevrons */}
				<div className="relative min-w-0 flex-1">
					{tabScroll.canScrollLeft && (
						<button
							type="button"
							onClick={tabScroll.scrollLeft}
							aria-label="Scroll tabs left"
							className="absolute left-0 top-0 z-10 flex h-full w-8 cursor-pointer items-center justify-start border-none p-0 pl-1 text-content-primary [background:linear-gradient(to_right,hsl(var(--surface-primary))_50%,transparent)]"
						>
							<ChevronLeftIcon className="size-3.5" />
						</button>
					)}
					<div
						ref={tabScroll.ref}
						className="flex w-full items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
					>
						{hasPR && prTab && (
							<Button
								id={`${tabIdPrefix}-tab-pr`}
								role="tab"
								aria-selected={effectiveTab === "pr"}
								onClick={() => setActiveTab("pr")}
								variant="outline"
								size="lg"
								className={cn(
									"shrink-0 h-6 px-3 gap-3 py-0 bg-surface-primary",
									effectiveTab === "pr" && "bg-surface-tertiary",
									hasPrDiffStats && "pr-0",
								)}
							>
								#{prTab.prNumber}
								<DiffStatBadge
									additions={prDiffAdditions}
									deletions={prDiffDeletions}
								/>
							</Button>
						)}
						{repoEntries.map(([repoRoot]) => {
							const stats = repoDiffStats.get(repoRoot);
							const additions = stats?.additions ?? 0;
							const deletions = stats?.deletions ?? 0;
							const hasStats = additions > 0 || deletions > 0;
							return (
								<Button
									key={repoRoot}
									id={`${tabIdPrefix}-tab-${repoRoot}`}
									role="tab"
									aria-selected={effectiveTab === repoRoot}
									onClick={() => setActiveTab(repoRoot)}
									variant="outline"
									size="lg"
									className={cn(
										"shrink-0 h-6 px-3 gap-3 py-0 bg-surface-primary",
										effectiveTab === repoRoot && "bg-surface-tertiary",
										hasStats && "pr-0",
									)}
								>
									{repoTabLabel(repoRoot)}
									<DiffStatBadge additions={additions} deletions={deletions} />
								</Button>
							);
						})}
					</div>
					{tabScroll.canScrollRight && (
						<button
							type="button"
							onClick={tabScroll.scrollRight}
							aria-label="Scroll tabs right"
							className="absolute right-0 top-0 z-10 flex h-full w-8 cursor-pointer items-center justify-end border-none p-0 pr-1 text-content-primary [background:linear-gradient(to_left,hsl(var(--surface-primary))_50%,transparent)]"
						>
							<ChevronRightIcon className="size-3.5" />
						</button>
					)}
				</div>
				{/* Center: chat title when expanded */}
				<div className="min-w-0 shrink-0 text-center">
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
				{/* Right side: close (mobile) / expand (desktop) */}
				{onClose && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						aria-label="Close panel"
						className="h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:hidden"
					>
						<XIcon />
					</Button>
				)}
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggleExpanded}
					aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
					className="hidden h-7 w-7 shrink-0 text-content-secondary hover:text-content-primary md:inline-flex"
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
						chatInputRef={chatInputRef}
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
