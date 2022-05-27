import { Theme } from "@material-ui/core/styles"
import dayjs from "dayjs"
import { WorkspaceBuildTransition } from "../api/types"
import { WorkspaceAgent, WorkspaceBuild } from "../api/typesGenerated"

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

export const DisplayStatusLanguage = {
  loading: "Loading...",
  started: "Running",
  starting: "Starting",
  stopping: "Stopping",
  stopped: "Stopped",
  deleting: "Deleting",
  deleted: "Deleted",
  canceling: "Canceling action",
  canceled: "Canceled action",
  failed: "Failed",
  queued: "Queued",
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
        status: DisplayStatusLanguage.loading,
      }
    case "started":
      return {
        color: theme.palette.success.main,
        status: `⦿ ${DisplayStatusLanguage.started}`,
      }
    case "starting":
      return {
        color: theme.palette.success.main,
        status: `⦿ ${DisplayStatusLanguage.starting}`,
      }
    case "stopping":
      return {
        color: theme.palette.text.secondary,
        status: `◍ ${DisplayStatusLanguage.stopping}`,
      }
    case "stopped":
      return {
        color: theme.palette.text.secondary,
        status: `◍ ${DisplayStatusLanguage.stopped}`,
      }
    case "deleting":
      return {
        color: theme.palette.text.secondary,
        status: `⦸ ${DisplayStatusLanguage.deleting}`,
      }
    case "deleted":
      return {
        color: theme.palette.text.secondary,
        status: `⦸ ${DisplayStatusLanguage.deleted}`,
      }
    case "canceling":
      return {
        color: theme.palette.warning.light,
        status: `◍ ${DisplayStatusLanguage.canceling}`,
      }
    case "canceled":
      return {
        color: theme.palette.text.secondary,
        status: `◍ ${DisplayStatusLanguage.canceled}`,
      }
    case "error":
      return {
        color: theme.palette.error.main,
        status: `ⓧ ${DisplayStatusLanguage.failed}`,
      }
    case "queued":
      return {
        color: theme.palette.text.secondary,
        status: `◍ ${DisplayStatusLanguage.queued}`,
      }
  }
  throw new Error("unknown status " + status)
}

export const getWorkspaceBuildDurationInSeconds = (build: WorkspaceBuild): number | undefined => {
  const isCompleted = build.job.started_at && build.job.completed_at

  if (!isCompleted) {
    return
  }

  const startedAt = dayjs(build.job.started_at)
  const completedAt = dayjs(build.job.completed_at)
  return completedAt.diff(startedAt, "seconds")
}

export const displayWorkspaceBuildDuration = (build: WorkspaceBuild, inProgressLabel = "In progress"): string => {
  const duration = getWorkspaceBuildDurationInSeconds(build)
  return duration ? `${duration} seconds` : inProgressLabel
}

export const DisplayAgentStatusLanguage = {
  connected: "⦿ Connected",
  connecting: "⦿ Connecting",
  disconnected: "◍ Disconnected",
}

export const getDisplayAgentStatus = (
  theme: Theme,
  agent: WorkspaceAgent,
): {
  color: string
  status: string
} => {
  switch (agent.status) {
    case undefined:
      return {
        color: theme.palette.text.secondary,
        status: DisplayStatusLanguage.loading,
      }
    case "connected":
      return {
        color: theme.palette.success.main,
        status: DisplayAgentStatusLanguage["connected"],
      }
    case "connecting":
      return {
        color: theme.palette.success.main,
        status: DisplayAgentStatusLanguage["connecting"],
      }
    case "disconnected":
      return {
        color: theme.palette.text.secondary,
        status: DisplayAgentStatusLanguage["disconnected"],
      }
  }
}
