import { Workspace, WorkspaceStatus } from "api/typesGenerated";
import { ReactNode } from "react";
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

  // These are buttons that should be with disabled UI elements
  "canceling",
  "deleted",
  "pending",
] as const;

/**
 * A button type supported by the workspace actions UI
 */
export type ButtonType = (typeof buttonTypes)[number];

export type ButtonMapping = {
  [key in ButtonType]: ReactNode;
};

interface WorkspaceAbilities {
  actions: ButtonType[];
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
    workspaceUpdatePolicy(workspace, canChangeVersions)
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
    actions: ["start", "stop"],
    canCancel: false,
    canAcceptJobs: true,
  },
  /**
   * disabled states
   */
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
