import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"

const useStyles = makeStyles((theme) => ({
  root: {
    marginTop: theme.spacing(3),
  },
}))

/**
 * SectionAction is a content box that call to actions should be placed
 * within
 */
export const SectionAction: FC<React.PropsWithChildren<unknown>> = ({ children }) => {
  const styles = useStyles()
  return <div className={styles.root}>{children}</div>
}
