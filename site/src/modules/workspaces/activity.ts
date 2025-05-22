import type { Workspace } from "api/typesGenerated";
import { isAfter, parseISO, sub } from "date-fns";

export type WorkspaceActivityStatus =
	| "ready"
	| "connected"
	| "inactive"
	| "notConnected"
	| "notRunning";

export function getWorkspaceActivityStatus(
	workspace: Workspace,
): WorkspaceActivityStatus {
	const builtAt = parseISO(workspace.latest_build.created_at);
	const usedAt = parseISO(workspace.last_used_at);
	const now = new Date();

	if (workspace.latest_build.status !== "running") {
		return "notRunning";
	}

	// This needs to compare to `usedAt` instead of `now`, because the "grace period" for
	// marking a workspace as "Connected" is a lot longer. If you compared `builtAt` to `now`,
	// you could end up switching from "Ready" to "Connected" without ever actually connecting.
	const isBuiltRecently = isAfter(builtAt, sub(usedAt, { seconds: 1 }));
	// By default, agents report connection stats every 30 seconds, so 2 minutes should be
	// plenty. Disconnection will be reflected relatively-quickly
	const isUsedRecently = isAfter(usedAt, sub(now, { minutes: 2 }));

	// If the build is still "fresh", it'll be a while before the `last_used_at` gets bumped in
	// a significant way by the agent, so just label it as ready instead of connected.
	// Wait until `last_used_at` is after the time that the build finished, _and_ still
	// make sure to check that it's recent, so that we don't show "Ready" indefinitely.
	if (isUsedRecently && isBuiltRecently && workspace.health.healthy) {
		return "ready";
	}

	if (isUsedRecently) {
		return "connected";
	}

	// TODO: It'd be nice if we could differentiate between "connected but inactive" and
	// "not connected", but that will require some relatively substantial backend work.
	return "inactive";
}
