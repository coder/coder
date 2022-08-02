import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import BlockIcon from "@material-ui/icons/Block"
import CloudQueueIcon from "@material-ui/icons/CloudQueue"
import CropSquareIcon from "@material-ui/icons/CropSquare"
import DeleteOutlineIcon from "@material-ui/icons/DeleteOutline"
import PlayCircleOutlineIcon from "@material-ui/icons/PlayCircleOutline"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { FC } from "react"
import { combineClasses } from "util/combineClasses"
import { WorkspaceActionButton } from "../WorkspaceActionButton/WorkspaceActionButton"
import { WorkspaceStateEnum } from "./constants"

export const Language = {
  start: "Start",
  stop: "Stop",
  delete: "Delete",
  cancel: "Cancel",
  update: "Update",
  // these labels are used in WorkspaceActions.tsx
  starting: "Starting...",
  stopping: "Stopping...",
  deleting: "Deleting...",
}

interface WorkspaceAction {
  handleAction: () => void
}

export const UpdateButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({ handleAction }) => {
  const styles = useStyles()

  return (
    <Button className={styles.actionButton} startIcon={<CloudQueueIcon />} onClick={handleAction}>
      {Language.update}
    </Button>
  )
}

export const StartButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({ handleAction }) => {
  const styles = useStyles()

  return (
    <WorkspaceActionButton
      className={styles.actionButton}
      icon={<PlayCircleOutlineIcon />}
      onClick={handleAction}
      label={Language.start}
    />
  )
}

export const StopButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({ handleAction }) => {
  const styles = useStyles()

  return (
    <WorkspaceActionButton
      className={styles.actionButton}
      icon={<CropSquareIcon />}
      onClick={handleAction}
      label={Language.stop}
    />
  )
}

export const DeleteButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({ handleAction }) => {
  const styles = useStyles()

  return (
    <WorkspaceActionButton
      className={styles.actionButton}
      icon={<DeleteOutlineIcon />}
      onClick={handleAction}
      label={Language.delete}
    />
  )
}

export const CancelButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({ handleAction }) => {
  const styles = useStyles()

  // this is an icon button, so it's important to include an aria label
  return (
    <WorkspaceActionButton
      icon={<BlockIcon />}
      onClick={handleAction}
      className={styles.cancelButton}
      ariaLabel="cancel action"
    />
  )
}

interface DisabledProps {
  workspaceState: WorkspaceStateEnum
}

export const DisabledButton: FC<React.PropsWithChildren<DisabledProps>> = ({ workspaceState }) => {
  const styles = useStyles()

  return (
    <Button disabled className={styles.actionButton}>
      {workspaceState}
    </Button>
  )
}

interface LoadingProps {
  label: string
}

export const ActionLoadingButton: FC<React.PropsWithChildren<LoadingProps>> = ({ label }) => {
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
    borderLeft: "1px solid #333740", // MUI disabled button
    borderRadius: "3px 0px 0px 3px",
  },
}))
