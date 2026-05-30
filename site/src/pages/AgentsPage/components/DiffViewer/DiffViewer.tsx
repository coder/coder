import { useTheme } from "@emotion/react";
import type {
	CodeViewHandle,
	CodeViewItem,
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
	VirtualFileMetrics,
} from "@pierre/diffs/react";
import { CodeView } from "@pierre/diffs/react";
import {
	type ComponentProps,
	type CSSProperties,
	type FC,
	Fragment,
	type ReactNode,
	useEffect,
	useRef,
} from "react";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";
import { changeColor, changeLabel } from "../../utils/diffColors";

interface DiffViewerProps {
	parsedFiles: readonly FileDiffMetadata[];
	cacheKeyPrefix?: string;
	isExpanded?: boolean;
	isLoading?: boolean;
	error?: unknown;
	emptyMessage?: string;
	diffStyle: DiffStyle;
	onLineNumberClick?: (
		fileName: string,
		props: { lineNumber: number; annotationSide: "additions" | "deletions" },
	) => void;
	onLineSelected?: (fileName: string, range: SelectedLineRange | null) => void;
	onLineSelectionChange?: (
		fileName: string,
		range: SelectedLineRange | null,
	) => void;
	getLineAnnotations?: (fileName: string) => DiffLineAnnotation<string>[];
	getSelectedLines?: (fileName: string) => SelectedLineRange | null;
	renderAnnotation?: (annotation: DiffLineAnnotation<string>) => ReactNode;
	scrollToFile?: string | null;
	onScrollToFileComplete?: () => void;
}

export type DiffStyle = "unified" | "split";
const DIFF_STYLE_KEY = "agents.diff-view-style";

const DIFF_VIEWER_LINE_HEIGHT = 16.5;

const diffViewerStyle = {
	"--diffs-font-family": '"Geist Mono Variable", monospace, monospace',
	"--diffs-header-font-family": '"Geist Variable", system-ui, sans-serif',
	"--diffs-font-size": "11px",
	"--diffs-line-height": `${DIFF_VIEWER_LINE_HEIGHT}px`,
} satisfies CSSProperties;

const diffViewerMetrics: Partial<VirtualFileMetrics> = {
	diffHeaderHeight: 32,
	hunkSeparatorHeight: 28,
	lineHeight: DIFF_VIEWER_LINE_HEIGHT,
};

const diffViewerSeparatorCSS = [
	":host { --diffs-bg-separator-override: transparent; }",
	"[data-separator='line-info'] { height: 28px !important; }",
	"[data-separator-content] {",
	"  display: flex !important;",
	"  align-items: center !important;",
	"  gap: 12px !important;",
	"  overflow: visible !important;",
	"  height: auto !important;",
	"  border-radius: 0 !important;",
	"  background-color: transparent !important;",
	"  font-size: 11px !important;",
	"  color: hsl(var(--content-secondary)) !important;",
	"  opacity: 0.8;",
	"}",
	"[data-separator-wrapper] { border-radius: 0 !important; }",
	"[data-unified] [data-separator='line-info'] [data-separator-wrapper] {",
	"  padding-inline: 0 !important;",
	"}",
	"[data-separator-content]::before,",
	"[data-separator-content]::after {",
	"  content: '' !important;",
	"  flex: 1 !important;",
	"  height: 1px !important;",
	"  background: hsl(var(--border-default)) !important;",
	"}",
].join(" ");

function countChangedLines(fileDiff: FileDiffMetadata) {
	let additions = 0;
	let deletions = 0;
	for (const hunk of fileDiff.hunks) {
		additions += hunk.additionLines;
		deletions += hunk.deletionLines;
	}
	return { additions, deletions };
}

function HeaderContent({ fileDiff }: { fileDiff: FileDiffMetadata }) {
	const { additions, deletions } = countChangedLines(fileDiff);
	return (
		<div className="flex h-8 min-w-0 items-center justify-between gap-3 border-0 border-b border-solid border-border-default bg-surface-secondary py-2 pr-1.5 pl-2.5 font-sans text-sm">
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
			{(additions > 0 || deletions > 0) && (
				<span className="inline-flex shrink-0 flex-row-reverse items-stretch overflow-hidden rounded-[3px] border border-solid border-border-default font-mono text-xs font-medium leading-5">
					{deletions > 0 && (
						<span className="flex items-center bg-surface-git-deleted px-1 text-git-deleted-bright">
							&minus;{deletions}
						</span>
					)}
					{additions > 0 && (
						<span className="flex items-center bg-surface-git-added px-1 text-git-added-bright">
							+{additions}
						</span>
					)}
				</span>
			)}
		</div>
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
			<div className="h-px flex-1 bg-border-default" />
			<Skeleton className="h-2 w-24" />
			<div className="h-px flex-1 bg-border-default" />
		</div>
	);
}

