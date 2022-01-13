import React, { useState } from "react"
import { Dialog, DialogActions, Button, DialogTitle, DialogContent, makeStyles, Box, Paper } from "@material-ui/core"
import { AddToQueue as AddWorkspaceIcon } from "@material-ui/icons"

import { EmptyState, Page, SplitButton } from "../components"

const WorkspacesPage: React.FC = () => {
  const styles = useStyles()

  const createWorkspace = () => {
    alert("create")
  }

  const button = {
    children: "New Workspace",
    onClick: createWorkspace,
  }

  return (
    <Page>
      <div className={styles.header}>
        <SplitButton<string>
          color="primary"
          onClick={createWorkspace}
          options={[
            {
              label: "New workspace",
              value: "custom",
            },
            {
              label: "New workspace from template",
              value: "template",
            },
          ]}
          startIcon={<AddWorkspaceIcon />}
          textTransform="none"
        />
      </div>

      <Paper style={{ maxWidth: "1380px", margin: "1em auto" }}>
        <Box pt={4} pb={4}>
          <EmptyState message="No workspaces available." button={button} />
        </Box>
      </Paper>
    </Page>
  )
}

const useStyles = makeStyles((theme) => ({
  header: {
    display: "flex",
    flexDirection: "row-reverse",
    justifyContent: "space-between",
    margin: "1em auto",
    maxWidth: "1380px",
    padding: theme.spacing(2, 6.25, 0),
    width: "100%",
  },
}))

export default WorkspacesPage
