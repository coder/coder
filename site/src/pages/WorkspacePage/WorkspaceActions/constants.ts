import { type Workspace, type WorkspaceStatus } from "api/typesGenerated";
import { workspaceUpdatePolicy } from "utils/workspace";

/**
 * An iterable of all button types supported by the workspace actions UI
 */
const buttonTypes = [
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

  // There's no need for a retrying state because retrying starts a transition
  // into one of the starting, stopping, or deleting states (based on the
  // WorkspaceTransition type)
  "retry",

  // These are buttons that should be used with disabled UI elements
  "canceling",
  "deleted",
  "pending",
] as const;

/**
 * A button type supported by the workspace actions UI
 */
export type ButtonType = (typeof buttonTypes)[number];

interface WorkspaceAbilities {
  actions: readonly ButtonType[];
  canCancel: boolean;
  canAcceptJobs: boolean;
}

export const actionsByWorkspaceStatus = (
  workspace: Workspace,
  status: WorkspaceStatus,
  canChangeVersions: boolean,
): WorkspaceAbilities => {
  if (workspace.dormant_at) {
    return {
      actions: ["activate"],
      canCancel: false,
      canAcceptJobs: false,
    };
  }
  if (
    workspace.outdated &&
    workspaceUpdatePolicy(workspace, canChangeVersions) === "always"
  ) {
    if (status === "running") {
      return {
        actions: ["stop"],
        canCancel: false,
        canAcceptJobs: true,
      };
    }
    if (status === "stopped") {
      return {
        actions: [],
        canCancel: false,
        canAcceptJobs: true,
      };
    }
  }
  return statusToActions[status];
};

const statusToActions: Record<WorkspaceStatus, WorkspaceAbilities> = {
  starting: {
    actions: ["starting"],
    canCancel: true,
    canAcceptJobs: false,
  },
  running: {
    actions: ["stop", "restart"],
    canCancel: false,
    canAcceptJobs: true,
  },
  stopping: {
    actions: ["stopping"],
    canCancel: true,
    canAcceptJobs: false,
  },
  stopped: {
    actions: ["start"],
    canCancel: false,
    canAcceptJobs: true,
  },
  canceled: {
    actions: ["start", "stop"],
    canCancel: false,
    canAcceptJobs: true,
  },

  // in the case of an error
  failed: {
    actions: ["retry"],
    canCancel: false,
    canAcceptJobs: true,
  },

  // Disabled states
  canceling: {
    actions: ["canceling"],
    canCancel: false,
    canAcceptJobs: false,
  },
  deleting: {
    actions: ["deleting"],
    canCancel: true,
    canAcceptJobs: false,
  },
  deleted: {
    actions: ["deleted"],
    canCancel: false,
    canAcceptJobs: false,
  },
  pending: {
    actions: ["pending"],
    canCancel: false,
    canAcceptJobs: false,
  },
};