function SkeletonFile({ groups }: { groups: readonly (readonly string[])[] }) {
	return (
		<div>
			<div className="flex h-8 items-center justify-between gap-3 border-0 border-b border-solid border-border-default bg-surface-secondary py-2 pr-1.5 pl-2.5">
				<div className="flex items-center gap-2">
					<Skeleton className="size-3 rounded-[2px]" />
					<Skeleton className="h-3 w-44" />
				</div>
				<Skeleton className="h-5 w-14 rounded-[3px]" />
			</div>
			<div className="py-1">
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
		<div role="status" aria-label="Loading diff" aria-busy>
			{DIFF_SKELETON_FILES.map((groups, fileIndex) => (
				<SkeletonFile key={fileIndex} groups={groups} />
			))}
		</div>
	);
}

export const DiffViewer: FC<DiffViewerProps> = ({
	parsedFiles,
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
	const codeViewRef = useRef<CodeViewHandle<string>>(null);
	const isDark = theme.palette.mode === "dark";

	const options: ComponentProps<typeof CodeView<string>>["options"] = {
		diffStyle,
		diffIndicators: "bars",
		overflow: "scroll",
		stickyHeaders: true,
		layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
		hunkSeparators: "line-info",
		itemMetrics: diffViewerMetrics,
		unsafeCSS: diffViewerSeparatorCSS,
		themeType: isDark ? "dark" : "light",
		theme: isDark ? "github-dark-high-contrast" : "github-light",
		enableLineSelection: true,
		enableGutterUtility: true,
		onLineNumberClick: (props, item) => {
			if (item.type === "diff" && props.type === "diff-line") {
				onLineNumberClick?.(item.item.id, props);
			}
		},
		onLineSelected: (range, item) => {
			if (item.type === "diff") {
				onLineSelected?.(item.item.id, range);
			}
		},
		onLineSelectionChange: (range, item) => {
			if (item.type === "diff") {
				onLineSelectionChange?.(item.item.id, range);
			}
		},
		onGutterUtilityClick: (range, item) => {
			if (item.type === "diff") {
				onLineSelected?.(item.item.id, range);
			}
		},
	};

	const items: CodeViewItem<string>[] = parsedFiles.map((fileDiff) => {
		const annotations = getLineAnnotations?.(fileDiff.name);
		return {
			id: fileDiff.name,
			type: "diff",
			fileDiff,
			annotations,
			version: annotations?.length ?? 0,
		};
	});

	const selectedLines = (() => {
		if (!getSelectedLines) return undefined;
		for (const fileDiff of parsedFiles) {
			const range = getSelectedLines(fileDiff.name);
			if (range) return { id: fileDiff.name, range };
		}
		return null;
	})();

	useEffect(() => {
		if (!scrollToFile) return;
		codeViewRef.current?.scrollTo({
			type: "item",
			id: scrollToFile,
			align: "start",
			behavior: "instant",
		});
		onScrollToFileComplete?.();
	}, [scrollToFile, onScrollToFileComplete]);

	if (isLoading) {
		return <DiffViewerSkeleton />;
	}

	if (error) {
		return <ErrorAlert error={error} />;
	}

	if (items.length === 0) {
		return <div>{emptyMessage}</div>;
	}

	return (
		<CodeView
			ref={codeViewRef}
			items={items}
			options={options}
			selectedLines={selectedLines}
			className="h-full min-h-0 min-w-0 overflow-auto"
			style={diffViewerStyle}
			renderCustomHeader={(item) =>
				item.type === "diff" ? <HeaderContent fileDiff={item.fileDiff} /> : null
			}
			renderAnnotation={(annotation) =>
				"side" in annotation ? renderAnnotation?.(annotation) : null
			}
		/>
	);
};
