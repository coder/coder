import { useTheme } from "@emotion/react";
import type { FileDiffMetadata } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { ToolCollapsible } from "./ToolCollapsible";
import {
	DIFFS_FONT_STYLE,
	type EditFilesFileEntry,
	getDiffViewerOptions,
	stripNoNewline,
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

	let label: string;
	if (isRunning) {
		if (files.length === 1) {
			label = `Editing ${files[0].path.split("/").pop() || files[0].path}…`;
		} else if (files.length > 1) {
			label = `Editing ${files.length} files…`;
		} else {
			label = "Editing files…";
		}
	} else if (files.length === 1) {
		const filename = files[0].path.split("/").pop() || files[0].path;
		label = `Edited ${filename}`;
	} else if (files.length > 1) {
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
					<span className="text-[13px]">{label}</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-current" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to edit files"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
					)}
				</>
			}
		>
			<div className="mt-1.5 space-y-1.5">
				{diffs.map((diff, i) =>
					diff ? (
						<ScrollArea
							key={files[i].path}
							data-testid="edit-file-diff"
							className="rounded-md border border-solid border-border-default text-2xs"
							viewportClassName="max-h-64"
							scrollBarClassName="w-1.5"
						>
							<FileDiff
								fileDiff={stripNoNewline(diff)}
								options={getDiffViewerOptions(isDark)}
								style={DIFFS_FONT_STYLE}
							/>
						</ScrollArea>
					) : null,
				)}
			</div>
		</ToolCollapsible>
	);
};
