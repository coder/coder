import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import { asRecord, asString, parseArgs, type ToolStatus } from "./utils";

export type ContextRequestKind = "clear" | "compact";

const followUpEcho = "Follow-up: ";

/**
 * Returns the trimmed `follow_up` string from the call args. When the
 * args carry none, returns the text after the first "Follow-up: " in
 * the result `output`, or an empty string when that is absent too.
 */
export const getContextRequestFollowUp = (
	args: unknown,
	result: unknown,
): string => {
	const argsRec = parseArgs(args);
	const fromArgs = argsRec ? asString(argsRec.follow_up).trim() : "";
	if (fromArgs) {
		return fromArgs;
	}
	const rec = asRecord(result);
	const output = rec ? asString(rec.output) : "";
	const echoIndex = output.indexOf(followUpEcho);
	if (echoIndex === -1) {
		return "";
	}
	return output.slice(echoIndex + followUpEcho.length).trim();
};

/**
 * Returns the error text of a context request result: the `error` or
 * `message` field of a record result, a string result when the call
 * is flagged as an error, otherwise undefined.
 */
export const getContextRequestErrorMessage = (
	result: unknown,
	isError: boolean,
): string | undefined => {
	const rec = asRecord(result);
	const fromRecord = rec ? asString(rec.error || rec.message) : "";
	if (fromRecord) {
		return fromRecord;
	}
	if (typeof result === "string" && isError && result) {
		return result;
	}
	return undefined;
};

/**
 * Returns the header label for the given kind and state: a progressive
 * label while running, a "rejected" label when the call errored, and
 * a "requested" label once it completed.
 */
export const getContextRequestLabel = ({
	kind,
	status,
	isError,
}: {
	kind: ContextRequestKind;
	status: ToolStatus;
	isError: boolean;
}): string => {
	if (status === "running") {
		return kind === "clear" ? "Clearing context…" : "Compacting context…";
	}
	if (isError || status === "error") {
		return kind === "clear" ? "Context clear rejected" : "Compaction rejected";
	}
	return kind === "clear" ? "Context clear requested" : "Compaction requested";
};

/**
 * Returns whether a context request row starts expanded: true while
 * the call is running or when it errored, false once it completed.
 */
export const isContextRequestExpandedByDefault = ({
	status,
	isError,
}: {
	status: ToolStatus;
	isError: boolean;
}): boolean => status === "running" || isError || status === "error";

type ContextRequestToolProps = {
	kind: ContextRequestKind;
	followUp: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
};

/**
 * Collapsible row for `clear_context` and `compact_context` tool
 * calls. The body renders the follow_up note as markdown and, when
 * the call errored, the rejection text below it.
 */
export const ContextRequestTool: React.FC<ContextRequestToolProps> = ({
	kind,
	followUp,
	status,
	isError,
	errorMessage,
}) => {
	const showError = isError || status === "error";
	const effectiveErrorMessage =
		errorMessage?.trim() ||
		(kind === "clear"
			? "Context clear request was rejected."
			: "Compaction request was rejected.");
	const hasFollowUp = followUp.trim().length > 0;
	const label = getContextRequestLabel({ kind, status, isError });

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={showError}
			errorMessage={effectiveErrorMessage}
			hasContent={hasFollowUp || showError}
			defaultExpanded={isContextRequestExpandedByDefault({ status, isError })}
		>
			<ToolCall.Header
				iconName={kind === "clear" ? "clear_context" : "compact_context"}
				label={label}
			/>
			<ToolCall.Content>
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					viewportTabIndex={0}
					viewportAriaLabel={
						showError ? "Follow-up note and rejection" : "Follow-up note"
					}
					scrollBarClassName="w-1.5"
				>
					<div className="space-y-2 px-3 py-2">
						{hasFollowUp ? (
							<Response className="text-[13px] leading-5">{followUp}</Response>
						) : (
							<p className="m-0 text-[13px] italic leading-5 text-content-secondary">
								No follow-up provided.
							</p>
						)}
						{showError && (
							<div className="border-0 border-t border-solid border-border-default pt-2 text-sm">
								<p className="m-0 text-content-secondary wrap-anywhere">
									{effectiveErrorMessage}
								</p>
							</div>
						)}
					</div>
				</ScrollArea>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
