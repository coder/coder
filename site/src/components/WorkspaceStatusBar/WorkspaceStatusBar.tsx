import Box from "@material-ui/core/Box"
import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { Link } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { WorkspaceStatus } from "../../pages/WorkspacePage/WorkspacePage"
import { TitleIconSize } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { Stack } from "../Stack/Stack"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"

export const Language = {
  stop: "Stop",
  start: "Start",
  retry: "Retry",
  update: "Update",
  settings: "Settings",
  started: "Running",
  stopped: "Stopped",
  starting: "Building",
  stopping: "Stopping",
  error: "Build Failed",
  loading: "Loading Status",
  deleting: "Deleting",
  deleted: "Deleted",
  // "Canceling" would be misleading because it refers to a build, not the workspace.
  // So just stall. When it is canceled it will appear as the error workspaceStatus.
  canceling: "Loading Status",
}

export interface WorkspaceStatusBarProps {
  organization?: TypesGen.Organization
  workspace: TypesGen.Workspace
  template?: TypesGen.Template
  handleStart: () => void
  handleStop: () => void
  handleRetry: () => void
  handleUpdate: () => void
  workspaceStatus: WorkspaceStatus
}

/**
 * Jobs submitted while another job is in progress will be discarded,
 * so check whether workspace job status has reached completion (whether successful or not).
 */
const canAcceptJobs = (workspaceStatus: WorkspaceStatus) =>
  ["started", "stopped", "deleted", "error"].includes(workspaceStatus)

/**
 * Component for the header at the top of the workspace page
 */
export const WorkspaceStatusBar: React.FC<WorkspaceStatusBarProps> = ({
  organization,
  template,
  workspace,
  handleStart,
  handleStop,
  handleRetry,
  handleUpdate,
  workspaceStatus,
}) => {
  const styles = useStyles()

  const templateLink = `/templates/${organization?.name}/${template?.name}`
  const settingsLink = "edit"

  return (
    <WorkspaceSection>
      <Stack spacing={1}>
        <div className={combineClasses([styles.horizontal, styles.reverse])}>
          <div className={styles.horizontal}>
            <Link className={styles.link} to={settingsLink}>
              {Language.settings}
            </Link>
          </div>

          {organization && template && (
            <Typography variant="body2" color="textSecondary">
              Back to{" "}
              <Link className={styles.link} to={templateLink}>
                {template.name}
              </Link>
            </Typography>
          )}
        </div>

        <div className={styles.horizontal}>
          <div className={styles.horizontal}>
            <Typography variant="h4">{workspace.name}</Typography>
            <Box className={styles.statusChip} role="status">
              {Language[workspaceStatus]}
            </Box>
          </div>

          <div className={styles.horizontal}>
            {workspaceStatus === "started" && (
              <Button onClick={handleStop} color="primary">
                {Language.stop}
              </Button>
            )}
            {workspaceStatus === "stopped" && (
              <Button onClick={handleStart} color="primary">
                {Language.start}
              </Button>
            )}
            {workspaceStatus === "error" && (
              <Button onClick={handleRetry} color="primary">
                {Language.retry}
              </Button>
            )}

            {workspace.outdated && canAcceptJobs(workspaceStatus) && (
              <Button onClick={handleUpdate} color="primary">
                {Language.update}
              </Button>
            )}
          </div>
        </div>
      </Stack>
    </WorkspaceSection>
  )
}

const useStyles = makeStyles((theme) => {
  return {
    link: {
      textDecoration: "none",
      color: theme.palette.text.primary,
    },
    icon: {
      width: TitleIconSize,
      height: TitleIconSize,
    },
    horizontal: {
      display: "flex",
      justifyContent: "space-between",
      alignItems: "center",
      gap: theme.spacing(2),
    },
    reverse: {
      flexDirection: "row-reverse",
    },
    statusChip: {
      border: `solid 1px ${theme.palette.text.hint}`,
      borderRadius: theme.shape.borderRadius,
      padding: theme.spacing(1),
    },
    vertical: {
      display: "flex",
      flexDirection: "column",
    },
  }
})
