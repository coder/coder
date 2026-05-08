import { useTheme } from "@emotion/react";
import { File as FileViewer } from "@pierre/diffs/react";
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
	getFileViewerOptionsMinimal,
	type ToolStatus,
} from "./utils";

export const ReadFileContent: React.FC<{
	path: string;
	content: string;
}> = ({ path, content }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";

	return (
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
	);
};

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
	const hasContent = content.length > 0;
	const isRunning = status === "running";
	const filename = path.split("/").pop() || path;
	const label = isRunning ? `Reading ${filename}…` : `Read ${filename}`;

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasContent}
			header={
				<>
					<span className="text-[13px]">{label}</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon className="size-3.5 shrink-0 text-current" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to read file"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="size-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
					)}
				</>
			}
		>
			<ReadFileContent path={path} content={content} />
		</ToolCollapsible>
	);
};
