import type { Workspace } from "#/api/typesGenerated";
import { CANCELLABLE_BUILD_STATUSES } from "#/modules/workspaces/status";

/**
 * An iterable of all action types supported by the workspace UI
 */
const actionTypes = [
	"start",
	"starting",
	// Appears beside start when an update is available.
	"updateAndStart",
	// Replaces start when an update is required.
	"updateAndStartRequireActiveVersion",
	"stop",
	"stopping",
	"restart",
	"restarting",
	// Appears beside restart when an update is available.
	"updateAndRestart",
	// Replaces restart when an update is required.
	"updateAndRestartRequireActiveVersion",
	"deleting",
	"updating",
	"activate",
	"activating",

	// There's no need for a retrying state because retrying starts a transition
	// into one of the starting, stopping, or deleting states (based on the
	// WorkspaceTransition type)
	"retry",
	"debug",

	// These are buttons that should be used with disabled UI elements
	"canceling",
	"deleted",
	"pending",
] as const;

export type ActionType = (typeof actionTypes)[number];

type ActionPermissions = {
	canDebug: boolean;
	isOwner: boolean;
};

type CancelPermissions = Pick<ActionPermissions, "isOwner">;

type WorkspaceAbilities = {
	actions: readonly ActionType[];
	canCancel: boolean;
	canAcceptJobs: boolean;
};

// Share the cancellability rules between row and bulk actions so they stay
// in sync with the backend permission checks.
export const canCancelWorkspaceBuild = (
	workspace: Pick<
		Workspace,
		"dormant_at" | "latest_build" | "template_allow_user_cancel_workspace_jobs"
	>,
	permissions: CancelPermissions,
): boolean => {
	if (workspace.dormant_at || workspace.latest_build.has_external_agent) {
		return false;
	}

	if (!CANCELLABLE_BUILD_STATUSES.includes(workspace.latest_build.status)) {
		return false;
	}

	if (workspace.latest_build.status === "pending") {
		return true;
	}

	return (
		workspace.template_allow_user_cancel_workspace_jobs || permissions.isOwner
	);
};

export const abilitiesByWorkspaceStatus = (
	workspace: Workspace,
	permissions: ActionPermissions,
): WorkspaceAbilities => {
	if (workspace.dormant_at) {
		return {
			actions: ["activate"],
			canCancel: false,
			canAcceptJobs: true,
		};
	}

	if (workspace.latest_build.has_external_agent) {
		return {
			actions: [],
			canCancel: false,
			canAcceptJobs: true,
		};
	}

	const status = workspace.latest_build.status;
	const canCancel = canCancelWorkspaceBuild(workspace, permissions);

	switch (status) {
		case "starting": {
			return {
				actions: ["starting"],
				canCancel,
				canAcceptJobs: false,
			};
		}
		case "running": {
			const actions: ActionType[] = ["stop"];

			if (workspace.template_require_active_version && workspace.outdated) {
				actions.push("updateAndRestartRequireActiveVersion");
			} else {
				if (workspace.outdated) {
					actions.unshift("updateAndRestart");
				}
				actions.push("restart");
			}

			return {
				actions,
				canCancel: false,
				canAcceptJobs: true,
			};
		}
		case "stopping": {
			return {
				actions: ["stopping"],
				canCancel,
				canAcceptJobs: false,
			};
		}
		case "stopped": {
			const actions: ActionType[] = [];

			if (workspace.template_require_active_version && workspace.outdated) {
				actions.push("updateAndStartRequireActiveVersion");
			} else {
				if (workspace.outdated) {
					actions.unshift("updateAndStart");
				}
				actions.push("start");
			}

			return {
				actions,
				canCancel: false,
				canAcceptJobs: true,
			};
		}
		case "canceled": {
			return {
				actions: ["start", "stop"],
				canCancel: false,
				canAcceptJobs: true,
			};
		}
		case "failed": {
			const actions: ActionType[] = ["retry"];

			if (permissions.canDebug) {
				actions.push("debug");
			}

			if (workspace.outdated) {
				actions.unshift("updateAndStart");
			}

			return {
				actions,
				canCancel: false,
				canAcceptJobs: true,
			};
		}

		// Disabled states
		case "pending": {
			return {
				actions: ["pending"],
				canCancel,
				canAcceptJobs: false,
			};
		}
		case "canceling": {
			return {
				actions: ["canceling"],
				canCancel: false,
				canAcceptJobs: false,
			};
		}
		case "deleting": {
			return {
				actions: ["deleting"],
				canCancel,
				canAcceptJobs: false,
			};
		}
		case "deleted": {
			return {
				actions: ["deleted"],
				canCancel: false,
				canAcceptJobs: false,
			};
		}

		default:
			throw new Error(`Unknown workspace status: ${status}`);
	}
};
