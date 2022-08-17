import { Theme } from "@material-ui/core/styles"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import { WorkspaceBuildTransition } from "../api/types"
import * as TypesGen from "../api/typesGenerated"

dayjs.extend(utc)

// all the possible states returned by the API
export enum WorkspaceStateEnum {
  starting = "Starting",
  started = "Started",
  stopping = "Stopping",
  stopped = "Stopped",
  canceling = "Canceling",
  canceled = "Canceled",
  deleting = "Deleting",
  deleted = "Deleted",
  queued = "Queued",
  error = "Error",
  loading = "Loading",
}

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

export const DisplayWorkspaceBuildStatusLanguage = {
  succeeded: "Succeeded",
  pending: "Pending",
  running: "Running",
  canceling: "Canceling",
  canceled: "Canceled",
  failed: "Failed",
}

export const getDisplayWorkspaceBuildStatus = (
  theme: Theme,
  build: TypesGen.WorkspaceBuild,
): {
  color: string
  status: string
} => {
  switch (build.job.status) {
    case "succeeded":
      return {
        color: theme.palette.success.main,
        status: `⦿ ${DisplayWorkspaceBuildStatusLanguage.succeeded}`,
      }
    case "pending":
      return {
        color: theme.palette.text.secondary,
        status: `⦿ ${DisplayWorkspaceBuildStatusLanguage.pending}`,
      }
    case "running":
      return {
        color: theme.palette.primary.main,
        status: `⦿ ${DisplayWorkspaceBuildStatusLanguage.running}`,
      }
    case "failed":
      return {
        color: theme.palette.text.secondary,
        status: `⦸ ${DisplayWorkspaceBuildStatusLanguage.failed}`,
      }
    case "canceling":
      return {
        color: theme.palette.warning.light,
        status: `◍ ${DisplayWorkspaceBuildStatusLanguage.canceling}`,
      }
    case "canceled":
      return {
        color: theme.palette.text.secondary,
        status: `◍ ${DisplayWorkspaceBuildStatusLanguage.canceled}`,
      }
  }
}

export const DisplayWorkspaceBuildInitiatedByLanguage = {
  autostart: "system/autostart",
  autostop: "system/autostop",
}

export const getDisplayWorkspaceBuildInitiatedBy = (build: TypesGen.WorkspaceBuild): string => {
  switch (build.reason) {
    case "initiator":
      return build.initiator_name
    case "autostart":
      return DisplayWorkspaceBuildInitiatedByLanguage.autostart
    case "autostop":
      return DisplayWorkspaceBuildInitiatedByLanguage.autostop
  }
}

export const getWorkspaceBuildDurationInSeconds = (
  build: TypesGen.WorkspaceBuild,
): number | undefined => {
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

export const isWorkspaceDeleted = (workspace: TypesGen.Workspace): boolean => {
  return getWorkspaceStatus(workspace.latest_build) === succeededToStatus["delete"]
}

export const defaultWorkspaceExtension = (
  __startDate?: dayjs.Dayjs,
): TypesGen.PutExtendWorkspaceRequest => {
  const now = __startDate ? dayjs(__startDate) : dayjs()
  const fourHoursFromNow = now.add(4, "hours").utc()

  return {
    deadline: fourHoursFromNow.format(),
  }
}

// You can see the favicon designs here: https://www.figma.com/file/YIGBkXUcnRGz2ZKNmLaJQf/Coder-v2-Design?node-id=560%3A620

type FaviconType =
  | "favicon"
  | "favicon-success"
  | "favicon-error"
  | "favicon-warning"
  | "favicon-running"

export const getFaviconByStatus = (build: TypesGen.WorkspaceBuild): FaviconType => {
  const status = getWorkspaceStatus(build)
  switch (status) {
    case undefined:
      return "favicon"
    case "started":
      return "favicon-success"
    case "starting":
      return "favicon-running"
    case "stopping":
      return "favicon-running"
    case "stopped":
      return "favicon"
    case "deleting":
      return "favicon"
    case "deleted":
      return "favicon"
    case "canceling":
      return "favicon-warning"
    case "canceled":
      return "favicon"
    case "error":
      return "favicon-error"
    case "queued":
      return "favicon"
  }
  throw new Error("unknown status " + status)
}
