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
	type EditFilesFileEntry,
	getDiffViewerOptions,
	stripNoNewline,
	type ToolStatus,
} from "./utils";

const EDIT_FILES_AUTO_DISPLAY_STATE: AgentDisplayState = "preview";

export const EditFilesTool: React.FC<{
	files: EditFilesFileEntry[];
	diffs: (FileDiffMetadata | null)[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	codeDiffDisplayMode?: TypesGen.AgentDisplayMode;
}> = ({ files, diffs, status, isError, errorMessage, codeDiffDisplayMode }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const isRunning = status === "running";
	const hasDiffs = diffs.some((d) => d !== null);
	const displayState = resolveAgentDisplayState(
		codeDiffDisplayMode,
		EDIT_FILES_AUTO_DISPLAY_STATE,
	);

	let verb = "Edited";
	if (isRunning) {
		verb = "Editing";
	} else if (isError) {
		verb = "Failed to edit";
	}
	let subject = "files";
	if (files.length === 1) {
		subject = getPathBasename(files[0].path);
	} else if (files.length > 1) {
		subject = `${files.length} files`;
	}
	const label = isRunning ? `${verb} ${subject}…` : `${verb} ${subject}`;
	const errorDetail = isError ? errorMessage?.trim() : undefined;

	return (
		<ToolCall.Root
			key={`${codeDiffDisplayMode ?? "auto"}:${EDIT_FILES_AUTO_DISPLAY_STATE}`}
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to edit files"}
			hasContent={hasDiffs || Boolean(errorDetail)}
			defaultView={displayState}
		>
			<ToolCall.Header iconName="edit_files" label={label} />
			<ToolCall.Content>
				{errorDetail && (
					<pre className="m-0 mt-1.5 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs leading-5 text-content-destructive">
						{errorDetail}
					</pre>
				)}
				<div className="mt-1.5 space-y-1.5">
					{diffs.map((diff, i) =>
						diff ? (
							<ScrollArea
								key={files[i].path}
								data-testid="edit-file-diff"
								className="rounded-md border border-solid border-border-default text-2xs"
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
						) : null,
					)}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
