import { type Workspace, type WorkspaceStatus } from "api/typesGenerated";
import { type ReactElement } from "react";

/**
 * Buttons supported by workspace actions. Canceling, Deleted, and Pending
 * should all be associated with disabled states
 */
type ButtonType =
  | Exclude<WorkspaceStatus, "failed" | "canceled" | "running" | "stopped">
  | "start"
  | "stop"
  | "restart"
  | "restarting"
  | "update"
  | "updating"
  | "activate"
  | "activating";

export type ButtonMapping = {
  [key in ButtonType]: ReactElement;
};

interface WorkspaceAbilities {
  actions: readonly ButtonType[];
  canCancel: boolean;
  canAcceptJobs: boolean;
}

export const actionsByWorkspaceStatus = (
  workspace: Workspace,
  status: WorkspaceStatus,
): WorkspaceAbilities => {
  if (workspace.dormant_at) {
    return {
      actions: ["activate"],
      canCancel: false,
      canAcceptJobs: false,
    };
  }
  return statusToActions[status];
};

const statusToActions = {
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
} as const satisfies Record<WorkspaceStatus, WorkspaceAbilities>;
