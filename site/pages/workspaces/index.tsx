import React from "react"
import { useRouter } from "next/router"
import { makeStyles } from "@material-ui/core/styles"
import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"
import AddWorkspaceIcon from "@material-ui/icons/AddToQueue"

import { EmptyState, SplitButton } from "../../components"
import { AppPage } from "../../components/PageTemplates"

const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const router = useRouter()

  const createWorkspace = () => {
    router.push("/workspaces/create")
  }

  const button = {
    children: "New Workspace",
    onClick: createWorkspace,
  }

  return (
    <AppPage>
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
    </AppPage>
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
