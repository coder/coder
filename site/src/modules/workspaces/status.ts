import type { WorkspaceStatus } from "api/typesGenerated";

/**
 * The set of all workspace statuses that indicate that the state for a
 * workspace is in the middle of a transition and will eventually reach a more
 * stable state/status.
 */
export const ACTIVE_BUILD_STATUSES: readonly WorkspaceStatus[] = [
	"canceling",
	"deleting",
	"pending",
	"starting",
	"stopping",
];
