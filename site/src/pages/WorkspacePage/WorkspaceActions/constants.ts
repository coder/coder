import type { Workspace } from "api/typesGenerated";

/**
 * An iterable of all action types supported by the workspace UI
 */
const actionTypes = [
	"start",
	"starting",
	// Replaces start when an update is required.
	"updateAndStart",
	"stop",
	"stopping",
	"restart",
	"restarting",
	// Replaces restart when an update is required.
	"updateAndRestart",
	"deleting",
	"update",
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

type WorkspaceAbilities = {
	actions: readonly ActionType[];
	canCancel: boolean;
	canAcceptJobs: boolean;
};

export const abilitiesByWorkspaceStatus = (
	workspace: Workspace,
	canDebug: boolean,
): WorkspaceAbilities => {
	if (workspace.dormant_at) {
		return {
			actions: ["activate"],
			canCancel: false,
			canAcceptJobs: true,
		};
	}

	const status = workspace.latest_build.status;

	switch (status) {
		case "starting": {
			return {
				actions: ["starting"],
				canCancel: true,
				canAcceptJobs: false,
			};
		}
		case "running": {
			const actions: ActionType[] = ["stop"];

			if (workspace.template_require_active_version && workspace.outdated) {
				actions.push("updateAndRestart");
			} else {
				if (workspace.outdated) {
					actions.unshift("update");
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
				canCancel: true,
				canAcceptJobs: false,
			};
		}
		case "stopped": {
			const actions: ActionType[] = [];

			if (workspace.template_require_active_version && workspace.outdated) {
				actions.push("updateAndStart");
			} else {
				if (workspace.outdated) {
					actions.unshift("update");
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

			if (canDebug) {
				actions.push("debug");
			}

			if (workspace.outdated) {
				actions.unshift("update");
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
				canCancel: false,
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
				canCancel: true,
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
