import type { ChatStatus } from "#/api/typesGenerated";

/**
 * Statuses in which the agent's turn is still in progress, so the
 * composer shows Stop rather than Send. `requires_action` counts
 * because the server is blocked on a client-executed tool call
 * mid-turn and accepts an interrupt to cancel it.
 */
const activeTurnStatuses: ReadonlySet<ChatStatus> = new Set<ChatStatus>([
	"running",
	"requires_action",
	"interrupting",
]);

/** Whether the chat's turn is still in progress. */
export const isChatTurnActive = (
	status: ChatStatus | null | undefined,
): boolean =>
	status !== null && status !== undefined && activeTurnStatuses.has(status);
