import { useTheme } from "@emotion/react";
import type { FileDiffMetadata } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import type React from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { getPathBasename } from "../../../utils/path";
import { DiffFileHeader } from "./DiffFileHeader";
import {
	type AgentDisplayState,
	isAgentDisplayFullyExpanded,
	resolveAgentDisplayState,
} from "./displayMode";
import { ToolCall } from "./ToolCall";
import {
	DIFFS_FONT_STYLE,
	getDiffViewerOptions,
	stripNoNewline,
	type ToolStatus,
} from "./utils";

const WRITE_FILE_AUTO_DISPLAY_STATE: AgentDisplayState = "collapsed";

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
		WRITE_FILE_AUTO_DISPLAY_STATE,
	);

	const filename = getPathBasename(path);
	const label = isRunning ? `Writing ${filename}…` : `Wrote ${filename}`;

	return (
		<ToolCall.Root
			key={`${codeDiffDisplayMode ?? "auto"}:${WRITE_FILE_AUTO_DISPLAY_STATE}`}
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to write file"}
			hasContent={hasDiff}
			defaultView={displayState}
		>
			<ToolCall.Header iconName="write_file" label={label} />
			<ToolCall.Content>
				{hasDiff && (
					<ScrollArea
						data-testid="write-file-diff"
						className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
						viewportClassName={
							isAgentDisplayFullyExpanded(displayState)
								? "max-h-[80vh]"
								: "max-h-64"
						}
						scrollBarClassName="w-1.5"
					>
						<FileDiff
							fileDiff={stripNoNewline(diff)}
							options={getDiffViewerOptions(isDark)}
							style={DIFFS_FONT_STYLE}
							renderCustomHeader={(fileDiff) => (
								<DiffFileHeader file={fileDiff} />
							)}
						/>
					</ScrollArea>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
