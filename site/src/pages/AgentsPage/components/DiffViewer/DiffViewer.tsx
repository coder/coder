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
import { Fragment, useEffect, useRef, useState } from "react";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { useTheme } from "#/theme/context";
import { countChangedLines } from "../../utils/countChangedLines";
import { changeColor, changeLabel } from "../../utils/diffColors";
import { SEPARATOR_CSS } from "../ChatElements/tools/utils";
import { useActiveFileTracking } from "./useActiveFileTracking";

type DiffViewerProps = {
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
// Space between file cards and around the list, in pixels.
const FILE_CARD_GAP = 2;
const FILE_LIST_PADDING = 8;

// Renders each file as a rounded card: the header row and the code sit on a
// surface-secondary background, and the code is inset 4px from the card
// edges so its horizontal scrollbar stays inside the card. A collapsed file
// has no <pre>, so its header takes the full card radius.
const FILE_CARD_CSS = [
	":host {",
	"  display: block;",
	"  border-radius: 6px;",
	"  background-color: hsl(var(--surface-secondary));",
	"}",
	"[data-diffs-header] {",
	"  border-radius: 6px 6px 0 0;",
	"  background-color: hsl(var(--surface-secondary)) !important;",
	"}",
	"[data-diffs-header]:not(:has(~ pre[data-diff])) {",
	"  border-radius: 6px;",
	"}",
	"pre[data-diff] {",
	"  margin: 0 4px 4px;",
	"  border-radius: 4px;",
	"  overflow: hidden;",
	"}",
	// The library sets scrollbar-gutter: stable on the code and hides the
	// vertical scrollbar with a zero-width ::-webkit-scrollbar. The inherited
	// global scrollbar-color makes Chromium ignore that rule, so classic
	// (always visible) scrollbars leave an empty strip on the right. The code
	// only scrolls horizontally; the outer list owns vertical scrolling.
	"[data-code] {",
	"  scrollbar-gutter: auto !important;",
	"}",
].join(" ");

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

// CodeView's syncItemRecord skips reusing a record when item.version is
// unchanged, so the version must reflect annotation content rather than count.
// Moving the active comment box to another line in the same file keeps the
// count at 1 but must still re-render, so fold each annotation's side and line
// into the version. Exported for unit tests.
export function annotationsVersion(
	annotations: readonly DiffLineAnnotation<string>[],
): number {
	if (annotations.length === 0) {
		return 0;
	}
	return annotations.reduce(
		(version, annotation) =>
			version * 31 +
			annotation.lineNumber * 2 +
			(annotation.side === "additions" ? 1 : 0),
		annotations.length,
	);
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
	onToggleCollapsed: () => void;
}) {
	const { additions, deletions } = countChangedLines(fileDiff);
	return (
		<div className="flex h-8 min-w-0 items-center justify-between gap-3 px-1 font-sans text-sm">
			<div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
				<button
					type="button"
					aria-expanded={!collapsed}
					aria-label={`${collapsed ? "Expand" : "Collapse"} ${fileDiff.name}`}
					onClick={onToggleCollapsed}
					className="flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-sm border-0 bg-transparent p-0 text-content-secondary hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-content-link"
				>
					<ChevronDownIcon
						className={cn(
							"size-3.5 transition-transform",
							collapsed && "-rotate-90",
						)}
					/>
				</button>
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
	// selection with just the active file, suppressing the echoed events.
	useEffect(() => {
		if (!activePath || selectedPathRef.current === activePath) return;
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

// The loading state mirrors the real diff layout: flat, full-width
// file headers with a change badge and stat pill, gutter-aligned code
// lines, and centered hunk separators. Keeping the same shape avoids a
// jarring swap when the parsed diff replaces the placeholder.
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
		<div className="overflow-hidden rounded-md bg-surface-secondary">
			<div className="flex h-8 items-center justify-between gap-3 px-2">
				<div className="flex items-center gap-2">
					<Skeleton className="size-3 rounded-[2px]" />
					<Skeleton className="h-3 w-44" />
				</div>
				<Skeleton className="h-3 w-14" />
			</div>
			<div className="mx-1 mb-1 rounded bg-surface-primary py-1">
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
			className="flex flex-col gap-0.5 p-2"
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
	const isDark = theme.palette.mode === "dark";
	const [activeFile, setActiveFile] = useState<string | null>(null);
	// Files whose diff body is hidden, keyed by file name. Every file starts
	// expanded.
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
			// CodeView only re-syncs an item when its version changes, so the
			// collapsed flag must be part of it.
			version: annotationsVersion(annotations) * 2 + (collapsed ? 1 : 0),
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
						activePath={activeFile}
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
				items={items}
				options={options}
				selectedLines={selectedLines}
				className="h-full min-h-0 min-w-0 flex-1 overflow-auto px-2"
				style={diffViewerStyle}
				onScroll={handleScroll}
				renderCustomHeader={(item) =>
					item.type === "diff" ? (
						<HeaderContent
							fileDiff={item.fileDiff}
							collapsed={collapsedFiles.has(item.id)}
							onToggleCollapsed={() =>
								setFileCollapsed(item.id, !collapsedFiles.has(item.id))
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
