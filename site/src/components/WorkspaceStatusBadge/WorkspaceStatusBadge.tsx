import CircularProgress from "@material-ui/core/CircularProgress"
import ErrorIcon from "@material-ui/icons/ErrorOutline"
import StopIcon from "@material-ui/icons/PauseOutlined"
import PlayIcon from "@material-ui/icons/PlayArrowOutlined"
import { WorkspaceBuild } from "api/typesGenerated"
import { Pill } from "components/Pill/Pill"
import i18next from "i18next"
import React from "react"
import { PaletteIndex } from "theme/palettes"
import { getWorkspaceStatus } from "util/workspace"

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
  const { t } = i18next

  switch (status) {
    case undefined:
      return {
        text: t("workspace_status.loading", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "started":
      return {
        type: "success",
        text: t("workspace_status.started", { ns: "common" }),
        icon: <PlayIcon />,
      }
    case "starting":
      return {
        type: "success",
        text: t("workspace_status.starting", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "stopping":
      return {
        type: "warning",
        text: t("workspace_status.stopping", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "stopped":
      return {
        type: "warning",
        text: t("workspace_status.stopped", { ns: "common" }),
        icon: <StopIcon />,
      }
    case "deleting":
      return {
        type: "warning",
        text: t("workspace_status.deleting", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "deleted":
      return {
        type: "error",
        text: t("workspace_status.deleted", { ns: "common" }),
        icon: <ErrorIcon />,
      }
    case "canceling":
      return {
        type: "warning",
        text: t("workspace_status.canceling", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "canceled":
      return {
        type: "warning",
        text: t("workspace_status.canceled", { ns: "common" }),
        icon: <ErrorIcon />,
      }
    case "error":
      return {
        type: "error",
        text: t("workspace_status.failed", { ns: "common" }),
        icon: <ErrorIcon />,
      }
    case "queued":
      return {
        type: "info",
        text: t("workspace_status.queued", { ns: "common" }),
        icon: <LoadingIcon />,
      }
  }
  throw new Error("unknown text " + status)
}

export type WorkspaceStatusBadgeProps = {
  build: WorkspaceBuild
  className?: string
}

export const WorkspaceStatusBadge: React.FC<React.PropsWithChildren<WorkspaceStatusBadgeProps>> = ({
  build,
  className,
}) => {
  const { text, icon, type } = getStatus(build)
  return <Pill className={className} icon={icon} text={text} type={type} />
}
