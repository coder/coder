import React from "react"
import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import Table from "@material-ui/core/Table"
import TableRow from "@material-ui/core/TableRow"
import TableCell from "@material-ui/core/TableCell"

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

  const action = {
    text: "Create Project",
    onClick: createProject,
  }

  return (
    <div className={styles.root}>
      <Navbar user={me} />

      <Header title="Projects" description="View available projects" subTitle="0 total" action={action} />

      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>
        <Table>
          <TableRow>
            <TableCell colSpan={999}>
              <Box p={4}>
                <EmptyState
                  button={{
                    children: "Create Project",
                    //icon: AddPhotoIcon,
                    onClick: createProject,
                  }}
                  message="No projects have been created yet"
                  description="Create a project to get started."
                />
              </Box>
            </TableCell>
          </TableRow>

        </Table>
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
