import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import Link from "next/link"
import { useRouter } from "next/router"
import useSWR from "swr"

import { Project, Workspace } from "../../../../api"
import { Header } from "../../../../components/Header"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { Navbar } from "../../../../components/Navbar"
import { Footer } from "../../../../components/Page"
import { Column, Table } from "../../../../components/Table"
import { useUser } from "../../../../contexts/UserContext"
import { ErrorSummary } from "../../../../components/ErrorSummary"
import { firstOrItem } from "../../../../util/array"
import { EmptyState } from "../../../../components/EmptyState"

const ProjectPage: React.FC = () => {
  const styles = useStyles()
  const { me, signOut } = useUser(true)

  const router = useRouter()
  const { project, organization } = router.query

  const { data: projectInfo, error: projectError } = useSWR<Project, Error>(
    () => `/api/v2/projects/${organization}/${project}`,
  )
  const { data: workspaces, error: workspacesError } = useSWR<Workspace[], Error>(
    () => `/api/v2/projects/${organization}/${project}/workspaces`,
  )

  if (projectError) {
    return <ErrorSummary error={projectError} />
  }

  if (workspacesError) {
    return <ErrorSummary error={workspacesError} />
  }

  if (!me || !projectInfo || !workspaces) {
    return <FullScreenLoader />
  }

  const createWorkspace = () => {
    void router.push(`/projects/${organization}/${project}/create`)
  }

  const emptyState = (
    <EmptyState
      button={{
        children: "Create Workspace",
        onClick: createWorkspace,
      }}
      message="No workspaces have been created yet"
      description="Create a workspace to get started"
    />
  )

  const columns: Column<Workspace>[] = [
    {
      key: "name",
      name: "Name",
      renderer: (nameField: string) => {
        return <Link href={`/workspaces/me/${nameField}`}>{nameField}</Link>
      },
    },
  ]

  const tableProps = {
    title: "Workspaces",
    columns,
    data: workspaces,
    emptyState: emptyState,
  }

  return (
    <div className={styles.root}>
      <Navbar user={me} onSignOut={signOut} />
      <Header
        title={firstOrItem(project, "")}
        description={firstOrItem(organization, "")}
        subTitle={`${workspaces.length} workspaces`}
        action={{
          text: "Create Workspace",
          onClick: createWorkspace,
        }}
      />

      <Paper style={{ maxWidth: "1380px", margin: "1em auto", width: "100%" }}>
        <Table {...tableProps} />
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

export default ProjectPage
