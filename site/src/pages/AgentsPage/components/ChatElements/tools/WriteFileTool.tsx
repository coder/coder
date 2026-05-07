import { useTheme } from "@emotion/react";
import type { FileDiffMetadata } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	isAgentDisplayFullyExpanded,
	resolveAgentDisplayState,
} from "./displayMode";
import { ToolCollapsible } from "./ToolCollapsible";
import {
	DIFFS_FONT_STYLE,
	getDiffViewerOptions,
	stripNoNewline,
	type ToolStatus,
} from "./utils";

export const WriteFileTool: React.FC<{
	path: string;
	diff: FileDiffMetadata | null;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	codeDiffDisplayMode?: TypesGen.AgentDisplayMode;
}> = ({ path, diff, status, isError, errorMessage, codeDiffDisplayMode }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const hasDiff = diff !== null;
	const isRunning = status === "running";
	const displayState = resolveAgentDisplayState(
		codeDiffDisplayMode,
		"collapsed",
	);

	const filename = path.split("/").pop() || path;
	const label = isRunning ? `Writing ${filename}…` : `Wrote ${filename}`;

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasDiff}
			displayMode={codeDiffDisplayMode}
			autoDisplayState="collapsed"
			header={
				<>
					<span className="text-[13px]">{label}</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-current" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to write file"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
					)}
				</>
			}
		>
			{hasDiff && (
				<ScrollArea
					data-testid="write-file-diff"
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName={
						isAgentDisplayFullyExpanded(displayState) ? undefined : "max-h-64"
					}
					scrollBarClassName="w-1.5"
				>
					<FileDiff
						fileDiff={stripNoNewline(diff)}
						options={getDiffViewerOptions(isDark)}
						style={DIFFS_FONT_STYLE}
					/>
				</ScrollArea>
			)}
		</ToolCollapsible>
	);
};
