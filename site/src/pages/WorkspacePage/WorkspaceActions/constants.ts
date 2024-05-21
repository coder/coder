import type { Workspace } from "api/typesGenerated";

/**
 * An iterable of all action types supported by the workspace UI
 */
export const actionTypes = [
  "start",
  "starting",
  "stop",
  "stopping",
  "restart",
  "restarting",
  "deleting",
  "update",
  "updating",
  "activate",
  "activating",
  "toggleFavorite",

  // There's no need for a retrying state because retrying starts a transition
  // into one of the starting, stopping, or deleting states (based on the
  // WorkspaceTransition type)
  "retry",
  "debug",

  // When a template requires updates, we aim to display a distinct update
  // button that clearly indicates a mandatory update.
  "updateAndStart",

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
      canAcceptJobs: false,
    };
  }

  const status = workspace.latest_build.status;
  if (status === "failed" && canDebug) {
    return {
      actions: ["retry", "debug"],
      canCancel: false,
      canAcceptJobs: true,
    };
  }

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

      // If the template requires the latest version, we prevent the user from
      // restarting the workspace without updating it first. In the Buttons
      // component, we display an UpdateAndStart component to facilitate this.
      if (!workspace.template_require_active_version) {
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

      // If the template requires the latest version, we prevent the user from
      // starting the workspace without updating it first. In the Buttons
      // component, we display an UpdateAndStart component to facilitate this.
      if (!workspace.template_require_active_version) {
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
      return {
        actions: ["retry"],
        canCancel: false,
        canAcceptJobs: true,
      };
    }

    // Disabled states
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
    case "pending": {
      return {
        actions: ["pending"],
        canCancel: false,
        canAcceptJobs: false,
      };
    }
    default: {
      throw new Error(`Unknown workspace status: ${status}`);
    }
  }
};
