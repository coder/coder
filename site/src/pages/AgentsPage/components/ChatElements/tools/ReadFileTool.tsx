import { useTheme } from "@emotion/react";
import { File as FileViewer } from "@pierre/diffs/react";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { getPathBasename } from "../../../utils/path";
import { asRecord, asString } from "../runtimeTypeUtils";
import { ToolCall } from "./ToolCall";
import {
	DIFFS_FONT_STYLE,
	getFileViewerOptionsMinimal,
	parseArgs,
	type ToolStatus,
} from "./utils";

const ReadFileContent: React.FC<{
	path: string;
	content: string;
}> = ({ path, content }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";

	return (
		<ScrollArea
			className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
			viewportClassName="max-h-64"
			orientation="both"
			scrollBarClassName="w-1.5"
			horizontalScrollBarClassName="h-1.5"
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

export const getReadFileToolData = ({
	args,
	result,
	isError,
}: {
	args?: unknown;
	result?: unknown;
	isError: boolean;
}) => {
	const parsedArgs = parseArgs(args);
	const path = parsedArgs ? asString(parsedArgs.path).trim() : "";
	const rec = asRecord(result);
	return {
		path: path || "file",
		content: rec ? asString(rec.content).trim() : "",
		isError,
		errorMessage: rec ? asString(rec.error || rec.message) : undefined,
	};
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
	expanded?: boolean;
	onExpandedChange?: (expanded: boolean) => void;
}> = ({
	path,
	content,
	status,
	isError,
	errorMessage,
	expanded,
	onExpandedChange,
}) => {
	const hasContent = content.length > 0 || isError;
	const isRunning = status === "running";
	const filename = getPathBasename(path);
	const label = isRunning ? `Reading ${filename}…` : `Read ${filename}`;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to read file"}
			hasContent={hasContent}
			expanded={expanded}
			onExpandedChange={onExpandedChange}
		>
			<ToolCall.Header iconName="read_file" label={label} />
			<ToolCall.Content>
				{isError && (
					<div className="mt-1 text-xs text-content-destructive">
						{errorMessage || "Failed to read file"}
					</div>
				)}
				{content.length > 0 && (
					<ReadFileContent path={path} content={content} />
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
