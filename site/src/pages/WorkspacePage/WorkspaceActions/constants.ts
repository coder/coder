import { type Workspace, type WorkspaceStatus } from "api/typesGenerated";

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
  "retryDebug",

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
  canRetryDebug: boolean,
): WorkspaceAbilities => {
  if (workspace.dormant_at) {
    return {
      actions: ["activate"],
      canCancel: false,
      canAcceptJobs: false,
    };
  }

  const status = workspace.latest_build.status;
  if (status === "failed" && canRetryDebug) {
    return {
      ...statusToAbility.failed,
      actions: ["retry", "retryDebug"],
    };
  }

  return statusToAbility[status];
};

const statusToAbility: Record<WorkspaceStatus, WorkspaceAbilities> = {
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
