import Box from "@material-ui/core/Box"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { Link } from "react-router-dom"
import { WorkspaceProps } from "../Workspace/Workspace"
import CloudCircleIcon from "@material-ui/icons/CloudCircle"
import { TitleIconSize } from "../../theme/constants"
import { makeStyles } from "@material-ui/core/styles"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"
import { Stack } from "../Stack/Stack"
import Button from "@material-ui/core/Button"

/**
 * Component for the header at the top of the workspace page
 */
export const WorkspaceStatusBar: React.FC<WorkspaceProps> = ({ organization, template, workspace }) => {
  const styles = useStyles()

  const templateLink = `/templates/${organization.name}/${template.name}`
  const action = "Start" // TODO don't let me merge this
  const outOfDate = false // TODO

  return (
    <WorkspaceSection>
      <Stack spacing={1}>
      <Typography variant="body2" color="textSecondary">
        Back to <Link className={styles.link} to={templateLink}>{template.name}</Link>
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
          <Button color="primary">{action}</Button>
          <Button color="primary" disabled={!outOfDate}>Update</Button>
          <Link className={styles.link} to={`workspaces/${workspace.id}/edit`}>Settings</Link>
        </div>
      </div>
      </Stack>
    </WorkspaceSection>
  )
}

const useStyles = makeStyles((theme) => {
  return {
    link: {
      textDecoration: 'none',
      color: theme.palette.text.primary
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
      gap: theme.spacing(1)
    },
    vertical: {
      display: "flex",
      flexDirection: "column",
    },
  }
})
