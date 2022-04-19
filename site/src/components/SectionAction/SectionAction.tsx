import { makeStyles } from "@material-ui/core/styles"
import React from "react"

const useStyles = makeStyles((theme) => ({
  root: {
    marginTop: theme.spacing(3),
  },
}))

/**
 * SectionAction is a content box that call to actions should be placed
 * within
 */
export const SectionAction: React.FC = ({ children }) => {
  const styles = useStyles()
  return <div className={styles.root}>{children}</div>
}
