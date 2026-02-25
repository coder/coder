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
	DIFF_VIEWER_OPTIONS,
	DIFFS_FONT_STYLE,
	type ToolStatus,
} from "./utils";

/**
 * Collapsed-by-default rendering for `write_file` tool calls. Shows
 * "Wrote <filename>" with a chevron; expanding reveals the unified diff.
 */
export const WriteFileTool: React.FC<{
	path: string;
	diff: FileDiffMetadata | null;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ path, diff, status, isError, errorMessage }) => {
	const hasDiff = diff !== null;
	const isRunning = status === "running";

	const filename = path.split("/").pop() || path;
	const label = isRunning ? `Writing ${filename}…` : `Wrote ${filename}`;

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasDiff}
			header={
				<>
					<span
						className={cn(
							"text-sm",
							isError ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{label}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to write file"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary" />
					)}
				</>
			}
		>
			{hasDiff && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileDiff
						fileDiff={diff}
						options={DIFF_VIEWER_OPTIONS}
						style={DIFFS_FONT_STYLE}
					/>
				</ScrollArea>
			)}
		</ToolCollapsible>
	);
};
