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

export const FilesChangedPanel: FC<FilesChangedPanelProps> = ({ chatId }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const diffOptions = useMemo(() => {
		const base = getDiffViewerOptions(isDark);
		return {
			...base,
			// Extend the base CSS to make file headers sticky so they
			// remain visible while scrolling through long diffs.
			unsafeCSS: `${base.unsafeCSS ?? ""} [data-diffs-header] { position: sticky; top: 0; z-index: 10; background-color: hsl(var(--surface-quaternary)) !important; } @media (prefers-color-scheme: dark) { [data-diffs-header] { background-color: hsl(var(--surface-secondary)) !important; } }`,
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
	const parsedPr = pullRequestUrl ? parsePullRequestUrl(pullRequestUrl) : null;

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
		<div className="flex h-full min-w-0 flex-col overflow-hidden">
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
