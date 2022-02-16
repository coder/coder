import Paper from "@material-ui/core/Paper"
import Typography from "@material-ui/core/Typography"
import { makeStyles } from "@material-ui/core/styles"
import React, { useEffect, useState } from "react"
import { getApiKey } from "../api"
import { CodeExample } from "../components/CodeExample"

import { FullScreenLoader } from "../components/Loader/FullScreenLoader"
import { useUser } from "../contexts/UserContext"

const CliAuthenticationPage: React.FC = () => {
  const { me } = useUser(true)
  const styles = useStyles()

  const [apiKey, setApiKey] = useState<string | null>(null)

  useEffect(() => {
    if (me?.id) {
      void getApiKey().then(({ key }) => {
        setApiKey(key)
      })
    }
  }, [me?.id])

  if (!apiKey) {
    return <FullScreenLoader />
  }

  return (
    <div className={styles.root}>
      <Paper className={styles.container}>
        <Typography className={styles.title}>Session Token</Typography>
        <CodeExample code={apiKey} />
      </Paper>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    width: "100vh",
    height: "100vw",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
  title: {
    marginBottom: theme.spacing(2),
  },
  container: {
    maxWidth: "680px",
    padding: theme.spacing(2),
  },
}))

export default CliAuthenticationPage
