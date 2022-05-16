import { WorkspaceBuildTransition } from "../api/types"
import { WorkspaceBuild } from "../api/typesGenerated"

export type WorkspaceStatus =
  | "queued"
  | "started"
  | "starting"
  | "stopped"
  | "stopping"
  | "error"
  | "loading"
  | "deleting"
  | "deleted"
  | "canceled"
  | "canceling"

const inProgressToStatus: Record<WorkspaceBuildTransition, WorkspaceStatus> = {
  start: "starting",
  stop: "stopping",
  delete: "deleting",
}

const succeededToStatus: Record<WorkspaceBuildTransition, WorkspaceStatus> = {
  start: "started",
  stop: "stopped",
  delete: "deleted",
}

// Converts a workspaces status to a human-readable form.
export const getWorkspaceStatus = (workspaceBuild?: WorkspaceBuild): WorkspaceStatus => {
  const transition = workspaceBuild?.transition as WorkspaceBuildTransition
  const jobStatus = workspaceBuild?.job.status
  switch (jobStatus) {
    case undefined:
      return "loading"
    case "succeeded":
      return succeededToStatus[transition]
    case "pending":
      return "queued"
    case "running":
      return inProgressToStatus[transition]
    case "canceling":
      return "canceling"
    case "canceled":
      return "canceled"
    case "failed":
      return "error"
  }
}
