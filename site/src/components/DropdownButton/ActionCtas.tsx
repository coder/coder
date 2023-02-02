import Tooltip from "@material-ui/core/Tooltip"
import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import BlockIcon from "@material-ui/icons/Block"
import CloudQueueIcon from "@material-ui/icons/CloudQueue"
import UpdateOutlined from "@material-ui/icons/UpdateOutlined"
import SettingsOutlined from "@material-ui/icons/SettingsOutlined"
import CropSquareIcon from "@material-ui/icons/CropSquare"
import DeleteOutlineIcon from "@material-ui/icons/DeleteOutline"
import PlayCircleOutlineIcon from "@material-ui/icons/PlayCircleOutline"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { combineClasses } from "util/combineClasses"
import { WorkspaceActionButton } from "../WorkspaceActionButton/WorkspaceActionButton"

interface WorkspaceAction {
  handleAction: () => void
}

export const UpdateButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Button
      className={styles.actionButton}
      startIcon={<CloudQueueIcon />}
      onClick={handleAction}
    >
      {t("actionButton.update")}
    </Button>
  )
}

export const ChangeVersionButton: FC<
  React.PropsWithChildren<WorkspaceAction>
> = ({ handleAction }) => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Button
      className={styles.actionButton}
      startIcon={<UpdateOutlined />}
      onClick={handleAction}
    >
      {t("actionButton.changeVersion")}
    </Button>
  )
}

export const BuildParametersButton: FC<
  React.PropsWithChildren<WorkspaceAction>
> = ({ handleAction }) => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Button
      className={styles.actionButton}
      startIcon={<SettingsOutlined />}
      onClick={handleAction}
    >
      {t("actionButton.buildParameters")}
    </Button>
  )
}

export const StartButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <WorkspaceActionButton
      className={styles.actionButton}
      icon={<PlayCircleOutlineIcon />}
      onClick={handleAction}
      label={t("actionButton.start")}
    />
  )
}

export const StopButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <WorkspaceActionButton
      className={styles.actionButton}
      icon={<CropSquareIcon />}
      onClick={handleAction}
      label={t("actionButton.stop")}
    />
  )
}

export const DeleteButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <WorkspaceActionButton
      className={styles.actionButton}
      icon={<DeleteOutlineIcon />}
      onClick={handleAction}
      label={t("actionButton.delete")}
    />
  )
}

export const CancelButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const styles = useStyles()

  // this is an icon button, so it's important to include an aria label
  return (
    <div>
      <Tooltip title="Cancel action">
        {/* We had to wrap the button to make it work with the tooltip. */}
        <div>
          <WorkspaceActionButton
            icon={<BlockIcon />}
            onClick={handleAction}
            className={styles.cancelButton}
            ariaLabel="cancel action"
          />
        </div>
      </Tooltip>
    </div>
  )
}

interface DisabledProps {
  label: string
}

export const DisabledButton: FC<React.PropsWithChildren<DisabledProps>> = ({
  label,
}) => {
  const styles = useStyles()

  return (
    <Button disabled className={styles.actionButton}>
      {label}
    </Button>
  )
}

interface LoadingProps {
  label: string
}

export const ActionLoadingButton: FC<React.PropsWithChildren<LoadingProps>> = ({
  label,
}) => {
  const styles = useStyles()
  return (
    <LoadingButton
      loading
      loadingLabel={label}
      className={combineClasses([styles.loadingButton, styles.actionButton])}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  actionButton: {
    // Set fixed width for the action buttons so they will not change the size
    // during the transitions
    width: theme.spacing(20),
    border: "none",
    borderRadius: `${theme.shape.borderRadius}px 0px 0px ${theme.shape.borderRadius}px`,
  },
  cancelButton: {
    "&.MuiButton-root": {
      padding: "0px 0px !important",
      border: "none",
      borderLeft: `1px solid ${theme.palette.divider}`,
      borderRadius: `0px ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px 0px`,
      width: "63px", // matching dropdown button so button grouping doesn't grow in size
    },
    "& .MuiButton-label": {
      marginLeft: "10px",
    },
  },
  // this is all custom to work with our button wrapper
  loadingButton: {
    border: "none",
    borderRadius: `${theme.shape.borderRadius} 0px 0px ${theme.shape.borderRadius}`,
  },
}))
