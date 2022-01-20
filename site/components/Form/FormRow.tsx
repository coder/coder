import { makeStyles } from "@material-ui/core/styles"
import React from "react"

const useStyles = makeStyles((theme) => ({
  row: {
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(2),
  },
}))

export const FormRow: React.FC = ({ children }) => {
  const styles = useStyles()
  return <div className={styles.row}>{children}</div>
}
