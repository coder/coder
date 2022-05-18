import { Theme } from "@material-ui/core/styles"
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

export const getDisplayStatus = (
  theme: Theme,
  build: WorkspaceBuild,
): {
  color: string
  status: string
} => {
  const status = getWorkspaceStatus(build)
  switch (status) {
    case undefined:
      return {
        color: theme.palette.text.secondary,
        status: "Loading...",
      }
    case "started":
      return {
        color: theme.palette.success.main,
        status: "⦿ Running",
      }
    case "starting":
      return {
        color: theme.palette.success.main,
        status: "⦿ Starting",
      }
    case "stopping":
      return {
        color: theme.palette.text.secondary,
        status: "◍ Stopping",
      }
    case "stopped":
      return {
        color: theme.palette.text.secondary,
        status: "◍ Stopped",
      }
    case "deleting":
      return {
        color: theme.palette.text.secondary,
        status: "⦸ Deleting",
      }
    case "deleted":
      return {
        color: theme.palette.text.secondary,
        status: "⦸ Deleted",
      }
    case "canceling":
      return {
        color: theme.palette.warning.light,
        status: "◍ Canceling",
      }
    case "canceled":
      return {
        color: theme.palette.text.secondary,
        status: "◍ Canceled",
      }
    case "error":
      return {
        color: theme.palette.error.main,
        status: "ⓧ Failed",
      }
    case "queued":
      return {
        color: theme.palette.text.secondary,
        status: "◍ Queued",
      }
  }
  throw new Error("unknown status " + status)
}
