import type React from "react";
import { ScrollableMarkdown } from "./ChatSummarizedTool";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

type ToolRowStatusProps = {
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
};

const rolloverReasonLabels: Partial<Record<string, string>> = {
	model: "Requested by the agent",
	limit: "Usable limit reached",
	overflow: "Provider context overflow",
};

// Values come from persisted tool rows, so inherited keys must not match.
const ownLabel = <T,>(labels: Partial<Record<string, T>>, key: string) =>
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
> = ({ reason, checkpoint, status, isError, errorMessage }) => {
	const reasonLabel = ownLabel(rolloverReasonLabels, reason);
	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage}
			hasContent={checkpoint.trim().length > 0}
		>
			<ToolCall.Header
				iconName="chat_context_rollover"
				label="Context window rollover"
				secondaryLabel={
					reasonLabel ? <SecondaryLabel>{reasonLabel}</SecondaryLabel> : null
				}
			/>
			<ToolCall.Content>
				<ScrollableMarkdown ariaLabel="Checkpoint">
					{checkpoint}
				</ScrollableMarkdown>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

/**
 * The prompt chatd sent when the window reached the handoff target, or the
 * final prompt near the usable limit.
 */
export const ContextHandoffTool: React.FC<
	ToolRowStatusProps & { stage: string; prompt: string }
> = ({ stage, prompt, status, isError, errorMessage }) => (
	<ToolCall.Root
		className="w-full"
		status={status}
		isError={isError}
		errorMessage={errorMessage}
		hasContent={prompt.trim().length > 0}
	>
		<ToolCall.Header
			iconName="chat_context_handoff"
			label={
				stage === "final"
					? "Context window ending: final handoff"
					: "Context handoff requested"
			}
		/>
		<ToolCall.Content>
			<ScrollableMarkdown ariaLabel="Handoff prompt">
				{prompt}
			</ScrollableMarkdown>
		</ToolCall.Content>
	</ToolCall.Root>
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
	<ToolCall.Root
		className="w-full"
		status={status}
		isError={isError}
		errorMessage={errorMessage || "Failed to save checkpoint"}
		hasContent={checkpoint.trim().length > 0}
	>
		<ToolCall.Header
			iconName="save_checkpoint"
			label={toolRowLabel(status, isError, {
				running: "Saving checkpoint…",
				failed: "Checkpoint not saved",
				done: "Saved checkpoint",
			})}
		/>
		<ToolCall.Content>
			<ScrollableMarkdown ariaLabel="Checkpoint">
				{checkpoint}
			</ScrollableMarkdown>
		</ToolCall.Content>
	</ToolCall.Root>
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

const chatHistoryVerbs: Partial<
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
	const verbs = ownLabel(chatHistoryVerbs, action) ?? {
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
	let label = `${verbs.done}${target}`;
	if (status === "running") {
		label = `${verbs.running}${target}…`;
	} else if (isError) {
		label = "Chat history lookup failed";
	}
	const counts: string[] = [];
	if (resultCount !== undefined && action !== "read_item") {
		counts.push(countLabel(resultCount, verbs.noun));
	}
	if (hasMore) {
		counts.push(`more ${verbs.noun}s available`);
	}

	return (
		<ToolCall.Root
			status={status}
			isError={isError}
			errorMessage={
				ownLabel(chatHistoryErrorMessages, errorCode) ??
				(errorMessage || "Failed to read chat history")
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
