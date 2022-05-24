import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React, { HTMLProps } from "react"
import { CardPadding, CardRadius } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"

export interface WorkspaceSectionProps {
  /**
   * action appears in the top right of the section card
   */
  action?: React.ReactNode
  contentsProps?: HTMLProps<HTMLDivElement>
  title?: string
}

export const WorkspaceSection: React.FC<WorkspaceSectionProps> = ({ action, children, contentsProps, title }) => {
  const styles = useStyles()

  return (
    <Paper className={styles.root} elevation={0}>
      {title && (
        <div className={styles.headerContainer}>
          <div className={styles.header}>
            <Typography variant="h6">{title}</Typography>
            {action && <div>{action}</div>}
          </div>
        </div>
      )}

      <div {...contentsProps} className={combineClasses([styles.contents, contentsProps?.className])}>
        {children}
      </div>
    </Paper>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: CardRadius,
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
    justifyContent: "space-between",
    marginBottom: theme.spacing(1),
    marginTop: theme.spacing(1),
    paddingLeft: CardPadding + theme.spacing(1),
    paddingRight: CardPadding / 2,
  },
}))
