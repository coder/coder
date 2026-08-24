import type { FileDiffMetadata } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import type React from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useTheme } from "#/theme/context";
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
	let label = `Wrote ${filename}`;
	if (isRunning) {
		label = `Writing ${filename}…`;
	} else if (isError) {
		label = `Failed to write ${filename}`;
	}
	// The diff is synthesized from tool args, so showing it on error could
	// misrepresent the content as written.
	const showDiff = hasDiff && !isError;
	const errorDetail = isError ? errorMessage?.trim() : undefined;

	return (
		<ToolCall.Root
			key={`${codeDiffDisplayMode ?? "auto"}:${WRITE_FILE_AUTO_DISPLAY_STATE}`}
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to write file"}
			hasContent={showDiff || Boolean(errorDetail)}
			defaultView={displayState}
		>
			<ToolCall.Header iconName="write_file" label={label} />
			<ToolCall.Content>
				{errorDetail && (
					<pre className="m-0 mt-1.5 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs leading-5 text-content-destructive">
						{errorDetail}
					</pre>
				)}
				{showDiff && (
					<ScrollArea
						data-testid="write-file-diff"
						className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
						viewportClassName={
							isAgentDisplayFullyExpanded(displayState)
								? "max-h-[80vh]"
								: "max-h-64"
						}
						viewportTabIndex={0}
						viewportAriaLabel={`Diff of ${path}`}
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
