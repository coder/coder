import React, { useState } from "react"
import { Dialog, DialogActions, Button, DialogTitle, DialogContent, makeStyles, Box, Paper } from "@material-ui/core"
import { AddToQueue as AddWorkspaceIcon } from "@material-ui/icons"

import { EmptyState, Page, SplitButton } from "./../components"

import { CreateWorkspace } from "./CreateWorkspace"

export const Workspaces: React.FC = () => {
  const styles = useStyles()
  const [open, setOpen] = useState(false)

  const startCreateWorkspace = () => {
    setOpen(true)
  }

  const handleClose = () => {
    setOpen(false)
  }

  const button = {
    children: "New Workspace",
    onClick: startCreateWorkspace,
  }

  return (
    <Page>
      <div className={styles.header}>
        <SplitButton<string>
          color="primary"
          onClick={startCreateWorkspace}
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

      <CreateDialog open={open} handleClose={handleClose} />
    </Page>
  )
}

const CreateDialog: React.FC<{ open: boolean; handleClose: () => void }> = ({ open, handleClose }) => {
  return (
    <Dialog fullWidth={true} maxWidth={"lg"} open={open} onClose={handleClose}>
      <DialogTitle>New Workspace</DialogTitle>
      <DialogContent>
        <CreateWorkspace />
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Close</Button>
      </DialogActions>
    </Dialog>
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
