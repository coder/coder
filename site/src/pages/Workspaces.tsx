import React, { useState } from "react"
import { AddToQueue as AddWorkspaceIcon } from "@material-ui/icons"

import { Confetti, EmptyState, Page, SplitButton } from "./../components"

import { Dialog, DialogActions, Button, DialogTitle, DialogContent, makeStyles, Box, Paper } from "@material-ui/core"

const useStyles2 = makeStyles((theme) => ({
  "@keyframes spin": {
    from: {
      transform: "rotateY(0deg)",
    },
    to: {
      transform: "rotateY(180deg)",
    },
  },
  triangle: {
    animation: "$spin 1s ease-in-out infinite alternate both",
  },
}))

export const Triangle: React.FC = () => {
  const size = 100

  const styles = useStyles2()

  const transparent = `${size}px solid transparent`
  const colored = `${size / 0.86}px solid black`

  return (
    <div
      className={styles.triangle}
      style={{
        width: 0,
        height: 0,
        borderLeft: transparent,
        borderRight: transparent,
        borderBottom: colored,
      }}
    />
  )
}

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
      <Dialog fullWidth={true} maxWidth={"lg"} open={open} onClose={handleClose}>
        <DialogTitle>New Workspace</DialogTitle>
        <DialogContent>
          <Confetti
            style={{
              minHeight: "500px",
              display: "flex",
              flexDirection: "column",
              justifyContent: "center",
              alignItems: "center",
            }}
          >
            <Triangle />

            <Box m={"2em"}>NEXT STEP: Let's create a workspace with a v2 provisioner!</Box>
          </Confetti>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose}>Close</Button>
        </DialogActions>
      </Dialog>
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
