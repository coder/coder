import type React from "react";
import { ScrollableMarkdown } from "./ScrollableMarkdown";
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
						: isManual
							? "Summarized (manual)"
							: "Summarized"
				}
			/>
			<ToolCall.Content>
				<ScrollableMarkdown ariaLabel="Conversation summary">
					{summary}
				</ScrollableMarkdown>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
