import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import CancelIcon from "@material-ui/icons/Cancel"
import CloudDownloadIcon from "@material-ui/icons/CloudDownload"
import PlayArrowRoundedIcon from "@material-ui/icons/PlayArrowRounded"
import StopIcon from "@material-ui/icons/Stop"
import React from "react"
import { Workspace } from "../../api/typesGenerated"
import { getWorkspaceStatus, WorkspaceStatus } from "../../util/workspace"
import { Stack } from "../Stack/Stack"
import { WorkspaceActionButton } from "../WorkspaceActionButton/WorkspaceActionButton"

export const Language = {
  stop: "Stop workspace",
  stopping: "Stopping workspace",
  start: "Start workspace",
  starting: "Starting workspace",
  cancel: "Cancel action",
  update: "Update workspace",
}

/**
 * Jobs submitted while another job is in progress will be discarded,
 * so check whether workspace job status has reached completion (whether successful or not).
 */
const canAcceptJobs = (workspaceStatus: WorkspaceStatus) =>
  ["started", "stopped", "deleted", "error", "canceled"].includes(workspaceStatus)

/**
 *  Jobs that are in progress (queued or pending) can be canceled.
 * @param workspaceStatus WorkspaceStatus
 * @returns boolean
 */
const canCancelJobs = (workspaceStatus: WorkspaceStatus) =>
  ["starting", "stopping", "deleting"].includes(workspaceStatus)

const canStart = (workspaceStatus: WorkspaceStatus) => ["stopped", "canceled", "error"].includes(workspaceStatus)

const canStop = (workspaceStatus: WorkspaceStatus) => ["started", "canceled", "error"].includes(workspaceStatus)

export interface WorkspaceActionsProps {
  workspace: Workspace
  handleStart: () => void
  handleStop: () => void
  handleUpdate: () => void
  handleCancel: () => void
}

export const WorkspaceActions: React.FC<WorkspaceActionsProps> = ({
  workspace,
  handleStart,
  handleStop,
  handleUpdate,
  handleCancel,
}) => {
  const styles = useStyles()
  const workspaceStatus = getWorkspaceStatus(workspace.latest_build)

  return (
    <Stack direction="row" spacing={1}>
      {canStart(workspaceStatus) && (
        <WorkspaceActionButton
          className={styles.actionButton}
          icon={<PlayArrowRoundedIcon />}
          onClick={handleStart}
          label={Language.start}
        />
      )}
      {canStop(workspaceStatus) && (
        <WorkspaceActionButton
          className={styles.actionButton}
          icon={<StopIcon />}
          onClick={handleStop}
          label={Language.stop}
        />
      )}
      {canCancelJobs(workspaceStatus) && (
        <WorkspaceActionButton
          className={styles.actionButton}
          icon={<CancelIcon />}
          onClick={handleCancel}
          label={Language.cancel}
        />
      )}
      {workspace.outdated && canAcceptJobs(workspaceStatus) && (
        <Button className={styles.actionButton} startIcon={<CloudDownloadIcon />} onClick={handleUpdate}>
          {Language.update}
        </Button>
      )}
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  actionButton: {
    // Set fixed width for the action buttons so they will not change the size
    // during the transitions
    width: theme.spacing(27),
  },
}))
