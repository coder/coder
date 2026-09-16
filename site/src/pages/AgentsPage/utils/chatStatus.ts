import type { ChatStatus } from "#/api/typesGenerated";

/**
 * Whether the agent's turn is still in progress, so the composer shows
 * Stop rather than Send. `requires_action` counts: the server is blocked
 * on a client-executed tool call and accepts an interrupt to cancel it.
 */
export const isChatTurnActive = (status: ChatStatus | null): boolean =>
	status === "running" ||
	status === "requires_action" ||
	status === "interrupting";
