import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import CloudQueueIcon from "@material-ui/icons/CloudQueue"
import CropSquareIcon from "@material-ui/icons/CropSquare"
import DeleteOutlineIcon from "@material-ui/icons/DeleteOutline"
import HighlightOffIcon from "@material-ui/icons/HighlightOff"
import PlayCircleOutlineIcon from "@material-ui/icons/PlayCircleOutline"
import { FC } from "react"
import { Workspace } from "../../api/typesGenerated"
import { WorkspaceStatus } from "../../util/workspace"
import { WorkspaceActionButton } from "../WorkspaceActionButton/WorkspaceActionButton"

export const Language = {
  start: "Start",
  stop: "Stop",
  delete: "Delete",
  cancel: "Cancel",
  update: "Update",
}

interface WorkspaceAction {
  handleAction: () => void
}

export const StartButton: FC<WorkspaceAction> = ({ handleAction }) => {
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

export const StopButton: FC<WorkspaceAction> = ({ handleAction }) => {
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

export const DeleteButton: FC<WorkspaceAction> = ({ handleAction }) => {
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

type UpdateAction = WorkspaceAction & {
  workspace: Workspace
  workspaceStatus: WorkspaceStatus
}

export const UpdateButton: FC<UpdateAction> = ({ handleAction, workspace, workspaceStatus }) => {
  const styles = useStyles()

  /**
   * Jobs submitted while another job is in progress will be discarded,
   * so check whether workspace job status has reached completion (whether successful or not).
   */
  const canAcceptJobs = (workspaceStatus: WorkspaceStatus) =>
    ["started", "stopped", "deleted", "error", "canceled"].includes(workspaceStatus)

  return (
    <>
      {workspace.outdated && canAcceptJobs(workspaceStatus) && (
        <Button
          className={styles.actionButton}
          startIcon={<CloudQueueIcon />}
          onClick={handleAction}
        >
          {Language.update}
        </Button>
      )}
    </>
  )
}

export const CancelButton: FC<WorkspaceAction> = ({ handleAction }) => {
  const styles = useStyles()

  return (
    <WorkspaceActionButton
      className={styles.actionButton}
      icon={<HighlightOffIcon />}
      onClick={handleAction}
      label={Language.cancel}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  actionButton: {
    // Set fixed width for the action buttons so they will not change the size
    // during the transitions
    width: theme.spacing(16),
    border: "none",
    borderRadius: `${theme.shape.borderRadius}px 0px 0px ${theme.shape.borderRadius}px`,
  },
}))
