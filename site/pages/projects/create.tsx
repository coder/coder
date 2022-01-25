import React from "react"
import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import AddWorkspaceIcon from "@material-ui/icons/AddToQueue"

import { useUser } from "../../contexts/UserContext"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"

const CreateProjectPage: React.FC = () => {
  const styles = useStyles()
  const { me, signOut } = useUser(true)

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
        </Box>
      </Paper>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "column",
  },
}))

export default CreateProjectPage
