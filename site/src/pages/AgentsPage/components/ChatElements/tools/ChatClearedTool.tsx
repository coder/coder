import type React from "react";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

export const ChatClearedTool: React.FC<{
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ status, isError, errorMessage }) => (
	<ToolCall.Root
		className="w-full"
		status={status}
		isError={isError}
		errorMessage={errorMessage || "Failed to clear conversation context"}
		hasContent={false}
	>
		<ToolCall.Header iconName="chat_cleared" label="Context cleared" />
	</ToolCall.Root>
);
