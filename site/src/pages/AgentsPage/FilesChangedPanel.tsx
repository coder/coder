import { useTheme } from "@emotion/react";
import type { FileDiffMetadata } from "@pierre/diffs";
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
	ExternalLinkIcon,
	GitBranchIcon,
	GitPullRequestIcon,
} from "lucide-react";
import {
	type ComponentProps,
	type FC,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";

interface FilesChangedPanelProps {
	chatId: string;
}

/**
 * Extracts a short label like "owner/repo#123" from a GitHub PR URL.
 * Falls back to the raw URL if parsing fails.
 */
function formatPullRequestLabel(url: string): string {
	try {
		const match = url.match(/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/);
		if (match) {
			return `${match[1]}/${match[2]}#${match[3]}`;
		}
	} catch {
		// Fall through to return the raw URL.
	}
	return url;
}

export const FilesChangedPanel: FC<FilesChangedPanelProps> = ({ chatId }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const diffOptions = useMemo(() => {
		const base = getDiffViewerOptions(isDark);
		return {
			...base,
			// Extend the base CSS to make file headers sticky so they
			// remain visible while scrolling through long diffs.
			unsafeCSS: `${base.unsafeCSS ?? ""} [data-diffs-header] { position: sticky; top: 0; z-index: 10; background-color: hsl(var(--surface-primary)) !important; }`,
		};
	}, [isDark]);

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
	const pullRequestLabel = pullRequestUrl
		? formatPullRequestLabel(pullRequestUrl)
		: undefined;

	if (diffContentsQuery.isLoading || diffStatusQuery.isLoading) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden border-0 border-solid">
				<div className="flex items-center gap-2 border-0 border-b border-solid px-4 py-3">
					<Skeleton className="h-4 w-4 rounded" />
					<Skeleton className="h-4 w-28" />
				</div>
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
		<div className="flex h-full min-w-0 flex-col overflow-hidden border-0 border-solid">
			{/* Header */}
			<div className="flex items-center justify-between gap-3 border-0 border-b border-solid px-4 py-3">
				<div className="flex min-w-0 items-center gap-2">
					{pullRequestUrl ? (
						<>
							<GitPullRequestIcon className="h-4 w-4 shrink-0 text-content-secondary" />
							<span className="truncate text-sm font-medium text-content-primary">
								{pullRequestLabel}
							</span>
						</>
					) : (
						<>
							<GitBranchIcon className="h-4 w-4 text-content-secondary" />
							<span className="text-sm font-medium text-content-primary">
								Files Changed
							</span>
						</>
					)}
				</div>

				{pullRequestUrl && (
					<a
						href={pullRequestUrl}
						target="_blank"
						rel="noreferrer"
						className="flex shrink-0 items-center gap-1.5 rounded-md border border-border-default px-2.5 py-1 text-xs text-content-secondary no-underline transition-colors hover:bg-surface-tertiary hover:text-content-primary"
					>
						View PR
						<ExternalLinkIcon className="h-3 w-3" />
					</a>
				)}
			</div>

			{/* Diff contents */}
			{parsedFiles.length === 0 ? (
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No file changes to display.
				</div>
			) : (
				<ScrollArea className="min-w-0 flex-1" scrollBarClassName="w-1.5">
					<div className="min-w-0 text-xs">
						{parsedFiles.map((fileDiff) => (
							<LazyFileDiff
								key={fileDiff.name}
								fileDiff={fileDiff}
								options={fileOptions}
							/>
						))}
					</div>
				</ScrollArea>
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
