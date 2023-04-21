import CircularProgress from "@material-ui/core/CircularProgress"
import ErrorIcon from "@material-ui/icons/ErrorOutline"
import StopIcon from "@material-ui/icons/StopOutlined"
import PlayIcon from "@material-ui/icons/PlayArrowOutlined"
import QueuedIcon from "@material-ui/icons/HourglassEmpty"
import { WorkspaceBuild } from "api/typesGenerated"
import { Pill } from "components/Pill/Pill"
import i18next from "i18next"
import { FC, ReactNode, PropsWithChildren } from "react"
import { PaletteIndex } from "theme/palettes"

const LoadingIcon: FC = () => {
  return <CircularProgress size={10} style={{ color: "#FFF" }} />
}

export const getStatus = (
  buildStatus: WorkspaceBuild["status"],
): {
  type?: PaletteIndex
  text: string
  icon: ReactNode
} => {
  const { t } = i18next

  switch (buildStatus) {
    case undefined:
      return {
        text: t("workspaceStatus.loading", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "running":
      return {
        type: "success",
        text: t("workspaceStatus.running", { ns: "common" }),
        icon: <PlayIcon />,
      }
    case "starting":
      return {
        type: "success",
        text: t("workspaceStatus.starting", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "stopping":
      return {
        type: "warning",
        text: t("workspaceStatus.stopping", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "stopped":
      return {
        type: "warning",
        text: t("workspaceStatus.stopped", { ns: "common" }),
        icon: <StopIcon />,
      }
    case "deleting":
      return {
        type: "warning",
        text: t("workspaceStatus.deleting", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "deleted":
      return {
        type: "error",
        text: t("workspaceStatus.deleted", { ns: "common" }),
        icon: <ErrorIcon />,
      }
    case "canceling":
      return {
        type: "warning",
        text: t("workspaceStatus.canceling", { ns: "common" }),
        icon: <LoadingIcon />,
      }
    case "canceled":
      return {
        type: "warning",
        text: t("workspaceStatus.canceled", { ns: "common" }),
        icon: <ErrorIcon />,
      }
    case "failed":
      return {
        type: "error",
        text: t("workspaceStatus.failed", { ns: "common" }),
        icon: <ErrorIcon />,
      }
    case "pending":
      return {
        type: "info",
        text: t("workspaceStatus.pending", { ns: "common" }),
        icon: <QueuedIcon />,
      }
  }
}

export type WorkspaceStatusBadgeProps = {
  build: WorkspaceBuild
  className?: string
}

export const WorkspaceStatusBadge: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ build, className }) => {
  const { text, icon, type } = getStatus(build.status)
  return <Pill className={className} icon={icon} text={text} type={type} />
}
