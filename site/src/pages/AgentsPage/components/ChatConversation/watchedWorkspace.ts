import type * as TypesGen from "#/api/typesGenerated";
import { getWorkspaceAgents } from "#/utils/workspace";
import { getWorkspaceAgent } from "./chatHelpers";

// Keep this list in sync with app fields consumed by the chat UI, or live
// updates to those fields can retain stale query data.
const watchedAgentAppFields: readonly (keyof TypesGen.WorkspaceApp)[] = [
	"id",
	"slug",
	"health",
	"hidden",
	"external",
	"command",
	"subdomain",
	"subdomain_name",
	"display_name",
];

/** @internal Exported for testing. */
export const isWatchedWorkspaceViewUnchanged = (
	prev: TypesGen.Workspace,
	next: TypesGen.Workspace,
	chatAgentId: string | undefined,
): boolean => {
	const prevAgent = getWorkspaceAgent(prev, chatAgentId);
	const nextAgent = getWorkspaceAgent(next, chatAgentId);
	const prevApps = prevAgent?.apps ?? [];
	const nextApps = nextAgent?.apps ?? [];
	return (
		prev.latest_build.id === next.latest_build.id &&
		prev.latest_build.status === next.latest_build.status &&
		prev.health.healthy === next.health.healthy &&
		prev.name === next.name &&
		prev.owner_name === next.owner_name &&
		prevAgent?.id === nextAgent?.id &&
		prevAgent?.status === nextAgent?.status &&
		prevAgent?.name === nextAgent?.name &&
		prevAgent?.expanded_directory === nextAgent?.expanded_directory &&
		prevAgent?.lifecycle_state === nextAgent?.lifecycle_state &&
		prevApps.length === nextApps.length &&
		prevApps.every((prevApp, index) => {
			const nextApp = nextApps[index];
			return watchedAgentAppFields.every(
				(field) => prevApp[field] === nextApp[field],
			);
		})
	);
};

/**
 * True when a running workspace has agents but the chat's agent ID is absent
 * from the latest build (stale after a rebuild, or not yet persisted). Chat
 * reads can return a repaired ID, so callers should refetch the chat.
 *
 * @internal Exported for testing.
 */
export const isChatAgentBindingUnresolved = (
	workspace: TypesGen.Workspace | undefined,
	chatAgentId: string | undefined,
): boolean => {
	if (workspace?.latest_build.status !== "running") {
		return false;
	}
	const agents = getWorkspaceAgents(workspace);
	return agents.length > 0 && !agents.some((agent) => agent.id === chatAgentId);
};

// Compile-time guard: ensures the workspace watcher bailout comparison
// covers every WorkspaceAgent field the UI reads. If WorkspaceAgent
// gains a new field, this will error until the field is either added
// to the comparison or explicitly excluded here.
type _UncoveredAgentFields = Omit<
	TypesGen.WorkspaceAgent,
	| "id"
	| "status"
	| "name"
	| "expanded_directory"
	| "lifecycle_state"
	// Fields below are intentionally not compared. They change
	// frequently (stats, metadata) or are objects/arrays that would
	// require deep comparison, and the UI does not read them.
	| "parent_id"
	| "created_at"
	| "updated_at"
	| "first_connected_at"
	| "last_connected_at"
	| "disconnected_at"
	| "started_at"
	| "ready_at"
	| "resource_id"
	| "instance_id"
	| "architecture"
	| "environment_variables"
	| "operating_system"
	| "logs_length"
	| "logs_overflowed"
	| "directory"
	| "version"
	| "api_version"
	| "apps"
	| "latency"
	| "connection_timeout_seconds"
	| "troubleshooting_url"
	| "subsystems"
	| "health"
	| "display_apps"
	| "log_sources"
	| "scripts"
	| "metadata"
	| "startup_script_behavior"
>;
// If this errors, a new field was added to WorkspaceAgent.
// Decide: does the UI read it? If yes, add it to the first
// section of the Omit above and to the bailout comparison
// in the workspace watcher message handler. If no, add it
// to the excluded section of the Omit.
const _agentFieldGuard: Record<keyof _UncoveredAgentFields, true> = {};
