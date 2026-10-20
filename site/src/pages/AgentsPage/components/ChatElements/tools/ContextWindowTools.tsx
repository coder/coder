import type React from "react";
import { MarkdownToolRow } from "./MarkdownToolRow";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

type ToolRowStatusProps = {
	status: ToolStatus;
	isError: boolean;
	errorMessage: string | undefined;
};

const rolloverReasonLabels: Partial<Record<string, string>> = {
	model: "Requested by the agent",
	limit: "Usable limit reached",
	overflow: "Provider context overflow",
};

// Values come from persisted tool rows, so inherited keys must not match.
const labelForOwnKey = <T,>(labels: Partial<Record<string, T>>, key: string) =>
	Object.hasOwn(labels, key) ? labels[key] : undefined;

const SecondaryLabel: React.FC<{ children: React.ReactNode }> = ({
	children,
}) => (
	<ToolCall.Label className="text-content-secondary/60">
		{children}
	</ToolCall.Label>
);

/**
 * Boundary row for a token budget rollover: a fresh context window that
 * starts from the latest checkpoint, when one was saved, instead of a
 * summary.
 */
export const ContextRolloverTool: React.FC<
	ToolRowStatusProps & { reason: string; checkpoint: string }
> = ({ reason, checkpoint, ...rowStatus }) => {
	const reasonLabel = labelForOwnKey(rolloverReasonLabels, reason);
	return (
		<MarkdownToolRow
			{...rowStatus}
			iconName="chat_context_rollover"
			label="Context window rollover"
			secondaryLabel={
				reasonLabel ? <SecondaryLabel>{reasonLabel}</SecondaryLabel> : null
			}
			bodyLabel="Rollover checkpoint"
			body={checkpoint}
		/>
	);
};

/**
 * The prompt chatd sent when the window reached the handoff target, or the
 * final prompt before chatd forces a rollover.
 */
export const ContextHandoffTool: React.FC<
	ToolRowStatusProps & { stage: string; prompt: string }
> = ({ stage, prompt, ...rowStatus }) => (
	<MarkdownToolRow
		{...rowStatus}
		iconName="chat_context_handoff"
		label={
			stage === "final"
				? "Context window ending: final handoff"
				: "Context handoff requested"
		}
		bodyLabel="Handoff prompt"
		body={prompt}
	/>
);

const toolRowLabel = (
	status: ToolStatus,
	isError: boolean,
	labels: { running: string; failed: string; done: string },
) => {
	if (status === "running") {
		return labels.running;
	}
	return isError || status === "error" ? labels.failed : labels.done;
};

/** The checkpoint the agent saved for future context windows. */
export const SaveCheckpointTool: React.FC<
	ToolRowStatusProps & { checkpoint: string }
> = ({ checkpoint, status, isError, errorMessage }) => (
	<MarkdownToolRow
		status={status}
		isError={isError}
		errorMessage={errorMessage || "Failed to save checkpoint"}
		iconName="save_checkpoint"
		label={toolRowLabel(status, isError, {
			running: "Saving checkpoint…",
			failed: "Checkpoint not saved",
			done: "Saved checkpoint",
		})}
		bodyLabel="Saved checkpoint"
		body={checkpoint}
	/>
);

/** chatd refuses the request when automatic rollover is disabled. */
export const NewContextTool: React.FC<
	ToolRowStatusProps & { refused: boolean }
> = ({ refused, status, isError, errorMessage }) => (
	<ToolCall.Root
		status={status}
		isError={isError || refused}
		errorMessage={errorMessage || "Failed to request a new context window"}
		hasContent={false}
	>
		<ToolCall.Header
			iconName="new_context"
			label={toolRowLabel(status, isError || refused, {
				running: "Requesting a new context window…",
				failed: refused
					? "New context window refused"
					: "New context window request failed",
				done: "Requested a new context window",
			})}
		/>
	</ToolCall.Root>
);

const chatHistoryWording: Partial<
	Record<string, { running: string; done: string; noun: string }>
> = {
	list_windows: {
		running: "Listing context windows",
		done: "Listed context windows",
		noun: "window",
	},
	list_items: {
		running: "Listing chat history",
		done: "Listed chat history",
		noun: "item",
	},
	search: {
		running: "Searching chat history",
		done: "Searched chat history",
		noun: "item",
	},
	read_item: {
		running: "Reading chat history",
		done: "Read chat history",
		noun: "item",
	},
};

const chatHistoryErrorMessages: Partial<Record<string, string>> = {
	history_timeout: "Reading chat history timed out",
	item_not_found: "The requested history item was not found",
	window_not_found: "The requested context window was not found",
	invalid_action: "Unknown chat history action",
	query_required: "Searching chat history needs a query",
	item_id_required: "Reading a history item needs an item ID",
	filters_unsupported: "This chat history action does not accept those filters",
	invalid_arguments: "Invalid chat history arguments",
};

const countLabel = (count: number, noun: string) =>
	`${count} ${noun}${count === 1 ? "" : "s"}`;

/** A read of the chat's own transcript across context windows. */
export const ChatHistoryTool: React.FC<
	ToolRowStatusProps & {
		action: string;
		searchQuery: string;
		itemId: string;
		resultCount: number | undefined;
		hasMore: boolean;
		/** Error code from a failed lookup, or empty on success. */
		errorCode: string;
	}
> = ({
	action,
	searchQuery,
	itemId,
	resultCount,
	hasMore,
	errorCode,
	status,
	isError: isToolError,
	errorMessage,
}) => {
	const isError = isToolError || errorCode !== "";
	const wording = labelForOwnKey(chatHistoryWording, action) ?? {
		running: "Using chat history",
		done: "Used chat history",
		noun: "item",
	};

	let target = "";
	if (action === "search" && searchQuery) {
		target = ` for "${searchQuery}"`;
	} else if (action === "read_item" && itemId) {
		target = ` item ${itemId}`;
	}

	let label = `${wording.done}${target}`;
	if (status === "running") {
		label = `${wording.running}${target}…`;
	} else if (isError) {
		label = "Chat history lookup failed";
	}

	const counts: string[] = [];
	if (resultCount !== undefined && action !== "read_item") {
		counts.push(countLabel(resultCount, wording.noun));
	}
	if (hasMore) {
		counts.push(`more ${wording.noun}s available`);
	}

	return (
		<ToolCall.Root
			status={status}
			isError={isError}
			errorMessage={
				(errorCode
					? labelForOwnKey(chatHistoryErrorMessages, errorCode)
					: errorMessage) || "Failed to read chat history"
			}
			hasContent={false}
		>
			<ToolCall.Header
				iconName="chat_history"
				label={label}
				secondaryLabel={
					status !== "running" && !isError && counts.length > 0 ? (
						<SecondaryLabel>{counts.join(", ")}</SecondaryLabel>
					) : null
				}
			/>
		</ToolCall.Root>
	);
};
