import type React from "react";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

/**
 * Collapsed-by-default rendering for `chat_summarized` tool calls.
 * Shows "Summarized" and reveals the summary only when expanded.
 * Manual compactions (user-requested via /compact) are labeled
 * distinctly from automatic threshold-triggered ones.
 */
export const ChatSummarizedTool: React.FC<{
	summary: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	source?: string;
}> = ({ summary, status, isError, errorMessage, source }) => {
	const hasSummary = summary.trim().length > 0;
	const isRunning = status === "running";
	const isManual = source === "manual";
	// The token-budget prototype reuses this card for context window
	// boundaries, which have no summary model call.
	const isNewWindow = source === "new_context";
	const isLimitWindow = source === "context_limit";

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to summarize conversation"}
			hasContent={hasSummary}
		>
			<ToolCall.Header
				iconName="chat_summarized"
				label={
					isRunning
						? "Summarizing…"
						: isNewWindow
							? "New context window"
							: isLimitWindow
								? "New context window (limit)"
								: isManual
									? "Summarized (manual)"
									: "Summarized"
				}
			/>
			<ToolCall.Content>
				<ToolCall.Scrollable
					className="mt-1.5 rounded-md border border-solid border-border"
					viewportClassName="max-h-64"
					ariaLabel="Conversation summary"
				>
					<div className="px-3 py-2">
						<Response>{summary}</Response>
					</div>
				</ToolCall.Scrollable>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
