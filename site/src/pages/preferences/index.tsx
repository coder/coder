import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { Header } from "../../components/Header"
import { Footer } from "../../components/Page"

export const PreferencesPage: React.FC = () => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <Header title="Preferences" />
      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>Preferences here!</Paper>
      <Footer />
    </div>
  )
}

const useStyles = makeStyles(() => ({
  root: {
    display: "flex",
    flexDirection: "column",
  },
}))
