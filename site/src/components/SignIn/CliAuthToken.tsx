import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { CodeExample } from "../CodeExample"

export interface CliAuthTokenProps {
  sessionToken: string
}

export const CliAuthToken: React.FC<CliAuthTokenProps> = ({ sessionToken }) => {
  const styles = useStyles()
  return (
    <Paper className={styles.container}>
      <Typography className={styles.title}>Session Token</Typography>
      <CodeExample code={sessionToken} />
    </Paper>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    marginBottom: theme.spacing(2),
  },
  container: {
    maxWidth: "680px",
    padding: theme.spacing(2),
  },
}))
