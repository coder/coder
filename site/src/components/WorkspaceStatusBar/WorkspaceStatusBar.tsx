import Box from "@material-ui/core/Box"
import Button from "@material-ui/core/Button"
import Divider from "@material-ui/core/Divider"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import CloudCircleIcon from "@material-ui/icons/CloudCircle"
import React from "react"
import { Link } from "react-router-dom"
import * as Types from "../../api/types"
import { TitleIconSize } from "../../theme/constants"
import { Stack } from "../Stack/Stack"
import { WorkspaceStatus } from "../Workspace/Workspace"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"

const Language = {
  stop: "Stop",
  start: "Start",
  update: "Update",
  settings: "Settings",
}
export interface WorkspaceStatusBarProps {
  organization: Types.Organization
  workspace: Types.Workspace
  template: Types.Template
  status: WorkspaceStatus
  handleUpdate: () => void
  handleToggle: () => void
}

/**
 * Component for the header at the top of the workspace page
 */
export const WorkspaceStatusBar: React.FC<WorkspaceStatusBarProps> = ({
  organization,
  template,
  workspace,
  status,
  handleUpdate,
  handleToggle,
}) => {
  const styles = useStyles()

  const templateLink = `/templates/${organization.name}/${template.name}`
  const statusToAction: Record<WorkspaceStatus, string> = {
    started: Language.stop,
    stopping: Language.stop,
    stopped: Language.start,
    starting: Language.start,
  }
  // Cannot start or stop in the middle of starting or stopping
  const statusToDisabled: Record<WorkspaceStatus, boolean> = {
    started: false,
    stopping: true,
    stopped: false,
    starting: true,
  }
  const action = statusToAction[status]
  const actionDisabled = statusToDisabled[status]

  return (
    <WorkspaceSection>
      <Stack spacing={1}>
        <Typography variant="body2" color="textSecondary">
          Back to{" "}
          <Link className={styles.link} to={templateLink}>
            {template.name}
          </Link>
        </Typography>
        <div className={styles.horizontal}>
          <div className={styles.horizontal}>
            <Box mr="1em">
              <CloudCircleIcon width={TitleIconSize} height={TitleIconSize} />
            </Box>
            <div className={styles.vertical}>
              <Typography variant="h4">{workspace.name}</Typography>
            </div>
          </div>
          <div className={styles.horizontal}>
            <Button onClick={handleToggle} disabled={actionDisabled} color="primary">
              {action}
            </Button>
            {workspace.outdated && (
              <Button onClick={handleUpdate} color="primary">
                {Language.update}
              </Button>
            )}
            <Divider orientation="vertical" flexItem />
            <Link className={styles.link} to={`workspaces/${workspace.id}/edit`}>
              {Language.settings}
            </Link>
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
      flexDirection: "row",
      justifyContent: "space-between",
      alignItems: "center",
      gap: theme.spacing(2),
    },
    vertical: {
      display: "flex",
      flexDirection: "column",
    },
  }
})
