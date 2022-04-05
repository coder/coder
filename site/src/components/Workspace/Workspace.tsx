import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import CloudCircleIcon from "@material-ui/icons/CloudCircle"
import React from "react"
import { Link } from "react-router-dom"
import * as Types from "../../api/types"
import * as Constants from "./constants"
import { WorkspaceSection } from "./WorkspaceSection"

export interface WorkspaceProps {
  organization: Types.Organization
  workspace: Types.Workspace
  project: Types.Project
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: React.FC<WorkspaceProps> = ({ organization, project, workspace }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.vertical}>
        <WorkspaceHeader organization={organization} project={project} workspace={workspace} />
        <div className={styles.horizontal}>
          <div className={styles.sidebarContainer}>
            <WorkspaceSection title="Applications">
              <Placeholder />
            </WorkspaceSection>
            <WorkspaceSection title="Dev URLs">
              <Placeholder />
            </WorkspaceSection>
            <WorkspaceSection title="Resources">
              <Placeholder />
            </WorkspaceSection>
          </div>
          <div className={styles.timelineContainer}>
            <WorkspaceSection title="Timeline">
              <div
                className={styles.vertical}
                style={{ justifyContent: "center", alignItems: "center", height: "300px" }}
              >
                <Placeholder />
              </div>
            </WorkspaceSection>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * Component for the header at the top of the workspace page
 */
export const WorkspaceHeader: React.FC<WorkspaceProps> = ({ organization, project, workspace }) => {
  const styles = useStyles()

  const projectLink = `/projects/${organization.name}/${project.name}`

  return (
    <Paper elevation={0} className={styles.section}>
      <div className={styles.horizontal}>
        <WorkspaceHeroIcon />
        <div className={styles.vertical}>
          <Typography variant="h4">{workspace.name}</Typography>
          <Typography variant="body2" color="textSecondary">
            <Link to={projectLink}>{project.name}</Link>
          </Typography>
        </div>
      </div>
    </Paper>
  )
}

/**
 * Component to render the 'Hero Icon' in the header of a workspace
 */
export const WorkspaceHeroIcon: React.FC = () => {
  return (
    <Box mr="1em">
      <CloudCircleIcon width={Constants.TitleIconSize} height={Constants.TitleIconSize} />
    </Box>
  )
}

/**
 * Temporary placeholder component until we have the sections implemented
 * Can be removed once the Workspace page has all the necessary sections
 */
const Placeholder: React.FC = () => {
  return (
    <div style={{ textAlign: "center", opacity: "0.5" }}>
      <Typography variant="caption">Not yet implemented</Typography>
    </div>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    root: {
      display: "flex",
      flexDirection: "column",
    },
    horizontal: {
      display: "flex",
      flexDirection: "row",
    },
    vertical: {
      display: "flex",
      flexDirection: "column",
    },
    section: {
      border: `1px solid ${theme.palette.divider}`,
      borderRadius: Constants.CardRadius,
      padding: Constants.CardPadding,
      margin: theme.spacing(1),
    },
    sidebarContainer: {
      display: "flex",
      flexDirection: "column",
      flex: "0 0 350px",
    },
    timelineContainer: {
      flex: 1,
    },
    icon: {
      width: Constants.TitleIconSize,
      height: Constants.TitleIconSize,
    },
  }
})
