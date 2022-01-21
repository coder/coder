import React from "react"
import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import AddWorkspaceIcon from "@material-ui/icons/AddToQueue"

import { EmptyState, SplitButton } from "../components"
import { Navbar } from "../components/Navbar"
import { Footer } from "../components/Page"
import { useUser } from "../contexts/UserContext"
import { FullScreenLoader } from "../components/Loader/FullScreenLoader"

const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const { me } = useUser(true)

  if (!me) {
    return <FullScreenLoader />
  }

  const createWorkspace = () => {
    alert("create")
  }

  const button = {
    children: "New Workspace",
    onClick: createWorkspace,
  }

  return (
    <div className={styles.root}>
      <Navbar user={me} />
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

      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>
        <Box pt={4} pb={4}>
          <EmptyState message="No workspaces available." button={button} />
        </Box>
      </Paper>
      <Footer />
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "column",
  },
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
