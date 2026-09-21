import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import { contextBoundarySourceSuffix, type ToolStatus } from "./utils";

/**
 * Collapsed-by-default rendering for `chat_summarized` tool calls.
 * The label reads "Summarized" followed by the source suffix, if any,
 * and the summary is revealed only when expanded.
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
						: `Summarized${contextBoundarySourceSuffix(source)}`
				}
			/>
			<ToolCall.Content>
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					viewportTabIndex={0}
					viewportAriaLabel="Conversation summary"
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
