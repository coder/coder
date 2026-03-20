import { useTheme } from "@emotion/react";
import type { FileDiffMetadata } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";
import { ToolCollapsible } from "./ToolCollapsible";
import {
	computeDiffStats,
	DIFFS_FONT_STYLE,
	type EditFilesFileEntry,
	getDiffViewerOptions,
	splitPath,
	type ToolStatus,
} from "./utils";

/**
 * Collapsed-by-default rendering for `edit_files` tool calls.
 * Shows "Edited <filename>" (or "Edited N files") with a chevron;
 * expanding reveals a unified diff for each file.
 */
export const EditFilesTool: React.FC<{
	files: EditFilesFileEntry[];
	diffs: (FileDiffMetadata | null)[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ files, diffs, status, isError, errorMessage }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const isRunning = status === "running";
	const hasDiffs = diffs.some((d) => d !== null);
	const isSingleFile = files.length === 1;
	const isMultiFile = files.length > 1;
	const singleFilePath = isSingleFile ? splitPath(files[0].path) : null;
	const singleFileStats = isSingleFile ? computeDiffStats(diffs[0]) : null;
	const totalStats = diffs.reduce(
		(acc, diff) => {
			const stats = computeDiffStats(diff);

			return {
				additions: acc.additions + stats.additions,
				deletions: acc.deletions + stats.deletions,
			};
		},
		{ additions: 0, deletions: 0 },
	);
	const headerStats = isSingleFile ? singleFileStats : totalStats;
	const hasHeaderStats =
		headerStats !== null &&
		(headerStats.additions > 0 || headerStats.deletions > 0);

	let label: string;
	if (isRunning) {
		if (isMultiFile) {
			label = `Editing ${files.length} files…`;
		} else {
			label = "Editing files…";
		}
	} else if (isMultiFile) {
		label = `Edited ${files.length} files`;
	} else {
		label = "Edited files";
	}

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasDiffs}
			defaultExpanded
			header={
				<>
					{isSingleFile && singleFilePath ? (
						<span
							className={cn(
								"text-sm",
								isError ? "text-content-destructive" : "text-content-secondary",
							)}
						>
							{isRunning ? "Editing " : "Edited "}
							{singleFilePath.directory && (
								<span className="opacity-70">{singleFilePath.directory}</span>
							)}
							<span className="font-semibold">{singleFilePath.filename}</span>
						</span>
					) : (
						<span
							className={cn(
								"text-sm",
								isError ? "text-content-destructive" : "text-content-secondary",
							)}
						>
							{label}
						</span>
					)}
					{hasHeaderStats && headerStats && (
						<span className="ml-auto flex shrink-0 items-center gap-1.5 text-xs tabular-nums">
							{headerStats.additions > 0 && (
								<span className="text-content-success">
									+{headerStats.additions}
								</span>
							)}
							{headerStats.deletions > 0 && (
								<span className="text-content-destructive">
									-{headerStats.deletions}
								</span>
							)}
						</span>
					)}
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to edit files"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			<div className="space-y-px">
				{diffs.map((diff, i) => {
					if (!diff) {
						return null;
					}

					const { directory: perFileDir, filename: perFileName } = splitPath(
						files[i].path,
					);
					const perFileStats = computeDiffStats(diff);
					const hasPerFileStats =
						perFileStats.additions > 0 || perFileStats.deletions > 0;

					if (!isMultiFile) {
						return (
							<ScrollArea
								key={files[i].path}
								className="text-2xs"
								viewportClassName="max-h-64"
								scrollBarClassName="w-1.5"
							>
								<FileDiff
									fileDiff={diff}
									options={{
										...getDiffViewerOptions(isDark),
										disableFileHeader: true,
									}}
									style={DIFFS_FONT_STYLE}
								/>
							</ScrollArea>
						);
					}

					return (
						<div key={files[i].path}>
							<div className="flex items-center gap-2 border-t border-border-default/20 bg-surface-tertiary/30 px-3 py-1 text-xs text-content-secondary">
								<span className="min-w-0 truncate">
									{perFileDir && (
										<span className="opacity-70">{perFileDir}</span>
									)}
									<span className="font-semibold">{perFileName}</span>
								</span>
								{hasPerFileStats && (
									<span className="ml-auto flex shrink-0 items-center gap-1.5 tabular-nums">
										{perFileStats.additions > 0 && (
											<span className="text-content-success">
												+{perFileStats.additions}
											</span>
										)}
										{perFileStats.deletions > 0 && (
											<span className="text-content-destructive">
												-{perFileStats.deletions}
											</span>
										)}
									</span>
								)}
							</div>
							<ScrollArea
								className="text-2xs"
								viewportClassName="max-h-64"
								scrollBarClassName="w-1.5"
							>
								<FileDiff
									fileDiff={diff}
									options={{
										...getDiffViewerOptions(isDark),
										disableFileHeader: true,
									}}
									style={DIFFS_FONT_STYLE}
								/>
							</ScrollArea>
						</div>
					);
				})}
			</div>
		</ToolCollapsible>
	);
};
