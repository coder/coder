import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"

export const TableContainer: FC = ({ children }) => {
  const styles = useStyles()

  return <div className={styles.wrapper}>{children}</div>
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    overflowX: "auto",
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
  },
}))
