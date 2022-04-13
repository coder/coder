import { makeStyles } from "@material-ui/core/styles"
import React from "react"

const useStyles = makeStyles((theme) => ({
  stack: {
    display: "flex",
    flexDirection: "column",
    gap: theme.spacing(2),
  },
}))

export const FormStack: React.FC = ({ children }) => {
  const styles = useStyles()

  return <div className={styles.stack}>{children}</div>
}
