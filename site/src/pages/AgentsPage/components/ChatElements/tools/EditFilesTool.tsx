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

	let label: string;
	if (isRunning) {
		if (files.length === 1) {
			label = `Editing ${getPathBasename(files[0].path)}…`;
		} else if (files.length > 1) {
			label = `Editing ${files.length} files…`;
		} else {
			label = "Editing files…";
		}
	} else if (files.length === 1) {
		const filename = getPathBasename(files[0].path);
		label = `Edited ${filename}`;
	} else if (files.length > 1) {
		label = `Edited ${files.length} files`;
	} else {
		label = "Edited files";
	}

	return (
		<ToolCall.Root
			key={`${codeDiffDisplayMode ?? "auto"}:${EDIT_FILES_AUTO_DISPLAY_STATE}`}
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to edit files"}
			hasContent={hasDiffs}
			defaultView={displayState}
		>
			<ToolCall.Header iconName="edit_files" label={label} />
			<ToolCall.Content>
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
