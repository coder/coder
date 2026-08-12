import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
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
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<div className="px-3 py-2">
						<Response>{summary}</Response>
					</div>
				</ScrollArea>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
