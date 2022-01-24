import React from "react"
import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"

import { EmptyState } from "../../components"
import { Navbar } from "../../components/Navbar"
import { Header } from "../../components/Header"
import { Footer } from "../../components/Page"
import { useUser } from "../../contexts/UserContext"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"

const ProjectsPage: React.FC = () => {
  const styles = useStyles()
  const { me } = useUser(true)

  if (!me) {
    return <FullScreenLoader />
  }

  const createProject = () => {
    alert("createProject")
  }

  const button = {
    children: "New Project",
    onClick: createProject,
  }

  const action = {
    text: "Create Project",
    onClick: createProject,
  }

  return (
    <div className={styles.root}>
      <Navbar user={me} />

      <Header title="Projects" description="View available projects" subTitle="0 total" action={action} />
      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>
        <Box pt={4} pb={4}>
          <EmptyState message="No projects available." button={button} />
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

export default ProjectsPage
