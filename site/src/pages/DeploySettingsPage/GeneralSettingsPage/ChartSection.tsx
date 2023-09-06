import Paper from "@mui/material/Paper"
import { makeStyles } from "@mui/styles"
import { HTMLProps, ReactNode, FC, PropsWithChildren } from "react"
import { combineClasses } from "utils/combineClasses"

export interface ChartSectionProps {
  /**
   * action appears in the top right of the section card
   */
  action?: ReactNode
  contentsProps?: HTMLProps<HTMLDivElement>
  title?: string | JSX.Element
}

export const ChartSection: FC<PropsWithChildren<ChartSectionProps>> = ({
  action,
  children,
  contentsProps,
  title,
}) => {
  const styles = useStyles()

  return (
    <Paper className={styles.root} elevation={0}>
      {title && (
        <div className={styles.header}>
          <h6 className={styles.title}>{title}</h6>
          {action && <div>{action}</div>}
        </div>
      )}

      <div
        {...contentsProps}
        className={combineClasses([styles.contents, contentsProps?.className])}
      >
        {children}
      </div>
    </Paper>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
  },
  contents: {
    margin: theme.spacing(2),
  },
  header: {
    alignItems: "center",
    display: "flex",
    justifyContent: "space-between",
    padding: theme.spacing(1.5, 2),
  },
  title: {
    margin: 0,
    fontSize: 14,
    fontWeight: 600,
  },
}))
