import { useTheme } from "@emotion/react";
import type {
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
	VirtualFileMetrics,
} from "@pierre/diffs";
import { Virtualizer } from "@pierre/diffs";
import { FileDiff, VirtualizerContext } from "@pierre/diffs/react";
import { ChevronRightIcon } from "lucide-react";
import {
	type ComponentProps,
	type FC,
	memo,
	type ReactNode,
	type RefObject,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { FileIcon } from "#/components/FileIcon/FileIcon";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	DIFFS_FONT_STYLE,
	getDiffViewerOptions,
} from "#/pages/AgentsPage/components/ChatElements/tools/utils";
import { getDiffRenderMode } from "#/pages/AgentsPage/components/DiffViewer/diffPerformance";
import { cn } from "#/utils/cn";
import {
	changeColor,
	changeLabel,
} from "../../pages/AgentsPage/utils/diffColors";

export interface ManagedDiffViewerProps {
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
	diffStyle: "unified" | "split";
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

/**
 * Minimum container width (px) at which the file tree sidebar is shown
 * alongside the diff list.
 */
const FILE_TREE_THRESHOLD = 1000;

/**
 * Extra CSS injected via the diff viewer's `unsafeCSS` option to make
 * file headers sticky with a solid background. The shared header styling
 * (font sizing, change-type badges, stat-count pills) lives in
 * `diffViewerCSS` from utils.ts and is already included in the base
 * options returned by `getDiffViewerOptions`.
 */
const STICKY_HEADER_CSS = [
	"[data-diffs-header] {",
	"  position: sticky; top: 0; z-index: 10;",
	"  background-color: hsl(var(--surface-secondary)) !important;",
	"  padding-block: 0 !important;",
	"}",
].join(" ");

const NON_STICKY_HEADER_CSS = [
	"[data-diffs-header] {",
	"  position: relative; top: auto; z-index: auto;",
	"  background-color: transparent !important;",
	"  padding-block: 0 !important;",
	"}",
].join(" ");

/** Width of the file tree sidebar in pixels. */
const FILE_TREE_WIDTH = 300;

/**
 * Estimated height per line in the diff viewer (px). The library's
 * shadow DOM applies box-sizing: border-box to all elements, and code
 * lines have no padding or border, so the rendered height equals the
 * CSS line-height: 11px × 1.5 = 16.5.
 */
const LINE_HEIGHT_PX = 16.5;

/** Height of the file header row rendered by @pierre/diffs. */
const HEADER_HEIGHT_PX = 36;

/**
 * Metrics that tell the @pierre/diffs virtualizer how tall each element
 * actually is after our CSS overrides. Without these the library falls
 * back to its built-in defaults (20 px lines, 44 px headers,
 * 32 px separators) which are larger than our custom styling, causing
 * visible blank buffers in the viewport.
 */
const VIRTUALIZER_METRICS: VirtualFileMetrics = {
	hunkLineCount: 50,
	lineHeight: LINE_HEIGHT_PX,
	diffHeaderHeight: 32,
	hunkSeparatorHeight: 28,
	fileGap: 2,
};

/**
 * Estimate the rendered pixel height of a file diff so the placeholder
 * occupies roughly the same space. This keeps the scroll position
 * stable as files are lazily mounted.
 */
function estimateDiffHeight(fileDiff: FileDiffMetadata): number {
	return HEADER_HEIGHT_PX + fileDiff.unifiedLineCount * LINE_HEIGHT_PX;
}

const NOOP_LINE_SELECTED = () => {};

interface FileTreeNode {
	name: string;
	fullPath: string;
	type: "file" | "directory";
	children: FileTreeNode[];
	fileDiff?: FileDiffMetadata;
}

/**
 * Builds a nested tree from a flat list of file diffs. Directory nodes
 * are created for every intermediate path segment. The result is sorted
 * with directories first, then alphabetically. Single-child directory
 * chains are collapsed so that e.g. `src/pages/AgentsPage` renders as
 * one row.
 */
function buildFileTree(files: readonly FileDiffMetadata[]): FileTreeNode[] {
	const root: FileTreeNode[] = [];

	for (const file of files) {
		const segments = file.name.split("/");
		let children = root;

		for (let index = 0; index < segments.length - 1; index++) {
			const segment = segments[index];
			let directory = children.find(
				(node) => node.type === "directory" && node.name === segment,
			);
			if (!directory) {
				directory = {
					name: segment,
					fullPath: segments.slice(0, index + 1).join("/"),
					type: "directory",
					children: [],
				};
				children.push(directory);
			}
			children = directory.children;
		}

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
		return nodes.sort((left, right) => {
			if (left.type !== right.type) {
				return left.type === "directory" ? -1 : 1;
			}
			return left.name.localeCompare(right.name);
		});
	};

	const collapse = (nodes: FileTreeNode[]): FileTreeNode[] => {
		for (const node of nodes) {
			if (node.type === "directory") {
				node.children = collapse(node.children);
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

function sortFilesByTree(
	parsedFiles: readonly FileDiffMetadata[],
	fileTree: readonly FileTreeNode[],
): readonly FileDiffMetadata[] {
	const order = new Map<string, number>();
	const walk = (nodes: readonly FileTreeNode[]) => {
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
		(left, right) => (order.get(left.name) ?? 0) - (order.get(right.name) ?? 0),
	);
}

function removeStaleEntries<T>(
	entries: Map<string, T>,
	activeFileNames: ReadonlySet<string>,
): void {
	for (const fileName of Array.from(entries.keys())) {
		if (!activeFileNames.has(fileName)) {
			entries.delete(fileName);
		}
	}
}

function getFileOptionsCacheKey(
	options: NonNullable<ComponentProps<typeof FileDiff>["options"]>,
	hasLineNumberClick: boolean,
	hasLineSelected: boolean,
): string {
	const theme = Array.isArray(options.theme)
		? options.theme.join(",")
		: String(options.theme ?? "");
	return [
		String(options.diffStyle ?? ""),
		String(options.diffIndicators ?? ""),
		String(options.overflow ?? ""),
		String(options.themeType ?? ""),
		theme,
		String(options.unsafeCSS ?? ""),
		hasLineNumberClick ? "click" : "noclick",
		hasLineSelected ? "select" : "noselect",
	].join("|");
}

function areLineAnnotationsEqual(
	left: readonly DiffLineAnnotation<string>[],
	right: readonly DiffLineAnnotation<string>[],
): boolean {
	if (left === right) {
		return true;
	}
	if (left.length !== right.length) {
		return false;
	}
	for (let index = 0; index < left.length; index++) {
		const leftAnnotation = left[index];
		const rightAnnotation = right[index];
		if (
			leftAnnotation.lineNumber !== rightAnnotation.lineNumber ||
			leftAnnotation.side !== rightAnnotation.side ||
			leftAnnotation.metadata !== rightAnnotation.metadata
		) {
			return false;
		}
	}
	return true;
}

function areSelectedLinesEqual(
	left: SelectedLineRange | null | undefined,
	right: SelectedLineRange | null | undefined,
): boolean {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return left == null && right == null;
	}
	return (
		left.start === right.start &&
		left.end === right.end &&
		left.side === right.side &&
		left.endSide === right.endSide
	);
}

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
					onClick={() => setExpanded((value) => !value)}
					className="flex w-full cursor-pointer items-center gap-1.5 rounded-none border-none bg-transparent py-1 text-left text-content-secondary outline-none hover:bg-surface-secondary"
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
				"flex w-full cursor-pointer items-center gap-1.5 rounded-none border-0 border-r-2 border-solid border-transparent bg-transparent py-1 text-left outline-none",
				"hover:bg-surface-secondary",
				isActive && "border-content-link bg-surface-secondary",
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

const FileTreeSidebar: FC<{
	fileTree: readonly FileTreeNode[];
	sortedFiles: readonly FileDiffMetadata[];
	diffViewportRef: RefObject<HTMLElement | null>;
	fileRefs: RefObject<Map<string, HTMLDivElement>>;
	scrollBehavior: ScrollBehavior;
	enableTreeSync: boolean;
	scrollToFile?: string | null;
}> = ({
	fileTree,
	sortedFiles,
	diffViewportRef,
	fileRefs,
	scrollBehavior,
	enableTreeSync,
	scrollToFile,
}) => {
	const [activeFile, setActiveFile] = useState<string | null>(
		sortedFiles[0]?.name ?? null,
	);

	useEffect(() => {
		setActiveFile((current) => {
			if (current && sortedFiles.some((file) => file.name === current)) {
				return current;
			}
			return sortedFiles[0]?.name ?? null;
		});
	}, [sortedFiles]);

	useEffect(() => {
		if (
			scrollToFile &&
			sortedFiles.some((file) => file.name === scrollToFile)
		) {
			setActiveFile(scrollToFile);
		}
	}, [scrollToFile, sortedFiles]);

	useEffect(() => {
		if (!enableTreeSync || sortedFiles.length === 0) {
			return;
		}

		const viewport = diffViewportRef.current;
		if (!viewport) {
			return;
		}

		const visibleFileNames = new Set<string>();
		const updateActiveFile = () => {
			const nextActiveFile =
				sortedFiles.find((file) => visibleFileNames.has(file.name))?.name ??
				sortedFiles[0]?.name ??
				null;
			if (nextActiveFile) {
				setActiveFile(nextActiveFile);
			}
		};

		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					if (!(entry.target instanceof HTMLDivElement)) {
						continue;
					}
					const fileName = entry.target.dataset.fileName;
					if (!fileName) {
						continue;
					}
					if (entry.isIntersecting) {
						visibleFileNames.add(fileName);
					} else {
						visibleFileNames.delete(fileName);
					}
				}
				updateActiveFile();
			},
			{
				root: viewport,
				threshold: 0,
				rootMargin: "-40px 0px -70% 0px",
			},
		);

		for (const element of fileRefs.current.values()) {
			observer.observe(element);
		}
		updateActiveFile();

		return () => {
			observer.disconnect();
		};
	}, [diffViewportRef, enableTreeSync, fileRefs, sortedFiles]);

	const handleFileClick = (fileName: string) => {
		const element = fileRefs.current.get(fileName);
		if (element) {
			element.scrollIntoView({ block: "start", behavior: scrollBehavior });
			setActiveFile(fileName);
		}
	};

	return (
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
	);
};

/**
 * Wraps the diff list in a Radix ScrollArea and wires up the
 * @pierre/diffs Virtualizer. Extracted into its own component so the
 * ref-callback stability lives here instead of in the parent.
 */
const DiffScrollContainer: FC<{
	children: ReactNode;
	className?: string;
	diffViewportRef: RefObject<HTMLElement | null>;
	virtualizerConfig: {
		overscrollSize: number;
		intersectionObserverMargin: number;
	};
}> = ({ children, className, diffViewportRef, virtualizerConfig }) => {
	const [virtualizer] = useState(() => new Virtualizer(virtualizerConfig));

	const contentRef = useCallback(
		(node: HTMLDivElement | null) => {
			const viewport = node?.closest<HTMLElement>(
				"[data-radix-scroll-area-viewport]",
			);
			if (!viewport) {
				return;
			}

			diffViewportRef.current = viewport;
			virtualizer.setup(viewport);

			return () => {
				virtualizer.cleanUp();
				diffViewportRef.current = null;
			};
		},
		[diffViewportRef, virtualizer],
	);

	return (
		<ScrollArea
			className={className}
			scrollBarClassName="w-1.5"
			viewportClassName="[&>div]:!block"
		>
			<VirtualizerContext value={virtualizer}>
				<div ref={contentRef} className="min-w-0 text-xs">
					{children}
				</div>
			</VirtualizerContext>
		</ScrollArea>
	);
};

interface LazyFileDiffProps {
	fileDiff: FileDiffMetadata;
	options: ComponentProps<typeof FileDiff>["options"];
	lineAnnotations?: DiffLineAnnotation<string>[];
	renderAnnotation?: (annotation: DiffLineAnnotation<string>) => ReactNode;
	selectedLines?: SelectedLineRange | null;
	lazyMountRootMargin: string;
}

const LazyFileDiff = memo(function LazyFileDiff({
	fileDiff,
	options,
	lineAnnotations,
	renderAnnotation: renderAnnotationProp,
	selectedLines,
	lazyMountRootMargin,
}: LazyFileDiffProps) {
	const placeholderRef = useRef<HTMLDivElement>(null);
	const [visible, setVisible] = useState(false);

	useEffect(() => {
		const element = placeholderRef.current;
		if (!element || visible) {
			return;
		}
		const observer = new IntersectionObserver(
			([entry]) => {
				if (entry.isIntersecting) {
					setVisible(true);
					observer.disconnect();
				}
			},
			{ rootMargin: lazyMountRootMargin },
		);
		observer.observe(element);
		return () => observer.disconnect();
	}, [lazyMountRootMargin, visible]);

	if (!visible) {
		return (
			<div
				ref={placeholderRef}
				style={{ height: estimateDiffHeight(fileDiff) }}
				className="space-y-2 p-4"
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
			metrics={VIRTUALIZER_METRICS}
			style={DIFFS_FONT_STYLE}
			lineAnnotations={lineAnnotations}
			renderAnnotation={renderAnnotationProp}
			selectedLines={selectedLines}
		/>
	);
});

export const ManagedDiffViewer: FC<ManagedDiffViewerProps> = ({
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
	const renderMode = useMemo(
		() => getDiffRenderMode(parsedFiles),
		[parsedFiles],
	);
	const fileTree = useMemo(() => buildFileTree(parsedFiles), [parsedFiles]);
	const sortedFiles = useMemo(
		() => sortFilesByTree(parsedFiles, fileTree),
		[fileTree, parsedFiles],
	);

	const lineNumberClickRef = useRef(onLineNumberClick);
	const lineSelectedRef = useRef(onLineSelected);
	const renderAnnotationRef = useRef(renderAnnotation);
	lineNumberClickRef.current = onLineNumberClick;
	lineSelectedRef.current = onLineSelected;
	renderAnnotationRef.current = renderAnnotation;

	const diffOptions = useMemo(() => {
		const base = getDiffViewerOptions(isDark);
		return {
			...base,
			diffStyle,
			overflow: renderMode.overflow,
			unsafeCSS: `${base.unsafeCSS ?? ""} ${
				renderMode.showStickyHeaders ? STICKY_HEADER_CSS : NON_STICKY_HEADER_CSS
			}`,
		};
	}, [diffStyle, isDark, renderMode.overflow, renderMode.showStickyHeaders]);

	const fileOptions = useMemo(
		() => ({
			...diffOptions,
			enableLineSelection: true,
			enableHoverUtility: true,
			onLineSelected: NOOP_LINE_SELECTED,
		}),
		[diffOptions],
	);

	const stableRenderAnnotation = useCallback(
		(annotation: DiffLineAnnotation<string>) =>
			renderAnnotationRef.current?.(annotation) ?? null,
		[],
	);
	const annotationRenderer = renderAnnotation
		? stableRenderAnnotation
		: undefined;

	const hasPerFileCallbacks = Boolean(onLineNumberClick || onLineSelected);
	const optionClickHandlersRef = useRef(
		new Map<
			string,
			(props: {
				lineNumber: number;
				annotationSide: "additions" | "deletions";
			}) => void
		>(),
	);
	const optionSelectedHandlersRef = useRef(
		new Map<
			string,
			(
				range: {
					start: number;
					end: number;
					side?: "additions" | "deletions";
					endSide?: "additions" | "deletions";
				} | null,
			) => void
		>(),
	);
	const perFileOptionsRef = useRef(
		new Map<string, ComponentProps<typeof FileDiff>["options"]>(),
	);
	const perFileOptionsKeyRef = useRef<string | null>(null);

	for (const file of sortedFiles) {
		if (onLineNumberClick && !optionClickHandlersRef.current.has(file.name)) {
			optionClickHandlersRef.current.set(file.name, (props) => {
				lineNumberClickRef.current?.(file.name, props);
			});
		}
		if (onLineSelected && !optionSelectedHandlersRef.current.has(file.name)) {
			optionSelectedHandlersRef.current.set(file.name, (range) => {
				lineSelectedRef.current?.(file.name, range);
			});
		}
	}

	const activeFileNames = new Set(sortedFiles.map((file) => file.name));
	removeStaleEntries(optionClickHandlersRef.current, activeFileNames);
	removeStaleEntries(optionSelectedHandlersRef.current, activeFileNames);
	removeStaleEntries(perFileOptionsRef.current, activeFileNames);

	const perFileOptionsKey = getFileOptionsCacheKey(
		fileOptions,
		Boolean(onLineNumberClick),
		Boolean(onLineSelected),
	);
	if (perFileOptionsKeyRef.current !== perFileOptionsKey) {
		perFileOptionsRef.current.clear();
		perFileOptionsKeyRef.current = perFileOptionsKey;
	}

	for (const file of sortedFiles) {
		if (perFileOptionsRef.current.has(file.name)) {
			continue;
		}
		const lineNumberClickHandler = optionClickHandlersRef.current.get(
			file.name,
		);
		const lineSelectedHandler =
			optionSelectedHandlersRef.current.get(file.name) ?? NOOP_LINE_SELECTED;
		perFileOptionsRef.current.set(file.name, {
			...fileOptions,
			...(onLineNumberClick && lineNumberClickHandler
				? { onLineNumberClick: lineNumberClickHandler }
				: {}),
			onLineSelected:
				onLineSelected != null ? lineSelectedHandler : NOOP_LINE_SELECTED,
		});
	}
	const perFileOptions = hasPerFileCallbacks ? perFileOptionsRef.current : null;

	const perFileAnnotationsRef = useRef(
		new Map<string, DiffLineAnnotation<string>[]>(),
	);
	const nextAnnotationFileNames = new Set<string>();
	if (getLineAnnotations) {
		for (const file of sortedFiles) {
			const annotations = getLineAnnotations(file.name);
			if (annotations.length === 0) {
				continue;
			}
			nextAnnotationFileNames.add(file.name);
			const previousAnnotations = perFileAnnotationsRef.current.get(file.name);
			if (
				!previousAnnotations ||
				!areLineAnnotationsEqual(previousAnnotations, annotations)
			) {
				perFileAnnotationsRef.current.set(file.name, annotations);
			}
		}
	}
	removeStaleEntries(perFileAnnotationsRef.current, nextAnnotationFileNames);
	const perFileAnnotations = getLineAnnotations
		? perFileAnnotationsRef.current
		: null;

	const perFileSelectedLinesRef = useRef(new Map<string, SelectedLineRange>());
	const nextSelectedLineFileNames = new Set<string>();
	if (getSelectedLines) {
		for (const file of sortedFiles) {
			const selectedLines = getSelectedLines(file.name);
			if (!selectedLines) {
				continue;
			}
			nextSelectedLineFileNames.add(file.name);
			const previousSelectedLines = perFileSelectedLinesRef.current.get(
				file.name,
			);
			if (!areSelectedLinesEqual(previousSelectedLines, selectedLines)) {
				perFileSelectedLinesRef.current.set(file.name, selectedLines);
			}
		}
	}
	removeStaleEntries(
		perFileSelectedLinesRef.current,
		nextSelectedLineFileNames,
	);
	const perFileSelectedLines = getSelectedLines
		? perFileSelectedLinesRef.current
		: null;

	const [containerWidth, setContainerWidth] = useState(0);
	const [containerEl, setContainerEl] = useState<HTMLDivElement | null>(null);

	useEffect(() => {
		if (!containerEl) {
			return;
		}
		setContainerWidth(containerEl.getBoundingClientRect().width);
		const observer = new ResizeObserver(([entry]) => {
			setContainerWidth(entry.contentRect.width);
		});
		observer.observe(containerEl);
		return () => observer.disconnect();
	}, [containerEl]);

	const showTree =
		(isExpanded || containerWidth >= FILE_TREE_THRESHOLD) &&
		sortedFiles.length > 0;

	const fileRefs = useRef<Map<string, HTMLDivElement>>(new Map());
	const diffViewportRef = useRef<HTMLElement | null>(null);
	const setFileRef = (fileName: string, element: HTMLDivElement | null) => {
		if (element) {
			fileRefs.current.set(fileName, element);
		} else {
			fileRefs.current.delete(fileName);
		}
	};

	useEffect(() => {
		if (!scrollToFile) {
			return;
		}
		const element = fileRefs.current.get(scrollToFile);
		if (element) {
			element.scrollIntoView({
				block: "start",
				behavior: renderMode.scrollBehavior,
			});
		}
		onScrollToFileComplete?.();
	}, [onScrollToFileComplete, renderMode.scrollBehavior, scrollToFile]);

	if (isLoading) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden">
				<div className="space-y-4 p-4">
					{Array.from({ length: 3 }, (_, index) => (
						<div key={index} className="space-y-2">
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

	if (error) {
		return (
			<div className="p-3">
				<ErrorAlert error={error} />
			</div>
		);
	}

	return (
		<div
			ref={setContainerEl}
			className="flex h-full min-w-0 flex-col overflow-hidden"
		>
			{sortedFiles.length === 0 ? (
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					{emptyMessage}
				</div>
			) : (
				<div className="flex min-w-0 flex-1 flex-row overflow-hidden">
					{showTree && (
						<FileTreeSidebar
							fileTree={fileTree}
							sortedFiles={sortedFiles}
							diffViewportRef={diffViewportRef}
							fileRefs={fileRefs}
							scrollBehavior={renderMode.scrollBehavior}
							enableTreeSync={renderMode.enableTreeSync}
							scrollToFile={scrollToFile}
						/>
					)}
					<DiffScrollContainer
						key={`${renderMode.virtualizerConfig.overscrollSize}:${renderMode.virtualizerConfig.intersectionObserverMargin}`}
						diffViewportRef={diffViewportRef}
						virtualizerConfig={renderMode.virtualizerConfig}
						className={cn(
							"min-w-0 flex-1",
							showTree &&
								"rounded-tl-md border-0 border-l border-t border-solid border-border-default",
						)}
					>
						{sortedFiles.map((fileDiff, index) => {
							const isLast = index === sortedFiles.length - 1;
							return (
								<div
									key={fileDiff.name}
									data-file-name={fileDiff.name}
									ref={(element) => setFileRef(fileDiff.name, element)}
									className={
										index > 0
											? "border-0 border-t border-solid border-border-default"
											: undefined
									}
								>
									<LazyFileDiff
										fileDiff={fileDiff}
										lazyMountRootMargin={renderMode.lazyMountRootMargin}
										options={perFileOptions?.get(fileDiff.name) ?? fileOptions}
										lineAnnotations={perFileAnnotations?.get(fileDiff.name)}
										renderAnnotation={annotationRenderer}
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
