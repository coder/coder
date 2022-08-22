import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles, Theme, useTheme } from "@material-ui/core/styles"
import ErrorIcon from "@material-ui/icons/ErrorOutline"
import StopIcon from "@material-ui/icons/PauseOutlined"
import PlayIcon from "@material-ui/icons/PlayArrowOutlined"
import { WorkspaceBuild } from "api/typesGenerated"
import React from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import { combineClasses } from "util/combineClasses"
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
  theme: Theme,
  build: WorkspaceBuild,
): {
  borderColor: string
  backgroundColor: string
  text: string
  icon: React.ReactNode
} => {
  const status = getWorkspaceStatus(build)
  switch (status) {
    case undefined:
      return {
        borderColor: theme.palette.text.secondary,
        backgroundColor: theme.palette.text.secondary,
        text: StatusLanguage.loading,
        icon: <LoadingIcon />,
      }
    case "started":
      return {
        borderColor: theme.palette.success.main,
        backgroundColor: theme.palette.success.dark,
        text: StatusLanguage.started,
        icon: <PlayIcon />,
      }
    case "starting":
      return {
        borderColor: theme.palette.success.main,
        backgroundColor: theme.palette.success.dark,
        text: StatusLanguage.starting,
        icon: <LoadingIcon />,
      }
    case "stopping":
      return {
        borderColor: theme.palette.warning.main,
        backgroundColor: theme.palette.warning.dark,
        text: StatusLanguage.stopping,
        icon: <LoadingIcon />,
      }
    case "stopped":
      return {
        borderColor: theme.palette.warning.main,
        backgroundColor: theme.palette.warning.dark,
        text: StatusLanguage.stopped,
        icon: <StopIcon />,
      }
    case "deleting":
      return {
        borderColor: theme.palette.warning.main,
        backgroundColor: theme.palette.warning.dark,
        text: StatusLanguage.deleting,
        icon: <LoadingIcon />,
      }
    case "deleted":
      return {
        borderColor: theme.palette.error.main,
        backgroundColor: theme.palette.error.dark,
        text: StatusLanguage.deleted,
        icon: <ErrorIcon />,
      }
    case "canceling":
      return {
        borderColor: theme.palette.warning.main,
        backgroundColor: theme.palette.warning.dark,
        text: StatusLanguage.canceling,
        icon: <LoadingIcon />,
      }
    case "canceled":
      return {
        borderColor: theme.palette.warning.main,
        backgroundColor: theme.palette.warning.dark,
        text: StatusLanguage.canceled,
        icon: <ErrorIcon />,
      }
    case "error":
      return {
        borderColor: theme.palette.error.main,
        backgroundColor: theme.palette.error.dark,
        text: StatusLanguage.failed,
        icon: <ErrorIcon />,
      }
    case "queued":
      return {
        borderColor: theme.palette.info.main,
        backgroundColor: theme.palette.info.dark,
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

export const WorkspaceStatusBadge: React.FC<React.PropsWithChildren<WorkspaceStatusBadgeProps>> = ({
  build,
  className,
}) => {
  const styles = useStyles()
  const theme = useTheme()
  const { text, icon, ...colorStyles } = getStatus(theme, build)
  return (
    <div
      className={combineClasses([styles.wrapper, className])}
      style={{ ...colorStyles }}
      role="status"
    >
      <div className={styles.iconWrapper}>{icon}</div>
      {text}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    display: "inline-flex",
    alignItems: "center",
    borderWidth: 1,
    borderStyle: "solid",
    borderRadius: 99999,
    fontSize: 14,
    fontWeight: 500,
    color: "#FFF",
    height: theme.spacing(3),
    paddingLeft: theme.spacing(0.75),
    paddingRight: theme.spacing(1.5),
    whiteSpace: "nowrap",
  },

  iconWrapper: {
    marginRight: theme.spacing(0.5),
    width: theme.spacing(2),
    height: theme.spacing(2),
    lineHeight: 0,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",

    "& > svg": {
      width: theme.spacing(2),
      height: theme.spacing(2),
    },
  },
}))
