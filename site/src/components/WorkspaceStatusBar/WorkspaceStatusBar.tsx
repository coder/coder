import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { Link } from "react-router-dom"
import { WorkspaceProps } from "../Workspace/Workspace"
import CloudCircleIcon from "@material-ui/icons/CloudCircle"
import { CardPadding, CardRadius, TitleIconSize } from "../../theme/constants"
import { makeStyles } from "@material-ui/core/styles"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"

/**
 * Component for the header at the top of the workspace page
 */
export const WorkspaceStatusBar: React.FC<WorkspaceProps> = ({ organization, template, workspace }) => {
  const styles = useStyles()

  const templateLink = `/templates/${organization.name}/${template.name}`

  return (
    <WorkspaceSection>
      <div className={styles.horizontal}>
        <Box mr="1em">
          <CloudCircleIcon width={TitleIconSize} height={TitleIconSize} />
        </Box>
        <div className={styles.vertical}>
          <Typography variant="h4">{workspace.name}</Typography>
          <Typography variant="body2" color="textSecondary">
            <Link to={templateLink}>{template.name}</Link>
          </Typography>
        </div>
      </div>
    </WorkspaceSection>
  )
}

const useStyles = makeStyles((theme) => {
  return {
    icon: {
      width: TitleIconSize,
      height: TitleIconSize,
    },
    horizontal: {
      display: "flex",
      flexDirection: "row",
    },
    vertical: {
      display: "flex",
      flexDirection: "column",
    },
  }
})
