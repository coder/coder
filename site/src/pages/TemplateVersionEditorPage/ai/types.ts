// Shared types for the template editor AI agent.

/** Tool call awaiting user approval (editFile, deleteFile, buildTemplate, or publishTemplate). */
export interface PendingToolCall {
	/** Approval request ID from the AI SDK. */
	approvalId: string;
	/** The tool call ID from the AI SDK. */
	toolCallId: string;
	/** Name of the tool being called. */
	toolName: "editFile" | "deleteFile" | "buildTemplate" | "publishTemplate";
	/** The arguments passed to the tool. */
	args: Record<string, unknown>;
}

/** Possible states for the agent conversation loop. */
export type AgentStatus = "idle" | "streaming" | "awaiting_approval" | "error";
