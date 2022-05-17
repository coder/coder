import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { CardPadding, CardRadius } from "../../theme/constants"

export interface WorkspaceSectionProps {
  title?: string
}

export const WorkspaceSection: React.FC<WorkspaceSectionProps> = ({ title, children }) => {
  const styles = useStyles()

  return (
    <Paper elevation={0} className={styles.root}>
      {title && (
        <div className={styles.headerContainer}>
          <div className={styles.header}>
            <Typography variant="h6">{title}</Typography>
          </div>
        </div>
      )}

      <div className={styles.contents}>{children}</div>
    </Paper>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: CardRadius,
    margin: theme.spacing(1),
  },
  headerContainer: {
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  contents: {
    margin: theme.spacing(2),
  },
  header: {
    alignItems: "center",
    display: "flex",
    flexDirection: "row",
    marginBottom: theme.spacing(1),
    marginTop: theme.spacing(1),
    paddingLeft: CardPadding + theme.spacing(1),
    paddingRight: CardPadding / 2,
  },
}))
