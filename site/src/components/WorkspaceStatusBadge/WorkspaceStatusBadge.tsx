import CircularProgress from "@material-ui/core/CircularProgress"
import ErrorIcon from "@material-ui/icons/ErrorOutline"
import StopIcon from "@material-ui/icons/PauseOutlined"
import PlayIcon from "@material-ui/icons/PlayArrowOutlined"
import { WorkspaceBuild } from "api/typesGenerated"
import { PaletteIndex, Pill } from "components/Pill/Pill"
import React from "react"
import { getWorkspaceStatus } from "util/workspace"

const StatusLanguage = {
  loading: "Loading",
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

const LoadingIcon: React.FC = () => {
  return <CircularProgress size={10} style={{ color: "#FFF" }} />
}

export const getStatus = (
  build: WorkspaceBuild,
): {
  type?: PaletteIndex
  text: string
  icon: React.ReactNode
} => {
  const status = getWorkspaceStatus(build)
  switch (status) {
    case undefined:
      return {
        text: StatusLanguage.loading,
        icon: <LoadingIcon />,
      }
    case "started":
      return {
        type: "success",
        text: StatusLanguage.started,
        icon: <PlayIcon />,
      }
    case "starting":
      return {
        type: "success",
        text: StatusLanguage.starting,
        icon: <LoadingIcon />,
      }
    case "stopping":
      return {
        type: "warning",
        text: StatusLanguage.stopping,
        icon: <LoadingIcon />,
      }
    case "stopped":
      return {
        type: "warning",
        text: StatusLanguage.stopped,
        icon: <StopIcon />,
      }
    case "deleting":
      return {
        type: "warning",
        text: StatusLanguage.deleting,
        icon: <LoadingIcon />,
      }
    case "deleted":
      return {
        type: "error",
        text: StatusLanguage.deleted,
        icon: <ErrorIcon />,
      }
    case "canceling":
      return {
        type: "warning",
        text: StatusLanguage.canceling,
        icon: <LoadingIcon />,
      }
    case "canceled":
      return {
        type: "warning",
        text: StatusLanguage.canceled,
        icon: <ErrorIcon />,
      }
    case "error":
      return {
        type: "error",
        text: StatusLanguage.failed,
        icon: <ErrorIcon />,
      }
    case "queued":
      return {
        type: "info",
        text: StatusLanguage.queued,
        icon: <LoadingIcon />,
      }
  }
  throw new Error("unknown text " + status)
}

export type WorkspaceStatusBadgeProps = {
  build: WorkspaceBuild
  className?: string
}

export const WorkspaceStatusBadge: React.FC<WorkspaceStatusBadgeProps> = ({ build, className }) => {
  const { text, icon, type } = getStatus(build)
  return <Pill className={className} icon={icon} text={text} type={type} />
}
