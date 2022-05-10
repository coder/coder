import Box from "@material-ui/core/Box"
import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { Link } from "react-router-dom"
import * as Types from "../../api/types"
import { WorkspaceStatus } from "../../pages/WorkspacePage/WorkspacePage"
import { TitleIconSize } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { Stack } from "../Stack/Stack"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"

const Language = {
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
}

export interface WorkspaceStatusBarProps {
  organization?: Types.Organization
  workspace: Types.Workspace
  template?: Types.Template
  handleStart: () => void
  handleStop: () => void
  handleRetry: () => void
  workspaceStatus: WorkspaceStatus
}

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
  workspaceStatus,
}) => {
  const styles = useStyles()

  const templateLink = `/templates/${organization?.name}/${template?.name}`

  return (
    <WorkspaceSection>
      <Stack spacing={1}>
        <div className={combineClasses([styles.horizontal, styles.reverse])}>
          <div className={styles.horizontal}>
            <Link className={styles.link} to={`workspaces/${workspace.id}/edit`}>
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
              {workspaceStatus === "started" && Language.started}
              {workspaceStatus === "starting" && Language.starting}
              {workspaceStatus === "stopped" && Language.stopped}
              {workspaceStatus === "stopping" && Language.stopping}
              {workspaceStatus === "error" && Language.error}
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

            {workspace.outdated && <Button color="primary">{Language.update}</Button>}
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
