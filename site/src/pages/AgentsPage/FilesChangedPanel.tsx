import { useTheme } from "@emotion/react";
import type { ChangeTypes, FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { chatDiffContents, chatDiffStatus } from "api/queries/chats";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
	DIFFS_FONT_STYLE,
	getDiffViewerOptions,
} from "components/ai-elements/tool/utils";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Skeleton } from "components/Skeleton/Skeleton";
import {
	Columns2Icon,
	ExternalLinkIcon,
	GitBranchIcon,
	GitPullRequestIcon,
	Rows3Icon,
} from "lucide-react";
import {
	type ComponentProps,
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";

interface FilesChangedPanelProps {
	chatId: string;
}

/**
 * Minimum container width (px) at which the file tree sidebar
 * is shown alongside the diff list.
 */
const FILE_TREE_THRESHOLD = 700;

type DiffStyle = "unified" | "split";
const DIFF_STYLE_KEY = "agents.diff-view-style";

function loadDiffStyle(): DiffStyle {
	if (typeof window === "undefined") {
		return "unified";
	}
	const stored = localStorage.getItem(DIFF_STYLE_KEY);
	if (stored === "split" || stored === "unified") {
		return stored;
	}
	return "unified";
}

/**
 * Width of the file tree sidebar in pixels.
 */
const FILE_TREE_WIDTH = 220;

/**
 * Parses a GitHub PR URL into its components.
 * Returns null if parsing fails.
 */
function parsePullRequestUrl(url: string): {
	owner: string;
	repo: string;
	number: string;
} | null {
	try {
		const match = url.match(/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/);
		if (match) {
			return { owner: match[1], repo: match[2], number: match[3] };
		}
	} catch {
		// Fall through.
	}
	return null;
}

/**
 * Returns a color class for the change-type indicator dot.
 */
function changeTypeColor(type: ChangeTypes): string {
	switch (type) {
		case "new":
			return "bg-green-500";
		case "deleted":
			return "bg-red-500";
		case "change":
		case "rename-changed":
			return "bg-yellow-500";
		case "rename-pure":
			return "bg-blue-400";
		default:
			return "bg-content-secondary";
	}
}

/**
 * Splits a file path into directory and basename.
 */
function splitPath(filePath: string): { dir: string; base: string } {
	const lastSlash = filePath.lastIndexOf("/");
	if (lastSlash === -1) {
		return { dir: "", base: filePath };
	}
	return {
		dir: filePath.slice(0, lastSlash + 1),
		base: filePath.slice(lastSlash + 1),
	};
}

export const FilesChangedPanel: FC<FilesChangedPanelProps> = ({ chatId }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const [diffStyle, setDiffStyle] = useState<DiffStyle>(loadDiffStyle);
	const handleSetDiffStyle = useCallback((style: DiffStyle) => {
		setDiffStyle(style);
		localStorage.setItem(DIFF_STYLE_KEY, style);
	}, []);

	const diffOptions = useMemo(() => {
		const base = getDiffViewerOptions(isDark);
		return {
			...base,
			diffStyle,
			// Extend the base CSS to make file headers sticky so they
			// remain visible while scrolling through long diffs.
			unsafeCSS: `${base.unsafeCSS ?? ""} [data-diffs-header] { position: sticky; top: 0; z-index: 10; background-color: hsl(var(--surface-quaternary)) !important; } @media (prefers-color-scheme: dark) { [data-diffs-header] { background-color: hsl(var(--surface-secondary)) !important; } }`,
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

	const diffStatusQuery = useQuery(chatDiffStatus(chatId));
	const diffContentsQuery = useQuery({
		...chatDiffContents(chatId),
		enabled: Boolean(diffStatusQuery.data?.url),
	});

	const parsedFiles = useMemo(() => {
		const diff = diffContentsQuery.data?.diff;
		if (!diff) {
			return [];
		}
		try {
			// The cacheKeyPrefix enables the worker pool's LRU cache
			// so highlighted ASTs are reused across re-renders instead
			// of being re-computed on every render cycle.
			const patches = parsePatchFiles(diff, `chat-${chatId}`);
			return patches.flatMap((p) => p.files);
		} catch {
			return [];
		}
	}, [diffContentsQuery.data?.diff, chatId]);

	const pullRequestUrl = diffStatusQuery.data?.url;
	const parsedPr = pullRequestUrl ? parsePullRequestUrl(pullRequestUrl) : null;

	// ---------------------------------------------------------------
	// Container width measurement via ResizeObserver so we can decide
	// whether to show the file tree sidebar without a prop from the
	// parent.
	// ---------------------------------------------------------------
	const containerRef = useRef<HTMLDivElement>(null);
	const [containerWidth, setContainerWidth] = useState(0);

	useEffect(() => {
		const el = containerRef.current;
		if (!el) {
			return;
		}
		const ro = new ResizeObserver(([entry]) => {
			setContainerWidth(entry.contentRect.width);
		});
		ro.observe(el);
		return () => ro.disconnect();
	}, []);

	const showTree =
		containerWidth >= FILE_TREE_THRESHOLD && parsedFiles.length > 0;

	// ---------------------------------------------------------------
	// Refs for each file diff wrapper so we can scroll-to and track
	// which file is currently visible.
	// ---------------------------------------------------------------
	const fileRefs = useRef<Map<string, HTMLDivElement>>(new Map());
	const diffScrollRef = useRef<HTMLDivElement>(null);
	const [activeFile, setActiveFile] = useState<string | null>(null);

	// Keep a ref callback that sets up per-file refs.
	const setFileRef = useCallback((name: string, el: HTMLDivElement | null) => {
		if (el) {
			fileRefs.current.set(name, el);
		} else {
			fileRefs.current.delete(name);
		}
	}, []);

	// IntersectionObserver to track which file is visible in the
	// diff scroll area. We observe all file wrapper elements and
	// pick the first one intersecting. We read parsedFiles.length
	// inside the effect so the observer re-subscribes when the set
	// of files changes.
	useEffect(() => {
		if (!showTree || parsedFiles.length === 0) {
			return;
		}

		const els = Array.from(fileRefs.current.values());
		if (els.length === 0) {
			return;
		}

		const observer = new IntersectionObserver(
			(entries) => {
				// Find the topmost visible entry by checking
				// boundingClientRect.top.
				let best: IntersectionObserverEntry | null = null;
				for (const entry of entries) {
					if (!entry.isIntersecting) {
						continue;
					}
					if (
						!best ||
						entry.boundingClientRect.top < best.boundingClientRect.top
					) {
						best = entry;
					}
				}
				if (best) {
					// Reverse-lookup the file name from the element.
					for (const [name, el] of fileRefs.current.entries()) {
						if (el === best.target) {
							setActiveFile(name);
							break;
						}
					}
				}
			},
			{ root: diffScrollRef.current, threshold: 0, rootMargin: "0px" },
		);

		for (const el of els) {
			observer.observe(el);
		}
		return () => observer.disconnect();
	}, [showTree, parsedFiles]);

	const handleFileClick = useCallback((name: string) => {
		const el = fileRefs.current.get(name);
		if (el) {
			el.scrollIntoView({ behavior: "smooth", block: "start" });
			setActiveFile(name);
		}
	}, []);

	if (diffContentsQuery.isLoading || diffStatusQuery.isLoading) {
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

	if (diffContentsQuery.isError) {
		return (
			<div className="p-3">
				<ErrorAlert error={diffContentsQuery.error} />
			</div>
		);
	}

	return (
		<div
			ref={containerRef}
			className="flex h-full min-w-0 flex-col overflow-hidden"
		>
			{/* Header */}
			<div className="flex items-center gap-3 px-3 py-2">
				{pullRequestUrl && parsedPr ? (
					<a
						href={pullRequestUrl}
						target="_blank"
						rel="noreferrer"
						className="flex min-w-0 items-center gap-1.5 text-xs text-content-secondary no-underline hover:text-content-primary"
					>
						<GitPullRequestIcon className="h-3.5 w-3.5 shrink-0" />
						<span className="truncate">
							<span className="text-content-secondary">
								{parsedPr.owner}/{parsedPr.repo}
							</span>
							<span className="text-content-primary">#{parsedPr.number}</span>
						</span>
						<ExternalLinkIcon className="h-3 w-3 shrink-0 opacity-50" />
					</a>
				) : pullRequestUrl ? (
					<a
						href={pullRequestUrl}
						target="_blank"
						rel="noreferrer"
						className="flex min-w-0 items-center gap-1.5 text-xs text-content-secondary no-underline hover:text-content-primary"
					>
						<GitPullRequestIcon className="h-3.5 w-3.5 shrink-0" />
						<span className="truncate">{pullRequestUrl}</span>
						<ExternalLinkIcon className="h-3 w-3 shrink-0 opacity-50" />
					</a>
				) : (
					<div className="flex items-center gap-1.5 text-xs text-content-secondary">
						<GitBranchIcon className="h-3.5 w-3.5" />
						<span>Uncommitted changes</span>
					</div>
				)}
				{/* Diff style toggle */}
				<div className="ml-auto flex items-center overflow-hidden rounded-md border border-solid border-border-default">
					<button
						type="button"
						onClick={() => handleSetDiffStyle("unified")}
						className={cn(
							"flex items-center px-1.5 py-0.5 cursor-pointer",
							diffStyle === "unified"
								? "bg-surface-secondary text-content-primary"
								: "text-content-secondary hover:text-content-primary",
						)}
						aria-label="Unified diff view"
					>
						<Rows3Icon className="h-3.5 w-3.5" />
					</button>
					<button
						type="button"
						onClick={() => handleSetDiffStyle("split")}
						className={cn(
							"flex items-center px-1.5 py-0.5 cursor-pointer",
							diffStyle === "split"
								? "bg-surface-secondary text-content-primary"
								: "text-content-secondary hover:text-content-primary",
						)}
						aria-label="Split diff view"
					>
						<Columns2Icon className="h-3.5 w-3.5" />
					</button>
				</div>
			</div>
			{/* Diff contents */}
			{parsedFiles.length === 0 ? (
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No file changes to display.
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
								{parsedFiles.map((f) => {
									const { dir, base } = splitPath(f.name);
									const isActive = activeFile === f.name;
									return (
										<button
											key={f.name}
											type="button"
											onClick={() => handleFileClick(f.name)}
											className={cn(
												"flex items-center gap-1.5 px-2 py-1 text-left",
												"hover:bg-surface-secondary",
												isActive && "bg-surface-secondary",
											)}
											title={f.name}
										>
											<span
												className={cn(
													"h-1.5 w-1.5 shrink-0 rounded-full",
													changeTypeColor(f.type),
												)}
											/>
											<span className="min-w-0 truncate">
												{dir && (
													<span className="text-2xs text-content-tertiary">
														{dir}
													</span>
												)}
												<span
													className={cn(
														"text-xs",
														isActive
															? "text-content-primary"
															: "text-content-secondary",
													)}
												>
													{base}
												</span>
											</span>
										</button>
									);
								})}
							</nav>
						</ScrollArea>
					)}
					{/* Diff list */}
					<ScrollArea
						className="min-w-0 flex-1"
						scrollBarClassName="w-1.5"
						viewportClassName="[&>div]:!block"
						ref={diffScrollRef}
					>
						<div className="min-w-0 text-xs">
							{parsedFiles.map((fileDiff) => (
								<div
									key={fileDiff.name}
									ref={(el) => setFileRef(fileDiff.name, el)}
								>
									<LazyFileDiff fileDiff={fileDiff} options={fileOptions} />
								</div>
							))}
						</div>
					</ScrollArea>
				</div>
			)}
		</div>
	);
};

// -----------------------------------------------------------------------
// Estimated height per line in the diff viewer (px). Derived from
// the --diffs-font-size (11px) and --diffs-line-height (1.5)
// values set via DIFFS_FONT_STYLE, plus 1px for the border/gap.
// -----------------------------------------------------------------------
const LINE_HEIGHT_PX = 17.5;

// Height of the file header row rendered by @pierre/diffs.
const HEADER_HEIGHT_PX = 36;

/**
 * Estimate the rendered pixel height of a file diff so the
 * placeholder occupies roughly the same space. This keeps the
 * scroll position stable as files are lazily mounted.
 */
function estimateDiffHeight(fileDiff: FileDiffMetadata): number {
	return HEADER_HEIGHT_PX + fileDiff.unifiedLineCount * LINE_HEIGHT_PX;
}

/**
 * Wraps a single `<FileDiff>` with an IntersectionObserver so the
 * heavy component (Shadow DOM + shiki highlighting) is only mounted
 * once the placeholder scrolls into or near the viewport.
 *
 * Once mounted the component stays mounted — we never unmount a
 * FileDiff that the user has already scrolled past, which avoids
 * layout shifts and repeated highlighting work.
 */
const LazyFileDiff: FC<{
	fileDiff: FileDiffMetadata;
	options: ComponentProps<typeof FileDiff>["options"];
}> = ({ fileDiff, options }) => {
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
			/>
		);
	}

	return (
		<FileDiff fileDiff={fileDiff} options={options} style={DIFFS_FONT_STYLE} />
	);
};
