import type { FileDiffMetadata } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { cn } from "cn";
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

const EDIT_FILES_AUTO_DISPLAY_STATE: AgentDisplayState = "preview";

/**
 * One file of an edit_files call. An "applied" row shows `diff`, which
 * is the server diff, or a diff built from the args while the call runs
 * or when an older result omits the file. A "rejected" row was not
 * written and shows `error`. An "unknown" row may or may not have been
 * written and shows `error`. An "unreported" row is a file the result
 * did not mention. A "failed" row belongs to a call that failed as a
 * whole and shows nothing.
 */
export type EditFilesRow = {
	path: string;
	status: "applied" | "rejected" | "unknown" | "unreported" | "failed";
	diff: FileDiffMetadata | null;
	error?: string;
};

export const EditFilesTool: React.FC<{
	files: EditFilesRow[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	codeDiffDisplayMode?: TypesGen.AgentDisplayMode;
}> = ({ files, status, isError, errorMessage, codeDiffDisplayMode }) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const isRunning = status === "running";
	const hasRows = files.some(
		(f) => f.diff !== null || (f.status !== "applied" && f.status !== "failed"),
	);
	const appliedCount = files.filter((f) => f.status === "applied").length;
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
	let label = isRunning ? `${verb} ${subject}…` : `${verb} ${subject}`;
	if (!isRunning && !isError && appliedCount < files.length) {
		label = `Edited ${appliedCount} of ${files.length} files`;
	}
	const errorDetail = isError ? errorMessage?.trim() : undefined;

	return (
		<ToolCall.Root
			key={`${codeDiffDisplayMode ?? "auto"}:${EDIT_FILES_AUTO_DISPLAY_STATE}`}
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to edit files"}
			hasContent={hasRows || Boolean(errorDetail)}
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
					{files.map((file) => {
						if (file.status !== "applied" && file.status !== "failed") {
							let label = `Edits to ${file.path} not applied`;
							let message = file.error;
							if (file.status === "unknown") {
								label = `Edits to ${file.path} may not have been applied`;
							} else if (file.status === "unreported") {
								label = `No result reported for ${file.path}`;
								message = "No result reported for this file.";
							}
							return (
								<div
									key={file.path}
									role="group"
									aria-label={label}
									className="rounded-md border border-solid border-border-default px-3 py-2 font-mono text-xs leading-5"
								>
									<div className="break-all text-content-secondary">
										{file.path}
									</div>
									<pre
										className={cn(
											"m-0 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs leading-5",
											file.status === "unreported"
												? "text-content-secondary"
												: "text-content-destructive",
										)}
									>
										{message}
									</pre>
								</div>
							);
						}
						const diff = file.diff;
						if (!diff) return null;
						return (
							<ScrollArea
								key={file.path}
								data-testid="edit-file-diff"
								className="rounded-md border border-solid border-border-default text-2xs"
								viewportClassName={
									isAgentDisplayFullyExpanded(displayState)
										? "max-h-[80vh]"
										: "max-h-64"
								}
								viewportTabIndex={0}
								viewportAriaLabel={`Diff of ${file.path}`}
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
						);
					})}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
