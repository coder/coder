import * as React from "react"
import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"
import { EmptyState, Page } from "./../components"

export const Workspaces: React.FC = () => {
  const button = {
    children: "New Workspace",
    onClick: () => alert("Not yet implemented"),
  }

  return (
    <Page>
      <Paper style={{ maxWidth: "1380px", margin: "1em auto" }}>
        <Box pt={4} pb={4}>
          <EmptyState message="No workspaces available." button={button} />
        </Box>
      </Paper>
    </Page>
  )
}
