import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import CloudDownloadIcon from "@material-ui/icons/CloudDownload"
import PlayArrowRoundedIcon from "@material-ui/icons/PlayArrowRounded"
import ReplayIcon from "@material-ui/icons/Replay"
import StopIcon from "@material-ui/icons/Stop"
import React from "react"
import { Link as RouterLink } from "react-router-dom"
import { Workspace } from "../../api/typesGenerated"
import { WorkspaceStatus } from "../../util/workspace"
import { Stack } from "../Stack/Stack"
import { WorkspaceActionButton } from "../WorkspaceActionButton/WorkspaceActionButton"

export const Language = {
  stop: "Stop workspace",
  stopping: "Stopping workspace",
  start: "Start workspace",
  starting: "Starting workspace",
  retry: "Retry",
  update: "Update workspace",
}

/**
 * Jobs submitted while another job is in progress will be discarded,
 * so check whether workspace job status has reached completion (whether successful or not).
 */
const canAcceptJobs = (workspaceStatus: WorkspaceStatus) =>
  ["started", "stopped", "deleted", "error", "canceled"].includes(workspaceStatus)

export interface WorkspaceActionsProps {
  workspace: Workspace
  workspaceStatus: WorkspaceStatus
  handleStart: () => void
  handleStop: () => void
  handleRetry: () => void
  handleUpdate: () => void
}

export const WorkspaceActions: React.FC<WorkspaceActionsProps> = ({
  workspace,
  workspaceStatus,
  handleStart,
  handleStop,
  handleRetry,
  handleUpdate,
}) => {
  const styles = useStyles()

  return (
    <Stack direction="row" spacing={1}>
      <Link underline="none" component={RouterLink} to="edit">
        <Button variant="outlined">Settings</Button>
      </Link>
      {(workspaceStatus === "started" || workspaceStatus === "stopping") && (
        <WorkspaceActionButton
          className={styles.actionButton}
          icon={<StopIcon />}
          onClick={handleStop}
          label={Language.stop}
          loadingLabel={Language.stopping}
          isLoading={workspaceStatus === "stopping"}
        />
      )}
      {(workspaceStatus === "stopped" || workspaceStatus === "starting") && (
        <WorkspaceActionButton
          className={styles.actionButton}
          icon={<PlayArrowRoundedIcon />}
          onClick={handleStart}
          label={Language.start}
          loadingLabel={Language.starting}
          isLoading={workspaceStatus === "starting"}
        />
      )}
      {workspaceStatus === "error" && (
        <Button className={styles.actionButton} startIcon={<ReplayIcon />} onClick={handleRetry}>
          {Language.retry}
        </Button>
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
    width: theme.spacing(30),
  },
}))
