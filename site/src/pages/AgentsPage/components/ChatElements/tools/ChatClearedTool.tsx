import type React from "react";
import { ToolCall } from "./ToolCall";
import { contextBoundarySourceSuffix, type ToolStatus } from "./utils";

/**
 * Static row for `chat_cleared` boundary markers. The label reads
 * "Context cleared" followed by the source suffix, if any.
 */
export const ChatClearedTool: React.FC<{
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	source?: string;
}> = ({ status, isError, errorMessage, source }) => (
	<ToolCall.Root
		className="w-full"
		status={status}
		isError={isError}
		errorMessage={errorMessage || "Failed to clear conversation context"}
		hasContent={false}
	>
		<ToolCall.Header
			iconName="chat_cleared"
			label={`Context cleared${contextBoundarySourceSuffix(source)}`}
		/>
	</ToolCall.Root>
);
