import { useTheme } from "@emotion/react";
import type {
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
} from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
	DIFFS_FONT_STYLE,
	getDiffViewerOptions,
} from "components/ai-elements/tool/utils";
import { FileIcon } from "components/FileIcon/FileIcon";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Skeleton } from "components/Skeleton/Skeleton";
import { ChevronRightIcon } from "lucide-react";
import {
	type ComponentProps,
	type FC,
	memo,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import { changeColor, changeLabel } from "./diffColors";

// -------------------------------------------------------------------
// Public interface
// -------------------------------------------------------------------

interface DiffViewerProps {
	/** Parsed file diffs to render. */
	parsedFiles: readonly FileDiffMetadata[];
	/** Cache key prefix for parsePatchFiles worker pool LRU cache. */
	cacheKeyPrefix?: string;
	/** Whether the panel is in expanded mode (affects file tree threshold). */
	isExpanded?: boolean;
	/** Loading state. */
	isLoading?: boolean;
	/** Error state. */
	error?: unknown;
	/** Empty state message. */
	emptyMessage?: string;
	/** Which diff rendering style to use. */
	diffStyle: DiffStyle;
	/**
	 * Called when a line number gutter element is clicked.
	 * Receives the file name and click metadata.
	 */
	onLineNumberClick?: (
		fileName: string,
		props: { lineNumber: number; annotationSide: "additions" | "deletions" },
	) => void;
	/**
	 * Called when a range of lines is selected (e.g. shift-click).
	 * Receives the file name and the selected range (or null on
	 * deselection).
	 */
	onLineSelected?: (
		fileName: string,
		range: {
			start: number;
			end: number;
			side?: "additions" | "deletions";
		} | null,
	) => void;
	/**
	 * Returns line annotations for the given file. Used to render
	 * inline widgets such as comment inputs.
	 */
	getLineAnnotations?: (fileName: string) => DiffLineAnnotation<string>[];
	/**
	 * Returns the selected line range for the given file, if any.
	 * Used to visually highlight the lines being commented on.
	 */
	getSelectedLines?: (fileName: string) => SelectedLineRange | null;
	/**
	 * Renderer for line annotations returned by `getLineAnnotations`.
	 */
	renderAnnotation?: (annotation: DiffLineAnnotation<string>) => ReactNode;
	/**
	 * When set to a file name, DiffViewer scrolls to that file and
	 * highlights it in the tree. The parent should reset this to
	 * null via `onScrollToFileComplete` after the scroll completes.
	 */
	scrollToFile?: string | null;
	/** Called after scrollToFile has been processed. */
	onScrollToFileComplete?: () => void;
}

// -------------------------------------------------------------------
// Constants
// -------------------------------------------------------------------

/**
 * Minimum container width (px) at which the file tree sidebar
 * is shown alongside the diff list.
 */
const FILE_TREE_THRESHOLD = 1000;

/**
 * Extra CSS injected via the diff viewer's `unsafeCSS` option to make
 * file headers sticky and adjust metadata layout.
 */
const STICKY_HEADER_CSS = [
	// Layout and sticky behavior.
	"[data-diffs-header] {",
	"  position: sticky; top: 0; z-index: 10;",
	"  font-size: 13px;",
	"  min-height: 32px !important;",
	"  padding-block: 0 !important;",
	"  padding-inline: 12px !important;",
	"  border-bottom: 1px solid hsl(var(--border-default));",
	"  background-color: hsl(var(--surface-secondary)) !important;",
	"}",

	// Keep the title in the site's sans-serif font, just a
	// touch smaller than the surrounding header text.
	"[data-diffs-header] [data-title] {",
	"  font-size: 12px;",
	"  color: hsl(var(--content-primary));",
	"}",

	// Hide the library's built-in change-type SVG icons and
	// replace them with a single-letter badge (A/D/M/R) via
	// CSS-generated content. The letter mirrors the file tree
	// sidebar and works even when the tree is hidden in narrow
	// layouts.
	"[data-change-icon] { display: none !important; }",
	"[data-diffs-header] [data-header-content]::before {",
	"  font-size: 11px;",
	"  font-weight: 600;",
	"  flex-shrink: 0;",
	"}",
	"[data-diffs-header][data-change-type='new'] [data-header-content]::before {",
	"  content: 'A';",
	"  color: hsl(var(--git-added));",
	"}",
	"[data-diffs-header][data-change-type='change'] [data-header-content]::before {",
	"  content: 'M';",
	"  color: hsl(var(--git-modified));",
	"}",
	"[data-diffs-header][data-change-type='deleted'] [data-header-content]::before {",
	"  content: 'D';",
	"  color: hsl(var(--git-deleted));",
	"}",
	"[data-diffs-header][data-change-type='rename-pure'] [data-header-content]::before,",
	"[data-diffs-header][data-change-type='rename-changed'] [data-header-content]::before {",
	"  content: 'R';",
	"  color: hsl(var(--git-modified));",
	"}",

	// Stat counts styled as compact pill badges matching the
	// DiffStatBadge component used in the PR header.
	"[data-diffs-header] [data-metadata] {",
	"  flex-direction: row-reverse;",
	"  gap: 0 !important;",
	"}",
	"[data-diffs-header] [data-additions-count],",
	"[data-diffs-header] [data-deletions-count] {",
	"  font-family: var(--diffs-font-family, var(--diffs-font-fallback));",
	"  font-size: 12px;",
	"  font-weight: 500;",
	"  line-height: 20px;",
	"  padding-inline: 6px;",
	"  border-radius: 3px;",
	"}",
	"[data-diffs-header] [data-additions-count] {",
	"  color: hsl(var(--git-added-bright)) !important;",
	"  background-color: hsl(var(--surface-git-added));",
	"}",
	"[data-diffs-header] [data-deletions-count] {",
	"  color: hsl(var(--git-deleted-bright)) !important;",
	"  background-color: hsl(var(--surface-git-deleted));",
	"}",
	// When both counts are present, flatten the touching inner
	// edges so they form one joined badge. DOM order is
	// [deletions][additions]; row-reverse puts additions left.
	"[data-deletions-count] + [data-additions-count] {",
	"  border-radius: 3px 0 0 3px;",
	"}",
	"[data-deletions-count]:has(+ [data-additions-count]) {",
	"  border-radius: 0 3px 3px 0;",
	"}",
].join(" ");

export type DiffStyle = "unified" | "split";
const DIFF_STYLE_KEY = "agents.diff-view-style";

export function loadDiffStyle(): DiffStyle {
	if (typeof window === "undefined") {
		return "unified";
	}
	const stored = localStorage.getItem(DIFF_STYLE_KEY);
	if (stored === "split" || stored === "unified") {
		return stored;
	}
	return "unified";
}

export function saveDiffStyle(style: DiffStyle): void {
	localStorage.setItem(DIFF_STYLE_KEY, style);
}

/** Width of the file tree sidebar in pixels. */
const FILE_TREE_WIDTH = 300;

// -------------------------------------------------------------------
// Estimated diff height for lazy loading
// -------------------------------------------------------------------

/**
 * Estimated height per line in the diff viewer (px). Derived from
 * the --diffs-font-size (11px) and --diffs-line-height (1.5)
 * values set via DIFFS_FONT_STYLE, plus 1px for the border/gap.
 */
const LINE_HEIGHT_PX = 17.5;

/** Height of the file header row rendered by @pierre/diffs. */
const HEADER_HEIGHT_PX = 36;

/**
 * Estimate the rendered pixel height of a file diff so the
 * placeholder occupies roughly the same space. This keeps the
 * scroll position stable as files are lazily mounted.
 */
function estimateDiffHeight(fileDiff: FileDiffMetadata): number {
	return HEADER_HEIGHT_PX + fileDiff.unifiedLineCount * LINE_HEIGHT_PX;
}

// -------------------------------------------------------------------
// File tree data model
// -------------------------------------------------------------------

interface FileTreeNode {
	name: string;
	fullPath: string;
	type: "file" | "directory";
	children: FileTreeNode[];
	fileDiff?: FileDiffMetadata;
}

/**
 * Builds a nested tree from a flat list of file diffs. Directory
 * nodes are created for every intermediate path segment. The
 * result is sorted with directories first, then alphabetically.
 * Single-child directory chains are collapsed so that e.g.
 * `src/pages/AgentsPage` renders as one row.
 */
function buildFileTree(files: readonly FileDiffMetadata[]): FileTreeNode[] {
	const root: FileTreeNode[] = [];

	for (const file of files) {
		const segments = file.name.split("/");
		let children = root;

		// Walk / create intermediate directory nodes.
		for (let i = 0; i < segments.length - 1; i++) {
			const seg = segments[i];
			let dir = children.find((n) => n.type === "directory" && n.name === seg);
			if (!dir) {
				dir = {
					name: seg,
					fullPath: segments.slice(0, i + 1).join("/"),
					type: "directory",
					children: [],
				};
				children.push(dir);
			}
			children = dir.children;
		}

		// Leaf file node.
		const fileName = segments[segments.length - 1];
		children.push({
			name: fileName,
			fullPath: file.name,
			type: "file",
			children: [],
			fileDiff: file,
		});
	}

	const sortNodes = (nodes: FileTreeNode[]): FileTreeNode[] => {
		for (const node of nodes) {
			if (node.children.length > 0) {
				node.children = sortNodes(node.children);
			}
		}
		return nodes.sort((a, b) => {
			if (a.type !== b.type) {
				return a.type === "directory" ? -1 : 1;
			}
			return a.name.localeCompare(b.name);
		});
	};

	// Collapse single-child directory chains into one node whose
	// name uses path separators, e.g. "src/pages/AgentsPage".
	const collapse = (nodes: FileTreeNode[]): FileTreeNode[] => {
		for (const node of nodes) {
			if (node.type === "directory") {
				node.children = collapse(node.children);
				// If this directory has exactly one child and it is also
				// a directory, merge them.
				while (
					node.children.length === 1 &&
					node.children[0].type === "directory"
				) {
					const child = node.children[0];
					node.name = `${node.name}/${child.name}`;
					node.fullPath = child.fullPath;
					node.children = child.children;
				}
			}
		}
		return nodes;
	};

	return collapse(sortNodes(root));
}

// -------------------------------------------------------------------
// Tree node renderer
// -------------------------------------------------------------------

const FileTreeNodeView: FC<{
	node: FileTreeNode;
	depth: number;
	activeFile: string | null;
	onFileClick: (fullPath: string) => void;
}> = ({ node, depth, activeFile, onFileClick }) => {
	const [expanded, setExpanded] = useState(true);

	if (node.type === "directory") {
		return (
			<div>
				<button
					type="button"
					onClick={() => setExpanded((v) => !v)}
					className="flex w-full items-center gap-1.5 rounded-none border-none bg-transparent py-1 text-left text-content-secondary hover:bg-surface-secondary cursor-pointer outline-none"
					style={{ paddingLeft: 4 + depth * 8, fontSize: 13 }}
					aria-expanded={expanded}
				>
					<ChevronRightIcon
						className={cn(
							"size-3 shrink-0 transition-transform",
							expanded && "rotate-90",
						)}
					/>
					<span className="truncate">{node.name}</span>
				</button>
				{expanded &&
					node.children.map((child) => (
						<FileTreeNodeView
							key={child.fullPath}
							node={child}
							depth={depth + 1}
							activeFile={activeFile}
							onFileClick={onFileClick}
						/>
					))}
			</div>
		);
	}

	const isActive = activeFile === node.fullPath;

	return (
		<button
			type="button"
			onClick={() => onFileClick(node.fullPath)}
			className={cn(
				"flex w-full items-center gap-1.5 rounded-none border-none bg-transparent py-1 text-left cursor-pointer outline-none border-0 border-r-2 border-solid border-transparent",
				"hover:bg-surface-secondary",
				isActive && "bg-surface-secondary border-content-link",
			)}
			style={{ paddingLeft: 4 + depth * 8 + 12, fontSize: 13 }}
			title={node.fullPath}
		>
			<FileIcon fileName={node.name} className="shrink-0" />
			<span
				className={cn(
					"truncate",
					changeColor(node.fileDiff?.type) ??
						(isActive ? "text-content-primary" : "text-content-secondary"),
				)}
			>
				{node.name}
			</span>
			{node.fileDiff?.type && (
				<span
					className={cn(
						"ml-auto shrink-0 pr-2 text-xs",
						changeColor(node.fileDiff.type),
					)}
				>
					{changeLabel(node.fileDiff.type)}
				</span>
			)}
		</button>
	);
};

// -------------------------------------------------------------------
// Lazy file diff wrapper
// -------------------------------------------------------------------

/**
 * Wraps a single `<FileDiff>` with an IntersectionObserver so the
 * heavy component (Shadow DOM + shiki highlighting) is only mounted
 * once the placeholder scrolls into or near the viewport.
 *
 * Once mounted the component stays mounted — we never unmount a
 * FileDiff that the user has already scrolled past, which avoids
 * layout shifts and repeated highlighting work.
 */
const LazyFileDiff = memo<{
	fileDiff: FileDiffMetadata;
	options: ComponentProps<typeof FileDiff>["options"];
	lineAnnotations?: DiffLineAnnotation<string>[];
	renderAnnotation?: (annotation: DiffLineAnnotation<string>) => ReactNode;
	selectedLines?: SelectedLineRange | null;
}>(
	({
		fileDiff,
		options,
		lineAnnotations,
		renderAnnotation: renderAnnotationProp,
		selectedLines,
	}) => {
		const placeholderRef = useRef<HTMLDivElement>(null);
		const [visible, setVisible] = useState(false);

		useEffect(() => {
			const el = placeholderRef.current;
			if (!el || visible) {
				return;
			}
			const observer = new IntersectionObserver(
				([entry]) => {
					if (entry.isIntersecting) {
						setVisible(true);
						observer.disconnect();
					}
				},
				// Pre-load files that are within one viewport-height of
				// the visible area so they are ready before the user
				// scrolls to them.
				{ rootMargin: "100% 0px" },
			);
			observer.observe(el);
			return () => observer.disconnect();
		}, [visible]);

		if (!visible) {
			return (
				<div
					ref={placeholderRef}
					style={{ height: estimateDiffHeight(fileDiff) }}
					className="p-4 space-y-2"
				>
					<Skeleton className="h-4 w-48" />
					<Skeleton className="h-3 w-full" />
					<Skeleton className="h-3 w-full" />
					<Skeleton className="h-3 w-3/4" />
				</div>
			);
		}

		return (
			<FileDiff
				fileDiff={fileDiff}
				options={options}
				style={DIFFS_FONT_STYLE}
				lineAnnotations={lineAnnotations}
				renderAnnotation={renderAnnotationProp}
				selectedLines={selectedLines}
			/>
		);
	},
	(prev, next) => {
		if (
			prev.fileDiff !== next.fileDiff ||
			prev.options !== next.options ||
			prev.lineAnnotations !== next.lineAnnotations ||
			prev.selectedLines !== next.selectedLines
		) {
			return false;
		}
		// When neither the previous nor next props include line
		// annotations, a new `renderAnnotation` reference is
		// irrelevant because there is nothing to render. Skip the
		// re-render so that unrelated state changes (e.g.
		// activeCommentBox) don't bust memo for every file.
		if (prev.renderAnnotation !== next.renderAnnotation) {
			return !prev.lineAnnotations && !next.lineAnnotations;
		}
		return true;
	},
);

// -------------------------------------------------------------------
// Main component
// -------------------------------------------------------------------

export const DiffViewer: FC<DiffViewerProps> = ({
	parsedFiles,
	isExpanded,
	isLoading,
	error,
	emptyMessage = "No file changes to display.",
	diffStyle,
	onLineNumberClick,
	onLineSelected,
	getLineAnnotations,
	getSelectedLines,
	renderAnnotation,
	scrollToFile,
	onScrollToFileComplete,
}) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";

	const diffOptions = useMemo(() => {
		const base = getDiffViewerOptions(isDark);
		return {
			...base,
			diffStyle,
			// Extend the base CSS to make file headers sticky so they
			// remain visible while scrolling through long diffs.
			unsafeCSS: `${base.unsafeCSS ?? ""} ${STICKY_HEADER_CSS}`,
		};
	}, [isDark, diffStyle]);

	// Memoize the per-file options object so every <FileDiff>
	// receives the same reference and avoids re-highlighting
	// when the parent re-renders.
	const fileOptions = useMemo(
		() => ({
			...diffOptions,
			overflow: "wrap" as const,
			enableLineSelection: true,
			enableHoverUtility: true,
			onLineSelected() {
				// TODO: Make this add context to the input so the
				// user can type.
			},
		}),
		[diffOptions],
	);

	// When the parent provides per-file callbacks (e.g. line click
	// handlers for comment inputs), build options per file. Otherwise
	// share a single stable object to avoid unnecessary re-highlights.
	const hasPerFileCallbacks = !!(onLineNumberClick || onLineSelected);

	const getOptionsForFile = useCallback(
		(fileName: string) => ({
			...diffOptions,
			overflow: "wrap" as const,
			enableLineSelection: true,
			enableHoverUtility: true,
			...(onLineNumberClick && {
				onLineNumberClick: (props: {
					lineNumber: number;
					annotationSide: "additions" | "deletions";
				}) => onLineNumberClick(fileName, props),
			}),
			onLineSelected: onLineSelected
				? (
						range: {
							start: number;
							end: number;
							side?: "additions" | "deletions";
						} | null,
					) => onLineSelected(fileName, range)
				: () => {
						// TODO: Make this add context to the input.
					},
		}),
		[diffOptions, onLineNumberClick, onLineSelected],
	);

	const fileTree = useMemo(() => buildFileTree(parsedFiles), [parsedFiles]);

	// Sort diff blocks in the same order the file tree displays them
	// (directories first, then alphabetical) so the rendering is
	// consistent regardless of whether the sidebar is visible.
	const sortedFiles = useMemo(() => {
		const order = new Map<string, number>();
		let idx = 0;
		const walk = (nodes: FileTreeNode[]) => {
			for (const node of nodes) {
				if (node.type === "file") {
					order.set(node.fullPath, idx++);
				} else {
					walk(node.children);
				}
			}
		};
		walk(fileTree);
		return [...parsedFiles].sort(
			(a, b) => (order.get(a.name) ?? 0) - (order.get(b.name) ?? 0),
		);
	}, [fileTree, parsedFiles]);

	// Pre-compute per-file options so each LazyFileDiff receives a
	// stable reference and avoids re-highlighting on parent re-render.
	const perFileOptions = useMemo(() => {
		if (!hasPerFileCallbacks) return null;
		const map = new Map<string, ComponentProps<typeof FileDiff>["options"]>();
		for (const file of sortedFiles) {
			map.set(file.name, getOptionsForFile(file.name));
		}
		return map;
	}, [hasPerFileCallbacks, sortedFiles, getOptionsForFile]);

	// Pre-compute per-file line annotations for the same reason.
	const perFileAnnotations = useMemo(() => {
		if (!getLineAnnotations) return null;
		const map = new Map<string, DiffLineAnnotation<string>[]>();
		for (const file of sortedFiles) {
			const annotations = getLineAnnotations(file.name);
			if (annotations.length > 0) {
				map.set(file.name, annotations);
			}
		}
		return map;
	}, [sortedFiles, getLineAnnotations]);

	// ---------------------------------------------------------------
	// Container width measurement via ResizeObserver so we can decide
	// whether to show the file tree sidebar without a prop from the
	// parent.
	// ---------------------------------------------------------------
	const [containerWidth, setContainerWidth] = useState(0);
	const roRef = useRef<ResizeObserver | null>(null);
	const containerRef = useCallback((el: HTMLDivElement | null) => {
		if (roRef.current) {
			roRef.current.disconnect();
			roRef.current = null;
		}
		if (!el) {
			return;
		}
		setContainerWidth(el.getBoundingClientRect().width);
		const ro = new ResizeObserver(([entry]) => {
			setContainerWidth(entry.contentRect.width);
		});
		ro.observe(el);
		roRef.current = ro;
	}, []);

	const showTree =
		(isExpanded || containerWidth >= FILE_TREE_THRESHOLD) &&
		sortedFiles.length > 0;

	// ---------------------------------------------------------------
	// Refs for each file diff wrapper so we can scroll-to and track
	// which file is currently visible.
	// ---------------------------------------------------------------
	const fileRefs = useRef<Map<string, HTMLDivElement>>(new Map());
	const [activeFile, setActiveFile] = useState<string | null>(null);

	// Keep a ref callback that sets up per-file refs.
	const setFileRef = useCallback((name: string, el: HTMLDivElement | null) => {
		if (el) {
			fileRefs.current.set(name, el);
		} else {
			fileRefs.current.delete(name);
		}
	}, []);

	// Track which file is at the top of the diff scroll area by
	// listening to scroll events on the viewport. The active file
	// is whichever file wrapper's top edge is closest to (but not
	// below) the container's top — i.e. the one whose sticky
	// header would be showing.
	const diffViewportRef = useRef<HTMLElement | null>(null);

	useEffect(() => {
		if (!showTree || sortedFiles.length === 0) {
			return;
		}

		const viewport = diffViewportRef.current;
		if (!viewport) {
			return;
		}

		let rafId = 0;
		const onScroll = () => {
			cancelAnimationFrame(rafId);
			rafId = requestAnimationFrame(() => {
				const containerTop = viewport.getBoundingClientRect().top;
				let bestName: string | null = null;
				let bestDistance = Number.POSITIVE_INFINITY;

				for (const [name, el] of fileRefs.current.entries()) {
					const rect = el.getBoundingClientRect();
					// The file "owns" the scroll position when its top
					// is at or above the container top and its bottom is
					// still below it.
					if (rect.bottom > containerTop && rect.top <= containerTop + 1) {
						const distance = Math.abs(rect.top - containerTop);
						if (distance < bestDistance) {
							bestDistance = distance;
							bestName = name;
						}
					}
				}

				// If nothing is at the top (e.g. scrolled to the very top
				// with padding), pick the first file whose top is closest
				// to the container top.
				if (!bestName) {
					for (const [name, el] of fileRefs.current.entries()) {
						const dist = Math.abs(
							el.getBoundingClientRect().top - containerTop,
						);
						if (dist < bestDistance) {
							bestDistance = dist;
							bestName = name;
						}
					}
				}

				if (bestName) {
					setActiveFile(bestName);
				}
			});
		};

		// Fire once to set initial state.
		onScroll();

		viewport.addEventListener("scroll", onScroll, { passive: true });
		return () => {
			cancelAnimationFrame(rafId);
			viewport.removeEventListener("scroll", onScroll);
		};
	}, [showTree, sortedFiles.length]);

	const handleFileClick = useCallback((name: string) => {
		const el = fileRefs.current.get(name);
		if (el) {
			el.scrollIntoView({ block: "start" });
			setActiveFile(name);
		}
	}, []);

	// Scroll to a file programmatically when the parent sets
	// scrollToFile. This enables external navigation (e.g.
	// clicking a file reference chip in the chat input).
	useEffect(() => {
		if (scrollToFile) {
			const el = fileRefs.current.get(scrollToFile);
			if (el) {
				el.scrollIntoView({ block: "start", behavior: "smooth" });
				setActiveFile(scrollToFile);
			}
			onScrollToFileComplete?.();
		}
	}, [scrollToFile, onScrollToFileComplete]);

	// ---------------------------------------------------------------
	// Viewport height for the last-file min-height trick: setting
	// min-height on the last file wrapper lets CSS handle the
	// "be at least viewport-tall" logic, removing the need for a
	// separate spacer div and a second ResizeObserver. Uses a ref
	// callback (same pattern as containerRef) so the measurement
	// lands during commit — before useEffect-based scroll logic.
	// ---------------------------------------------------------------
	const [viewportHeight, setViewportHeight] = useState(0);
	const scrollAreaRef = useCallback((node: HTMLElement | null) => {
		const vp = node?.querySelector<HTMLElement>(
			"[data-radix-scroll-area-viewport]",
		);
		diffViewportRef.current = vp ?? null;

		if (!vp) return;

		setViewportHeight(vp.clientHeight);
		const ro = new ResizeObserver(([entry]) => {
			setViewportHeight(entry.contentRect.height);
		});
		ro.observe(vp);
		return () => {
			ro.disconnect();
			diffViewportRef.current = null;
		};
	}, []);

	// ---------------------------------------------------------------
	// Loading state
	// ---------------------------------------------------------------
	if (isLoading) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden">
				<div className="space-y-4 p-4">
					{Array.from({ length: 3 }, (_, i) => (
						<div key={i} className="space-y-2">
							<Skeleton className="h-4 w-48" />
							<Skeleton className="h-3 w-full" />
							<Skeleton className="h-3 w-full" />
							<Skeleton className="h-3 w-3/4" />
						</div>
					))}
				</div>
			</div>
		);
	}

	// ---------------------------------------------------------------
	// Error state
	// ---------------------------------------------------------------
	if (error) {
		return (
			<div className="p-3">
				<ErrorAlert error={error} />
			</div>
		);
	}

	// ---------------------------------------------------------------
	// Main render
	// ---------------------------------------------------------------
	return (
		<div
			ref={containerRef}
			className="flex h-full min-w-0 flex-col overflow-hidden"
		>
			{/* Diff contents */}
			{sortedFiles.length === 0 ? (
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					{emptyMessage}
				</div>
			) : (
				<div className="flex min-w-0 flex-1 flex-row overflow-hidden">
					{/* File tree sidebar */}
					{showTree && (
						<ScrollArea
							className="shrink-0 border-r border-border"
							style={{ width: FILE_TREE_WIDTH }}
							scrollBarClassName="w-1"
						>
							<nav className="flex flex-col py-1">
								{fileTree.map((node) => (
									<FileTreeNodeView
										key={node.fullPath}
										node={node}
										depth={1}
										activeFile={activeFile}
										onFileClick={handleFileClick}
									/>
								))}
							</nav>
						</ScrollArea>
					)}
					{/* Diff list */}
					<ScrollArea
						className={cn(
							"min-w-0 flex-1",
							showTree &&
								"border-0 border-l border-t border-solid border-border-default rounded-tl-md",
						)}
						scrollBarClassName="w-1.5"
						viewportClassName="[&>div]:!block"
						ref={scrollAreaRef}
					>
						<div className="min-w-0 text-xs">
							{sortedFiles.map((fileDiff, i) => {
								const isLast = i === sortedFiles.length - 1;
								return (
									<div
										key={fileDiff.name}
										ref={(el) => setFileRef(fileDiff.name, el)}
										style={isLast ? { minHeight: viewportHeight } : undefined}
									>
										<LazyFileDiff
											fileDiff={fileDiff}
											options={
												perFileOptions?.get(fileDiff.name) ?? fileOptions
											}
											lineAnnotations={perFileAnnotations?.get(fileDiff.name)}
											renderAnnotation={renderAnnotation}
											selectedLines={getSelectedLines?.(fileDiff.name)}
										/>
										{isLast && (
											<div className="flex items-center justify-center py-4 text-xs text-content-secondary">
												{`${sortedFiles.length} ${sortedFiles.length === 1 ? "file" : "files"} changed`}
											</div>
										)}
									</div>
								);
							})}
						</div>
					</ScrollArea>
				</div>
			)}
		</div>
	);
};
