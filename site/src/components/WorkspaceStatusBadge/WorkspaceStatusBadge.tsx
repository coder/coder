import CircularProgress from "@mui/material/CircularProgress"
import ErrorIcon from "@mui/icons-material/ErrorOutline"
import StopIcon from "@mui/icons-material/StopOutlined"
import PlayIcon from "@mui/icons-material/PlayArrowOutlined"
import QueuedIcon from "@mui/icons-material/HourglassEmpty"
import { Workspace, WorkspaceBuild } from "api/typesGenerated"
import { Pill } from "components/Pill/Pill"
import i18next from "i18next"
import { FC, PropsWithChildren } from "react"
import { makeStyles } from "@mui/styles"
import { combineClasses } from "utils/combineClasses"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import {
  ImpendingDeletionBadge,
  ImpendingDeletionText,
} from "components/WorkspaceDeletion"

const LoadingIcon: FC = () => {
  return <CircularProgress size={10} style={{ color: "#FFF" }} />
}

export const getStatus = (buildStatus: WorkspaceBuild["status"]) => {
  const { t } = i18next

  switch (buildStatus) {
    case undefined:
      return {
        text: t("workspaceStatus.loading", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "running":
      return {
        type: "success",
        text: t("workspaceStatus.running", { ns: "common" }),
        icon: <PlayIcon />,
      } as const
    case "starting":
      return {
        type: "success",
        text: t("workspaceStatus.starting", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "stopping":
      return {
        type: "warning",
        text: t("workspaceStatus.stopping", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "stopped":
      return {
        type: "warning",
        text: t("workspaceStatus.stopped", { ns: "common" }),
        icon: <StopIcon />,
      } as const
    case "deleting":
      return {
        type: "warning",
        text: t("workspaceStatus.deleting", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "deleted":
      return {
        type: "error",
        text: t("workspaceStatus.deleted", { ns: "common" }),
        icon: <ErrorIcon />,
      } as const
    case "canceling":
      return {
        type: "warning",
        text: t("workspaceStatus.canceling", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "canceled":
      return {
        type: "warning",
        text: t("workspaceStatus.canceled", { ns: "common" }),
        icon: <ErrorIcon />,
      } as const
    case "failed":
      return {
        type: "error",
        text: t("workspaceStatus.failed", { ns: "common" }),
        icon: <ErrorIcon />,
      } as const
    case "pending":
      return {
        type: "info",
        text: t("workspaceStatus.pending", { ns: "common" }),
        icon: <QueuedIcon />,
      } as const
  }
}

export type WorkspaceStatusBadgeProps = {
  workspace: Workspace
  className?: string
}

export const WorkspaceStatusBadge: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ workspace, className }) => {
  const { text, icon, type } = getStatus(workspace.latest_build.status)
  return (
    <ChooseOne>
      {/* <ImpendingDeletionBadge/> determines its own visibility */}
      <Cond condition={Boolean(ImpendingDeletionBadge({ workspace }))}>
        <ImpendingDeletionBadge workspace={workspace} />
      </Cond>
      <Cond>
        <Pill className={className} icon={icon} text={text} type={type} />
      </Cond>
    </ChooseOne>
  )
}

export const WorkspaceStatusText: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ workspace, className }) => {
  const styles = useStyles()
  const { text, type } = getStatus(workspace.latest_build.status)

  return (
    <ChooseOne>
      {/* <ImpendingDeletionText/> determines its own visibility */}
      <Cond condition={Boolean(ImpendingDeletionText({ workspace }))}>
        <ImpendingDeletionText workspace={workspace} />
      </Cond>
      <Cond>
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
      </Cond>
    </ChooseOne>
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
