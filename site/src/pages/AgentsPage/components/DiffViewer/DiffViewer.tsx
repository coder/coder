import { useTheme } from "@emotion/react";
import type {
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
} from "@pierre/diffs";
import { Virtualizer } from "@pierre/diffs";
import { FileDiff, VirtualizerContext } from "@pierre/diffs/react";
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
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import { changeColor, changeLabel } from "../../utils/diffColors";

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
			endSide?: "additions" | "deletions";
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
 * file headers sticky with a solid background. The shared header
 * styling (font sizing, change-type badges, stat-count pills) lives
 * in `diffViewerCSS` from utils.ts and is already included in the
 * base options returned by `getDiffViewerOptions`.
 */
const STICKY_HEADER_CSS = [
	"[data-diffs-header] {",
	"  position: sticky; top: 0; z-index: 10;",
	"  background-color: hsl(var(--surface-secondary)) !important;",
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
// Virtualized scroll container
// -------------------------------------------------------------------

/**
 * Wraps the diff list in a Radix ScrollArea and wires up the
 * @pierre/diffs Virtualizer. Extracted into its own component so
 * that the useCallback for ref-callback stability lives here
 * instead of in DiffViewer. This lets the React Compiler skip
 * only this small wrapper (where it can't preserve useCallback)
 * while still optimizing the much larger parent.
 *
 * The ref callback is placed on a content div *inside* the
 * ScrollArea, then walks up with closest() to the Radix viewport.
 * React fires children's refs bottom-up during commit, so every
 * VirtualizedFileDiff instance has already connected to the
 * virtualizer by the time this ref fires and calls setup().
 */
const DiffScrollContainer: FC<{
	children: ReactNode;
	className?: string;
	diffViewportRef: React.RefObject<HTMLElement | null>;
	onViewportHeight: (height: number) => void;
}> = ({ children, className, diffViewportRef, onViewportHeight }) => {
	const [virtualizer] = useState(() => new Virtualizer());

	// useCallback is required for correctness: in React 19 an
	// unstable ref callback triggers old-cleanup → new-callback
	// on every render, which calls virtualizer.cleanUp() and
	// wipes the observer map. The compiler can't preserve this
	// useCallback, but that only causes it to skip this small
	// wrapper — not the entire DiffViewer.
	const contentRef = useCallback(
		(node: HTMLDivElement | null) => {
			const viewport = node?.closest<HTMLElement>(
				"[data-radix-scroll-area-viewport]",
			);
			if (!viewport) return;

			diffViewportRef.current = viewport;
			virtualizer.setup(viewport);

			onViewportHeight(viewport.clientHeight);
			const ro = new ResizeObserver(([entry]) => {
				onViewportHeight(entry.contentRect.height);
			});
			ro.observe(viewport);
			return () => {
				ro.disconnect();
				virtualizer.cleanUp();
				diffViewportRef.current = null;
			};
		},
		[virtualizer, diffViewportRef, onViewportHeight],
	);

	return (
		<ScrollArea
			className={className}
			scrollBarClassName="w-1.5"
			viewportClassName="[&>div]:!block"
		>
			<VirtualizerContext.Provider value={virtualizer}>
				<div ref={contentRef} className="min-w-0 text-xs">
					{children}
				</div>
			</VirtualizerContext.Provider>
		</ScrollArea>
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

	const diffOptions = (() => {
		const base = getDiffViewerOptions(isDark);
		return {
			...base,
			diffStyle,
			// Extend the base CSS to make file headers sticky so they
			// remain visible while scrolling through long diffs.
			unsafeCSS: `${base.unsafeCSS ?? ""} ${STICKY_HEADER_CSS}`,
		};
	})();

	const fileOptions = {
		...diffOptions,
		overflow: "wrap" as const,
		enableLineSelection: true,
		enableHoverUtility: true,
		onLineSelected() {
			// TODO: Make this add context to the input so the
			// user can type.
		},
	};

	// When the parent provides per-file callbacks (e.g. line click
	// handlers for comment inputs), build options per file. Otherwise
	// share a single stable object to avoid unnecessary re-highlights.
	const hasPerFileCallbacks = !!(onLineNumberClick || onLineSelected);

	const getOptionsForFile = (fileName: string) => ({
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
						endSide?: "additions" | "deletions";
					} | null,
				) => onLineSelected(fileName, range)
			: () => {
					// TODO: Make this add context to the input.
				},
	});

	const fileTree = buildFileTree(parsedFiles);

	// Sort diff blocks in the same order the file tree displays them
	// (directories first, then alphabetical) so the rendering is
	// consistent regardless of whether the sidebar is visible.
	const sortedFiles = (() => {
		const order = new Map<string, number>();
		const walk = (nodes: FileTreeNode[]) => {
			for (const node of nodes) {
				if (node.type === "file") {
					order.set(node.fullPath, order.size);
				} else {
					walk(node.children);
				}
			}
		};
		walk(fileTree);
		return [...parsedFiles].sort(
			(a, b) => (order.get(a.name) ?? 0) - (order.get(b.name) ?? 0),
		);
	})();

	// Pre-compute per-file options so each LazyFileDiff receives a
	// stable reference and avoids re-highlighting on parent re-render.
	const perFileOptions = (() => {
		if (!hasPerFileCallbacks) return null;
		const map = new Map<string, ComponentProps<typeof FileDiff>["options"]>();
		for (const file of sortedFiles) {
			map.set(file.name, getOptionsForFile(file.name));
		}
		return map;
	})();

	// Pre-compute per-file line annotations for the same reason.
	const perFileAnnotations = (() => {
		if (!getLineAnnotations) return null;
		return new Map(
			sortedFiles
				.map((f) => [f.name, getLineAnnotations(f.name)] as const)
				.filter(
					(entry): entry is [string, DiffLineAnnotation<string>[]] =>
						entry[1].length > 0,
				),
		);
	})();

	// Pre-compute per-file selected lines so each LazyFileDiff
	// receives a stable reference. Without this, calling
	// getSelectedLines during render returns a new object every
	// time, which busts the memo comparator and forces an
	// expensive Shadow DOM + shiki re-highlight.
	const perFileSelectedLines = (() => {
		if (!getSelectedLines) return null;
		return new Map(
			sortedFiles
				.map((f) => [f.name, getSelectedLines(f.name)] as const)
				.filter(
					(entry): entry is [string, SelectedLineRange] => entry[1] != null,
				),
		);
	})();

	// ---------------------------------------------------------------
	// Container width measurement via ResizeObserver so we can decide
	// whether to show the file tree sidebar without a prop from the
	// parent.
	// ---------------------------------------------------------------
	const [containerWidth, setContainerWidth] = useState(0);
	const [containerEl, setContainerEl] = useState<HTMLDivElement | null>(null);

	useEffect(() => {
		if (!containerEl) return;
		setContainerWidth(containerEl.getBoundingClientRect().width);
		const ro = new ResizeObserver(([entry]) => {
			setContainerWidth(entry.contentRect.width);
		});
		ro.observe(containerEl);
		return () => ro.disconnect();
	}, [containerEl]);

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
	const setFileRef = (name: string, el: HTMLDivElement | null) => {
		if (el) {
			fileRefs.current.set(name, el);
		} else {
			fileRefs.current.delete(name);
		}
	};

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

	const handleFileClick = (name: string) => {
		const el = fileRefs.current.get(name);
		if (el) {
			el.scrollIntoView({ block: "start" });
			setActiveFile(name);
		}
	};

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

	const [viewportHeight, setViewportHeight] = useState(0);
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
			ref={setContainerEl}
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
					<DiffScrollContainer
						diffViewportRef={diffViewportRef}
						onViewportHeight={setViewportHeight}
						className={cn(
							"min-w-0 flex-1",
							showTree &&
								"border-0 border-l border-t border-solid border-border-default rounded-tl-md",
						)}
					>
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
										options={perFileOptions?.get(fileDiff.name) ?? fileOptions}
										lineAnnotations={perFileAnnotations?.get(fileDiff.name)}
										renderAnnotation={renderAnnotation}
										selectedLines={
											perFileSelectedLines?.get(fileDiff.name) ?? null
										}
									/>
									{isLast && (
										<div className="flex items-center justify-center py-4 text-xs text-content-secondary">
											{`${sortedFiles.length} ${sortedFiles.length === 1 ? "file" : "files"} changed`}
										</div>
									)}
								</div>
							);
						})}
					</DiffScrollContainer>
				</div>
			)}
		</div>
	);
};
