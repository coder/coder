import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import { asRecord, asString, parseArgs, type ToolStatus } from "./utils";

export type ContextRequestKind = "clear" | "compact";

const followUpEcho = "Follow-up: ";

/**
 * Returns the follow_up text of a clear_context or compact_context
 * call: the call args carry it directly, and a successful result echoes
 * it in `output` after "Follow-up: ".
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
 * Header label for a context request row. An error result is a
 * rejection: the tool did not run and nothing changed.
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
	return kind === "clear" ? "Clearing context" : "Compacting context";
};

type ContextRequestToolProps = {
	kind: ContextRequestKind;
	followUp: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
};

/**
 * Rendering for `clear_context` and `compact_context` tool calls. The
 * follow_up note is the message the model continues from after the
 * boundary.
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
			defaultExpanded
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
					viewportAriaLabel="Follow-up note"
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
							<div
								role="alert"
								className="border-0 border-t border-solid border-border-default pt-2 text-sm"
							>
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
