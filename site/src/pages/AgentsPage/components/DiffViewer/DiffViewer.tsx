import { DEFAULT_CODE_VIEW_FILE_METRICS } from "@pierre/diffs";
import type {
	CodeViewHandle,
	CodeViewItem,
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
	VirtualFileMetrics,
} from "@pierre/diffs/react";
import { CodeView } from "@pierre/diffs/react";
import type { FileTreeSortComparator, GitStatusEntry } from "@pierre/trees";
import { FileTree, useFileTree } from "@pierre/trees/react";
import { cn } from "cn";
import { ChevronDownIcon } from "lucide-react";
import { Fragment, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { useTheme } from "#/theme/context";
import { countChangedLines } from "../../utils/countChangedLines";
import { changeColor, changeLabel } from "../../utils/diffColors";
import { SEPARATOR_CSS } from "../ChatElements/tools/utils";
import { useActiveFileTracking } from "./useActiveFileTracking";

type DiffViewerProps = {
	/**
	 * Files from parseDiffString. DiffViewer uses each file's cacheKey to
	 * detect content changes.
	 */
	parsedFiles: readonly FileDiffMetadata[];
	isExpanded: boolean;
	isLoading?: boolean;
	error?: unknown;
	emptyMessage?: string;
	diffStyle: DiffStyle;
	onLineNumberClick: (
		fileName: string,
		props: { lineNumber: number; annotationSide: "additions" | "deletions" },
	) => void;
	/** Fires when a line selection is committed (e.g. on pointer up). */
	onLineSelected: (fileName: string, range: SelectedLineRange | null) => void;
	/** Fires continuously as the selection range changes during a drag. */
	onLineSelectionChange: (
		fileName: string,
		range: SelectedLineRange | null,
	) => void;
	getLineAnnotations: (fileName: string) => DiffLineAnnotation<string>[];
	getSelectedLines: (fileName: string) => SelectedLineRange | null;
	renderAnnotation?: (
		annotation: DiffLineAnnotation<string>,
	) => React.ReactNode;
	scrollToFile?: string | null;
	onScrollToFileComplete?: () => void;
};

export type DiffStyle = "unified" | "split";
const DIFF_STYLE_KEY = "agents.diff-view-style";

const DIFF_VIEWER_LINE_HEIGHT = 16.5;
const DIFF_HEADER_HEIGHT = 32;
const HUNK_SEPARATOR_HEIGHT = 28;
// Vertical gap between file cards, in pixels.
const FILE_CARD_GAP = 2;
// Padding around the file list, in pixels. The right side is 0 while a
// classic scrollbar is shown.
const FILE_LIST_PADDING = 8;
const FILE_CARD_RADIUS = 6;
// Inset of the code block from the card's sides and bottom, in pixels.
const FILE_CODE_INSET = 4;

// The code block is inset so its horizontal scrollbar stays inside the card.
// A collapsed file renders no <pre>, so its header takes the full radius.
const FILE_CARD_CSS = `
:host {
  display: block;
  border-radius: ${FILE_CARD_RADIUS}px;
  background-color: hsl(var(--surface-secondary));
}
[data-diffs-header] {
  border-radius: ${FILE_CARD_RADIUS}px ${FILE_CARD_RADIUS}px 0 0;
  background-color: hsl(var(--surface-secondary)) !important;
}
[data-diffs-header]:not(:has(~ pre[data-diff])) {
  border-radius: ${FILE_CARD_RADIUS}px;
}
/* A stuck header covers its own card, which would fill in its rounded top
   corners. Mask the corners with the panel background. This assumes the
   panel behind the list is surface-primary; on another surface the corners
   would show the wrong color. The spread must be at least the radius. */
[data-diffs-header]::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  box-shadow: 0 0 0 ${FILE_CARD_RADIUS}px hsl(var(--surface-primary));
  clip-path: inset(0);
  pointer-events: none;
}
pre[data-diff] {
  margin: 0 ${FILE_CODE_INSET}px ${FILE_CODE_INSET}px;
  border-radius: ${FILE_CODE_INSET}px;
  overflow: hidden;
}
/* The global scrollbar-color makes Chromium ignore the library's
   ::-webkit-scrollbar rules for the code, which hide its vertical scrollbar
   and size the horizontal one to --diffs-scrollbar-gutter. Resetting it
   brings those rules back. The library's thumb is transparent until hover,
   so give it the app's scrollbar color. The library measures the gutter
   before these rules apply (15px with classic scrollbars), so pin it to the
   library's fallback: the styled scrollbar height on desktop, 0 on iOS.
   Firefox styles the code with scrollbar-color, so it keeps the library's
   value. */
@supports not (-moz-appearance: none) {
  :host {
    --diffs-scrollbar-gutter-override: var(--diffs-scrollbar-gutter-fallback);
  }
  [data-code] {
    scrollbar-color: auto;
  }
  [data-code]::-webkit-scrollbar-thumb {
    background-color: hsl(var(--surface-scrollbar));
  }
}
`;

// Minimum width (px) of the diff container at which the file tree sidebar is
// shown alongside the diff. Below this the diff takes the full width unless the
// viewer is explicitly expanded.
const FILE_TREE_THRESHOLD = 1000;

const diffViewerStyle = {
	"--diffs-font-family": '"Geist Mono Variable", monospace, monospace',
	"--diffs-header-font-family": '"Geist Variable", system-ui, sans-serif',
	"--diffs-font-size": "11px",
	"--diffs-line-height": `${DIFF_VIEWER_LINE_HEIGHT}px`,
} satisfies React.CSSProperties;

const diffViewerMetrics: Partial<VirtualFileMetrics> = {
	diffHeaderHeight: DIFF_HEADER_HEIGHT,
	hunkSeparatorHeight: HUNK_SEPARATOR_HEIGHT,
	lineHeight: DIFF_VIEWER_LINE_HEIGHT,
	// CodeView computes file heights from these metrics, so they must include
	// the code block's bottom inset from FILE_CARD_CSS.
	paddingBottom: DEFAULT_CODE_VIEW_FILE_METRICS.spacing + FILE_CODE_INSET,
};

const fileTreeStyle = {
	height: "100%",
	"--trees-font-family-override": '"Geist Variable", system-ui, sans-serif',
	"--trees-font-size-override": "13px",
	"--trees-border-color-override": "hsl(var(--border-default))",
	"--trees-bg-override": "hsl(var(--surface-primary))",
	"--trees-fg-override": "hsl(var(--content-primary))",
	"--trees-muted-fg-override": "hsl(var(--content-secondary))",
	"--trees-selected-bg-override": "hsl(var(--surface-secondary))",
	"--trees-padding-inline-override": "0px",
	"--trees-item-margin-x-override": "0px",
	"--trees-border-radius-override": "0px",
	"--trees-git-added-color-override": "hsl(var(--git-added))",
	"--trees-git-deleted-color-override": "hsl(var(--git-deleted))",
	"--trees-git-modified-color-override": "hsl(var(--git-modified))",
	"--trees-git-renamed-color-override": "hsl(var(--git-modified))",
} satisfies React.CSSProperties;

// Single full-path ordering rule shared by the sidebar tree and the flat diff
// list so the two cannot drift apart. useFileTree applies the sort comparator
// to the whole flat set of path entries (directories included), so it must be
// a full-path comparator, not a basename one: at the first differing segment a
// directory sorts before a file, dot-prefixed names first, then
// case-insensitive locale order; a shorter path sorts before its descendants.
function compareTreeEntries(
	aSegments: readonly string[],
	aIsDirectory: boolean,
	bSegments: readonly string[],
	bIsDirectory: boolean,
): number {
	const shared = Math.min(aSegments.length, bSegments.length);
	for (let i = 0; i < shared; i++) {
		const aSegment = aSegments[i];
		const bSegment = bSegments[i];
		if (aSegment === bSegment) {
			continue;
		}
		// A segment is a directory when it is not the last one, or when the
		// entry itself is a directory whose final segment this is.
		const aSegmentIsDir = i < aSegments.length - 1 || aIsDirectory;
		const bSegmentIsDir = i < bSegments.length - 1 || bIsDirectory;
		if (aSegmentIsDir !== bSegmentIsDir) {
			return aSegmentIsDir ? -1 : 1;
		}
		const aIsDot = aSegment.charCodeAt(0) === 46;
		const bIsDot = bSegment.charCodeAt(0) === 46;
		if (aIsDot !== bIsDot) {
			return aIsDot ? -1 : 1;
		}
		return aSegment.toLowerCase().localeCompare(bSegment.toLowerCase());
	}
	if (aSegments.length !== bSegments.length) {
		return aSegments.length - bSegments.length;
	}
	if (aIsDirectory === bIsDirectory) {
		return 0;
	}
	return aIsDirectory ? -1 : 1;
}

// Passed to useFileTree so the sidebar uses compareTreeEntries rather than the
// library's undocumented default sort. The entry carries the full segment
// list. Exported for unit tests that verify the tree order matches the diff.
export const treeSortComparator: FileTreeSortComparator = (left, right) =>
	compareTreeEntries(
		left.segments,
		left.isDirectory,
		right.segments,
		right.isDirectory,
	);

// Orders the flat diff list to the sidebar tree's leaf order so scrolling moves
// monotonically down the tree. Diff entries are always files, so it runs the
// same compareTreeEntries the tree uses, keeping the two orders identical by
// construction. Exported for unit tests that pin the shared ordering.
export function compareTreePaths(a: string, b: string): number {
	return compareTreeEntries(a.split("/"), false, b.split("/"), false);
}

// Stays a 32-bit integer so later inputs are not lost to floating-point
// precision.
const hashStep = (hash: number, value: number): number =>
	(Math.imul(hash, 31) + value) | 0;

// Folds each annotation's side and line, so moving the comment box to another
// line changes the result even when the count stays the same. Exported for
// unit tests.
export function annotationsVersion(
	annotations: readonly DiffLineAnnotation<string>[],
): number {
	if (annotations.length === 0) {
		return 0;
	}
	return annotations.reduce(
		(version, annotation) =>
			hashStep(
				version,
				annotation.lineNumber * 2 + (annotation.side === "additions" ? 1 : 0),
			),
		annotations.length,
	);
}

// CodeView's syncItemRecord re-syncs an item only when its version changes,
// so every render input must feed it. cacheKey is the content hash
// parseDiffString stamps. Exported for unit tests.
export function diffItemVersion(
	fileDiff: FileDiffMetadata,
	annotations: readonly DiffLineAnnotation<string>[],
	collapsed: boolean,
): number {
	let version = annotationsVersion(annotations);
	for (const char of fileDiff.cacheKey ?? "") {
		version = hashStep(version, char.charCodeAt(0));
	}
	return hashStep(version, collapsed ? 1 : 0);
}

// The library forces classic, space-reserving scrollbars via
// scrollbar-gutter: stable, leaving a permanent empty strip on the right.
// Restore the default gutter so the scrollbar overlays content while scrolling
// instead of reserving width, and keep it slim when it does appear.
const fileTreeUnsafeCSS = [
	"[data-file-tree-virtualized-scroll='true'] {",
	"  scrollbar-gutter: auto !important;",
	"  scrollbar-width: thin;",
	"}",
].join(" ");

function gitStatusForFile(
	fileDiff: FileDiffMetadata,
): GitStatusEntry["status"] {
	switch (fileDiff.type) {
		case "new":
			return "added";
		case "deleted":
			return "deleted";
		case "rename-pure":
		case "rename-changed":
			return "renamed";
		default:
			return "modified";
	}
}

function HeaderContent({
	fileDiff,
	collapsed,
	onToggleCollapsed,
}: {
	fileDiff: FileDiffMetadata;
	collapsed: boolean;
	onToggleCollapsed: (button: HTMLButtonElement) => void;
}) {
	const { additions, deletions } = countChangedLines(fileDiff);
	return (
		<div className="flex h-8 min-w-0 items-center justify-between gap-3 px-1 font-sans text-sm">
			<div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
				<Button
					variant="subtle"
					size="icon"
					aria-expanded={!collapsed}
					aria-label={`${collapsed ? "Expand" : "Collapse"} ${fileDiff.name}`}
					onClick={(event) => onToggleCollapsed(event.currentTarget)}
					className="size-5 min-w-0 shrink-0 rounded-sm p-0 hover:bg-surface-tertiary [&>svg]:p-0"
				>
					<ChevronDownIcon
						className={cn(
							"size-3.5! transition-transform",
							collapsed && "-rotate-90",
						)}
					/>
				</Button>
				<div className="flex min-w-0 items-baseline gap-2 overflow-hidden">
					<span
						className={cn(
							"shrink-0 text-[11px] font-semibold leading-none",
							changeColor(fileDiff.type),
						)}
					>
						{changeLabel(fileDiff.type)}
					</span>
					{fileDiff.prevName && fileDiff.prevName !== fileDiff.name && (
						<span className="truncate text-xs text-content-secondary">
							{fileDiff.prevName}
						</span>
					)}
					<span className="truncate text-xs font-medium text-content-primary">
						{fileDiff.name}
					</span>
				</div>
			</div>
			{(additions > 0 || deletions > 0) && (
				<span className="flex shrink-0 items-center gap-1.5 pr-1 text-xs font-medium tabular-nums">
					{additions > 0 && (
						<span className="text-git-added-bright">+{additions}</span>
					)}
					{deletions > 0 && (
						<span className="text-git-deleted-bright">&minus;{deletions}</span>
					)}
				</span>
			)}
		</div>
	);
}

function DiffFileTree({
	files,
	activePath,
	onUserSelectPath,
}: {
	files: readonly FileDiffMetadata[];
	activePath: string | null;
	onUserSelectPath: (fileName: string) => void;
}) {
	const paths = files.map((file) => file.name);
	const gitStatus = files.map((file) => ({
		path: file.name,
		status: gitStatusForFile(file),
	}));
	// Tracks the row currently selected in the model so scroll-driven syncing
	// can skip redundant selects.
	const selectedPathRef = useRef<string | null>(null);
	// Set while we replace the selection programmatically. The model emits
	// selection events synchronously, so this drops the echoes from our own
	// deselect/select calls in one window instead of looping back into
	// onUserSelectPath. A user click can only land between effect runs, when
	// the flag is already false.
	const isSyncingSelectionRef = useRef(false);
	const { model } = useFileTree({
		density: "compact",
		flattenEmptyDirectories: false,
		gitStatus,
		initialExpansion: "open",
		sort: treeSortComparator,
		unsafeCSS: fileTreeUnsafeCSS,
		onSelectionChange: (selectedPaths) => {
			const selectedPath = selectedPaths.at(-1) ?? null;
			selectedPathRef.current = selectedPath;
			if (isSyncingSelectionRef.current) {
				return;
			}
			if (selectedPath) {
				onUserSelectPath(selectedPath);
			}
		},
		paths,
	});

	useEffect(() => {
		model.resetPaths(paths);
		model.setGitStatus(gitStatus);
	}, [gitStatus, model, paths]);

	// Drive the tree selection from the active file. The @pierre/trees item
	// handle's select() is additive, so a scrolled-past file stays
	// highlighted unless its selection is cleared. Replace the whole
	// selection with just the active file, suppressing the echoed events. A
	// null activePath clears the selection, so clicking the same row again
	// counts as a change and fires onUserSelectPath.
	useEffect(() => {
		if (selectedPathRef.current === activePath) return;
		if (!activePath) {
			isSyncingSelectionRef.current = true;
			for (const path of model.getSelectedPaths()) {
				model.getItem(path)?.deselect();
			}
			isSyncingSelectionRef.current = false;
			selectedPathRef.current = null;
			return;
		}
		const item = model.getItem(activePath);
		if (!item) return;
		isSyncingSelectionRef.current = true;
		for (const path of model.getSelectedPaths()) {
			if (path !== activePath) {
				model.getItem(path)?.deselect();
			}
		}
		item.select();
		isSyncingSelectionRef.current = false;
		selectedPathRef.current = activePath;
	}, [activePath, model]);

	return (
		<FileTree
			model={model}
			className="block h-full min-h-0 w-full"
			style={fileTreeStyle}
		/>
	);
}

export function loadDiffStyle(): DiffStyle {
	const stored = localStorage.getItem(DIFF_STYLE_KEY);
	if (stored === "split" || stored === "unified") {
		return stored;
	}
	return "unified";
}

export function saveDiffStyle(style: DiffStyle): void {
	localStorage.setItem(DIFF_STYLE_KEY, style);
}

// The loading state mirrors the real file card layout so the parsed diff
// replaces it without a visible jump. Change both together.
function SkeletonLine({ width }: { width: string }) {
	return (
		<div
			className="flex items-center gap-3 px-2.5"
			style={{ height: DIFF_VIEWER_LINE_HEIGHT }}
		>
			<Skeleton className="h-2 w-4 shrink-0" />
			<Skeleton className={cn("h-2", width)} />
		</div>
	);
}

function SkeletonSeparator() {
	return (
		<div className="flex items-center gap-3 px-2.5 py-2">
			<div className="h-px flex-1 bg-border" />
			<Skeleton className="h-2 w-24" />
			<div className="h-px flex-1 bg-border" />
		</div>
	);
}

function SkeletonFile({ groups }: { groups: readonly (readonly string[])[] }) {
	return (
		<div
			className="overflow-hidden bg-surface-secondary"
			style={{ borderRadius: FILE_CARD_RADIUS }}
		>
			<div className="flex h-8 items-center justify-between gap-3 px-1">
				<div className="flex items-center gap-1.5">
					<div className="size-5 shrink-0" />
					<Skeleton className="size-3 rounded-[2px]" />
					<Skeleton className="h-3 w-44" />
				</div>
				<Skeleton className="h-3 w-14" />
			</div>
			<div
				className="bg-surface-primary py-1"
				style={{
					margin: `0 ${FILE_CODE_INSET}px ${FILE_CODE_INSET}px`,
					borderRadius: FILE_CODE_INSET,
				}}
			>
				{groups.map((lines, groupIndex) => (
					<Fragment key={groupIndex}>
						{groupIndex > 0 && <SkeletonSeparator />}
						{lines.map((width, lineIndex) => (
							<SkeletonLine key={lineIndex} width={width} />
						))}
					</Fragment>
				))}
			</div>
		</div>
	);
}

const DIFF_SKELETON_FILES = [
	[
		["w-3/4", "w-1/2", "w-5/6"],
		["w-2/3", "w-2/5", "w-3/4", "w-1/3"],
	],
	[
		["w-1/2", "w-5/6"],
		["w-2/3", "w-1/3"],
	],
] as const;

function DiffViewerSkeleton() {
	return (
		<div
			role="status"
			aria-label="Loading diff"
			aria-busy
			className="flex flex-col"
			style={{ gap: FILE_CARD_GAP, padding: FILE_LIST_PADDING }}
		>
			{DIFF_SKELETON_FILES.map((groups, fileIndex) => (
				<SkeletonFile key={fileIndex} groups={groups} />
			))}
		</div>
	);
}

export const DiffViewer: React.FC<DiffViewerProps> = ({
	parsedFiles,
	isExpanded,
	isLoading,
	error,
	emptyMessage = "No file changes to display.",
	diffStyle,
	onLineNumberClick,
	onLineSelected,
	onLineSelectionChange,
	getLineAnnotations,
	getSelectedLines,
	renderAnnotation,
	scrollToFile,
	onScrollToFileComplete,
}) => {
	const theme = useTheme();
	const codeViewRef = useRef<CodeViewHandle<string, undefined>>(null);
	// CodeView's scroll root, set through its containerRef prop.
	const [scrollerEl, setScrollerEl] = useState<HTMLDivElement | null>(null);
	const isDark = theme.palette.mode === "dark";
	const [activeFile, setActiveFile] = useState<string | null>(null);
	// Names persist for the mount, so a file that leaves the diff and later
	// returns keeps its collapsed state.
	const [collapsedFiles, setCollapsedFiles] = useState<ReadonlySet<string>>(
		() => new Set(),
	);
	const setFileCollapsed = (fileName: string, collapsed: boolean) => {
		setCollapsedFiles((current) => {
			if (current.has(fileName) === collapsed) return current;
			const next = new Set(current);
			if (collapsed) {
				next.add(fileName);
			} else {
				next.delete(fileName);
			}
			return next;
		});
	};
	const toggleFileCollapsed = (
		fileName: string,
		wasCollapsed: boolean,
		button: HTMLButtonElement,
	) => {
		if (!wasCollapsed) {
			// While the header is stuck, the file's top is above the viewport.
			// Collapsing keeps scrollTop, so without aligning the file's top
			// first, the collapsed header ends up above the viewport.
			const host = button.closest("diffs-container");
			if (
				host &&
				scrollerEl &&
				host.getBoundingClientRect().top <
					scrollerEl.getBoundingClientRect().top
			) {
				codeViewRef.current?.scrollTo({
					type: "item",
					id: fileName,
					align: "start",
					behavior: "instant",
				});
			}
		}
		setFileCollapsed(fileName, !wasCollapsed);
	};

	// Measure the diff container so the file tree only appears when there is
	// enough horizontal room, rather than keying off the viewport width.
	const [containerEl, setContainerEl] = useState<HTMLDivElement | null>(null);
	const [containerWidth, setContainerWidth] = useState(0);
	useEffect(() => {
		if (!containerEl) return;
		setContainerWidth(containerEl.getBoundingClientRect().width);
		const observer = new ResizeObserver(([entry]) => {
			setContainerWidth(entry.contentRect.width);
		});
		observer.observe(containerEl);
		return () => observer.disconnect();
	}, [containerEl]);

	// A classic scrollbar already separates the cards from the panel edge, so
	// the list drops its right padding while one is shown.
	const [hasClassicScrollbar, setHasClassicScrollbar] = useState(false);
	useLayoutEffect(() => {
		if (!scrollerEl) return;
		const update = () =>
			setHasClassicScrollbar(
				scrollerEl.offsetWidth - scrollerEl.clientWidth > 0,
			);
		update();
		const observer = new ResizeObserver(update);
		observer.observe(scrollerEl);
		return () => observer.disconnect();
	}, [scrollerEl]);

	const showTree = isExpanded || containerWidth >= FILE_TREE_THRESHOLD;
	const handleScroll = useActiveFileTracking({
		enabled: showTree,
		onActiveFileChange: (path) =>
			setActiveFile((current) => (current === path ? current : path)),
	});

	const options: React.ComponentProps<typeof CodeView<string>>["options"] = {
		diffStyle,
		diffIndicators: "bars",
		overflow: "scroll",
		stickyHeaders: true,
		layout: {
			paddingTop: FILE_LIST_PADDING,
			paddingBottom: FILE_LIST_PADDING,
			gap: FILE_CARD_GAP,
		},
		hunkSeparators: "line-info",
		itemMetrics: diffViewerMetrics,
		unsafeCSS: `${SEPARATOR_CSS} ${FILE_CARD_CSS}`,
		themeType: isDark ? "dark" : "light",
		theme: isDark ? "github-dark-high-contrast" : "github-light",
		enableLineSelection: true,
		enableGutterUtility: true,
		onLineNumberClick: (props, item) => {
			if (item.type === "diff" && props.type === "diff-line") {
				onLineNumberClick(item.item.id, props);
			}
		},
		onLineSelected: (range, item) => {
			if (item.type === "diff") {
				onLineSelected(item.item.id, range);
			}
		},
		onLineSelectionChange: (range, item) => {
			if (item.type === "diff") {
				onLineSelectionChange(item.item.id, range);
			}
		},
		onGutterUtilityClick: (range, item) => {
			if (item.type === "diff") {
				onLineSelected(item.item.id, range);
			}
		},
	};

	// Render the diff in the same order the sidebar tree lays out its leaves so
	// scrolling moves monotonically down the tree.
	const sortedFiles = [...parsedFiles].sort((a, b) =>
		compareTreePaths(a.name, b.name),
	);

	const items: CodeViewItem<string>[] = sortedFiles.map((fileDiff) => {
		const annotations = getLineAnnotations(fileDiff.name);
		const collapsed = collapsedFiles.has(fileDiff.name);
		return {
			id: fileDiff.name,
			type: "diff",
			fileDiff,
			annotations,
			collapsed,
			version: diffItemVersion(fileDiff, annotations, collapsed),
		};
	});

	const selectedLines = (() => {
		for (const fileDiff of sortedFiles) {
			const range = getSelectedLines(fileDiff.name);
			if (range) return { id: fileDiff.name, range };
		}
		return null;
	})();

	const canScroll = !isLoading && !error && items.length > 0;

	useEffect(() => {
		if (!scrollToFile) return;
		// CodeView is not mounted while loading/erroring/empty, so the ref is
		// null. Skip the scroll without signalling completion so the parent
		// keeps the pending target and retries once the view is ready.
		if (!canScroll || !codeViewRef.current) return;
		// Expanding here rather than in the parent keeps collapse state owned
		// by DiffViewer; a file reference must land on visible code.
		setFileCollapsed(scrollToFile, false);
		codeViewRef.current.scrollTo({
			type: "item",
			id: scrollToFile,
			align: "start",
			behavior: "instant",
		});
		onScrollToFileComplete?.();
	}, [scrollToFile, onScrollToFileComplete, canScroll]);

	if (isLoading) {
		return <DiffViewerSkeleton />;
	}

	if (error) {
		return <ErrorAlert error={error} />;
	}

	if (items.length === 0) {
		return (
			<div className="flex h-full flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
				{emptyMessage}
			</div>
		);
	}

	return (
		<div
			ref={setContainerEl}
			className="flex h-full min-h-0 min-w-0 overflow-hidden"
		>
			{showTree && (
				<aside className="h-full min-h-0 w-72 shrink-0 border-0 border-r border-solid border-border">
					<DiffFileTree
						files={sortedFiles}
						// A collapsed file is never selected, so clicking its row
						// fires onUserSelectPath and expands it.
						activePath={
							activeFile !== null && !collapsedFiles.has(activeFile)
								? activeFile
								: null
						}
						onUserSelectPath={(fileName) => {
							setActiveFile(fileName);
							setFileCollapsed(fileName, false);
							codeViewRef.current?.scrollTo({
								type: "item",
								id: fileName,
								align: "start",
								behavior: "instant",
							});
						}}
					/>
				</aside>
			)}
			<CodeView
				ref={codeViewRef}
				containerRef={setScrollerEl}
				items={items}
				options={options}
				selectedLines={selectedLines}
				className="h-full min-h-0 min-w-0 flex-1 overflow-auto"
				style={{
					...diffViewerStyle,
					paddingLeft: FILE_LIST_PADDING,
					paddingRight: hasClassicScrollbar ? 0 : FILE_LIST_PADDING,
				}}
				onScroll={handleScroll}
				renderCustomHeader={(item) =>
					item.type === "diff" ? (
						<HeaderContent
							fileDiff={item.fileDiff}
							collapsed={item.collapsed === true}
							onToggleCollapsed={(button) =>
								toggleFileCollapsed(item.id, item.collapsed === true, button)
							}
						/>
					) : null
				}
				renderAnnotation={(annotation) =>
					"side" in annotation ? renderAnnotation?.(annotation) : null
				}
			/>
		</div>
	);
};
