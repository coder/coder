import type React from "react";
import { MarkdownToolRow } from "./MarkdownToolRow";
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
	const isRunning = status === "running";
	const isManual = source === "manual";

	return (
		<MarkdownToolRow
			iconName="chat_summarized"
			label={
				isRunning
					? "Summarizing…"
					: isManual
						? "Summarized (manual)"
						: "Summarized"
			}
			bodyLabel="Conversation summary"
			body={summary}
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to summarize conversation"}
		/>
	);
};
