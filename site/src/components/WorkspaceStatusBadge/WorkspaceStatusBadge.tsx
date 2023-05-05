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
import { makeStyles } from "@material-ui/core/styles"
import { combineClasses } from "utils/combineClasses"

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

export const WorkspaceStatusText: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ build, className }) => {
  const styles = useStyles()
  const { text, type } = getStatus(build.status)
  return (
    <span
      role="status"
      className={combineClasses([
        className,
        styles.root,
        styles[`type-${type}`],
      ])}
    >
      {text}
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  root: { fontWeight: 600 },
  "type-error": {
    color: theme.palette.error.light,
  },
  "type-warning": {
    color: theme.palette.warning.light,
  },
  "type-success": {
    color: theme.palette.success.light,
  },
  "type-info": {
    color: theme.palette.info.light,
  },
  "type-undefined": {
    color: theme.palette.text.secondary,
  },
  "type-primary": {
    color: theme.palette.text.primary,
  },
  "type-secondary": {
    color: theme.palette.text.secondary,
  },
}))
