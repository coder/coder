import { Theme } from "@material-ui/core/styles"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import { WorkspaceBuildTransition } from "../api/types"
import * as TypesGen from "../api/typesGenerated"

dayjs.extend(utc)

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
export const getWorkspaceStatus = (workspaceBuild?: TypesGen.WorkspaceBuild): WorkspaceStatus => {
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

// Localize workspace status and provide corresponding color from theme
export const getDisplayStatus = (
  theme: Theme,
  build: TypesGen.WorkspaceBuild,
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
        color: theme.palette.primary.main,
        status: `⦿ ${DisplayStatusLanguage.starting}`,
      }
    case "stopping":
      return {
        color: theme.palette.primary.main,
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

export const getWorkspaceBuildDurationInSeconds = (build: TypesGen.WorkspaceBuild): number | undefined => {
  const isCompleted = build.job.started_at && build.job.completed_at

  if (!isCompleted) {
    return
  }

  const startedAt = dayjs(build.job.started_at)
  const completedAt = dayjs(build.job.completed_at)
  return completedAt.diff(startedAt, "seconds")
}

export const displayWorkspaceBuildDuration = (
  build: TypesGen.WorkspaceBuild,
  inProgressLabel = "In progress",
): string => {
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
  agent: TypesGen.WorkspaceAgent,
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

export const isWorkspaceOn = (workspace: TypesGen.Workspace): boolean => {
  const transition = workspace.latest_build.transition
  const status = workspace.latest_build.job.status
  return transition === "start" && status === "succeeded"
}

export const defaultWorkspaceExtension = (__startDate?: dayjs.Dayjs): TypesGen.PutExtendWorkspaceRequest => {
  const now = __startDate ? dayjs(__startDate) : dayjs()
  const NinetyMinutesFromNow = now.add(90, "minutes").utc()

  return {
    deadline: NinetyMinutesFromNow.format(),
  }
}

export const workspaceQueryToFilter = (query?: string): TypesGen.WorkspaceFilter => {
  const defaultFilter: TypesGen.WorkspaceFilter = {}
  const preparedQuery = query?.trim().replace(/  +/g, " ")

  if (!preparedQuery) {
    return defaultFilter
  } else {
    const parts = preparedQuery.split(" ")

    for (const part of parts) {
      const [key, val] = part.split(":")
      if (key && val) {
        if (key === "owner") {
          return {
            owner: val,
          }
        }
        // skip invalid key pairs
        continue
      }

      const [username, name] = part.split("/")
      if (username && name) {
        return {
          owner: username,
          name: name,
        }
      }
      return {
        name: part,
      }
    }

    return defaultFilter
  }
}
