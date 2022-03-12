import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import { Link, useNavigate, useParams } from "react-router-dom"
import useSWR from "swr"

import { Organization, Project, Workspace } from "../../../../api"
import { Header } from "../../../../components/Header"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { Navbar } from "../../../../components/Navbar"
import { Footer } from "../../../../components/Page"
import { Column, Table } from "../../../../components/Table"
import { useUser } from "../../../../contexts/UserContext"
import { ErrorSummary } from "../../../../components/ErrorSummary"
import { firstOrItem } from "../../../../util/array"
import { EmptyState } from "../../../../components/EmptyState"
import { unsafeSWRArgument } from "../../../../util"

export const ProjectPage: React.FC = () => {
  const styles = useStyles()
  const { me, signOut } = useUser(true)
  const navigate = useNavigate()
  const { project: projectName, organization: organizationName } = useParams()

  const { data: organizationInfo, error: organizationError } = useSWR<Organization, Error>(
    () => `/api/v2/users/me/organizations/${organizationName}`,
  )

  const { data: projectInfo, error: projectError } = useSWR<Project, Error>(
    () => `/api/v2/organizations/${unsafeSWRArgument(organizationInfo).id}/projects/${projectName}`,
  )

  // TODO: The workspaces endpoint was recently changed, so that we can't get
  // workspaces per-project. This just grabs all workspaces... and then
  // later filters them to match the current project.
  const { data: workspaces, error: workspacesError } = useSWR<Workspace[], Error>(() => `/api/v2/users/me/workspaces`)

  if (organizationError) {
    return <ErrorSummary error={organizationError} />
  }

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
    navigate(`/projects/${organizationName}/${projectName}/create`)
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
      renderer: (nameField: string, workspace: Workspace) => {
        return <Link to={`/workspaces/${workspace.id}`}>{nameField}</Link>
      },
    },
  ]

  const perProjectWorkspaces = workspaces.filter((workspace) => {
    return workspace.project_id === projectInfo.id
  })

  const tableProps = {
    title: "Workspaces",
    columns,
    data: perProjectWorkspaces,
    emptyState: emptyState,
  }

  return (
    <div className={styles.root}>
      <Navbar user={me} onSignOut={signOut} />
      <Header
        title={firstOrItem(projectName, "")}
        description={firstOrItem(organizationName, "")}
        subTitle={`${perProjectWorkspaces.length} workspaces`}
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
