import { useTheme } from "@emotion/react";
import { File as FileViewer } from "@pierre/diffs/react";
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
	DIFFS_FONT_STYLE,
	getFileViewerOptionsMinimal,
	type ToolStatus,
} from "./utils";

/**
 * Collapsed-by-default rendering for `read_file` tool calls. Shows
 * "Read <filename>" with a chevron; expanding reveals the file viewer.
 */
export const ReadFileTool: React.FC<{
	path: string;
	content: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ path, content, status, isError, errorMessage }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const hasContent = content.length > 0;
	const isRunning = status === "running";

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasContent}
			header={
				<>
					<span
						className={cn(
							"text-sm",
							isError ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						Read {path.split("/").pop() || path}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to read file"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			<ScrollArea
				className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
				viewportClassName="max-h-64"
				scrollBarClassName="w-1.5"
			>
				<FileViewer
					file={{
						name: path,
						contents: content,
					}}
					options={getFileViewerOptionsMinimal(isDark)}
					style={DIFFS_FONT_STYLE}
				/>
			</ScrollArea>
		</ToolCollapsible>
	);
};
