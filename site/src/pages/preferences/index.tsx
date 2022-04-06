import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"
import React from "react"
import { Header } from "../../components/Header"
import { Footer } from "../../components/Page"

export const PreferencesPage: React.FC = () => {
  return (
    <Box display="flex" flexDirection="column">
      <Header title="Preferences" />
      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>Preferences here!</Paper>
      <Footer />
    </Box>
  )
}
